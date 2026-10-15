import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronRight, Pin, PinOff } from "lucide-react";
import { api, ApiError } from "../api";
import { useAuth } from "../auth";
import { formatDate } from "../utils";
import type { ChatMessage } from "../types";
import { ErrorNotice, Modal } from "./ui";

export function usePersonalChatPins(scopeId: string, enabled: boolean) {
  const { user } = useAuth();
  const client = useQueryClient();
  const queryKey = ["personal-chat-pins", scopeId, user?.id];
  const query = useQuery({
    queryKey,
    queryFn: ({ signal }) => api.personalChatPins(scopeId, signal),
    enabled,
    refetchInterval: enabled ? 15000 : false,
    retry: (failures, error) =>
      !(error instanceof ApiError && [403, 404].includes(error.status)) &&
      failures < 2,
  });
  const [announcement, setAnnouncement] = useState("");
  const mutation = useMutation({
    mutationFn: ({
      messageId,
      pinned,
    }: {
      messageId: string;
      pinned: boolean;
    }) => api.setPersonalChatPin(scopeId, messageId, pinned),
    onSuccess: (_, { pinned }) => {
      setAnnouncement(
        pinned
          ? "Сообщение закреплено для участников чата."
          : "Сообщение откреплено.",
      );
      void client.invalidateQueries({ queryKey });
      void client.invalidateQueries({
        queryKey: ["personal-chat", scopeId, user?.id],
      });
    },
    onError: () => {
      void client.invalidateQueries({ queryKey });
    },
  });
  return {
    items: enabled && !query.isError ? query.data?.items || [] : [],
    query,
    mutation,
    announcement,
  };
}

/** Pins use the same message-context navigation as search, including older history. */
export function PersonalChatPins({
  items,
  onSelect,
  onUnpin,
  busy,
  error,
  loading,
}: {
  items: ChatMessage[];
  onSelect: (id: string) => void;
  onUnpin?: (id: string) => void;
  busy: boolean;
  error: unknown;
  loading: boolean;
}) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const first = items[0];
  const preview = (message: ChatMessage) =>
    message.text || message.attachments[0]?.filename || "Вложение";
  const select = (id: string) => {
    setOpen(false);
    onSelect(id);
  };
  return (
    <>
      {first && (
        <div className="chat-pinned-bar" aria-label="Закреплённые сообщения">
          <button
            type="button"
            className="chat-pinned-jump"
            onClick={() => select(first.id)}
          >
            <Pin size={20} aria-hidden="true" />
            <span className="chat-pinned-copy">
              <strong>Закреплённое сообщение</strong>
              <span>
                {first.senderName}: {preview(first)}
              </span>
            </span>
          </button>
          <button
            ref={trigger}
            type="button"
            className="chat-pinned-count"
            aria-label={`Все закреплённые сообщения (${items.length})`}
            aria-haspopup="dialog"
            onClick={() => setOpen(true)}
          >
            {items.length}
            <ChevronRight size={16} aria-hidden="true" />
          </button>
        </div>
      )}
      {open && (
        <Modal
          title="Закреплённые сообщения"
          onClose={() => setOpen(false)}
          returnFocus={() => trigger.current}
        >
          <p className="field-hint">
            Важное под рукой у всех участников чата. Можно закрепить до 20
            сообщений.
          </p>
          <ErrorNotice error={error} />
          {!items.length && !error && (
            <p className="field-hint" role="status">
              {loading
                ? "Загружаем закреплённые сообщения…"
                : "Закреплённых сообщений пока нет."}
            </p>
          )}
          <div className="chat-pinned-list">
            {items.map((message) => (
              <div className="chat-pinned-item" key={message.id}>
                <button
                  type="button"
                  className="chat-pinned-jump"
                  onClick={() => select(message.id)}
                >
                  <Pin size={18} aria-hidden="true" />
                  <span className="chat-pinned-copy">
                    <strong>{message.senderName}</strong>
                    <span>{preview(message)}</span>
                    <small>{formatDate(message.createdAt)}</small>
                  </span>
                </button>
                {onUnpin && (
                  <button
                    type="button"
                    className="icon-button"
                    disabled={busy}
                    title="Открепить сообщение"
                    aria-label={`Открепить сообщение от ${message.senderName}`}
                    onClick={(event) => {
                      // The row may disappear; keep keyboard focus inside the dialog.
                      event.currentTarget
                        .closest('[role="dialog"]')
                        ?.querySelector<HTMLButtonElement>("button")
                        ?.focus();
                      onUnpin(message.id);
                    }}
                  >
                    <PinOff size={18} aria-hidden="true" />
                  </button>
                )}
              </div>
            ))}
          </div>
        </Modal>
      )}
    </>
  );
}
