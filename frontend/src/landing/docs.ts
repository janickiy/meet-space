import { productArticles } from "./docs-product";
import { installationArticles } from "./docs-install";
import { interfaceArticle } from "./docs-interface";
import { DOC_SLUGS, documentationTitles } from "./docs-catalog";

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

export { DOC_SLUGS } from "./docs-catalog";

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
  title: documentationTitles["overview"],
  description:
    "Руководство по использованию MeetSpace, развёртыванию, настройке и администрированию собственного сервера.",
  sections: [
    {
      id: "about-meetspace",
      title: "О MeetSpace",
      paragraphs: [
        "MeetSpace это корпоративная платформа для видеоконференций, общения и совместной работы. В едином цифровом пространстве объединены видеозвонки, личные и групповые чаты, обмен файлами, календарь и запись встреч.",
        "Сервис доступен через веб-интерфейс, а также в виде приложений для **Windows**, **Linux**, **macOS** и **Android**. Для управления личным кабинетом требуется регистрация и авторизация, однако подключаться к звонкам по ссылке-приглашению можно без создания учётной записи - в качестве гостя.",
        "Платформа поддерживает **self-hosted** развёртывание на собственных **VDS**/**VPS**-серверах компании. Такой формат установки позволяет организации использовать корпоративный домен, полностью контролировать хранение данных, гибко настраивать права доступа, параметры сервера и внешние интеграции.",
      ],
    },
    {
      id: "start-here",
      title: "С чего начать",
      paragraphs: [
        "Ознакомьтесь с возможностями MeetSpace, изучите руководство по использованию платформы или перейдите к инструкциям по установке и настройке сервера. Выберите интересующий раздел ниже.",
      ],
    },
    {
      id: "what-this-guide-covers",
      title: "Что вы найдёте в документации",
      paragraphs: [
        "Инструкции описывают открытый исходный код и веб-интерфейс MeetSpace. Названия разделов и кнопок соответствуют веб-приложению.",
      ],
      bullets: [
        "Работа с продуктом - встречи, звонки, чаты, календарь, записи, файлы и настройки рабочего пространства.",
        "Установка и обновление - подготовка сервера, системные требования, обновления, резервное копирование и устранение неполадок. Интерактивная установка и ручное развёртывание описаны отдельно.",
        "Возможности и ограничения - что зависит от версии MeetSpace, прав пользователя и конфигурации сервера. Для записи и внешних интеграций может потребоваться дополнительная настройка.",
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
