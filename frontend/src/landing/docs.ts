import { productArticles } from "./docs-product";
import { installationArticles } from "./docs-install";
import { interfaceArticle } from "./docs-interface";

export interface DocumentationScreenshot {
  src: string;
  alt: string;
  caption: string;
  width: number;
  height: number;
}

export interface DocumentationSection {
  id: string;
  title: string;
  hideHeading?: boolean;
  paragraphs?: string[];
  subsections?: {
    title: string;
    introParagraphs?: string[];
    bullets?: string[];
    steps?: string[];
    paragraphs: string[];
    screenshots?: DocumentationScreenshot[];
  }[];
  bullets?: string[];
  steps?: string[];
  code?: { language: string; value: string };
  note?: string;
  links?: { label: string; href: string }[];
  actions?: {
    label: string;
    href: string;
    icon: "email" | "telegram";
  }[];
  table?: { headings: string[]; rows: string[][] };
  screenshots?: DocumentationScreenshot[];
}

export interface DocumentationArticle {
  slug: string;
  title: string;
  description: string;
  lead?: string;
  sections: DocumentationSection[];
}

export const DOC_SLUGS = [
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
] as const;

export const documentationGroups = [
  {
    title: "О продукте",
    slugs: ["overview", "features", "architecture", "interface", "support"],
  },
  {
    title: "Установка и обновление",
    slugs: [
      "before-you-begin",
      "system-requirements",
      "source-and-releases",
      "installation",
      "updates-and-backups",
      "troubleshooting",
    ],
  },
];

const overview: DocumentationArticle = {
  slug: "overview",
  title: "Документация MeetSpace",
  description:
    "Руководство по использованию MeetSpace, развёртыванию, настройке и администрированию собственного сервера.",
  sections: [
    {
      id: "about-meetspace",
      title: "О MeetSpace",
      paragraphs: [
        "**MeetSpace** - веб-платформа для корпоративного общения и совместной работы. Система объединяет видеоконференции, аудиозвонки, личные и групповые чаты, обмен файлами, календарь и записи встреч в едином рабочем пространстве.",
        "Платформа доступна через веб-браузер, а также приложения для Windows, Linux и Android. Для использования личного рабочего пространства необходима регистрация и авторизация. При этом пользователи могут присоединяться к отдельным встречам по ссылке-приглашению в качестве гостей без создания учётной записи.",
        "MeetSpace поддерживает самостоятельное развёртывание на VDS/VPS-серверах. При таком способе установки организация получает возможность использовать собственный домен, управлять хранением данных, правами доступа пользователей, настройками сервера и подключением внешних интеграций.",
      ],
    },
    {
      id: "start-here",
      title: "С чего начать",
      paragraphs: [
        "Выберите нужную задачу: изучите интерфейс и возможности, установите MeetSpace на свой сервер или найдите решение проблемы. Карточки ниже помогут перейти к подходящему руководству.",
      ],
    },
    {
      id: "what-this-guide-covers",
      title: "Что вы найдёте в документации",
      paragraphs: [
        "Инструкции описывают открытый исходный код и веб-интерфейс MeetSpace. Названия разделов и кнопок соответствуют веб-приложению.",
      ],
      bullets: [
        "**Работа с продуктом** - встречи, звонки, чаты, календарь, записи, файлы и настройки рабочего пространства.",
        "**Установка и обновление** - подготовка сервера, системные требования, обновления, резервное копирование и устранение неполадок. Интерактивная установка и ручное развёртывание описаны отдельно.",
        "**Возможности и ограничения** - что зависит от версии MeetSpace, прав пользователя и конфигурации сервера. Для записи и внешних интеграций может потребоваться дополнительная настройка.",
      ],
      links: [
        {
          label: "Открыть репозиторий исходного кода",
          href: "https://github.com/janickiy/meet-space",
        },
        {
          label: "Примечания к выпуску",
          href: "https://github.com/janickiy/meet-space/releases",
        },
      ],
    },
  ],
};

const allArticles: DocumentationArticle[] = [
  overview,
  ...productArticles,
  interfaceArticle,
  ...installationArticles,
];
export const documentationArticles: DocumentationArticle[] = DOC_SLUGS.map(
  (slug) => {
    const article = allArticles.find((item) => item.slug === slug);
    if (!article) throw new Error(`Missing documentation article: ${slug}`);
    return article;
  },
);

export function documentationPath(slug: string) {
  return slug === "overview" ? "/docs" : `/docs/${slug}`;
}
