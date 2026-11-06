import test from "node:test";
import assert from "node:assert/strict";
import {
  createAdminErrorReporter,
  telemetryRoute,
  browserFamily,
  sanitizedStack,
} from "../src/clientTelemetry.ts";

const origin = "https://admin.example.test:8081";
function fixture(extra = {}) {
  const sent = [];
  let now = 0,
    active = true;
  const reporter = createAdminErrorReporter({
    version: "20261106-errors-test",
    origin,
    pathname: () => "/errors?token=private",
    userAgent: "Chrome/123 exact private user agent",
    isActive: () => active,
    now: () => now,
    send: async (event, signal) => {
      sent.push({ event, signal });
    },
    ...extra,
  });
  return {
    sent,
    reporter,
    setTime: (n) => {
      now = n;
    },
    setActive: (a) => {
      active = a;
    },
  };
}
test("admin telemetry only includes strict route and same-origin JS coordinates", () => {
  const f = fixture();
  const error = new Error("private-password-token-cookie-email");
  error.stack = `Error: private-password-token-cookie-email\n at fn (${origin}/assets/index-AbCd1234.js:10:20)\n at secret (https://outside.test/assets/index-AbCd1234.js:30:40)\n at private (${origin}/assets/index-AbCd1234.js?token=secret:30:40)`;
  f.reporter.report("uncaught_error", error);
  assert.deepEqual(f.sent[0].event, {
    version: "20261106-errors-test",
    route: "/errors",
    code: "uncaught_error",
    browser: "chromium",
    stack: [{ file: "index-AbCd1234.js", line: 10, column: 20 }],
  });
  assert.doesNotMatch(
    JSON.stringify(f.sent),
    /password|token|cookie|email|outside|user agent/,
  );
  assert.equal(telemetryRoute("/users/private-user-id"), "unknown");
  assert.equal(telemetryRoute("/settings#private"), "/settings");
  assert.equal(browserFamily("Firefox/121 private"), "firefox");
  assert.deepEqual(sanitizedStack("private error", origin), []);
  f.reporter.close();
});
test("inactive sessions never report; duplicates, minute and page budgets stay bounded", () => {
  const f = fixture();
  f.setActive(false);
  f.reporter.report("uncaught_error", null);
  assert.equal(f.sent.length, 0);
  f.setActive(true);
  for (let i = 0; i < 10; i++) f.reporter.report("uncaught_error", null);
  assert.equal(f.sent.length, 1);
  for (let i = 1; i < 8; i++) {
    f.setTime(i * 10_001);
    f.reporter.report("uncaught_error", null);
  }
  assert.equal(f.sent.length, 7); // minute budget resets at 60s
  for (let minute = 2; minute < 10; minute++) {
    for (let slot = 0; slot < 5; slot++) {
      f.setTime(minute * 60_000 + slot * 10_001);
      f.reporter.report("uncaught_error", null);
    }
  }
  assert.equal(f.sent.length, 20);
  f.reporter.close();
});
test("closing aborts pending reports and adapter errors do not recurse", async () => {
  const pending = [];
  const f = fixture({
    send: (_event, signal) => {
      pending.push(signal);
      return new Promise(() => {});
    },
  });
  f.reporter.report("unhandled_rejection", new Error("private"));
  assert.equal(pending.length, 1);
  f.reporter.close();
  assert.equal(pending[0].aborted, true);
  f.reporter.report("react_render_error", null);
  assert.equal(pending.length, 1);
  const throwing = fixture({
    send: () => {
      throw Error("private");
    },
  });
  assert.doesNotThrow(() => throwing.reporter.report("uncaught_error", null));
  throwing.reporter.close();
  const rejecting = fixture({ send: () => Promise.reject(Error("private")) });
  rejecting.reporter.report("uncaught_error", null);
  await new Promise((resolve) => setImmediate(resolve));
  rejecting.reporter.close();
});
