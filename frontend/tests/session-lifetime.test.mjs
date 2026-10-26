import assert from "node:assert/strict";
import { afterEach, beforeEach, test } from "node:test";
import {
  accessTokenTTLSeconds,
  readSession,
  SESSION_KEY,
} from "../src/utils.ts";

const originalStorage = globalThis.sessionStorage;
const originalNow = Date.now;
const now = Date.UTC(2026, 9, 26);
const guestConferenceId = "00000000-0000-4000-8000-000000000002";
const user = {
  id: "00000000-0000-4000-8000-000000000001",
  email: "synthetic@example.invalid",
  displayName: null,
  createdAt: new Date(now).toISOString(),
  updatedAt: new Date(now).toISOString(),
};

beforeEach(() => {
  const values = new Map();
  globalThis.sessionStorage = {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
    removeItem: (key) => values.delete(key),
  };
  Date.now = () => now;
});

afterEach(() => {
  Date.now = originalNow;
  if (originalStorage === undefined) delete globalThis.sessionStorage;
  else globalThis.sessionStorage = originalStorage;
});

test("only a valid guest scope gets the three-hour client ceiling", () => {
  assert.equal(accessTokenTTLSeconds({ guestConferenceId }), 10800);
  for (const value of [undefined, user, { guestConferenceId: "invalid" }])
    assert.equal(accessTokenTTLSeconds(value), 3600);
});

test("a three-hour guest session survives cache reload", () => {
  const session = {
    token: "synthetic-guest-token",
    expiresAt: now + 10800_000,
    user: { ...user, guestConferenceId },
  };
  sessionStorage.setItem(SESSION_KEY, JSON.stringify(session));
  assert.deepEqual(readSession(), session);
});

test("cache rejects lifetimes beyond the appropriate account ceiling", () => {
  for (const [metadata, seconds] of [
    [user, 3601],
    [{ ...user, guestConferenceId }, 10801],
    [{ ...user, guestConferenceId: "invalid" }, 7200],
    [undefined, 7200],
  ]) {
    sessionStorage.setItem(
      SESSION_KEY,
      JSON.stringify({
        token: "synthetic-token",
        expiresAt: now + seconds * 1000,
        user: metadata,
      }),
    );
    assert.equal(readSession(), null);
    assert.equal(sessionStorage.getItem(SESSION_KEY), null);
  }
});
