import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { readFile } from "node:fs/promises";
import http from "node:http";
import { stripTypeScriptTypes } from "node:module";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

// Optional local-browser regression: never connects to a deployed MeetSpace instance.
const root = fileURLToPath(new URL("../../", import.meta.url));
const playwrightModule = process.env.PLAYWRIGHT_MODULE_PATH;
const { chromium } = await import(
  playwrightModule ? pathToFileURL(playwrightModule).href : "playwright"
);
const current = await readFile(path.join(root, "frontend/src/api.ts"), "utf8");
const baseline = execFileSync("git", ["show", "HEAD:frontend/src/api.ts"], {
  cwd: root,
  encoding: "utf8",
});
const modules = new Map(
  [
    ["/baseline.js", baseline],
    ["/current.js", current],
  ].map(([url, source]) => [
    url,
    stripTypeScriptTypes(source, { mode: "transform" }),
  ]),
);
const openResponses = new Set();
let status = 503;
const server = http.createServer((request, response) => {
  if (request.url === "/api/v1/notifications/events") {
    openResponses.add(response);
    response.on("close", () => openResponses.delete(response));
    response.writeHead(status, {
      "Content-Type": "text/html",
      "Cache-Control": "no-store",
    });
    response.write("synthetic unfinished response\n");
  } else if (modules.has(request.url)) {
    response.writeHead(200, { "Content-Type": "text/javascript" });
    response.end(modules.get(request.url));
  } else {
    response.writeHead(200, { "Content-Type": "text/html" });
    response.end("<!doctype html><title>Local SSE cleanup regression</title>");
  }
});
await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const origin = `http://127.0.0.1:${server.address().port}`;
const browser = await chromium.launch({
  headless: true,
  ...(process.env.CHROME_EXECUTABLE_PATH
    ? { executablePath: process.env.CHROME_EXECUTABLE_PATH }
    : {}),
  args: ["--disable-background-networking"],
});
const results = [];
try {
  for (const variant of ["baseline", "current"]) {
    for (const responseStatus of [503, 200]) {
      status = responseStatus;
      const context = await browser.newContext({ serviceWorkers: "block" });
      await context.route("**/*", (route) =>
        route.request().url().startsWith(`${origin}/`)
          ? route.continue()
          : route.abort(),
      );
      const page = await context.newPage();
      await page.goto(origin);
      const session = await context.newCDPSession(page);
      await session.send("HeapProfiler.enable");
      await session.send("HeapProfiler.collectGarbage");
      const before = await session.send("Runtime.getHeapUsage");
      const cycles = 5;
      const rejected = await page.evaluate(
        async ({ variant, cycles }) => {
          const { api } = await import(`/${variant}.js`);
          let rejected = 0;
          for (let index = 0; index < cycles; index++) {
            try {
              await api.notificationEvents(new AbortController().signal);
            } catch {
              rejected++;
            }
          }
          return rejected;
        },
        { variant, cycles },
      );
      // Five requests stay below the browser's six-connection HTTP/1 pool limit.
      await page.waitForTimeout(500);
      const openBeforeGC = openResponses.size;
      await session.send("HeapProfiler.collectGarbage");
      await page.waitForTimeout(500);
      const after = await session.send("Runtime.getHeapUsage");
      const item = {
        variant,
        responseStatus,
        cycles,
        rejected,
        openBeforeGC,
        openAfterGC: openResponses.size,
        usedHeapBytesBefore: before.usedSize,
        usedHeapBytesAfter: after.usedSize,
      };
      results.push(item);
      assert.equal(rejected, cycles);
      if (variant === "current") assert.equal(item.openBeforeGC, 0);
      await context.close();
      for (const response of openResponses) response.destroy();
      openResponses.clear();
    }
  }
  console.log(
    JSON.stringify(
      {
        browser: await browser.version(),
        scope:
          "Loopback-only HTTP/1.1 fixture, five sequential requests per scenario; synthetic unfinished error/invalid-MIME responses. Heap deltas include module warmup and are not leak measurements.",
        results,
      },
      null,
      2,
    ),
  );
} finally {
  await browser.close();
  for (const response of openResponses) response.destroy();
  server.closeAllConnections();
  await new Promise((resolve) => server.close(resolve));
}
