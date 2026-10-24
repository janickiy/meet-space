import { productArticles } from "./docs-product";
import { installationArticles } from "./docs-install";
import { interfaceArticle } from "./docs-interface";

export interface DocumentationSection {
  id: string;
  title: string;
  hideHeading?: boolean;
  paragraphs?: string[];
  subsections?: {
    title: string;
    bullets?: string[];
    paragraphs: string[];
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
  screenshots?: {
    src: string;
    alt: string;
    caption: string;
    width: number;
    height: number;
  }[];
}

export interface DocumentationArticle {
  slug: string;
  title: string;
  description: string;
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
    "Руководство по работе с MeetSpace, установке и обслуживанию собственного сервера.",
  sections: [
    {
      id: "about-meetspace",
      title: "О MeetSpace",
      paragraphs: [
        "**MeetSpace** - веб платформа для корпоративного общения. В одном рабочем пространстве доступны видеоконференции, звонки, личные и групповые чаты, обмен файлами, календарь и записи встреч.",
        "Используйте **веб-приложение** или поддерживаемые **десктопные и Android-клиенты**. Для личного рабочего пространства нужна учётная запись. По приглашению можно присоединиться к отдельной встрече как гость без регистрации.",
        "MeetSpace можно развернуть **на собственном Linux-сервере**. Организация управляет доменом, хранением данных, настройками доступа и подключёнными интеграциями.",
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
        "Инструкции описывают открытый исходный код и веб-интерфейс MeetSpace. Руководство написано **на русском языке**, а названия разделов и кнопок соответствуют веб-приложению.",
      ],
      bullets: [
        "**Работа с продуктом** - встречи, звонки, чаты, календарь, записи, файлы и настройки рабочего пространства.",
        "**Установка и обслуживание** - подготовка сервера, системные требования, обновления, резервное копирование и устранение неполадок. Интерактивная установка и ручное развёртывание описаны отдельно.",
        "**Возможности и ограничения** - что зависит от версии MeetSpace, прав пользователя и конфигурации сервера. Для записи, аналитики, субтитров и внешних интеграций может потребоваться дополнительная настройка.",
      ],
      note: "**MeetSpace находится на стадии бета-тестирования.** При настройке и использовании платформы учитывайте возможности установленной версии и включённых на сервере сервисов.",
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
