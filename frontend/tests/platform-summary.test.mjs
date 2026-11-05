import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { api, configureAuth } from "../src/api.ts";

const originalFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = originalFetch;
  configureAuth(null, () => false);
});

test("legacy diagnostics use the user API outside the admin identity boundary", async () => {
  const item = { apiReady: true, activeConferences: 0 };
  globalThis.fetch = async (input, init) => {
    assert.equal(input, "/api/v1/platform/summary");
    assert.equal(init.method ?? "GET", "GET");
    return Response.json({ status: "success", item });
  };
  assert.deepEqual((await api.adminSummary()).item, item);
});
