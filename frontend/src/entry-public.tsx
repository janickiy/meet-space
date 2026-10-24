import { renderToString } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { PublicPage } from "./landing/PublicPage";
import { pageMetadata, publicPath, socialMetadata } from "./landing/metadata";

export {
  PUBLIC_PATHS,
  PUBLIC_CANONICAL_PATHS,
  PUBLIC_SITE_ORIGIN,
} from "./landing/metadata";

function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/g, (character) => {
    const entities: Record<string, string> = {
      "&": "&amp;",
      "<": "&lt;",
      ">": "&gt;",
      '"': "&quot;",
      "'": "&#39;",
    };
    return entities[character];
  });
}

export function renderPublicPage(pathname: string) {
  const path = publicPath(pathname);
  if (!path) throw new Error(`No public page configured for ${pathname}`);
  const metadata = pageMetadata(path);
  const html = renderToString(
    <MemoryRouter initialEntries={[path]}>
      <PublicPage path={path} />
    </MemoryRouter>,
  );
  const head = [
    `<title>${escapeHtml(metadata.title)}</title>`,
    `<meta name="description" content="${escapeHtml(metadata.description)}" />`,
    `<meta name="robots" content="${metadata.robots}" />`,
    `<link rel="canonical" href="${escapeHtml(metadata.canonical!)}" />`,
    ...socialMetadata(metadata).map(
      ([property, content]) =>
        `<meta property="${property}" content="${escapeHtml(content)}" />`,
    ),
    `<script type="application/ld+json" data-public-metadata="true">${JSON.stringify(metadata.structuredData).replace(/</g, "\\u003c")}</script>`,
  ].join("\n    ");
  return { html, head, language: metadata.language };
}
