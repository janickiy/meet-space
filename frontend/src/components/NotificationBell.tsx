import { useId, useState } from "react";
import { Link } from "react-router";
import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import {
  ArrowUpRight,
  Bell,
  CalendarClock,
  Check,
  ChevronRight,
  FileText,
  MessageCircle,
  Play,
  Users,
  Video,
} from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import { formatDate } from "../utils";
import { notificationLink } from "../intelligence";
import type { Notification } from "../types";
import { Button, ErrorNotice, Loading, Modal } from "./ui";
import "./notification-modal.css";

function notificationIcon(item: Notification) {
  if (item.type === "chat.message")
    return item.payload.conversationType === "group" ? Users : MessageCircle;
  if (item.type.includes("recording")) return Play;
  if (item.type.includes("transcript") || item.type.includes("summary"))
    return FileText;
  if (item.type === "conference.soon" || item.type === "conference.rescheduled")
    return CalendarClock;
  if (item.type === "conference.invited") return Users;
  return Video;
}

/**
 * notificationLabel переводит тип и нагрузку уведомления в понятную подпись для списка.
 *
 * @args
 *   - item (Notification) — элемент списка, который обрабатывает текущий шаг.
 *
 * @returns string — вычисленное значение: "Запись встречи готова"; "Скоро начнётся встреча"; "Вас пригласили войти во встречу"; "Запрос на вход отклонён"; "Вы исключены из встречи"; "Обновление вашей конференции".
 */
export function notificationLabel(item: Notification): string {
  if (item.type === "chat.message") {
    const place = item.payload.conversationId
      ? item.payload.conversationType === "group"
        ? "в группе"
        : "в личном чате"
      : "в чате встречи";
    return item.payload.isReply
      ? `Ответ на ваше сообщение ${place}`
      : `Новое сообщение ${place}`;
  }
  if (item.type === "transcript.failed")
    return "Не удалось подготовить расшифровку";
  if (item.type === "summary.failed")
    return "Не удалось подготовить итоги встречи";
  if (item.type.includes("transcript")) return "Расшифровка встречи готова";
  if (item.type.includes("summary")) return "Итоги встречи готовы";
  if (item.type.includes("recording")) return "Запись встречи готова";
  if (item.type === "conference.soon") return "Скоро начнётся встреча";
  if (item.type === "conference.invited") return "Вас пригласили на встречу";
  if (item.type === "conference.rescheduled") return "Время встречи изменилось";
  if (item.type === "conference.cancelled") return "Встреча отменена";
  if (item.type === "processing.failed")
    return "Обработка материалов не завершена";
  if (item.payload.admissionState === "admitted")
    return "Вас пригласили войти во встречу";
  if (item.payload.admissionState === "rejected")
    return "Запрос на вход отклонён";
  if (item.payload.admissionState === "kicked")
    return "Вы исключены из встречи";
  return "Обновление вашей конференции";
}
/**
 * NotificationBell показывает непрочитанные уведомления, поддерживает страницы и отметку прочтения.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function NotificationBell() {
  const { user } = useAuth();
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const unreadDescriptionId = useId();
  const query = useInfiniteQuery({
    queryKey: ["notifications", user?.id],
    initialPageParam: undefined as string | undefined,
    /**
     * queryFn загружает данные запроса с его сигналом отмены для кеша React Query.
     *
     * @args
     *   - объект параметров: pageParam — свойство текущего компонента; signal — сигнал отмены запроса или потока.
     *
     * @returns вычисленное значение: api.notifications(pageParam, signal).
     */
    queryFn: ({ pageParam, signal }) => api.notifications(pageParam, signal),
    /**
     * getNextPageParam извлекает курсор продолжения серверной страницы.
     *
     * @args
     *   - last — последняя загруженная страница, по которой определяется продолжение.
     *
     * @returns вычисленное значение: last.nextCursor || undefined.
     */
    getNextPageParam: (last) => last.nextCursor || undefined,
    refetchInterval: 15000,
  });
  const read = useMutation({
    mutationFn: api.readNotification,
    /**
     * onSettled обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
     *
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["notifications", user?.id] });
    },
  });
  const unread = query.data?.pages[0]?.unreadCount || 0;
  const rows = [
    ...new Map(
      (
        query.data?.pages.flatMap(
          /**
           * Обработчик flatMap преобразует текущий элемент в данные или представление результирующего списка.
           *
           * @args
           *   - page — изолированная страница Playwright.
           *
           * @returns преобразованное значение текущего элемента для результирующего набора.
           */ (page) => page.items,
        ) || []
      ).map(
        /**
         * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
         *
         * @args
         *   - item — элемент списка, который обрабатывает текущий шаг.
         *
         * @returns преобразованное значение текущего элемента для результирующего набора.
         */ (item) => [item.id, item],
      ),
    ).values(),
  ];
  return (
    <>
      <button
        className="icon-button notification-bell"
        aria-label="Уведомления"
        aria-describedby={unread > 0 ? unreadDescriptionId : undefined}
        onClick={
          /**
           * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
           *
           *
           * @returns вычисленное значение: setOpen(true).
           */ () => setOpen(true)
        }
      >
        <Bell size={21} aria-hidden="true" />
        {unread > 0 && (
          <>
            <span className="notification-dot" aria-hidden="true" />
            <span id={unreadDescriptionId} className="sr-only">
              {unread} непрочитанных уведомлений
            </span>
          </>
        )}
      </button>
      {open && (
        <Modal
          title="Уведомления"
          className="notification-modal"
          onClose={
            /**
             * onClose обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
             *
             *
             * @returns вычисленное значение: setOpen(false).
             */ () => setOpen(false)
          }
        >
          <div className="notification-modal-summary" role="status">
            {query.data &&
              (unread > 0 ? (
                <span className="notification-modal-count">
                  Новых: {unread}
                </span>
              ) : (
                <span>Всё прочитано</span>
              ))}
          </div>
          <div className="notification-modal-body">
            <ErrorNotice error={query.error || read.error} />
            {query.isPending ? (
              <Loading />
            ) : (
              <>
                {!rows.length && !query.isError && (
                  <div className="notification-modal-empty">
                    <span>
                      <Bell size={26} aria-hidden="true" />
                    </span>
                    <h3>Вы ничего не пропустили</h3>
                    <p>
                      Здесь появятся сообщения, приглашения и материалы ваших
                      встреч.
                    </p>
                  </div>
                )}
                <ul
                  className="notification-modal-list"
                  aria-label="Уведомления"
                >
                  {rows.map(
                    /**
                     * Обработчик rows.map преобразует один элемент набора в представление или данные следующего шага.
                     *
                     * @args
                     *   - item — элемент списка, который обрабатывает текущий шаг.
                     *
                     * @returns преобразованное значение текущего элемента для результирующего набора.
                     */ (item) => {
                      const Icon = notificationIcon(item);
                      return (
                        <li
                          key={item.id}
                          className={`notification-modal-item${item.readAt ? "" : " is-unread"}`}
                        >
                          <span
                            className="notification-modal-icon"
                            aria-hidden="true"
                          >
                            <Icon size={19} />
                          </span>
                          <div className="notification-modal-copy">
                            <div className="notification-modal-title">
                              <h3>{notificationLabel(item)}</h3>
                              {!item.readAt && (
                                <span
                                  className="notification-modal-dot"
                                  role="img"
                                  aria-label="Непрочитанное"
                                />
                              )}
                            </div>
                            <time dateTime={item.createdAt}>
                              {formatDate(item.createdAt)}
                            </time>
                            {(item.payload.conferenceId ||
                              item.payload.conversationId) && (
                              <Link
                                className="notification-modal-link"
                                to={notificationLink(item)}
                                onClick={
                                  /**
                                   * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                                   *
                                   *
                                   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                                   */ () => {
                                    if (!item.readAt) read.mutate(item.id);
                                    setOpen(false);
                                  }
                                }
                              >
                                {item.payload.conversationId
                                  ? "Открыть чат"
                                  : item.type === "chat.message"
                                    ? "Открыть комментарий"
                                    : "Открыть встречу"}
                                <ArrowUpRight size={14} aria-hidden="true" />
                              </Link>
                            )}
                          </div>
                          {!item.readAt && (
                            <Button
                              variant="secondary"
                              className="notification-modal-read"
                              busy={
                                read.isPending && read.variables === item.id
                              }
                              onClick={
                                /**
                                 * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                                 *
                                 *
                                 * @returns вычисленное значение: read.mutate(item.id).
                                 */ () => read.mutate(item.id)
                              }
                              aria-label={`Отметить как прочитанное: ${notificationLabel(item)}`}
                              title="Отметить как прочитанное"
                            >
                              <Check size={17} aria-hidden="true" />
                            </Button>
                          )}
                        </li>
                      );
                    },
                  )}
                </ul>
                {query.hasNextPage && (
                  <Button
                    variant="outline"
                    className="notification-modal-more"
                    busy={query.isFetchingNextPage}
                    onClick={
                      /**
                       * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                       *
                       *
                       * @returns вычисленное значение: void query.fetchNextPage().
                       */ () => void query.fetchNextPage()
                    }
                  >
                    Ещё уведомления
                  </Button>
                )}
                {query.isError && (
                  <Button
                    variant="outline"
                    onClick={
                      /**
                       * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                       *
                       *
                       * @returns вычисленное значение: void query.refetch().
                       */ () => void query.refetch()
                    }
                  >
                    Попробовать снова
                  </Button>
                )}
              </>
            )}
          </div>
          <footer className="notification-modal-footer">
            <Link to="/notifications" onClick={() => setOpen(false)}>
              Все уведомления
              <ChevronRight size={16} aria-hidden="true" />
            </Link>
          </footer>
        </Modal>
      )}
    </>
  );
}
