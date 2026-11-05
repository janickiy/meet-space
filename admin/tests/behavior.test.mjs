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

test("password length is counted as Unicode code points with a letter and a digit", () => {
  assert.equal(validatePassword("НадёжныйПароль123"), null);
  assert.match(validatePassword("OnlyLettersPassword"), /букву и цифру/);
  assert.match(validatePassword("123456789012"), /букву и цифру/);
  assert.match(validatePassword("А1😀😀😀😀😀😀"), /12 до 128/);
  assert.match(validatePassword("a1".repeat(70)), /12 до 128/);
});

test("durations remain useful for recordings longer than one hour", () => {
  assert.equal(duration(null), "—");
  assert.equal(duration(0), "0:00");
  assert.equal(duration(3661), "1:01:01");
});
