const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const http = require("node:http");
const path = require("node:path");
const zlib = require("node:zlib");
const { chromium } = require(
  process.env.PLAYWRIGHT_MODULE_PATH || "playwright",
);
const [
  directory = "dist",
  reportPath = "/tmp/meetspace-public-routes.json",
  mode = "check",
] = process.argv.slice(2);
const root = path.resolve(directory);
const paths = [
  "/",
  "/welcome",
  "/download",
  "/self-hosted",
  "/docs",
  "/docs/self-hosted",
  "/docs/overview",
  "/docs/features",
  "/docs/architecture",
  "/docs/interface",
  "/docs/support",
  "/docs/before-you-begin",
  "/docs/system-requirements",
  "/docs/source-and-releases",
  "/docs/installation",
  "/docs/updates-and-backups",
  "/docs/troubleshooting",
];

(async () => {
  const server = http.createServer(async (req, res) => {
    try {
      const pathname = new URL(req.url, "http://localhost").pathname;
      if (pathname.startsWith("/api/")) {
        res.writeHead(401, { "Content-Type": "application/json" });
        return res.end(JSON.stringify({ message: "unauthorized" }));
      }
      const relative = paths.includes(pathname)
        ? path.join(pathname, "index.html")
        : path.extname(pathname)
          ? pathname
          : "/app.html";
      const file = path.join(root, relative);
      if (!file.startsWith(root + path.sep)) throw new Error("Invalid path");
      const data = await fs.readFile(file);
      const type =
        {
          ".html": "text/html",
          ".js": "text/javascript",
          ".css": "text/css",
          ".svg": "image/svg+xml",
          ".woff": "font/woff",
          ".woff2": "font/woff2",
        }[path.extname(file)] || "application/octet-stream";
      res.writeHead(200, { "Content-Type": type, "Cache-Control": "no-store" });
      res.end(data);
    } catch {
      res.writeHead(404);
      res.end();
    }
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const origin = `http://127.0.0.1:${server.address().port}`;
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  const report = { mode, routes: [], loginJS: [], errors: [] };
  try {
    for (const pathname of paths) {
      const page = await browser.newPage();
      const errors = [],
        requests = [];
      page.on("pageerror", (e) => errors.push(e.message));
      page.on("request", (r) => {
        if (r.url().includes("/api/")) requests.push(r.url());
      });
      await page.goto(origin + pathname);
      await page.locator("h1").first().waitFor();
      await page.waitForLoadState("networkidle");
      const content = await page.evaluate(() => ({
        heading: document.querySelector("h1").textContent,
        title: document.title,
        description: document.querySelector('meta[name="description"]')
          ?.content,
        canonical: document.querySelector('link[rel="canonical"]')?.href,
        schema: document.querySelector("script[data-public-metadata]")
          ?.textContent,
        text: document.querySelector("main")?.textContent,
        overflowing: document.documentElement.scrollWidth > innerWidth + 1,
      }));
      assert.deepEqual(errors, [], pathname);
      if (pathname !== "/") assert.deepEqual(requests, [], pathname);
      assert.equal(content.overflowing, false, pathname);
      report.routes.push({ pathname, ...content, errors });
      await page.close();
    }
    const page = await browser.newPage();
    page.on("pageerror", (e) => report.errors.push(e.message));
    await page.goto(origin + "/login");
    await page.getByRole("heading", { name: "Вход в MeetSpace" }).waitFor();
    await page.waitForLoadState("networkidle");
    const scriptPaths = await page.evaluate(() =>
      performance
        .getEntriesByType("resource")
        .filter((r) => new URL(r.name).pathname.endsWith(".js"))
        .map((r) => new URL(r.name).pathname),
    );
    for (const asset of [...new Set(scriptPaths)]) {
      const bytes = await fs.readFile(path.join(root, asset));
      report.loginJS.push({
        asset,
        bytes: bytes.length,
        gzipBytes: zlib.gzipSync(bytes).length,
      });
      if (mode !== "baseline")
        assert.equal(
          bytes.includes(Buffer.from("Как понимать значения в таблице")),
          false,
          "Documentation body must not load on /login",
        );
    }
    await page.evaluate(() => {
      history.pushState({}, "", "/docs/features");
      dispatchEvent(new PopStateEvent("popstate"));
    });
    await page
      .getByRole("heading", { name: "Возможности", exact: true, level: 1 })
      .waitFor();
    await page.getByPlaceholder(/Поиск по документации/).fill("Docker");
    const firstResult = page.locator("#docs-search-results a").first();
    await firstResult.waitFor();
    const target = await firstResult.getAttribute("href");
    await firstResult.click();
    await page.waitForURL(origin + target);
    await page.locator(new URL(origin + target).hash).waitFor();
    await page.close();
    assert.deepEqual(report.errors, []);
    report.completed = true;
    await fs.writeFile(reportPath, JSON.stringify(report, null, 2));
    console.log(
      JSON.stringify({
        completed: true,
        routes: report.routes.length,
        loginJS: report.loginJS,
      }),
    );
  } finally {
    await browser.close();
    await new Promise((resolve) => server.close(resolve));
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
