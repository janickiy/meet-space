import { DOC_SLUGS, documentationArticles } from "./docs";

const configuredSite =
  import.meta.env.VITE_PUBLIC_SITE_URL || "https://meeting.janickiy.com";
const site = new URL(configuredSite);
if (
  !["https:", "http:"].includes(site.protocol) ||
  site.username ||
  site.password ||
  site.pathname !== "/" ||
  site.search ||
  site.hash
) {
  throw new Error("VITE_PUBLIC_SITE_URL must be an absolute HTTP(S) origin");
}

export const PUBLIC_SITE_ORIGIN = site.origin;
export const PUBLIC_PATHS = [
  "/",
  "/welcome",
  "/download",
  "/self-hosted",
  "/docs",
  "/docs/self-hosted",
  ...DOC_SLUGS.map((slug) => `/docs/${slug}` as const),
] as const;
export type PublicPath = (typeof PUBLIC_PATHS)[number];
export const PUBLIC_CANONICAL_PATHS = PUBLIC_PATHS.filter(
  (path) =>
    path !== "/welcome" &&
    path !== "/docs/overview" &&
    path !== "/docs/self-hosted",
);

const publicCopy = {
  home: {
    title:
      "MeetSpace - видеоконференции, обмен сообщениями и совместная работа",
    description:
      "Объедините видеовстречи, сообщения, файлы и записи. Используйте MeetSpace в веб-версии, на Android, Windows или Linux, а также разместите рабочее пространство на собственном сервере.",
    schemaType: "WebApplication",
  },
  "/download": {
    title: "Скачать MeetSpace - Android, Windows, Linux и веб-версия",
    description:
      "Скачайте MeetSpace для Android, Windows и Linux или откройте веб-приложение. Выберите архитектуру и пакет, ознакомьтесь с требованиями и следуйте инструкциям по установке.",
    schemaType: "CollectionPage",
  },
  "/self-hosted": {
    title: "MeetSpace на своём сервере - ваша инфраструктура и данные",
    description:
      "Разверните MeetSpace на собственной инфраструктуре. Используйте свой домен, управляйте хранением данных и настройками сервера. Установка с Docker и подробное руководство.",
    schemaType: "WebPage",
  },
} as const;

export function publicPath(pathname: string): PublicPath | null {
  const normalized = pathname.replace(/\/+$/, "") || "/";
  return PUBLIC_PATHS.find((path) => path === normalized) || null;
}

export interface PageMetadata {
  language: "en" | "ru";
  title: string;
  description: string;
  canonical: string | null;
  robots: string;
  structuredData: Record<string, unknown> | null;
}

export function pageMetadata(
  pathname: string,
  authenticated = false,
): PageMetadata {
  const path = publicPath(pathname);
  if (path && !(path === "/" && authenticated)) {
    if (path === "/docs" || path.startsWith("/docs/")) {
      const slug =
        path === "/docs" || path === "/docs/overview"
          ? "overview"
          : path === "/docs/self-hosted"
            ? "installation"
            : path.slice("/docs/".length);
      const article = documentationArticles.find((item) => item.slug === slug);
      if (!article) throw new Error(`No documentation article for ${path}`);
      const canonical = `${PUBLIC_SITE_ORIGIN}${slug === "overview" ? "/docs" : `/docs/${slug}`}`;
      const title =
        slug === "overview"
          ? "Документация MeetSpace"
          : `${article.title} - Документация MeetSpace`;
      return {
        language: "ru",
        title,
        description: article.description,
        canonical,
        robots: "index, follow",
        structuredData: {
          "@context": "https://schema.org",
          "@type": "TechArticle",
          name: article.title,
          headline: title,
          description: article.description,
          url: canonical,
          inLanguage: "ru",
          isPartOf: {
            "@type": "WebSite",
            name: "Документация MeetSpace",
            url: `${PUBLIC_SITE_ORIGIN}/docs`,
          },
        },
      };
    }
    const home = path === "/" || path === "/welcome";
    const { title, description, schemaType } =
      publicCopy[home ? "home" : (path as "/download" | "/self-hosted")];
    const canonical = `${PUBLIC_SITE_ORIGIN}${home ? "/" : path}`;
    return {
      language: "ru",
      title,
      description,
      canonical,
      robots: "index, follow",
      structuredData: {
        "@context": "https://schema.org",
        "@type": schemaType,
        name: home ? "MeetSpace" : title,
        description,
        url: canonical,
        inLanguage: "ru",
        ...(home && {
          applicationCategory: "CommunicationApplication",
        }),
      },
    };
  }
  const title = pathname.startsWith("/login")
    ? "Вход — MeetSpace"
    : pathname.startsWith("/register")
      ? "Регистрация — MeetSpace"
      : "Рабочее пространство — MeetSpace";
  return {
    language: "ru",
    title,
    description: "Вход и работа в вашем пространстве MeetSpace.",
    canonical: null,
    robots: "noindex, nofollow",
    structuredData: null,
  };
}

export function socialMetadata(metadata: PageMetadata) {
  if (!metadata.canonical) return [];
  return [
    ["og:type", "website"],
    ["og:locale", metadata.language === "en" ? "en_US" : "ru_RU"],
    ["og:site_name", "MeetSpace"],
    ["og:title", metadata.title],
    ["og:description", metadata.description],
    ["og:url", metadata.canonical],
    ["og:image", `${PUBLIC_SITE_ORIGIN}/media/meetspace-group-chat-v6.webp`],
    ["og:image:width", "1536"],
    ["og:image:height", "1024"],
    [
      "og:image:alt",
      metadata.language === "en"
        ? "A team video meeting in MeetSpace"
        : "Командная видеовстреча в MeetSpace",
    ],
  ];
}
