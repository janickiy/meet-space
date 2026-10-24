import { Landing } from "../pages/Landing";
import { DocumentationPage } from "../pages/DocumentationPage";
import { DownloadPage, SelfHostedOverview } from "../pages/MarketingPages";
import type { PublicPath } from "./metadata";

/** Public rendering deliberately excludes account, API, and appearance providers. */
export function PublicPage({ path }: { path: PublicPath }) {
  if (path === "/docs" || path.startsWith("/docs/")) {
    return (
      <DocumentationPage
        slug={
          path === "/docs"
            ? "overview"
            : path === "/docs/self-hosted"
              ? "installation"
              : path.slice("/docs/".length)
        }
      />
    );
  }
  switch (path) {
    case "/download":
      return <DownloadPage />;
    case "/self-hosted":
      return <SelfHostedOverview />;
    default:
      return <Landing />;
  }
}
