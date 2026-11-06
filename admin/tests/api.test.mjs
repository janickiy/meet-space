import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { api, APIError, configureSession, queryString } from "../src/api.ts";

const nativeFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = nativeFetch;
  configureSession(null);
});

test("mutations use same-origin credentials and an in-memory CSRF token, including DELETE", async () => {
  configureSession({
    administrator: { id: "admin" },
    csrfToken: "csrf-for-test",
  });
  globalThis.fetch = async (url, options) => {
    assert.equal(url, "/api/v1/admin/recordings/42");
    assert.equal(options.credentials, "same-origin");
    assert.equal(options.headers["X-CSRF-Token"], "csrf-for-test");
    assert.equal(options.headers["Content-Type"], "application/json");
    assert.equal(options.headers.Authorization, undefined);
    assert.equal(options.body, "{}");
    assert.equal(options.redirect, "error");
    return new Response(null, { status: 204 });
  };
  assert.equal(await api("/recordings/42", { method: "DELETE" }), null);
});

test("clearing a session removes its CSRF token before a new login", async () => {
  configureSession({ administrator: {}, csrfToken: "previous-secret" });
  configureSession(null);
  globalThis.fetch = async (_, options) => {
    assert.equal(options.headers["X-CSRF-Token"], undefined);
    assert.deepEqual(JSON.parse(options.body), {
      username: "operator",
      password: "test",
    });
    return Response.json({ administrator: {}, csrfToken: "next" });
  };
  await api("/auth/login", {
    method: "POST",
    body: { username: "operator", password: "test" },
  });
});

test("expired-session responses trigger logout but bad login does not", async () => {
  let expired = 0;
  configureSession(null, () => {
    expired++;
  });
  globalThis.fetch = async () =>
    Response.json(
      { error: "unauthorized", message: "Сессия завершена" },
      { status: 401 },
    );
  await assert.rejects(
    api("/auth/login", { method: "POST" }),
    (error) => error instanceof APIError && error.status === 401,
  );
  assert.equal(expired, 0);
  await assert.rejects(
    api("/users"),
    (error) =>
      error instanceof APIError && error.message === "Сессия завершена",
  );
  assert.equal(expired, 1);
});

test("network cancellation stays abortable instead of becoming a server error", async () => {
  const controller = new AbortController();
  controller.abort();
  let called = false;
  globalThis.fetch = async () => {
    called = true;
    return Response.json({});
  };
  await assert.rejects(
    api("/users", { signal: controller.signal }),
    (error) => error.name === "AbortError",
  );
  assert.equal(called, false);
});

test("filters are encoded and sentinel values never reach the API", () => {
  const result = queryString({
    page: 1,
    q: "Анна & status=blocked",
    status: "all",
    ignored: undefined,
    empty: "",
  });
  const parsed = new URLSearchParams(result);
  assert.equal(parsed.get("q"), "Анна & status=blocked");
  assert.equal(parsed.get("status"), null);
  assert.equal(parsed.get("page"), "1");
});

test("the API client cannot send session credentials to an external URL", async () => {
  let called = false;
  globalThis.fetch = async () => {
    called = true;
    return Response.json({});
  };
  await assert.rejects(api("//example.com/collect"));
  await assert.rejects(api("https://example.com/collect"));
  assert.equal(called, false);
});

test("server conflict keeps its Russian explanation available for last-admin protection", async () => {
  globalThis.fetch = async () =>
    Response.json(
      {
        error: "conflict",
        message: "Нельзя удалить последнего активного администратора",
      },
      { status: 409 },
    );
  await assert.rejects(
    api("/administrators/last", { method: "DELETE" }),
    (error) =>
      error instanceof APIError &&
      error.status === 409 &&
      error.message.includes("последнего"),
  );
});

test("a late 401 from a previous administrator cannot sign out the new session", async () => {
  let resolveResponse;
  let expired = 0;
  configureSession({ administrator: { id: "first" }, csrfToken: "first" });
  globalThis.fetch = () =>
    new Promise((resolve) => {
      resolveResponse = resolve;
    });
  const pending = api("/users");
  configureSession(
    { administrator: { id: "second" }, csrfToken: "second" },
    () => {
      expired++;
    },
  );
  resolveResponse(
    Response.json({ message: "Сессия завершена" }, { status: 401 }),
  );
  await assert.rejects(pending, (error) => error.name === "AbortError");
  assert.equal(expired, 0);
});

test("successful responses from an old session are discarded after logout", async () => {
  let resolveResponse;
  configureSession({ administrator: { id: "first" }, csrfToken: "first" });
  globalThis.fetch = () =>
    new Promise((resolve) => {
      resolveResponse = resolve;
    });
  const pending = api("/auth/me");
  configureSession(null);
  resolveResponse(
    Response.json({ administrator: { id: "first" }, csrfToken: "first" }),
  );
  await assert.rejects(pending, (error) => error.name === "AbortError");
});

test("aborting response-body parsing is not converted into successful null data", async () => {
  globalThis.fetch = async () => ({
    ok: true,
    status: 200,
    json: async () => {
      throw new DOMException("Aborted", "AbortError");
    },
  });
  await assert.rejects(api("/users"), (error) => error.name === "AbortError");
});

test("successful non-JSON responses are surfaced as protocol errors", async () => {
  globalThis.fetch = async () =>
    new Response("<html>Proxy error</html>", { status: 200 });
  await assert.rejects(
    api("/users"),
    (error) => error instanceof APIError && error.status === 502,
  );
});

test("internal paths cannot escape the administrative namespace", async () => {
  let called = false;
  globalThis.fetch = async () => {
    called = true;
    return Response.json({});
  };
  for (const path of [
    "/../users",
    "/%2e%2e/users",
    "/users/%2f..",
    "/users\\..\\..\\auth",
  ])
    await assert.rejects(api(path));
  assert.equal(called, false);
});

test("a session switch aborts in-flight mutations as well as reads", async () => {
  configureSession({ administrator: { id: "first" }, csrfToken: "first" });
  let signal;
  globalThis.fetch = async (_, options) => {
    signal = options.signal;
    return new Promise((_, reject) =>
      signal.addEventListener("abort", () => reject(signal.reason), {
        once: true,
      }),
    );
  };
  const request = api("/users/test", {
    method: "PATCH",
    body: { status: "blocked" },
  });
  configureSession(null);
  assert.equal(signal.aborted, true);
  await assert.rejects(request, (error) => error.name === "AbortError");
});

test("unmounting a caller cancels an already pending request", async () => {
  const caller = new AbortController();
  let forwarded;
  globalThis.fetch = async (_, options) => {
    forwarded = options.signal;
    return new Promise((_, reject) =>
      forwarded.addEventListener("abort", () => reject(forwarded.reason), {
        once: true,
      }),
    );
  };
  const request = api("/users", { signal: caller.signal });
  caller.abort();
  assert.equal(forwarded.aborted, true);
  await assert.rejects(request, (error) => error.name === "AbortError");
});

test("an unresponsive request is bounded by a 30 second timeout", async (context) => {
  context.mock.timers.enable({ apis: ["setTimeout"] });
  let signal;
  globalThis.fetch = async (_, options) => {
    signal = options.signal;
    return new Promise((_, reject) =>
      signal.addEventListener("abort", () => reject(signal.reason), {
        once: true,
      }),
    );
  };
  const request = api("/system");
  context.mock.timers.tick(29_999);
  assert.equal(signal.aborted, false);
  context.mock.timers.tick(1);
  assert.equal(signal.aborted, true);
  await assert.rejects(
    request,
    (error) =>
      error instanceof APIError &&
      error.status === 0 &&
      error.message.includes("вовремя"),
  );
});
