import { Fragment, useEffect, useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Search } from "lucide-react";
import { ApiError, personalChatAPI } from "../api";
import { useAuth } from "../auth";
import { formatDate } from "../utils";
import { Button, ErrorNotice, Modal } from "./ui";
import "./personal-chat-search.css";

function SearchExcerpt({ text, query }: { text: string; query: string }) {
  const match = text.toLocaleLowerCase().indexOf(query.toLocaleLowerCase());
  const start = Math.max(0, match - 60);
  const end = Math.min(
    text.length,
    Math.max(start + 220, match + query.length),
  );
  const excerpt = text.slice(start, end);
  const parts = [];
  let offset = 0;
  const lower = excerpt.toLocaleLowerCase();
  const term = query.toLocaleLowerCase();
  let found = lower.indexOf(term);
  while (found !== -1) {
    parts.push(
      <Fragment key={offset}>
        {excerpt.slice(offset, found)}
        <mark>{excerpt.slice(found, found + query.length)}</mark>
      </Fragment>,
    );
    offset = found + query.length;
    found = lower.indexOf(term, offset);
  }
  return (
    <>
      {start > 0 && "…"}
      {parts}
      {excerpt.slice(offset)}
      {end < text.length && "…"}
    </>
  );
}

/** Searches the authorised history without marking results read or replacing a draft. */
export function PersonalChatSearchModal({
  conversationId,
  conversationName,
  onClose,
  onSelect,
  onAccessDenied,
  returnFocus,
}: {
  conversationId: string;
  conversationName: string;
  onClose: () => void;
  onSelect: (messageId: string) => void;
  onAccessDenied: () => void;
  returnFocus: () => HTMLElement | null;
}) {
  const { user } = useAuth();
  const [text, setText] = useState("");
  const [search, setSearch] = useState("");
  useEffect(() => {
    const timer = window.setTimeout(() => setSearch(text.trim()), 300);
    return () => window.clearTimeout(timer);
  }, [text]);
  const query = useInfiniteQuery({
    queryKey: ["personal-chat-search", conversationId, user?.id, search],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      personalChatAPI.searchMessages(conversationId, search, pageParam, signal),
    enabled: !!user && Array.from(search).length >= 2,
    getNextPageParam: (page) => page.nextCursor || undefined,
    retry: false,
  });
  const forbidden =
    query.error instanceof ApiError && [403, 404].includes(query.error.status);
  useEffect(() => {
    if (forbidden) onAccessDenied();
  }, [forbidden, onAccessDenied]);
  const current = search === text.trim();
  const messages =
    current && !query.isError
      ? [
          ...new Map(
            (query.data?.pages.flatMap((page) => page.items) || [])
              .filter((message) => !message.deletedAt && !message.call)
              .map((message) => [message.id, message]),
          ).values(),
        ]
      : [];
  return (
    <Modal
      title="Поиск в чате"
      className="personal-chat-search-modal"
      onClose={onClose}
      returnFocus={returnFocus}
    >
      <p className="personal-chat-search-name">{conversationName}</p>
      <label className="personal-chat-search-field">
        <Search size={19} aria-hidden="true" />
        <input
          type="search"
          aria-label="Поиск сообщений"
          data-autofocus
          value={text}
          maxLength={200}
          placeholder="Введите текст сообщения"
          onChange={(event) => setText(event.target.value)}
        />
      </label>
      <div
        className="personal-chat-search-results"
        aria-label="Результаты поиска"
      >
        {Array.from(text.trim()).length < 2 ? (
          <p className="field-hint">
            Введите минимум 2 символа для поиска по истории чата.
          </p>
        ) : !current || query.isPending ? (
          <p role="status">Ищем сообщения…</p>
        ) : forbidden ? (
          <p role="status">Чат больше недоступен.</p>
        ) : (
          <>
            <ErrorNotice error={query.error} />
            {query.isError && (
              <Button variant="outline" onClick={() => void query.refetch()}>
                Повторить поиск
              </Button>
            )}
            {!!messages.length && (
              <p className="field-hint" role="status">
                Найдено: {messages.length}
                {query.hasNextPage ? "+" : ""}
              </p>
            )}
            <ul>
              {messages.map((message) => (
                <li key={message.id}>
                  <button
                    type="button"
                    className="personal-chat-search-result"
                    onClick={() => onSelect(message.id)}
                  >
                    <span className="personal-chat-search-meta">
                      <strong>{message.senderName}</strong>
                      <time dateTime={message.createdAt}>
                        {formatDate(message.createdAt)}
                      </time>
                    </span>
                    <span className="personal-chat-search-excerpt">
                      <SearchExcerpt text={message.text} query={search} />
                    </span>
                    <span className="personal-chat-search-jump">
                      Перейти к сообщению
                    </span>
                  </button>
                </li>
              ))}
            </ul>
            {!messages.length && !query.isError && (
              <p role="status">Сообщения не найдены.</p>
            )}
            {query.hasNextPage && !query.isError && (
              <Button
                variant="outline"
                busy={query.isFetchingNextPage}
                onClick={() => void query.fetchNextPage()}
              >
                Ещё сообщения
              </Button>
            )}
          </>
        )}
      </div>
    </Modal>
  );
}
