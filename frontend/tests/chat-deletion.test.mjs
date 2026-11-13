import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { ApiError, configureAuth, createChatAPI } from "../src/api.ts";
import { deleteChatMessages } from "../src/chatDeletion.ts";

const nativeFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = nativeFetch;
  configureAuth(null, () => false);
});

test("rate limiting stops a batch and preserves the failed and unprocessed messages", async () => {
  const controller = new AbortController();
  const calls = [];
  const completed = [];
  const limit = new ApiError(429, "Слишком много запросов.");
  const result = await deleteChatMessages(
    ["first", "limited", "unprocessed"],
    async (id, signal) => {
      assert.equal(signal, controller.signal);
      calls.push(id);
      if (id === "limited") throw limit;
      return { id, deletedAt: "2026-11-13T21:00:00Z" };
    },
    controller.signal,
    (id, message) => completed.push([id, message.id]),
  );
  assert.deepEqual(calls, ["first", "limited"]);
  assert.deepEqual(completed, [["first", "first"]]);
  assert.deepEqual(result.deleted, ["first"]);
  assert.deepEqual(result.remaining, ["limited", "unprocessed"]);
  assert.equal(result.error, limit);
});

test("each unique message is deleted once and no two requests run concurrently", async () => {
  const controller = new AbortController();
  const calls = [];
  const completed = [];
  const releases = [];
  let active = 0;
  const pending = deleteChatMessages(
    ["first", "second", "first", "third", "second"],
    async (id) => {
      active++;
      assert.equal(
        active,
        1,
        "bulk deletion must not issue concurrent requests",
      );
      calls.push(id);
      await new Promise((resolve) => releases.push(resolve));
      active--;
      return { id };
    },
    controller.signal,
    (id) => completed.push(id),
  );
  for (let index = 0; index < 3; index++) {
    assert.equal(calls.length, index + 1);
    assert.equal(active, 1);
    releases[index]();
    await new Promise(setImmediate);
  }
  const result = await pending;
  assert.deepEqual(calls, ["first", "second", "third"]);
  assert.deepEqual(completed, calls);
  assert.deepEqual(result, { deleted: calls, remaining: [], error: null });
  assert.equal(active, 0);
});

test("a cancelled selection never begins deleting messages", async () => {
  const controller = new AbortController();
  controller.abort();
  const result = await deleteChatMessages(
    ["first", "second"],
    async () => assert.fail("a cancelled batch must not send a request"),
    controller.signal,
    () => assert.fail("a cancelled batch has no completed deletions"),
  );
  assert.deepEqual(result.deleted, []);
  assert.deepEqual(result.remaining, ["first", "second"]);
});

test("cancelling an in-flight deletion prevents every later request", async () => {
  const controller = new AbortController();
  const calls = [];
  const pending = deleteChatMessages(
    ["first", "second", "third"],
    (id, signal) => {
      calls.push(id);
      return new Promise((resolve, reject) => {
        signal.addEventListener("abort", () => reject(signal.reason), {
          once: true,
        });
      });
    },
    controller.signal,
    () => assert.fail("the cancelled request did not confirm deletion"),
  );
  controller.abort();
  const result = await pending;
  assert.deepEqual(calls, ["first"]);
  assert.deepEqual(result.deleted, []);
  assert.deepEqual(result.remaining, ["first", "second", "third"]);
  assert.equal(result.error.name, "AbortError");
});

test("cancellation after a confirmed deletion keeps that success and stops the next request", async () => {
  const controller = new AbortController();
  const calls = [];
  const result = await deleteChatMessages(
    ["first", "second"],
    async (id) => {
      calls.push(id);
      return { id };
    },
    controller.signal,
    () => controller.abort(),
  );
  assert.deepEqual(calls, ["first"]);
  assert.deepEqual(result.deleted, ["first"]);
  assert.deepEqual(result.remaining, ["second"]);
});

for (const namespace of ["conferences", "conversations"]) {
  test(`${namespace} deletion forwards cancellation and rejects a late response after abort`, async () => {
    configureAuth("synthetic-token");
    const controller = new AbortController();
    let finish;
    globalThis.fetch = (url, options) => {
      assert.equal(url, `/api/v1/${namespace}/scope/messages/message`);
      assert.equal(options.method, "DELETE");
      assert.equal(options.signal, controller.signal);
      assert.equal(
        options.headers.get("Authorization"),
        "Bearer synthetic-token",
      );
      return new Promise((resolve) => {
        finish = resolve;
      });
    };
    const pending = createChatAPI(namespace).deleteMessage(
      "scope",
      "message",
      controller.signal,
    );
    controller.abort();
    finish(Response.json({ item: { id: "message", deletedAt: "now" } }));
    await assert.rejects(pending, (error) => error.name === "AbortError");
  });
}
