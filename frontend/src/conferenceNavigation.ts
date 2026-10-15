import type { Conference } from "./types";

export const PAST_CONFERENCES_PATH = "/conferences?view=past";

/** Completed meetings share the Past list; open meetings keep their room link. */
export function conferenceLink(
  conference: Pick<Conference, "id" | "status">,
): string {
  return ["finished", "cancelled"].includes(conference.status)
    ? PAST_CONFERENCES_PATH
    : `/meetings/${encodeURIComponent(conference.id)}`;
}
