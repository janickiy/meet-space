import { spawnSync } from "node:child_process";
import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { fileURLToPath, pathToFileURL } from "node:url";
import { dirname, join, resolve } from "node:path";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const temporary = join(root, ".prerender");
const output = join(root, "dist");

try {
  const result = spawnSync(
    process.execPath,
    [
      join(root, "node_modules/vite/bin/vite.js"),
      "build",
      "--ssr",
      "src/entry-public.tsx",
      "--outDir",
      ".prerender",
    ],
    { cwd: root, stdio: "inherit", env: process.env },
  );
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(`Public SSR build failed (exit ${result.status})`);
  }
  const {
    PUBLIC_PATHS,
    PUBLIC_CANONICAL_PATHS,
    PUBLIC_SITE_ORIGIN,
    renderPublicPage,
  } = await import(pathToFileURL(join(temporary, "entry-public.js")).href);
  const template = await readFile(join(output, "index.html"), "utf8");
  if (!template.includes('<div id="root"></div>')) {
    throw new Error("Public prerender requires the empty Vite root template");
  }
  // The SPA fallback stays neutral, even when JavaScript is disabled.
  await writeFile(join(output, "app.html"), template);
  for (const path of PUBLIC_PATHS) {
    const { html, head, language } = renderPublicPage(path);
    const document = template
      .replace(/<html\s+lang="[^"]+"/, `<html lang="${language}"`)
      .replace(/<title>[\s\S]*?<\/title>/, "")
      .replace(/<meta\s+name="(?:description|robots)"[\s\S]*?\/>/g, "")
      .replace("</head>", `${head}\n  </head>`)
      .replace(
        '<div id="root"></div>',
        `<div id="root" data-prerender-path="${path}">${html}</div>`,
      );
    const directory = path === "/" ? output : join(output, path.slice(1));
    await mkdir(directory, { recursive: true });
    await writeFile(join(directory, "index.html"), document);
    console.log(`Prerendered ${path}`);
  }
  await writeFile(
    join(output, "sitemap.xml"),
    `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${PUBLIC_CANONICAL_PATHS.map((path) => `  <url><loc>${PUBLIC_SITE_ORIGIN}${path}</loc></url>`).join("\n")}\n</urlset>\n`,
  );
  await writeFile(
    join(output, "robots.txt"),
    [
      "User-agent: *",
      ...PUBLIC_PATHS.flatMap((path) =>
        path === "/" ? ["Allow: /$"] : [`Allow: ${path}$`, `Allow: ${path}/$`],
      ),
      "Allow: /assets/",
      "Allow: /media/",
      "Allow: /branding/",
      "Allow: /favicon.svg",
      "Disallow: /",
      "",
      `Sitemap: ${PUBLIC_SITE_ORIGIN}/sitemap.xml`,
      "",
    ].join("\n"),
  );
} finally {
  await rm(temporary, { recursive: true, force: true });
}
