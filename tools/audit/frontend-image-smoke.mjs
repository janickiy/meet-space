import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";

// Validate only a locally built frontend image; no API or external site is used.
const image = process.argv[2];
assert.ok(image, "Provide the local frontend image tag as the first argument");
const docker = (...args) =>
  execFileSync("docker", args, { encoding: "utf8", timeout: 30_000 }).trim();
const container = docker(
  "run",
  "-d",
  "--memory=256m",
  "--cpus=1",
  "--label",
  "meetspace.audit=frontend-smoke",
  "--publish",
  "127.0.0.1::8080",
  "--env",
  "PUBLIC_FRONTEND_URL=https://audit.invalid",
  "--env",
  "API_UPSTREAM=127.0.0.1:9",
  image,
);
try {
  const address = docker("port", container, "8080/tcp");
  assert.match(address, /^127\.0\.0\.1:\d+$/);
  const origin = `http://${address}`;
  const request = (path) =>
    fetch(`${origin}${path}`, {
      signal: AbortSignal.timeout(2_000),
      redirect: "error",
    });
  let ready = false;
  for (let attempt = 0; attempt < 30; attempt++) {
    try {
      const response = await request("/healthz");
      ready =
        response.status === 200 && (await response.text()).trim() === "ok";
    } catch {}
    if (ready) break;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  assert.ok(ready, "Frontend did not become healthy");
  const page = await request("/login");
  assert.equal(page.status, 200);
  assert.equal(page.headers.get("x-content-type-options"), "nosniff");
  assert.equal(page.headers.get("referrer-policy"), "no-referrer");
  const csp = page.headers.get("content-security-policy") ?? "";
  assert.ok(csp.includes("script-src 'self'"));
  assert.ok(csp.includes("frame-ancestors 'none'"));
  assert.ok(csp.includes("wss://audit.invalid"));
  assert.match(page.headers.get("cache-control") ?? "", /no-cache/);
  const html = await page.text();
  const assetPath = html.match(/src="(\/assets\/[^\"]+\.js)"/)?.[1];
  assert.ok(assetPath, "Built JavaScript asset was not found");
  const asset = await request(assetPath);
  assert.equal(asset.status, 200);
  assert.match(asset.headers.get("cache-control") ?? "", /max-age=31536000/);
  await asset.arrayBuffer();
  const missing = await request("/assets/audit-missing.js");
  assert.equal(missing.status, 404);
  await missing.arrayBuffer();
  docker("exec", container, "nginx", "-t");
  console.log(
    JSON.stringify({
      image,
      healthy: true,
      headers: "pass",
      spaRoute: "pass",
      assetCaching: "pass",
    }),
  );
} finally {
  docker("rm", "-f", container);
}
