import assert from "node:assert/strict";
import { test } from "node:test";
import {
  buildSettingsChanges,
  ldapPreset,
  sanitizeSettings,
  settingsFailureMessage,
} from "../src/settings.ts";

const values = {
  ldapEnabled: true,
  ldapServerURL: "ldaps://ldap:6360",
  ldapBaseDN: "dc=meetspace,dc=test",
  ldapBindDN: "uid=service,ou=people,dc=meetspace,dc=test",
  ...ldapPreset("generic"),
};
const fixture = {
  revision: 3,
  items: [
    ...Object.entries(values).map(([key, value]) => ({
      key,
      type: typeof value,
      value,
      editable: true,
      group: "ldap",
      secret: false,
      configured: false,
    })),
    {
      key: "ldapBindPassword",
      type: "secret",
      value: "must-never-render",
      group: "ldap",
      editable: true,
      secret: true,
      configured: true,
    },
  ],
};

test("LDAP write-only password is scrubbed and omitted when left blank", () => {
  assert.equal(sanitizeSettings(fixture).items.at(-1).value, null);
  assert.deepEqual(
    buildSettingsChanges(fixture, { ldapBindPassword: "" }, "ldap"),
    { changes: {}, errors: {} },
  );
  assert.equal(
    settingsFailureMessage(new Error("Invalid synthetic-ldap-password"), {
      ldapBindPassword: "synthetic-ldap-password",
    }).includes("synthetic-ldap-password"),
    false,
  );
});

test("LDAP connection URL rejects embedded credentials and other protocols", () => {
  for (const ldapServerURL of [
    "https://ldap.test",
    "ldaps://user:pass@ldap.test",
    "ldaps://ldap.test/path",
    "ldaps://ldap.test?bind=secret",
  ]) {
    assert.ok(
      buildSettingsChanges(fixture, { ldapServerURL }, "ldap").errors
        .ldapServerURL,
    );
  }
  for (const ldapServerURL of [
    "ldaps://localhost:6360",
    "ldap://ad.company.test:389",
  ]) {
    assert.deepEqual(
      buildSettingsChanges(fixture, { ldapServerURL }, "ldap").errors,
      {},
    );
  }
});

test("enabling LDAP requires a configured service password", () => {
  const empty = {
    ...fixture,
    items: fixture.items.map((item) => ({ ...item, configured: false })),
  };
  assert.ok(buildSettingsChanges(empty, {}, "ldap").errors.ldapBindPassword);
  assert.deepEqual(
    buildSettingsChanges(
      empty,
      { ldapBindPassword: "synthetic-password" },
      "ldap",
    ).errors,
    {},
  );
});

test("LDAP presets switch immutable identity attributes without touching server or password", () => {
  const ad = ldapPreset("ad");
  assert.equal(ad.ldapIDAttribute, "objectGUID");
  assert.equal(ad.ldapLoginAttribute, "sAMAccountName");
  assert.equal(ldapPreset("generic").ldapIDAttribute, "entryUUID");
  assert.equal(ad.ldapBindPassword, undefined);
  assert.equal(ad.ldapServerURL, undefined);
  assert.ok(
    buildSettingsChanges(fixture, { ldapNestedGroups: true }, "ldap").errors
      .ldapNestedGroups,
  );
});
