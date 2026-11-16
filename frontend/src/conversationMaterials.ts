import type { ConferenceChatMaterial } from "./types";

/** Ссылки из сообщений открываются только как явные HTTP(S)-адреса. */
export function sharedMaterialURL(value?: string): string | undefined {
  if (
    !value ||
    !/^https?:\/\//i.test(value) ||
    /[\u0000-\u0020\u007f\\]/u.test(value)
  )
    return undefined;
  try {
    const url = new URL(value);
    return ["http:", "https:"].includes(url.protocol) &&
      !url.username &&
      !url.password
      ? value
      : undefined;
  } catch {
    return undefined;
  }
}

/** Пересекающиеся страницы не дублируют вложения и не возвращают удалённые сообщения. */
export function visibleConversationMaterials(
  pages: ReadonlyArray<{ items: ConferenceChatMaterial[] }>,
  historyClearedThrough = 0,
): ConferenceChatMaterial[] {
  const versions = new Map<string, number>();
  for (const page of pages)
    for (const { message } of page.items)
      versions.set(
        message.id,
        Math.max(versions.get(message.id) ?? 0, message.version),
      );
  const items = new Map<string, ConferenceChatMaterial>();
  for (const page of pages)
    for (const item of page.items) {
      const { message, attachment, url } = item;
      if (
        message.deletedAt ||
        message.version !== versions.get(message.id) ||
        !/^\d+$/.test(message.sequence) ||
        BigInt(message.sequence) <= BigInt(historyClearedThrough) ||
        (attachment && attachment.status !== "attached") ||
        (!attachment && !sharedMaterialURL(url))
      )
        continue;
      const key = `${message.id}:${attachment?.id || url}`;
      if (!items.has(key)) items.set(key, item);
    }
  return [...items.values()];
}

export function materialMonth(createdAt: string): string {
  const date = new Date(createdAt);
  return Number.isFinite(date.getTime())
    ? new Intl.DateTimeFormat("ru", { month: "long", year: "numeric" }).format(
        date,
      )
    : "Без даты";
}
