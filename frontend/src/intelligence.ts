import type { Notification, SearchResult } from "./types";
import { PAST_CONFERENCES_PATH } from "./conferenceNavigation";

/**
 * Форматирует смещение записи, не принимая отрицательные и бесконечные значения.
 * @args milliseconds — смещение в миллисекундах.
 * @return Подпись времени для кнопки перехода.
 */
export function recordingTime(milliseconds: number): string {
  const seconds = Math.floor(
    Math.max(0, Number.isFinite(milliseconds) ? milliseconds : 0) / 1000,
  );
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return `${hours ? `${hours}:` : ""}${String(minutes).padStart(2, "0")}:${String(seconds % 60).padStart(2, "0")}`;
}

/**
 * Строит внутреннюю ссылку без вставки текста результата в адрес или HTML.
 * @args result — разрешённый сервером результат поиска.
 * @return Список завершённых встреч для материалов либо ссылка на текущую встречу.
 */
export function searchResultLink(result: SearchResult): string {
  return result.recordingId || ["transcript", "summary"].includes(result.type)
    ? PAST_CONFERENCES_PATH
    : `/conferences/${encodeURIComponent(result.conferenceId)}`;
}

/**
 * Открывает нужный материал по уведомлению, не выдавая дополнительных прав.
 * @args notification — личное уведомление со ссылочными идентификаторами.
 * @return Безопасный внутренний маршрут.
 */
export function notificationLink(notification: Notification): string {
  if (notification.type === "chat.message") {
    if (notification.payload.conversationId) {
      const params = new URLSearchParams();
      if (notification.payload.messageId)
        params.set("message", notification.payload.messageId);
      return `/personal/${encodeURIComponent(notification.payload.conversationId)}${params.size ? `?${params}` : ""}`;
    }
    if (!notification.payload.conferenceId) return "/notifications";
    const params = new URLSearchParams({ chat: "1" });
    if (notification.payload.messageId)
      params.set("message", notification.payload.messageId);
    return `/meetings/${encodeURIComponent(notification.payload.conferenceId)}?${params}`;
  }
  if (!notification.payload.conferenceId) return "/notifications";
  if (
    notification.payload.recordingId ||
    notification.payload.transcriptId ||
    notification.payload.summaryId ||
    ["conference.finished", "conference.cancelled"].includes(notification.type)
  )
    return PAST_CONFERENCES_PATH;
  return `/conferences/${encodeURIComponent(notification.payload.conferenceId)}`;
}
