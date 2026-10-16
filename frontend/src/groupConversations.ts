import type { InfiniteData, QueryClient } from "@tanstack/react-query";
import type {
  FolderPage,
  GroupConversation,
  Item,
  PersonalPage,
} from "./types";

/** Applies a confirmed personal preference without replacing newer messages or membership. */
export function observeGroupConversationNotifications(
  client: QueryClient,
  id: string,
  userId: string,
  enabled: boolean,
  cancelPendingReads = true,
) {
  // Confirmed preference changes cancel older reads; routine message delivery keeps them running.
  if (cancelPendingReads) {
    for (const key of [
      "personal-list",
      "personal-summary",
      "folder-items",
      "folder-candidates",
    ])
      void client.cancelQueries({ queryKey: [key, userId] });
    void client.cancelQueries({ queryKey: ["personal-detail", id, userId] });
  }
  client.setQueryData<Item<GroupConversation>>(
    ["personal-detail", id, userId],
    (current) =>
      current?.item.type === "group" && current.item.id === id
        ? {
            ...current,
            item: { ...current.item, notificationsEnabled: enabled },
          }
        : current,
  );
  const update = (item: PersonalPage["items"][number]) =>
    item.id === id && item.type === "group"
      ? { ...item, notificationsEnabled: enabled }
      : item;
  client.setQueriesData<InfiniteData<PersonalPage>>(
    { queryKey: ["personal-list", userId] },
    (current) =>
      current
        ? {
            ...current,
            pages: current.pages.map((page) => ({
              ...page,
              items: page.items.map(update),
            })),
          }
        : current,
  );
  client.setQueryData<PersonalPage>(["personal-summary", userId], (current) =>
    current ? { ...current, items: current.items.map(update) } : current,
  );
  for (const key of ["folder-items", "folder-candidates"])
    client.setQueriesData<InfiniteData<FolderPage>>(
      { queryKey: [key, userId] },
      (current) =>
        current
          ? {
              ...current,
              pages: current.pages.map((page) => ({
                ...page,
                items: page.items.map((entry) =>
                  entry.type === "conversation" &&
                  entry.item.type === "group" &&
                  entry.item.id === id
                    ? { ...entry, item: update(entry.item) }
                    : entry,
                ),
              })),
            }
          : current,
    );
}
