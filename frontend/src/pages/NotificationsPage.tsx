import { useRef, useState } from "react";
import { Link } from "react-router";
import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import {
  ArrowUpRight,
  Bell,
  Check,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import { notificationLabel } from "../components/NotificationBell";
import { Button, ErrorNotice, Loading } from "../components/ui";
import { notificationLink } from "../notificationLinks";
import { formatDate } from "../utils";
import "./history-notifications.css";

const pageSize = 15;

/** Resets pagination when the signed-in account changes. */
export function NotificationsPage() {
  const { user } = useAuth();
  return (
    <NotificationHistory key={user?.id || "anonymous"} userId={user?.id} />
  );
}

/** Displays one cursor-backed page of fifteen notifications for userId. */
function NotificationHistory({ userId }: { userId?: string }) {
  const client = useQueryClient();
  const [pageIndex, setPageIndex] = useState(0);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const query = useInfiniteQuery({
    queryKey: ["notifications", userId, "history", pageSize],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      api.notifications(pageParam, signal, pageSize),
    getNextPageParam: (last) => last.nextCursor || undefined,
    enabled: !!userId,
  });
  const read = useMutation({
    mutationFn: api.readNotification,
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["notifications", userId] });
    },
  });
  const pages = query.data?.pages || [];
  const lastLoadedPage = pages.reduce(
    (last, page, index) => (page.items.length ? index : last),
    0,
  );
  const currentPage = Math.min(pageIndex, lastLoadedPage);
  const rows = [
    ...new Map(
      (pages[currentPage]?.items || []).map((item) => [item.id, item]),
    ).values(),
  ].slice(0, pageSize);
  const unread = query.data?.pages[0]?.unreadCount || 0;
  const hasNext = currentPage < lastLoadedPage || query.hasNextPage;

  /** Moves to a loaded page and returns focus to the list heading. */
  function showPage(index: number) {
    setPageIndex(index);
    headingRef.current?.focus({ preventScroll: true });
    headingRef.current?.scrollIntoView({ block: "start" });
  }

  /** Advances only after the next cursor page has loaded successfully. */
  async function nextPage() {
    if (query.isFetching || !hasNext) return;
    const next = currentPage + 1;
    if (next <= lastLoadedPage && pages[next]?.items.length) {
      showPage(next);
      return;
    }
    const result = await query.fetchNextPage();
    if (!result.isError && result.data?.pages[next]?.items.length)
      showPage(next);
  }

  return (
    <div className="notifications-page">
      <section className="page-heading">
        <div>
          <h1 ref={headingRef} tabIndex={-1}>
            Уведомления
          </h1>
          <p>Сообщения и ответы в чатах, приглашения и материалы встреч.</p>
        </div>
        <span className="notifications-count" role="status">
          Непрочитанных: {unread}
        </span>
      </section>
      <section className="content-card" aria-label="Список уведомлений">
        <ErrorNotice error={query.error || read.error} />
        {query.isPending ? (
          <Loading />
        ) : (
          <>
            {!rows.length && !query.isError && (
              <div className="notifications-empty">
                <Bell size={32} aria-hidden="true" />
                <h2>Пока тихо</h2>
                <p>
                  Пока нет уведомлений. Здесь появятся сообщения и ответы в
                  личных чатах, группах и комментариях встреч.
                </p>
              </div>
            )}
            <div className="notification-list notifications-page-list">
              {rows.map((item) => (
                <article
                  key={item.id}
                  className={`notification-row ${item.readAt ? "" : "notification-unread"}`}
                >
                  <span className="notification-page-icon" aria-hidden="true">
                    <Bell size={22} />
                  </span>
                  <div className="notification-page-copy">
                    <div className="notification-page-title">
                      <strong>{notificationLabel(item)}</strong>
                      {!item.readAt && (
                        <span
                          className="notification-unread-dot"
                          aria-label="Непрочитанное"
                        />
                      )}
                    </div>
                    <time dateTime={item.createdAt}>
                      {formatDate(item.createdAt)}
                    </time>
                    <div className="notification-page-actions">
                      {(item.payload.conferenceId ||
                        item.payload.conversationId) && (
                        <Link
                          to={notificationLink(item)}
                          onClick={() => {
                            if (!item.readAt) read.mutate(item.id);
                          }}
                        >
                          {item.payload.conversationId
                            ? "Открыть чат"
                            : item.type === "chat.message"
                              ? "Открыть комментарий"
                              : "Открыть встречу"}{" "}
                          <ArrowUpRight size={15} aria-hidden="true" />
                        </Link>
                      )}
                      {!item.readAt && (
                        <Button
                          variant="outline"
                          busy={read.isPending && read.variables === item.id}
                          onClick={() => read.mutate(item.id)}
                          aria-label={`Отметить как прочитанное: ${notificationLabel(item)}`}
                        >
                          <Check size={17} /> Прочитано
                        </Button>
                      )}
                    </div>
                  </div>
                </article>
              ))}
            </div>
            {(rows.length > 0 || currentPage > 0) && (
              <nav
                className="notifications-pagination"
                aria-label="Страницы уведомлений"
                aria-busy={query.isFetching}
              >
                <Button
                  variant="outline"
                  aria-label="Предыдущая страница"
                  disabled={currentPage === 0 || query.isFetching}
                  onClick={() => showPage(currentPage - 1)}
                >
                  <ChevronLeft size={17} aria-hidden="true" />
                  Назад
                </Button>
                <span className="notifications-page-number" role="status">
                  Страница {currentPage + 1}
                  <small>По {pageSize} уведомлений</small>
                </span>
                <Button
                  variant="outline"
                  aria-label="Следующая страница"
                  disabled={!hasNext || query.isFetching}
                  busy={query.isFetchingNextPage}
                  onClick={() => void nextPage()}
                >
                  Далее
                  <ChevronRight size={17} aria-hidden="true" />
                </Button>
              </nav>
            )}
            {query.isError && (
              <Button
                variant="outline"
                onClick={() =>
                  void (query.isFetchNextPageError
                    ? nextPage()
                    : query.refetch())
                }
              >
                Попробовать снова
              </Button>
            )}
          </>
        )}
      </section>
    </div>
  );
}
