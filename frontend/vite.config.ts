import { defineConfig } from "vite";
import type { Plugin } from "vite";
import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import { buildMetadataPlugin, readBuildMetadata } from "./build-metadata.ts";

const buildMetadata = readBuildMetadata(process.env);
const telemetryFlag = process.env.VITE_CLIENT_TELEMETRY_ENABLED || "false";
if (!["true", "false"].includes(telemetryFlag))
  throw new Error("VITE_CLIENT_TELEMETRY_ENABLED must be true or false");

/** Match the production server's public documents and neutral private fallback. */
function publicPreviewPlugin(): Plugin {
  return {
    name: "public-static-preview",
    configurePreviewServer(server) {
      const output = resolve(server.config.root, server.config.build.outDir);
      server.middlewares.use(async (request, response, next) => {
        if (!["GET", "HEAD"].includes(request.method || "")) return next();
        const path = new URL(request.url || "/", "http://localhost").pathname;
        if (path.startsWith("/api/")) return next();
        const normalized = path.replace(/\/+$/, "") || "/";
        const publicDocuments: Record<string, string> = {
          "/": "index.html",
          "/welcome": "welcome/index.html",
          "/download": "download/index.html",
          "/self-hosted": "self-hosted/index.html",
          "/docs": "docs/index.html",
          "/docs/self-hosted": "docs/self-hosted/index.html",
        };
        const documentationArticle = /^\/docs\/[a-z0-9-]+$/.test(normalized);
        const publicDocument =
          publicDocuments[normalized] ||
          (documentationArticle
            ? `${normalized.slice(1)}/index.html`
            : undefined);
        if (!publicDocument && /\.[^/]+$/.test(path)) return next();
        try {
          const html = await readFile(
            resolve(output, publicDocument || "app.html"),
          );
          response.setHeader("Content-Type", "text/html; charset=utf-8");
          response.setHeader("Cache-Control", "no-cache");
          if (!publicDocument)
            response.setHeader("X-Robots-Tag", "noindex, nofollow");
          response.end(request.method === "HEAD" ? undefined : html);
        } catch (error) {
          if (
            documentationArticle &&
            error instanceof Error &&
            "code" in error &&
            error.code === "ENOENT"
          ) {
            response.statusCode = 404;
            response.setHeader("Content-Type", "text/plain; charset=utf-8");
            response.setHeader("X-Robots-Tag", "noindex, nofollow");
            response.end(
              request.method === "HEAD"
                ? undefined
                : "Documentation page not found",
            );
            return;
          }
          next(error);
        }
      });
    },
  };
}

export default defineConfig(({ isSsrBuild }) => ({
  plugins: [react(), buildMetadataPlugin(buildMetadata), publicPreviewPlugin()],
  define: {
    __APP_BUILD__: JSON.stringify(buildMetadata),
    __CLIENT_TELEMETRY_ENABLED__: telemetryFlag === "true",
  },
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": {
        target: process.env.API_PROXY_TARGET || "http://127.0.0.1:8085",
        changeOrigin: false,
        ws: true,
      },
    },
  },
  // Bundle icons for the Node render while keeping a single external React instance.
  ssr: { noExternal: ["lucide-react"] },
  build: {
    target: ["chrome120", "firefox120", "safari17"],
    sourcemap: false,
    copyPublicDir: !isSsrBuild,
  },
}));
