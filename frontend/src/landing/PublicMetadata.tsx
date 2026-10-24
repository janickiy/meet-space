import { useEffect } from "react";
import { useLocation } from "react-router";
import { pageMetadata, socialMetadata } from "./metadata";

/** Keeps public metadata in sync with SPA navigation and removes it in the app. */
export function PublicMetadata({
  authenticated = false,
}: {
  authenticated?: boolean;
}) {
  const { pathname } = useLocation();
  useEffect(() => {
    if (!window.location.hash) window.scrollTo(0, 0);
  }, [pathname]);
  useEffect(() => {
    const metadata = pageMetadata(pathname, authenticated);
    document.title = metadata.title;
    document.documentElement.lang = metadata.language;
    document
      .querySelectorAll(
        'meta[name="description"], meta[name="robots"], meta[property^="og:"], link[rel="canonical"], script[data-public-metadata]',
      )
      .forEach((element) => element.remove());
    for (const [name, content] of [
      ["description", metadata.description],
      ["robots", metadata.robots],
    ]) {
      const element = document.createElement("meta");
      element.name = name;
      element.content = content;
      document.head.append(element);
    }
    for (const [property, content] of socialMetadata(metadata)) {
      const element = document.createElement("meta");
      element.setAttribute("property", property);
      element.content = content;
      document.head.append(element);
    }
    if (metadata.canonical) {
      const canonical = document.createElement("link");
      canonical.rel = "canonical";
      canonical.href = metadata.canonical;
      document.head.append(canonical);
    }
    if (metadata.structuredData) {
      const structuredData = document.createElement("script");
      structuredData.type = "application/ld+json";
      structuredData.dataset.publicMetadata = "true";
      structuredData.textContent = JSON.stringify(metadata.structuredData);
      document.head.append(structuredData);
    }
  }, [pathname, authenticated]);
  return null;
}
