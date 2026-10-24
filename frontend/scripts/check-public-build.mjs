import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { readFile, access } from "node:fs/promises";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const expectedOrigin = process.env.VITE_PUBLIC_SITE_URL
  ? new URL(process.env.VITE_PUBLIC_SITE_URL).origin
  : null;
const marketingOnly = process.env.VITE_MARKETING_ONLY === "true";
const applicationOrigin = new URL(
  process.env.VITE_PUBLIC_APP_URL || "https://meeting.janickiy.com",
).origin;
const checkoutCommit = execFileSync("git", ["rev-parse", "HEAD"], {
  cwd: root,
  encoding: "utf8",
}).trim();
const documentationSlugs = [
  "overview",
  "features",
  "architecture",
  "interface",
  "support",
  "before-you-begin",
  "system-requirements",
  "source-and-releases",
  "installation",
  "updates-and-backups",
  "troubleshooting",
];
const routes = [
  "/",
  "/welcome",
  "/download",
  "/self-hosted",
  "/docs",
  "/docs/self-hosted",
  ...documentationSlugs.map((slug) => `/docs/${slug}`),
];
const canonicalPath = (route) =>
  route === "/welcome"
    ? "/"
    : route === "/docs/overview"
      ? "/docs"
      : route === "/docs/self-hosted"
        ? "/docs/installation"
        : route;
const canonicalRoutes = [...new Set(routes.map(canonicalPath))];
const appRoutes = new Set([
  ...routes,
  "/login",
  "/register",
  "/meetings/new",
  "/personal",
]);
const documents = new Map(
  await Promise.all(
    routes.map(async (route) => [
      route,
      await readFile(
        resolve(root, `dist${route === "/" ? "" : route}/index.html`),
        "utf8",
      ),
    ]),
  ),
);
const documentIds = new Map(
  [...documents].map(([route, html]) => [
    route,
    new Set([...html.matchAll(/\bid="([^"]+)"/g)].map((match) => match[1])),
  ]),
);
let checks = 0;
for (const [route, html] of documents) {
  const isDocumentation = route === "/docs" || route.startsWith("/docs/");
  const language = "ru";
  assert.equal(
    (html.match(/<h1(?:\s|>)/g) || []).length,
    1,
    `${route}: one h1`,
  );
  assert.match(html, /data-prerender-path=/);
  assert.ok(
    html.includes(`<html lang="${language}"`),
    `${route}: correct document language`,
  );
  if (isDocumentation) {
    assert.match(html, /Документация/, `${route}: Russian documentation UI`);
    assert.doesNotMatch(
      html,
      /Search documentation|English documentation|On this page|Skip to content/,
      `${route}: no obsolete English documentation labels`,
    );
    assert.match(html, /<main\b/, `${route}: article landmark`);
    assert.match(html, /<nav\b/, `${route}: documentation navigation`);
  } else {
    assert.doesNotMatch(
      html,
      /Get started|Download MeetSpace|Skip to content|Your space/,
      `${route}: no obsolete English UI copy`,
    );
    assert.match(
      html,
      /[\u0400-\u04ff]/,
      `${route}: public content is Russian`,
    );
  }
  assert.match(html, /<meta name="robots" content="index, follow"/);
  assert.match(html, /<meta property="og:title"/);
  assert.ok(
    html.includes('<meta property="og:locale" content="ru_RU"'),
    `${route}: localized social metadata`,
  );
  assert.match(html, /<link rel="canonical" href="https:\/\//);
  const canonical = html.match(/<link rel="canonical" href="([^"]+)"/)?.[1];
  assert.ok(canonical);
  if (expectedOrigin) assert.equal(new URL(canonical).origin, expectedOrigin);
  assert.equal(
    new URL(canonical).pathname,
    canonicalPath(route),
    `${route}: page-specific canonical`,
  );
  const schema = html.match(
    /<script type="application\/ld\+json"[^>]*>(.*?)<\/script>/s,
  )?.[1];
  assert.ok(schema);
  assert.equal(JSON.parse(schema)["@context"], "https://schema.org");
  assert.equal(JSON.parse(schema).inLanguage, language);
  assert.equal(JSON.parse(schema).url, canonical);
  const ids = documentIds.get(route);
  for (const anchor of html.matchAll(/<a\b([^>]+)>/g)) {
    const attributes = anchor[1];
    const href = attributes.match(/\bhref="([^"]+)"/)?.[1];
    assert.ok(href && href !== "#", `${route}: no empty links`);
    if (href.startsWith("#"))
      assert.ok(ids.has(href.slice(1)), `Missing anchor ${href}`);
    if (href.startsWith("/")) {
      const target = new URL(href, "https://meet.example.com");
      const targetPath = target.pathname.replace(/\/+$/, "") || "/";
      if (targetPath.startsWith("/media/docs/")) {
        assert.equal(
          target.origin,
          "https://meet.example.com",
          `${route}: documentation screenshot link is local`,
        );
        assert.match(
          targetPath,
          /\.(?:png|webp|jpe?g)$/i,
          `${route}: documentation screenshot link is an image`,
        );
        await access(resolve(root, "dist", targetPath.slice(1)));
      } else {
        assert.ok(appRoutes.has(targetPath), `Unknown app link ${href}`);
        if (marketingOnly) {
          assert.ok(
            documents.has(targetPath),
            `${route}: private action must use the application host: ${href}`,
          );
        }
        if (target.hash && documentIds.has(targetPath)) {
          assert.ok(
            documentIds
              .get(targetPath)
              .has(decodeURIComponent(target.hash.slice(1))),
            `${route}: missing cross-page anchor ${href}`,
          );
        }
      }
    }
    if (attributes.includes('target="_blank"'))
      assert.match(attributes, /rel="noopener noreferrer"/);
    const source = href.match(
      /github\.com\/janickiy\/meet-space\/blob\/([a-f0-9]{40})\/([^#]+)/,
    );
    // Other audited revisions can contain files absent from the current checkout.
    if (source?.[1] === checkoutCommit) {
      await access(resolve(root, "..", source[2]));
    }
  }
  for (const image of html.matchAll(/<img\b[^>]*\bsrc="([^"]+)"/g)) {
    assert.ok(image[1].startsWith("/"), "Product images are local");
    await access(resolve(root, "dist", image[1].slice(1).split("?")[0]));
  }
  if (route === "/" || route === "/welcome") {
    assert.ok(ids.has("product-preview"), `${route}: product preview exists`);
    if (marketingOnly) {
      for (const action of [
        "/login",
        "/register",
        "/meetings/new",
        "/personal",
      ]) {
        assert.ok(
          html.includes(`href="${applicationOrigin}${action}"`),
          `${route}: external application action ${action}`,
        );
      }
    }
  }
  if (route === "/" || route === "/welcome" || route === "/download") {
    for (const repository of ["meet-space-desktop", "meet-space-android"]) {
      assert.ok(
        html.includes(`href="https://github.com/janickiy/${repository}"`),
        `${route}: official ${repository} repository link`,
      );
      assert.ok(
        html.includes(`href="https://github.com/janickiy/${repository}/raw/`),
        `${route}: published ${repository} installer link`,
      );
    }
  }
  checks++;
}
const privateShell = await readFile(resolve(root, "dist/app.html"), "utf8");
assert.match(privateShell, /noindex, nofollow/);
assert.ok(!privateShell.includes('rel="canonical"'));
assert.ok(!privateShell.includes("hero-title"));
assert.ok(!privateShell.includes("data-prerender-path"));
const sitemap = await readFile(resolve(root, "dist/sitemap.xml"), "utf8");
assert.equal((sitemap.match(/<url>/g) || []).length, canonicalRoutes.length);
assert.ok(!sitemap.includes("/login"));
const sitemapUrls = [...sitemap.matchAll(/<loc>([^<]+)<\/loc>/g)].map(
  (match) => new URL(match[1]).pathname,
);
assert.deepEqual(
  [...new Set(sitemapUrls)].sort(),
  canonicalRoutes.sort(),
  "Every canonical public page is listed once",
);
const robots = await readFile(resolve(root, "dist/robots.txt"), "utf8");
assert.match(robots, /Disallow: \//);
assert.match(robots, /Allow: \/docs\/self-hosted\$/);
assert.match(robots, /Allow: \/download\$/);
assert.match(robots, /Allow: \/self-hosted\$/);
for (const slug of documentationSlugs) {
  assert.ok(robots.includes(`Allow: /docs/${slug}$`));
}
console.log(
  `PASS: ${checks} Russian public documents, local assets and source links, cross-page anchors, external-link protection, official download links, canonical metadata, sitemap and private SPA shell.`,
);
