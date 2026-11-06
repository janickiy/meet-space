import assert from "node:assert/strict";
import { afterEach, beforeEach, test } from "node:test";
import {
  api,
  ApiError,
  configureAuth,
  getAccessToken,
  isAccountBlocked,
} from "../src/api.ts";
const originalFetch = globalThis.fetch;
let rejected;
const response = (status, message) =>
  new Response(JSON.stringify({ message }), {
    status,
    headers: { "Content-Type": "application/json" },
  });
beforeEach(() => {
  rejected = [];
  configureAuth(null, (token, reason) => {
    rejected.push({ token, reason });
    return false;
  });
});
afterEach(() => {
  globalThis.fetch = originalFetch;
  configureAuth(null, () => false);
});
test("protected blocked response rejects only the token used by the request", async () => {
  configureAuth("current-token");
  globalThis.fetch = async () => response(403, "account blocked");
  await assert.rejects(
    api.me(),
    (error) =>
      isAccountBlocked(error) && error.message === "Аккаунт заблокирован.",
  );
  assert.deepEqual(rejected, [{ token: "current-token", reason: "blocked" }]);
});
test("ordinary permission failure never rejects an active account", async () => {
  configureAuth("current-token");
  globalThis.fetch = async () => response(403, "access denied");
  await assert.rejects(
    api.me(),
    (error) => error instanceof ApiError && !isAccountBlocked(error),
  );
  assert.deepEqual(rejected, []);
  assert.equal(getAccessToken(), "current-token");
});
test("response from an account switched away cannot reject the new account", async () => {
  configureAuth("old-token");
  let finish;
  globalThis.fetch = () =>
    new Promise((resolve) => {
      finish = resolve;
    });
  const pending = api.me();
  configureAuth(null);
  configureAuth("new-token");
  finish(response(403, "account blocked"));
  await assert.rejects(
    pending,
    (error) => error instanceof ApiError && error.status === 409,
  );
  assert.deepEqual(rejected, []);
  assert.equal(getAccessToken(), "new-token");
});
test("public auth forms translate blocked and duplicate email without rejecting another session", async () => {
  configureAuth("other-account");
  globalThis.fetch = async () => response(403, "account blocked");
  await assert.rejects(
    api.login("blocked@example.test", "synthetic-password"),
    isAccountBlocked,
  );
  await assert.rejects(
    api.recoverPassword("blocked@example.test"),
    isAccountBlocked,
  );
  globalThis.fetch = async () => response(409, "email is already registered");
  await assert.rejects(
    api.register("blocked@example.test", "synthetic-password", "QA"),
    (error) =>
      error.message === "Пользователь с таким email уже есть в системе.",
  );
  assert.deepEqual(rejected, []);
});
test("unavailable recovery describes the current service capability", async () => {
  globalThis.fetch = async () => response(503, "password recovery unavailable");
  await assert.rejects(
    api.recoverPassword("active@example.test"),
    (error) =>
      error.message ===
      "Восстановление пароля недоступно. Обратитесь к администратору сервиса.",
  );
});
test("blocked file request uses the same scoped session rejection", async () => {
  configureAuth("file-token");
  globalThis.fetch = async () => response(403, "account blocked");
  await assert.rejects(api.userAvatar("synthetic-user"), isAccountBlocked);
  assert.deepEqual(rejected, [{ token: "file-token", reason: "blocked" }]);
});
