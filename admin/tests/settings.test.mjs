import assert from "node:assert/strict";
import { test } from "node:test";
import {
  buildSettingsChanges,
  mebibyte,
  sanitizeSettings,
  settingInput,
  settingsFailureMessage,
} from "../src/settings.ts";

const setting = (key, type, value, extra = {}) => ({
  key,
  label: key,
  description: "",
  type,
  value,
  editable: true,
  group: "limits",
  secret: false,
  configured: false,
  ...extra,
});
const limits = {
  revision: 7,
  items: [
    setting("attachmentMaxBytes", "number", 20 * mebibyte, {
      min: 1024,
      max: 100 * mebibyte,
      step: 1,
    }),
    setting("mediaMaxPeers", "number", 20, { min: 1, max: 100, step: 1 }),
    setting("recordingRetentionDays", "number", 7, {
      group: "storage",
      min: 1,
      max: 3650,
      step: 1,
    }),
    setting("publicURL", "string", "https://example.test", { editable: false }),
  ],
};
const mail = {
  revision: 2,
  items: [
    setting("smtpEnabled", "boolean", false),
    setting("emailVerificationRequired", "boolean", false),
    setting("smtpHost", "string", ""),
    setting("smtpPort", "number", 465, { min: 1, max: 65535, step: 1 }),
    setting("smtpUsername", "string", ""),
    setting("smtpPassword", "secret", null, {
      secret: true,
      configured: false,
    }),
    setting("smtpFrom", "string", ""),
    setting("smtpTLSMode", "string", "tls", { options: ["tls", "starttls"] }),
  ].map((item) => ({ ...item, group: "mail" })),
};

test("attachment limits display MiB and send integer bytes within metadata bounds", () => {
  assert.equal(settingInput(limits.items[0]), "20");
  assert.deepEqual(
    buildSettingsChanges(limits, { attachmentMaxBytes: "32" }, "limits"),
    {
      changes: { attachmentMaxBytes: 32 * mebibyte },
      errors: {},
    },
  );
  assert.deepEqual(
    buildSettingsChanges(limits, { attachmentMaxBytes: "0.5" }, "limits")
      .changes,
    {
      attachmentMaxBytes: mebibyte / 2,
    },
  );
  for (const value of ["", "NaN", "Infinity", "0", "101", "0.0000001"])
    assert(
      buildSettingsChanges(limits, { attachmentMaxBytes: value }, "limits")
        .errors.attachmentMaxBytes,
    );
});

test("a group save omits other groups, readonly keys and unchanged values", () => {
  const result = buildSettingsChanges(
    limits,
    {
      mediaMaxPeers: "50",
      recordingRetentionDays: "14",
      publicURL: "https://changed.test",
      attachmentMaxBytes: "20",
    },
    "limits",
  );
  assert.deepEqual(result, { changes: { mediaMaxPeers: 50 }, errors: {} });
  for (const value of ["0", "101", "1.5"])
    assert(
      buildSettingsChanges(limits, { mediaMaxPeers: value }, "limits").errors
        .mediaMaxPeers,
    );
  assert.deepEqual(
    buildSettingsChanges(limits, { mediaMaxPeers: "100" }, "limits").errors,
    {},
  );
});

test("write-only secrets are removed before caching and an empty input preserves the password", () => {
  const response = {
    ...mail,
    items: mail.items.map((item) =>
      item.key === "smtpPassword"
        ? { ...item, value: "synthetic-response-secret", configured: true }
        : item,
    ),
  };
  const safe = sanitizeSettings(response);
  const password = safe.items.find((item) => item.key === "smtpPassword");
  assert.equal(password.value, null);
  assert.equal(password.configured, true);
  assert.equal(settingInput(password), "");
  assert.deepEqual(
    buildSettingsChanges(
      safe,
      { smtpPassword: "", smtpHost: "smtp.example.test" },
      "mail",
    ).changes,
    { smtpHost: "smtp.example.test" },
  );
  assert.equal(
    response.items.find((item) => item.key === "smtpPassword").value,
    "synthetic-response-secret",
  );
});

test("enabling email verification requires enabled and complete SMTP settings", () => {
  assert(
    buildSettingsChanges(mail, { emailVerificationRequired: true }, "mail")
      .errors.emailVerificationRequired,
  );
  const incomplete = buildSettingsChanges(
    mail,
    { smtpEnabled: true, emailVerificationRequired: true },
    "mail",
  );
  for (const key of ["smtpHost", "smtpUsername", "smtpPassword", "smtpFrom"])
    assert(incomplete.errors[key]);
  const complete = buildSettingsChanges(
    mail,
    {
      smtpEnabled: true,
      emailVerificationRequired: true,
      smtpHost: "smtp.example.test",
      smtpUsername: "mailer",
      smtpPassword: "synthetic-smtp-password",
      smtpFrom: "MeetSpace <mail@example.test>",
    },
    "mail",
  );
  assert.deepEqual(complete.errors, {});
  assert.equal(complete.changes.smtpPassword, "synthetic-smtp-password");
});

test("SMTP and verification can be disabled atomically without clearing the stored password", () => {
  const configured = {
    ...mail,
    items: mail.items.map((item) => {
      const values = {
        smtpEnabled: true,
        emailVerificationRequired: true,
        smtpHost: "smtp.example.test",
        smtpUsername: "mailer",
        smtpFrom: "mail@example.test",
      };
      return {
        ...item,
        value: values[item.key] ?? item.value,
        configured: item.key === "smtpPassword",
      };
    }),
  };
  assert(
    buildSettingsChanges(configured, { smtpEnabled: false }, "mail").errors
      .emailVerificationRequired,
  );
  assert.deepEqual(
    buildSettingsChanges(
      configured,
      {
        smtpEnabled: false,
        emailVerificationRequired: false,
        smtpPassword: "",
      },
      "mail",
    ),
    {
      changes: { smtpEnabled: false, emailVerificationRequired: false },
      errors: {},
    },
  );
});

test("mail validation catches invalid mode, sender, host, port and oversized secret", () => {
  const result = buildSettingsChanges(
    mail,
    {
      smtpTLSMode: "plaintext",
      smtpFrom: "invalid",
      smtpHost: "https://smtp.example.test",
      smtpPort: "65536",
      smtpPassword: "x".repeat(4097),
    },
    "mail",
  );
  for (const key of [
    "smtpTLSMode",
    "smtpFrom",
    "smtpHost",
    "smtpPort",
    "smtpPassword",
  ])
    assert(result.errors[key]);
  assert.equal(
    buildSettingsChanges(
      mail,
      { smtpFrom: "Команда <mail@example.test>" },
      "mail",
    ).errors.smtpFrom,
    undefined,
  );
  assert(
    buildSettingsChanges(mail, { smtpPassword: "invalid\0value" }, "mail")
      .errors.smtpPassword,
  );
});

test("a server error cannot echo a submitted SMTP password into the interface", () => {
  const secret = "synthetic-replacement-secret";
  const message = settingsFailureMessage(new Error(`Unexpected ${secret}`), {
    smtpPassword: secret,
  });
  assert(!message.includes(secret));
  assert(message.includes("••••••••"));
});
