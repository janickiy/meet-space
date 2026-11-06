import assert from "node:assert/strict";
import { test } from "node:test";
import {
  bytes,
  canDeleteRecording,
  count,
  duration,
  percent,
  validateAdministrator,
  validatePassword,
} from "../src/format.ts";

test("missing metrics remain unknown while legitimate zero is visible", () => {
  assert.equal(count(null), "Нет данных");
  assert.equal(count(undefined), "Нет данных");
  assert.equal(count(0), "0");
  assert.equal(bytes(null), "Нет данных");
  assert.equal(bytes(0), "0 Б");
  assert.equal(percent(42, null), null);
  assert.equal(percent(0, 100), 0);
});

test("recording deletion requires a safe terminal state and server permission", () => {
  for (const status of ["ready", "partial_ready", "failed", "cancelled"]) {
    assert.equal(canDeleteRecording(status, true), true);
    assert.equal(canDeleteRecording(status, false), false);
  }
  for (const status of [
    "recording",
    "starting",
    "stopping",
    "degraded",
    "finalizing",
    "uploading",
    "unknown-new-state",
  ]) {
    assert.equal(canDeleteRecording(status, true), false);
  }
});

test("username-only identity and password confirmation are validated", () => {
  assert.match(
    validateAdministrator("admin@example.com", "", "", false),
    /Логин/,
  );
  assert.match(
    validateAdministrator(
      "operator",
      "LongPassword123",
      "DifferentPassword456",
      true,
    ),
    /не совпадают/,
  );
  assert.equal(
    validateAdministrator(
      "operator.one",
      "LongPassword123",
      "LongPassword123",
      true,
    ),
    null,
  );
});

test("passwords accept 8–128 characters without composition requirements", () => {
  const syntheticNumericPassword = "90716428";
  assert.equal(validatePassword(syntheticNumericPassword), null);
  assert.match(
    validatePassword(syntheticNumericPassword.slice(0, 7)),
    /8 до 128/,
  );
  assert.equal(validatePassword("abcdefgh"), null);
  assert.equal(validatePassword("1".repeat(128)), null);
  assert.match(validatePassword("1".repeat(129)), /8 до 128/);
  assert.equal(
    validateAdministrator(
      "operator",
      syntheticNumericPassword,
      syntheticNumericPassword,
      true,
    ),
    null,
  );
});

test("password limits count Unicode code points and retain the 512 byte UTF-8 boundary", () => {
  assert.equal(validatePassword("😀".repeat(8)), null);
  assert.match(validatePassword("😀".repeat(7)), /8 до 128/);
  assert.equal(validatePassword("е\u0301".repeat(4)), null);
  const maximum = "😀".repeat(128);
  assert.equal([...maximum].length, 128);
  assert.equal(new TextEncoder().encode(maximum).length, 512);
  assert.equal(validatePassword(maximum), null);
  assert.match(validatePassword("😀".repeat(129)), /8 до 128/);
});

test("durations remain useful for recordings longer than one hour", () => {
  assert.equal(duration(null), "—");
  assert.equal(duration(0), "0:00");
  assert.equal(duration(3661), "1:01:01");
});
