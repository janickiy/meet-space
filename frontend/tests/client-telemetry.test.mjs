import assert from "node:assert/strict";
import { registerHooks } from "node:module";
import test from "node:test";

registerHooks({
  resolve(specifier, context, nextResolve) {
    if (
      specifier === "./buildInfo" &&
      context.parentURL.endsWith("/clientTelemetry.ts")
    ) {
      return {
        url: "data:text/javascript,export const appBuild={version:'fixture'};export const clientTelemetryEnabled=false;",
        shortCircuit: true,
      };
    }
    return nextResolve(specifier, context);
  },
});

const { telemetryRoute, createClientErrorReporter } =
  await import("../src/clientTelemetry.ts");

test("verification and password links retain only their fixed telemetry route", () => {
  for (const route of ["/verify-email", "/reset-password"]) {
    assert.equal(telemetryRoute(route), route);
    assert.equal(telemetryRoute(`${route}#token=PrivateFragmentCanary`), route);
    assert.equal(
      telemetryRoute(
        `${route}?token=PrivateQueryCanary#token=PrivateFragmentCanary`,
      ),
      route,
    );
    assert.equal(telemetryRoute(`${route}/PrivatePathCanary`), "unknown");
  }
});

test("email link tokens never enter serialized telemetry, including error messages and stack URLs", () => {
  const origin = "https://meet.example.test";
  for (const route of ["/verify-email", "/reset-password"]) {
    const sent = [];
    const reporter = createClientErrorReporter({
      enabled: true,
      version: "fixture",
      origin,
      pathname: () =>
        `${route}?token=PrivateQueryCanary#token=PrivateFragmentCanary`,
      userAgent: "Chrome/123 PrivateBrowserCanary",
      fetch: async (url, options) => {
        sent.push({ url, options });
        return new Response(null, { status: 202 });
      },
    });
    const error = new Error("PrivateErrorCanary PrivateFragmentCanary");
    error.stack = `Error: PrivateErrorCanary\n at PrivateFunctionCanary (${origin}/assets/index-AbCd1234.js:10:20)\n at private (${origin}/assets/index-AbCd1234.js?token=PrivateQueryCanary:30:40)\n at private (${origin}/assets/index-AbCd1234.js#token=PrivateFragmentCanary:30:40)`;
    reporter("uncaught_error", error);
    assert.equal(sent.length, 1);
    assert.equal(sent[0].url, "/api/v1/client-errors");
    assert.deepEqual(JSON.parse(sent[0].options.body), {
      version: "fixture",
      route,
      code: "uncaught_error",
      browser: "chromium",
      stack: [{ file: "index-AbCd1234.js", line: 10, column: 20 }],
    });
    assert.doesNotMatch(JSON.stringify(sent), /Private\w+Canary|token=|\?|#/);
    assert.equal(sent[0].options.credentials, "omit");
    assert.equal(sent[0].options.referrerPolicy, "no-referrer");
  }
});
