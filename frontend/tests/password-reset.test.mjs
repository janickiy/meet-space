import assert from "node:assert/strict";
import { afterEach, beforeEach, test } from "node:test";
import { api, ApiError, configureAuth, getAccessToken } from "../src/api.ts";

const originalFetch = globalThis.fetch;
let rejected;
beforeEach(() => {
  rejected = [];
  configureAuth("unrelated-account-token", (token) => {
    rejected.push(token);
    return false;
  });
});
afterEach(() => {
  globalThis.fetch = originalFetch;
  configureAuth(null, () => false);
});

test("reset posts credentials only in the body without using another active session", async () => {
  const controller = new AbortController();
  globalThis.fetch = async (url, options) => {
    assert.equal(url, "/api/v1/auth/reset-password");
    assert.equal(options.method, "POST");
    assert.equal(options.credentials, "omit");
    assert.equal(options.headers.has("Authorization"), false);
    assert.equal(options.signal, controller.signal);
    assert.deepEqual(JSON.parse(options.body), {
      token: "synthetic-one-time-link",
      password: "synthetic-new-password",
    });
    return new Response(JSON.stringify({ status: "success" }), { status: 200 });
  };
  assert.deepEqual(
    await api.resetPassword(
      "synthetic-one-time-link",
      "synthetic-new-password",
      controller.signal,
    ),
    { status: "success" },
  );
  assert.equal(getAccessToken(), "unrelated-account-token");
});

test("expired or reused links have an actionable message", async () => {
  globalThis.fetch = async () =>
    new Response(
      JSON.stringify({ message: "invalid or expired password reset link" }),
      { status: 422 },
    );
  await assert.rejects(
    api.resetPassword("used-token", "synthetic-new-password"),
    (error) =>
      error instanceof ApiError &&
      error.status === 422 &&
      error.message.includes("уже использована") &&
      error.message.includes("Запросите новую ссылку"),
  );
  assert.deepEqual(rejected, []);
});

test("blocked reset does not revoke an unrelated browser account", async () => {
  globalThis.fetch = async () =>
    new Response(JSON.stringify({ message: "account blocked" }), {
      status: 403,
    });
  await assert.rejects(
    api.resetPassword("blocked-token", "synthetic-new-password"),
    (error) =>
      error instanceof ApiError && error.message === "Аккаунт заблокирован.",
  );
  assert.deepEqual(rejected, []);
  assert.equal(getAccessToken(), "unrelated-account-token");
});
