import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./api";
import { date } from "./format";
import "./directory-settings.css";

interface DirectoryStatus {
  status: string;
  queued: boolean;
  lastStartedAt?: string | null;
  lastCompletedAt?: string | null;
  lastSucceededAt?: string | null;
  errorCode?: string;
  imported: number;
  updated: number;
  disabled: number;
  total: number;
}

const statusLabels: Record<string, string> = {
  idle: "Ещё не запускалась",
  queued: "В очереди",
  running: "Выполняется",
  succeeded: "Завершена",
  success: "Завершена",
  failed: "Не выполнена",
};

/** Проверяет только сохранённые параметры; пароли не попадают в запросы диагностики. */
export function DirectoryActions({
  revision,
  disabled,
  enabled,
}: {
  revision: number;
  disabled: boolean;
  enabled: boolean;
}) {
  const client = useQueryClient();
  const [operation, setOperation] = useState<"test" | "sync" | null>(null);
  const [message, setMessage] = useState("");
  const [failure, setFailure] = useState("");
  const pending = useRef<AbortController | null>(null);
  const status = useQuery({
    queryKey: ["admin", "directory-status"],
    queryFn: ({ signal }) =>
      api<DirectoryStatus>("/directory/status", { signal }),
    retry: false,
    staleTime: 0,
    refetchInterval: (query) =>
      query.state.data?.queued ||
      ["running", "queued"].includes(query.state.data?.status || "")
        ? 2000
        : false,
    refetchIntervalInBackground: false,
  });
  useEffect(() => {
    setMessage("");
    setFailure("");
    return () => pending.current?.abort();
  }, [revision]);
  const syncPending =
    status.data?.queued ||
    ["running", "queued"].includes(status.data?.status || "");
  async function perform(action: "test" | "sync") {
    if (
      pending.current ||
      disabled ||
      (action === "sync" && (!enabled || syncPending))
    )
      return;
    const controller = new AbortController();
    pending.current = controller;
    setOperation(action);
    setFailure("");
    setMessage("");
    try {
      if (action === "test") {
        const result = await api<{
          reachable: boolean;
          matchedUsers: number;
          message: string;
        }>("/directory/test", {
          method: "POST",
          body: { revision },
          signal: controller.signal,
        });
        if (!controller.signal.aborted) {
          if (result.reachable)
            setMessage(
              `${result.message || "Подключение установлено."} Проверено записей: ${result.matchedUsers}.`,
            );
          else
            setFailure(result.message || "Не удалось подключиться к каталогу.");
        }
      } else {
        await api<{ status: string }>("/directory/sync", {
          method: "POST",
          body: { revision },
          signal: controller.signal,
        });
        if (!controller.signal.aborted) {
          setMessage("Синхронизация поставлена в очередь.");
          await client.invalidateQueries({
            queryKey: ["admin", "directory-status"],
          });
        }
      }
    } catch (error) {
      if (!controller.signal.aborted)
        setFailure(
          error instanceof Error
            ? error.message
            : "Не удалось выполнить действие.",
        );
    } finally {
      if (pending.current === controller) {
        pending.current = null;
        if (!controller.signal.aborted) setOperation(null);
      }
    }
  }
  return (
    <section
      className="directory-actions"
      aria-labelledby="directory-actions-heading"
    >
      <h3 id="directory-actions-heading">Проверка и синхронизация</h3>
      <p className="muted">
        Сначала сохраните параметры подключения. Проверка не изменяет
        пользователей.
      </p>
      <div className="directory-action-buttons">
        <button
          type="button"
          className="button"
          disabled={disabled || operation !== null}
          onClick={() => void perform("test")}
        >
          {operation === "test" ? "Проверяем…" : "Проверить подключение"}
        </button>
        <button
          type="button"
          className="button"
          disabled={disabled || operation !== null || !enabled || syncPending}
          onClick={() => void perform("sync")}
        >
          {operation === "sync" || syncPending
            ? "Синхронизация…"
            : "Синхронизировать сейчас"}
        </button>
        <button
          type="button"
          className="button"
          disabled={status.isFetching}
          onClick={() => void status.refetch()}
        >
          Обновить статус
        </button>
      </div>
      {disabled && (
        <p className="muted">
          Сохраните изменения перед проверкой и синхронизацией.
        </p>
      )}
      {message && <p role="status">{message}</p>}
      {failure && (
        <p className="form-error" role="alert">
          {failure}
        </p>
      )}
      {status.isError && (
        <p className="form-error" role="alert">
          Не удалось получить статус синхронизации. Нажмите «Обновить статус».
        </p>
      )}
      {status.data && (
        <dl className="directory-sync-status" aria-live="polite">
          <div>
            <dt>Состояние</dt>
            <dd>
              {status.data.queued
                ? "В очереди"
                : statusLabels[status.data.status] || "Нет данных"}
            </dd>
          </div>
          <div>
            <dt>Последняя успешная синхронизация</dt>
            <dd>{date(status.data.lastSucceededAt)}</dd>
          </div>
          <div>
            <dt>Последний запуск</dt>
            <dd>{date(status.data.lastStartedAt)}</dd>
          </div>
          <div>
            <dt>Найдено / добавлено / обновлено / отключено</dt>
            <dd>
              {status.data.total} / {status.data.imported} /{" "}
              {status.data.updated} / {status.data.disabled}
            </dd>
          </div>
          {status.data.errorCode && (
            <div>
              <dt>Код ошибки</dt>
              <dd>{status.data.errorCode}</dd>
            </div>
          )}
        </dl>
      )}
    </section>
  );
}
