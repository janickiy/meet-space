import type { Notification } from "./types";
import { PAST_CONFERENCES_PATH } from "./conferenceNavigation";

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
    ["conference.finished", "conference.cancelled"].includes(notification.type)
  )
    return PAST_CONFERENCES_PATH;
  return `/conferences/${encodeURIComponent(notification.payload.conferenceId)}`;
}
