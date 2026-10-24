import type { Conference } from "./types";

export const PAST_CONFERENCES_PATH = "/conferences?view=past";

/** Завершённые совещания отображаются в списке «Прошлые», а открытые совещания сохраняют ссылку на комнату. */
export function conferenceLink(
  conference: Pick<Conference, "id" | "status">,
): string {
  return ["finished", "cancelled"].includes(conference.status)
    ? PAST_CONFERENCES_PATH
    : `/meetings/${encodeURIComponent(conference.id)}`;
}
