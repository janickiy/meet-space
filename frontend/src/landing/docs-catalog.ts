// Короткий каталог маршрутов и заголовков не загружает тексты всех статей в рабочее пространство.
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

export const documentationTitles: Record<(typeof DOC_SLUGS)[number], string> = {
  overview: "Документация MeetSpace",
  features: "Возможности",
  architecture: "Архитектура",
  interface: "Интерфейс",
  support: "Техническая поддержка",
  "before-you-begin": "Перед началом работы",
  "system-requirements": "Системные требования",
  "source-and-releases": "Исходный код и релизы",
  installation: "Установка",
  "updates-and-backups": "Обновления и резервное копирование",
  troubleshooting: "Устранение неполадок",
};
