import assert from "node:assert/strict";
import { afterEach, beforeEach, test } from "node:test";
import { api, ApiError, configureAuth } from "../src/api.ts";

const nativeFetch = globalThis.fetch;
beforeEach(() => {
  configureAuth(null, () => false);
  configureAuth("account-a-token");
});
afterEach(() => {
  globalThis.fetch = nativeFetch;
  configureAuth(null, () => false);
});

function switchAccount() {
  configureAuth(null);
  configureAuth("account-b-token");
}

test("a late 204 mutation cannot report success after switching accounts", async () => {
  let resolve;
  globalThis.fetch = () =>
    new Promise((done) => {
      resolve = done;
    });
  const pending = api.disconnectCalendar("calendar-a");
  switchAccount();
  resolve(new Response(null, { status: 204 }));
  await assert.rejects(
    pending,
    (error) => error instanceof ApiError && error.status === 409,
  );
});

test("an unchanged account still accepts an empty 204 response", async () => {
  globalThis.fetch = async () => new Response(null, { status: 204 });
  assert.equal(await api.disconnectCalendar("calendar-a"), undefined);
});

test("JSON body cancellation remains AbortError without session recovery", async () => {
  let recoveries = 0;
  configureAuth("account-a-token", () => {
    recoveries++;
    return false;
  });
  for (const status of [200, 401]) {
    globalThis.fetch = async () => ({
      status,
      ok: status === 200,
      json: async () => {
        throw new DOMException("Cancelled body", "AbortError");
      },
    });
    await assert.rejects(api.me(), (error) => error.name === "AbortError");
  }
  assert.equal(recoveries, 0);
});

test("invalid successful JSON still surfaces a protocol error", async () => {
  globalThis.fetch = async () => new Response("proxy HTML", { status: 200 });
  await assert.rejects(
    api.me(),
    (error) => error instanceof ApiError && error.status === 502,
  );
});

for (const kind of ["profile", "group"]) {
  test(`late ${kind} avatar metadata is discarded when identity changes while reading the body`, async () => {
    let finishBody;
    let bodyStarted;
    const started = new Promise((resolve) => {
      bodyStarted = resolve;
    });
    globalThis.fetch = async () => ({
      ok: true,
      status: 200,
      json: () =>
        new Promise((resolve) => {
          finishBody = resolve;
          bodyStarted();
        }),
    });
    const file = new File(["synthetic JPEG"], "avatar.jpg", {
      type: "image/jpeg",
    });
    const pending =
      kind === "profile"
        ? api.putProfileAvatar(file)
        : api.putGroupAvatar("group-a", file);
    await started;
    switchAccount();
    finishBody({
      status: "success",
      user: { id: "user-a" },
      item: { id: "group-a" },
    });
    await assert.rejects(pending, (error) => error.name === "AbortError");
  });
}

test("cancelled binary error bodies cannot initiate session recovery", async () => {
  let recoveries = 0;
  configureAuth("account-a-token", () => {
    recoveries++;
    return false;
  });
  globalThis.fetch = async () => ({
    ok: false,
    status: 401,
    json: async () => {
      throw new DOMException("Cancelled body", "AbortError");
    },
  });
  await assert.rejects(
    api.userAvatar("user-a"),
    (error) => error.name === "AbortError",
  );
  assert.equal(recoveries, 0);
});

test("valid avatar metadata remains available to its original account", async () => {
  const expected = { status: "success", user: { id: "user-a" } };
  globalThis.fetch = async () => Response.json(expected);
  const file = new File(["synthetic JPEG"], "avatar.jpg", {
    type: "image/jpeg",
  });
  assert.deepEqual(await api.putProfileAvatar(file), expected);
});
