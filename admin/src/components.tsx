import { useEffect, useRef } from "react";
import type { ReactNode } from "react";
import {
  AlertCircle,
  ChevronLeft,
  ChevronRight,
  Inbox,
  RefreshCw,
  Search,
  X,
} from "lucide-react";
import { count, initials } from "./format";

export function Heading({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle: string;
  children?: ReactNode;
}) {
  return (
    <div className="page-heading">
      <div>
        <h1>{title}</h1>
        <p>{subtitle}</p>
      </div>
      {children && <div className="actions">{children}</div>}
    </div>
  );
}
export function Badge({
  children,
  kind = "neutral",
}: {
  children: ReactNode;
  kind?: "good" | "bad" | "warn" | "neutral";
}) {
  return (
    <span className={`badge ${kind}`}>
      <span className="dot" />
      {children}
    </span>
  );
}
export function StatusBadge({ status }: { status: string | null | undefined }) {
  const labels: Record<string, [string, "good" | "bad" | "warn" | "neutral"]> =
    {
      active: ["Активен", "good"],
      blocked: ["Заблокирован", "bad"],
      recording: ["Идёт запись", "bad"],
      ready: ["Готова", "good"],
      completed: ["Завершена", "neutral"],
      done: ["Готова", "good"],
      finished: ["Завершена", "neutral"],
      ended: ["Завершена", "neutral"],
      cancelled: ["Отменена", "neutral"],
      partial_ready: ["Частично готова", "warn"],
      finalizing: ["Обработка", "warn"],
      uploading: ["Загрузка", "warn"],
      degraded: ["Запись с ошибками", "warn"],
      processing: ["Обработка", "warn"],
      running: ["В работе", "warn"],
      pending: ["Ожидание", "warn"],
      queued: ["В очереди", "warn"],
      starting: ["Запускается", "warn"],
      stopping: ["Завершается", "warn"],
      created: ["Создана", "neutral"],
      scheduled: ["Запланирована", "neutral"],
      error: ["Ошибка", "bad"],
      failed: ["Ошибка", "bad"],
      available: ["Доступен", "good"],
      healthy: ["Доступен", "good"],
      ok: ["Доступен", "good"],
      up: ["Доступен", "good"],
      unavailable: ["Недоступен", "bad"],
      unhealthy: ["Недоступен", "bad"],
      down: ["Недоступен", "bad"],
      unknown: ["Нет данных", "neutral"],
      disabled: ["Выключен", "neutral"],
      off: ["Выключена", "neutral"],
      none: ["Выключена", "neutral"],
    };
  const [text, kind] = labels[status || "unknown"] || [
    status || "Нет данных",
    "neutral",
  ];
  return <Badge kind={kind}>{text}</Badge>;
}
export function Person({
  name,
  description,
}: {
  name: string;
  description?: ReactNode;
}) {
  return (
    <div className="person">
      <span className="avatar">{initials(name)}</span>
      <div>
        <div className="cell-title">{name || "Без имени"}</div>
        {description && <div className="cell-sub">{description}</div>}
      </div>
    </div>
  );
}
export function Metric({
  label,
  value,
  icon,
  children,
}: {
  label: string;
  value: ReactNode;
  icon: ReactNode;
  children?: ReactNode;
}) {
  return (
    <article className="metric">
      <div className="metric-label">
        {label}
        {icon}
      </div>
      <div
        className={`metric-value ${value === "Нет данных" ? "unavailable" : ""}`}
      >
        {value}
      </div>
      <div className="metric-foot">
        {children || "Нет дополнительных данных"}
      </div>
    </article>
  );
}
export function EmptyState({
  title = "Нет данных",
  message = "Здесь появятся данные платформы.",
  children,
}: {
  title?: string;
  message?: string;
  children?: ReactNode;
}) {
  return (
    <div className="empty-state">
      <span className="state-icon">
        <Inbox />
      </span>
      <h2>{title}</h2>
      <p>{message}</p>
      {children}
    </div>
  );
}
export function ErrorState({
  error,
  retry,
}: {
  error: unknown;
  retry?: () => void;
}) {
  return (
    <div className="empty-state" role="alert">
      <span className="state-icon error">
        <AlertCircle />
      </span>
      <h2>Не удалось загрузить данные</h2>
      <p>
        {error instanceof Error ? error.message : "Сервер временно недоступен."}
      </p>
      {retry && (
        <button className="button" onClick={retry}>
          <RefreshCw />
          Повторить
        </button>
      )}
    </div>
  );
}
export function Skeleton({ rows = 5 }: { rows?: number }) {
  return (
    <div className="skeleton-panel" role="status" aria-label="Загрузка данных">
      {Array.from({ length: rows }, (_, index) => (
        <div className="skeleton" key={index} />
      ))}
    </div>
  );
}
export function Panel({
  title,
  subtitle,
  actions,
  children,
  className = "",
}: {
  title?: string;
  subtitle?: string;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={`panel ${className}`}>
      {title && (
        <div className="panel-head">
          <div>
            <h2>{title}</h2>
            {subtitle && <p>{subtitle}</p>}
          </div>
          {actions}
        </div>
      )}
      {children}
    </section>
  );
}
export function Table({
  headings,
  children,
}: {
  headings: string[];
  children: ReactNode;
}) {
  return (
    <div className="table-scroll">
      <table>
        <thead>
          <tr>
            {headings.map((text, index) => (
              <th scope="col" key={index}>
                {text || <span className="sr-only">Действия</span>}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>{children}</tbody>
      </table>
    </div>
  );
}
export function Pagination({
  page,
  pageSize,
  total,
  setPage,
  setPageSize,
}: {
  page: number;
  pageSize: number;
  total: number;
  setPage: (page: number) => void;
  setPageSize: (size: number) => void;
}) {
  const pages = Math.max(1, Math.ceil(total / pageSize));
  return (
    <div className="table-footer">
      <span role="status" aria-live="polite">
        {total
          ? `${count((page - 1) * pageSize + 1)}–${count(Math.min(page * pageSize, total))} из ${count(total)}`
          : "Нет результатов"}
      </span>
      <div className="pagination">
        <label className="page-size">
          <span>Строк</span>
          <select
            aria-label="Строк на странице"
            value={pageSize}
            onChange={(event) => setPageSize(Number(event.target.value))}
          >
            {[10, 25, 50, 100].map((size) => (
              <option key={size}>{size}</option>
            ))}
          </select>
        </label>
        <button
          className="icon-button"
          disabled={page <= 1}
          onClick={() => setPage(page - 1)}
          aria-label="Предыдущая страница"
        >
          <ChevronLeft />
        </button>
        <span className="page-current">
          {count(page)} / {count(pages)}
        </span>
        <button
          className="icon-button"
          disabled={page >= pages}
          onClick={() => setPage(page + 1)}
          aria-label="Следующая страница"
        >
          <ChevronRight />
        </button>
      </div>
    </div>
  );
}
export function SearchBox({
  value,
  onChange,
  placeholder,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
}) {
  return (
    <label className="search">
      <Search />
      <input
        aria-label={placeholder}
        placeholder={placeholder}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  );
}
export function Note({
  children,
  icon,
}: {
  children: ReactNode;
  icon: ReactNode;
}) {
  return (
    <div className="note">
      {icon}
      <span>{children}</span>
    </div>
  );
}
export function Modal({
  title,
  children,
  footer,
  close,
  busy = false,
}: {
  title: string;
  children: ReactNode;
  footer?: ReactNode;
  close: () => void;
  busy?: boolean;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const closeRef = useRef(close);
  const busyRef = useRef(busy);
  closeRef.current = close;
  busyRef.current = busy;
  useEffect(() => {
    const previous =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    ref.current
      ?.querySelector<HTMLElement>("input, button, select, textarea")
      ?.focus();
    function keydown(event: KeyboardEvent) {
      if (event.key === "Escape" && !busyRef.current) closeRef.current();
      if (event.key !== "Tab" || !ref.current) return;
      const elements = [
        ...ref.current.querySelectorAll<HTMLElement>(
          "button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), a[href]",
        ),
      ].filter((element) => !element.hidden);
      const first = elements[0],
        last = elements[elements.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last?.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first?.focus();
      }
    }
    document.addEventListener("keydown", keydown);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", keydown);
      previous?.focus();
    };
  }, []);
  return (
    <div
      className="overlay"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget && !busy) close();
      }}
    >
      <div
        className="modal"
        ref={ref}
        role="dialog"
        aria-modal="true"
        aria-labelledby="modal-title"
      >
        <div className="modal-head">
          <h2 id="modal-title">{title}</h2>
          <button
            className="icon-button"
            onClick={close}
            disabled={busy}
            aria-label="Закрыть"
          >
            <X />
          </button>
        </div>
        <div className="modal-body">{children}</div>
        {footer && <div className="modal-footer">{footer}</div>}
      </div>
    </div>
  );
}
