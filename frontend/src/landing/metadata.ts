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
      "MeetSpace - единая платформа для корпоративного общения и совместной работы",
    description:
      "Видеозвонки, чаты, файлы, календарь и запись конференций. MeetSpace доступна через веб-интерфейс и приложения для Windows, Linux, macOS и Android. Развёртывание на своём сервере (Self-Hosted).",
    schemaType: "SoftwareApplication",
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

export const documentationSeo: Record<
  (typeof DOC_SLUGS)[number],
  { title: string; description: string; keywords: string[] }
> = {
  overview: {
    title: "Документация MeetSpace - возможности, установка и настройка",
    description:
      "Руководство MeetSpace: видеоконференции, корпоративные чаты и совместная работа. Установка на собственный сервер, настройка и администрирование платформы.",
    keywords: [
      "MeetSpace",
      "документация MeetSpace",
      "корпоративное общение",
      "видеоконференции",
      "совместная работа",
      "Self-Hosted",
    ],
  },
  features: {
    title: "Возможности MeetSpace - видеозвонки, чаты и запись встреч",
    description:
      "Возможности MeetSpace: видеозвонки, личные и групповые чаты, обмен файлами, календарь встреч и запись конференций.",
    keywords: [
      "возможности MeetSpace",
      "видеозвонки",
      "корпоративный мессенджер",
      "групповые чаты",
      "обмен файлами",
      "календарь встреч",
      "запись конференций",
    ],
  },
  architecture: {
    title: "Архитектура MeetSpace - WebRTC, сервисы и хранение данных",
    description:
      "Архитектура MeetSpace: API, WebRTC и SFU, сервис записи, PostgreSQL и S3. Как устроены видеосвязь, чаты, хранение данных и мониторинг платформы.",
    keywords: [
      "архитектура MeetSpace",
      "WebRTC",
      "SFU",
      "PostgreSQL",
      "S3",
      "сервер видеоконференций",
      "мониторинг",
    ],
  },
  interface: {
    title: "Интерфейс MeetSpace - руководство по встречам и чатам",
    description:
      "Руководство по интерфейсу MeetSpace: создание встреч, микрофон и камера, личные и групповые чаты, календарь, записи, папки и настройки аккаунта.",
    keywords: [
      "интерфейс MeetSpace",
      "руководство пользователя",
      "создание встречи",
      "личные сообщения",
      "групповой чат",
      "настройки аккаунта",
    ],
  },
  support: {
    title: "Поддержка MeetSpace - помощь и обратная связь",
    description:
      "Помощь по MeetSpace: куда обратиться с вопросами об использовании и установке платформы, как описать проблему и связаться с автором проекта.",
    keywords: [
      "поддержка MeetSpace",
      "помощь MeetSpace",
      "обратная связь",
      "проблемы с видеосвязью",
      "установка MeetSpace",
    ],
  },
  "before-you-begin": {
    title: "Подготовка к установке MeetSpace - сервер, домен и HTTPS",
    description:
      "Подготовьте собственный сервер для MeetSpace: проверьте Linux, домен и DNS, HTTPS, сетевые порты, доступ к Docker и данные для установки.",
    keywords: [
      "подготовка сервера MeetSpace",
      "Self-Hosted",
      "Linux VPS",
      "домен и DNS",
      "HTTPS",
      "Docker",
    ],
  },
  "system-requirements": {
    title: "Системные требования MeetSpace - Linux, Docker и сеть",
    description:
      "Требования MeetSpace к серверу: операционная система, Docker, процессор, память, место на диске и сетевые порты для видеоконференций и записи встреч.",
    keywords: [
      "системные требования MeetSpace",
      "сервер для видеоконференций",
      "Linux",
      "Docker Compose",
      "оперативная память",
      "сетевые порты",
    ],
  },
  "source-and-releases": {
    title: "Исходный код и выпуски MeetSpace - загрузка с GitHub",
    description:
      "Исходный код и выпуски MeetSpace: официальный репозиторий GitHub, выбор версии, готовые пакеты и проверка файлов перед установкой на сервер.",
    keywords: [
      "исходный код MeetSpace",
      "выпуски MeetSpace",
      "GitHub",
      "скачать MeetSpace",
      "пакет установки",
      "проверка контрольных сумм",
    ],
  },
  installation: {
    title: "Установка MeetSpace на свой сервер - пошаговое руководство",
    description:
      "Установите MeetSpace на собственный Linux-сервер: мастер установки, Docker Compose, домен, HTTPS, почта и проверка видеоконференций после запуска.",
    keywords: [
      "установка MeetSpace",
      "MeetSpace на своём сервере",
      "Self-Hosted",
      "Docker Compose",
      "сервер видеоконференций",
      "настройка SMTP",
    ],
  },
  "updates-and-backups": {
    title: "Обновление и резервное копирование MeetSpace",
    description:
      "Как обновлять MeetSpace и создавать резервные копии: подготовка выпуска, остановка активных операций, сохранение PostgreSQL, S3 и конфигурации сервера.",
    keywords: [
      "обновление MeetSpace",
      "резервное копирование MeetSpace",
      "восстановление данных",
      "PostgreSQL",
      "S3",
      "обслуживание сервера",
    ],
  },
  troubleshooting: {
    title: "Устранение неполадок MeetSpace - установка, связь и записи",
    description:
      "Диагностика MeetSpace: ошибки установки и входа, проблемы со звуком, видео и записью, настройка DNS, HTTPS, TURN и проверка журналов сервисов.",
    keywords: [
      "ошибки MeetSpace",
      "устранение неполадок",
      "не работает микрофон",
      "проблемы с видеосвязью",
      "ошибка записи конференции",
      "TURN",
      "журналы сервисов",
    ],
  },
};

export function publicPath(pathname: string): PublicPath | null {
  const normalized = pathname.replace(/\/+$/, "") || "/";
  return PUBLIC_PATHS.find((path) => path === normalized) || null;
}

export interface PageMetadata {
  language: "en" | "ru";
  title: string;
  description: string;
  canonical: string | null;
  keywords: string[];
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
      const { title, description, keywords } =
        documentationSeo[article.slug as (typeof DOC_SLUGS)[number]];
      return {
        language: "ru",
        title,
        description,
        canonical,
        keywords,
        robots: "index, follow",
        structuredData: {
          "@context": "https://schema.org",
          "@type": "TechArticle",
          name: article.title,
          headline: title,
          description,
          url: canonical,
          keywords,
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
      keywords: [],
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
    keywords: [],
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
