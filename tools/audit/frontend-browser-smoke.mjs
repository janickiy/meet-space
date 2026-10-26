import assert from "node:assert/strict";
import { readFile, mkdir, writeFile } from "node:fs/promises";
import http from "node:http";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

// Built-frontend smoke and bounded navigation sampling, with all API calls mocked.
const root = fileURLToPath(new URL("../../", import.meta.url));
const directories = process.argv.slice(2).map((value) => path.resolve(value));
if (!directories.length)
  throw new Error("Pass one or more built frontend directories");
const warmup = Number(process.env.AUDIT_NAVIGATION_WARMUP || 5);
const cycles = Number(process.env.AUDIT_NAVIGATION_CYCLES || 30);
if (
  !Number.isInteger(warmup) ||
  warmup < 0 ||
  warmup > 100 ||
  !Number.isInteger(cycles) ||
  cycles < 10 ||
  cycles > 300 ||
  cycles % 10
)
  throw new Error(
    "Use 0–100 warmup cycles and 10–300 measured cycles in multiples of 10",
  );
const { chromium } = await import(
  process.env.PLAYWRIGHT_MODULE_PATH
    ? pathToFileURL(process.env.PLAYWRIGHT_MODULE_PATH).href
    : "playwright"
);
const output = process.env.AUDIT_OUTPUT_DIR
  ? path.resolve(process.env.AUDIT_OUTPUT_DIR)
  : path.join(root, "tmp/web-audit-browser");
await mkdir(output, { recursive: true });
const mime = {
  ".js": "text/javascript",
  ".css": "text/css",
  ".html": "text/html",
  ".json": "application/json",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".webp": "image/webp",
  ".jpg": "image/jpeg",
  ".woff2": "font/woff2",
  ".woff": "font/woff",
  ".wav": "audio/wav",
  ".ico": "image/x-icon",
};
let activeDirectory = directories[0];
const server = http.createServer(async (request, response) => {
  try {
    const pathname = decodeURIComponent(
      new URL(request.url, "http://localhost").pathname,
    );
    const requested = path.resolve(activeDirectory, `.${pathname}`);
    if (
      !requested.startsWith(`${activeDirectory}${path.sep}`) &&
      requested !== activeDirectory
    ) {
      response.writeHead(403).end();
      return;
    }
    const filename = path.extname(requested)
      ? requested
      : path.join(activeDirectory, "index.html");
    const content = await readFile(filename);
    response.writeHead(200, {
      "Content-Type":
        mime[path.extname(filename)] || "application/octet-stream",
    });
    response.end(content);
  } catch {
    response.writeHead(404).end();
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
const report = {
  browser: await browser.version(),
  scope: `Loopback static production builds; all API calls mocked as unauthenticated. No real users, backend, video calls, credentials, or production services. Desktop: ${warmup} warmup + ${cycles} SPA navigation cycles (landing/login/register/landing). Mobile smoke at 390px. GC heap samples are finite-run evidence, not proof of absence of leaks or production Web Vitals.`,
  cases: [],
};
try {
  for (const directory of directories) {
    activeDirectory = directory;
    for (const viewport of [
      { width: 1440, height: 1000 },
      { width: 390, height: 844 },
    ]) {
      const context = await browser.newContext({
        viewport,
        reducedMotion: "reduce",
        serviceWorkers: "block",
      });
      const item = {
        build: path.basename(directory),
        viewport,
        checks: [],
        errors: [],
        blockedExternal: [],
        apiRequests: [],
        heapSamples: [],
        heapSnapshots: [],
      };
      report.cases.push(item);
      await context.route("**/*", async (route) => {
        const request = route.request();
        const url = new URL(request.url());
        if (url.origin !== origin) {
          item.blockedExternal.push(url.origin);
          await route.abort();
        } else if (url.pathname.startsWith("/api/")) {
          item.apiRequests.push(`${request.method()} ${url.pathname}`);
          await route.fulfill({
            status: 401,
            contentType: "application/json",
            body: JSON.stringify({ status: "failed", message: "unauthorized" }),
          });
        } else await route.continue();
      });
      const page = await context.newPage();
      page.setDefaultTimeout(10000);
      page.on("pageerror", (error) => item.errors.push(error.message));
      const check = (name, passed) => {
        item.checks.push({ name, passed });
        assert.equal(passed, true, name);
      };
      await page.goto(origin);
      await page.locator("#home-title").waitFor();
      await page.evaluate(() => document.fonts.ready);
      check(
        "landing has no horizontal overflow",
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth + 1,
        ),
      );
      await page.screenshot({
        path: path.join(output, `${item.build}-${viewport.width}-landing.png`),
        fullPage: true,
      });
      await page.locator(".home-account-nav a").click();
      await page.locator("#email").waitFor();
      check("login navigation", new URL(page.url()).pathname === "/login");
      check(
        "login has no horizontal overflow",
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth + 1,
        ),
      );
      await page.locator('form button[type="submit"]').click();
      await page
        .getByText("Введите корректный email.", { exact: true })
        .waitFor();
      check("empty login validation", true);
      await page.locator('.auth-mode-switch a[href="/register"]').click();
      await page
        .getByRole("heading", { name: "Создайте аккаунт", exact: true })
        .waitFor();
      check(
        "registration navigation",
        new URL(page.url()).pathname === "/register",
      );
      check(
        "registration has no horizontal overflow",
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth + 1,
        ),
      );
      await page.screenshot({
        path: path.join(output, `${item.build}-${viewport.width}-register.png`),
        fullPage: true,
      });
      await page.goto(`${origin}/personal`);
      await page.locator("#email").waitFor();
      check(
        "protected page redirects to login",
        new URL(page.url()).pathname === "/login",
      );
      check(
        "protected redirect preserves destination",
        new URL(page.url()).searchParams.get("next") === "/personal",
      );
      await page.locator(".auth-back").click();
      await page.locator("#home-title").waitFor();
      if (viewport.width === 1440) {
        const session = await context.newCDPSession(page);
        await session.send("HeapProfiler.enable");
        const cycle = async () => {
          await page.locator(".home-account-nav a").click();
          await page.locator('.auth-mode-switch a[href="/register"]').click();
          await page
            .getByRole("heading", { name: "Создайте аккаунт", exact: true })
            .waitFor();
          await page.locator(".auth-back").click();
          await page.locator("#home-title").waitFor();
        };
        const sample = async (completedCycles) => {
          await page.waitForTimeout(250);
          await session.send("HeapProfiler.collectGarbage");
          await page.waitForTimeout(100);
          await session.send("HeapProfiler.collectGarbage");
          const heap = await session.send("Runtime.getHeapUsage");
          const dom = await session.send("Memory.getDOMCounters");
          item.heapSamples.push({
            cycles: completedCycles,
            usedHeapBytes: heap.usedSize,
            ...dom,
          });
          if (
            process.env.AUDIT_HEAP_SNAPSHOTS === "1" &&
            (completedCycles === 0 || completedCycles === cycles)
          ) {
            const chunks = [];
            const collect = ({ chunk }) => chunks.push(chunk);
            session.on("HeapProfiler.addHeapSnapshotChunk", collect);
            try {
              await session.send("HeapProfiler.takeHeapSnapshot");
            } finally {
              session.off("HeapProfiler.addHeapSnapshotChunk", collect);
            }
            const filename = `${item.build}-heap-${completedCycles}.heapsnapshot`;
            await writeFile(path.join(output, filename), chunks.join(""));
            item.heapSnapshots.push(filename);
          }
        };
        for (let index = 0; index < warmup; index++) await cycle();
        await sample(0);
        for (let index = 1; index <= cycles; index++) {
          await cycle();
          if (index % 10 === 0) await sample(index);
        }
      }
      check("no uncaught page errors", item.errors.length === 0);
      check("no external requests", item.blockedExternal.length === 0);
      await context.close();
    }
  }
} finally {
  await browser.close();
  server.closeAllConnections();
  await new Promise((resolve) => server.close(resolve));
  await writeFile(
    path.join(output, "frontend-smoke.json"),
    `${JSON.stringify(report, null, 2)}\n`,
  );
  console.log(JSON.stringify(report, null, 2));
}
