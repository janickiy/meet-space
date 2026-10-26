import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { api, ApiError, configureAuth } from "../src/api.ts";

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
  configureAuth(null, () => false);
});

/** Create a deliberately unfinished response and observe explicit cancellation. */
function response(status = 200, contentType = "text/event-stream") {
  let cancellations = 0;
  const body = new ReadableStream({
    cancel() {
      cancellations++;
    },
  });
  return {
    value: new Response(body, {
      status,
      headers: { "Content-Type": contentType },
    }),
    cancellations: () => cancellations,
  };
}

test("repeated rejected notification responses release every stream", async () => {
  const opened = [];
  globalThis.fetch = async () => {
    const item = response(503, "application/json");
    opened.push(item);
    return item.value;
  };
  for (let cycle = 0; cycle < 50; cycle++) {
    await assert.rejects(
      api.notificationEvents(new AbortController().signal),
      (error) => error instanceof ApiError && error.status === 503,
    );
  }
  assert.equal(opened.length, 50);
  assert.equal(
    opened.reduce((sum, item) => sum + item.cancellations(), 0),
    50,
    "each failed connection must be canceled before the next retry",
  );
});

test("invalid SSE content type releases an otherwise successful response", async () => {
  const item = response(200, "text/html");
  globalThis.fetch = async () => item.value;
  await assert.rejects(
    api.notificationEvents(new AbortController().signal),
    (error) => error instanceof ApiError && error.status === 502,
  );
  assert.equal(item.cancellations(), 1);
});

test("401 response is released before session recovery and replacement request", async () => {
  const unauthorized = response(401, "application/json");
  const success = response();
  let requests = 0;
  configureAuth("synthetic-old-token", () => {
    assert.equal(unauthorized.cancellations(), 1);
    configureAuth("synthetic-renewed-token");
    return true;
  });
  globalThis.fetch = async () =>
    (++requests === 1 ? unauthorized : success).value;
  const result = await api.notificationEvents(new AbortController().signal);
  assert.equal(requests, 2);
  assert.equal(result, success.value);
  assert.equal(success.cancellations(), 0, "caller owns the valid SSE body");
  await result.body.cancel();
});

test("response is released even when session recovery throws", async () => {
  const item = response(401, "application/json");
  configureAuth("synthetic-old-token", () => {
    throw new Error("synthetic recovery failure");
  });
  globalThis.fetch = async () => item.value;
  await assert.rejects(
    api.notificationEvents(new AbortController().signal),
    /synthetic recovery failure/,
  );
  assert.equal(item.cancellations(), 1);
});

test("a response from a previous account is canceled rather than returned", async () => {
  const item = response();
  configureAuth("synthetic-first-account");
  globalThis.fetch = async () => {
    configureAuth(null);
    configureAuth("synthetic-second-account");
    return item.value;
  };
  await assert.rejects(
    api.notificationEvents(new AbortController().signal),
    (error) => error instanceof DOMException && error.name === "AbortError",
  );
  assert.equal(item.cancellations(), 1);
});

test("abort during response acquisition cancels the body", async () => {
  const item = response();
  const controller = new AbortController();
  globalThis.fetch = async () => {
    controller.abort();
    return item.value;
  };
  await assert.rejects(
    api.notificationEvents(controller.signal),
    (error) => error instanceof DOMException && error.name === "AbortError",
  );
  assert.equal(item.cancellations(), 1);
});

test("valid SSE response remains open for the caller to consume", async () => {
  const item = response(200, "text/event-stream; charset=utf-8");
  globalThis.fetch = async () => item.value;
  const result = await api.notificationEvents(new AbortController().signal);
  assert.equal(result, item.value);
  assert.equal(item.cancellations(), 0);
  await result.body.cancel();
  assert.equal(item.cancellations(), 1);
});
