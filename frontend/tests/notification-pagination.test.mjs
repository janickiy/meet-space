import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { api, configureAuth } from "../src/api.ts";

const originalFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = originalFetch;
  configureAuth(null, () => false);
});

test("notification requests use the requested modal or history page size", async () => {
  for (const limit of [10, 15]) {
    const controller = new AbortController();
    globalThis.fetch = async (input, init) => {
      const url = new URL(input, "https://synthetic.invalid");
      assert.equal(url.pathname, "/api/v1/notifications");
      assert.equal(url.searchParams.get("limit"), String(limit));
      assert.equal(url.searchParams.has("cursor"), false);
      assert.equal(init.signal, controller.signal);
      return Response.json({ items: [], nextCursor: "next", unreadCount: 79 });
    };
    const page = await api.notifications(undefined, controller.signal, limit);
    assert.equal(page.unreadCount, 79, "the unread total is not the page size");
    assert.equal(page.nextCursor, "next");
  }
});

test("notification pagination preserves opaque cursor bytes", async () => {
  const cursor = "opaque+/=& cursor";
  globalThis.fetch = async (input) => {
    const url = new URL(input, "https://synthetic.invalid");
    assert.equal(url.searchParams.get("cursor"), cursor);
    assert.equal(url.searchParams.get("limit"), "15");
    assert.equal([...url.searchParams.keys()].length, 2);
    return Response.json({ items: [], nextCursor: null, unreadCount: 0 });
  };
  await api.notifications(cursor, undefined, 15);
});

test("notification callers without an explicit size keep the previous default", async () => {
  globalThis.fetch = async (input) => {
    assert.equal(
      new URL(input, "https://synthetic.invalid").searchParams.get("limit"),
      "30",
    );
    return Response.json({ items: [], nextCursor: null, unreadCount: 0 });
  };
  await api.notifications();
});
