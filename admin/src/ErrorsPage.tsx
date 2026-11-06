import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  AlertTriangle,
  ArrowUpRight,
  ChevronLeft,
  ChevronRight,
  Clock3,
  RefreshCw,
  Search,
} from "lucide-react";
import { api, queryString } from "./api";
import {
  Badge,
  EmptyState,
  ErrorState,
  Heading,
  Metric,
  Modal,
  Panel,
  Skeleton,
  Table,
} from "./components";
import { count, date } from "./format";

type ErrorEvent = {
  id: string;
  service: string;
  instance: string;
  level: string;
  code: string;
  message: string;
  route: string;
  requestId: string;
  release: string;
  firstSeen: string;
  lastSeen: string;
  count: number;
  browser?: string;
  stack?: { file: string; line: number; column: number }[];
};
type ErrorPage = {
  items: ErrorEvent[];
  nextCursor: string | null;
  summary: { lastHour: number; lastDay: number; total: number };
  retentionDays: number;
  maxRecords: number;
};
type Filters = {
  service: string;
  level: string;
  period: string;
  from: string;
  to: string;
  code: string;
  requestId: string;
};
const defaults: Filters = {
  service: "all",
  level: "all",
  period: "14d",
  from: "",
  to: "",
  code: "",
  requestId: "",
};
const services: Record<string, string> = {
  api: "API",
  "media-worker": "Медиа",
  "record-worker": "Запись",
  "product-worker": "Фоновые задачи",
  frontend: "Сайт",
  "admin-frontend": "Админка",
};
const periods: Record<string, number> = {
  "1h": 60 * 60 * 1000,
  "24h": 24 * 60 * 60 * 1000,
  "7d": 7 * 24 * 60 * 60 * 1000,
  "14d": 14 * 24 * 60 * 60 * 1000,
};
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function levelBadge(level: string) {
  const warning = ["WARN", "WARNING"].includes(level.toUpperCase());
  return (
    <Badge kind={warning ? "warn" : "bad"}>
      {warning ? "Предупреждение" : "Ошибка"}
    </Badge>
  );
}
function filterParameters(filters: Filters, cursor: string) {
  const custom = filters.period === "custom";
  return queryString({
    service: filters.service,
    level: filters.level,
    from: custom
      ? new Date(filters.from).toISOString()
      : new Date(Date.now() - periods[filters.period]).toISOString(),
    to: custom ? new Date(filters.to).toISOString() : undefined,
    code: filters.code || undefined,
    requestId: filters.requestId || undefined,
    cursor: cursor || undefined,
    pageSize: 25,
  });
}
function DetailLine({
  label,
  value,
}: {
  label: string;
  value?: string | number;
}) {
  return (
    <div className="detail-row">
      <span>{label}</span>
      <strong>{value || "—"}</strong>
    </div>
  );
}
function ErrorDetails({ id, close }: { id: string; close: () => void }) {
  const result = useQuery({
    queryKey: ["admin", "errors", "detail", id],
    queryFn: ({ signal }) =>
      api<ErrorEvent>(`/errors/${encodeURIComponent(id)}`, { signal }),
    retry: false,
    staleTime: 0,
    refetchOnWindowFocus: false,
  });
  const event = result.data;
  return (
    <Modal title="Подробности ошибки" close={close}>
      {result.isPending ? (
        <Skeleton rows={4} />
      ) : result.isError ? (
        <ErrorState error={result.error} retry={() => void result.refetch()} />
      ) : event ? (
        <div className="error-details">
          <div className="error-detail-title">
            {levelBadge(event.level)}
            <code>{event.code}</code>
          </div>
          <p className="error-detail-message">{event.message}</p>
          <div className="detail-list">
            <DetailLine
              label="Сервис"
              value={services[event.service] || event.service}
            />
            <DetailLine label="Экземпляр" value={event.instance} />
            <DetailLine label="Первое событие" value={date(event.firstSeen)} />
            <DetailLine
              label="Последнее событие"
              value={date(event.lastSeen)}
            />
            <DetailLine label="Событий в группе" value={count(event.count)} />
            <DetailLine label="Маршрут" value={event.route} />
            <DetailLine label="ID последнего запроса" value={event.requestId} />
            <DetailLine label="Версия" value={event.release} />
            {event.browser && (
              <DetailLine label="Браузер" value={event.browser} />
            )}
          </div>
          {!!event.stack?.length && (
            <div className="error-frames">
              <h3>Позиции в JavaScript</h3>
              <ul>
                {event.stack.map((frame, index) => (
                  <li key={index}>
                    <code>
                      {frame.file}:{frame.line}:{frame.column}
                    </code>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      ) : null}
    </Modal>
  );
}

export function ErrorsPage() {
  const [filters, setFilters] = useState<Filters>(defaults);
  const [draft, setDraft] = useState<Filters>(defaults);
  const [filterError, setFilterError] = useState("");
  const [cursors, setCursors] = useState([""]);
  const [selected, setSelected] = useState<string | null>(null);
  const [automatic, setAutomatic] = useState(true);
  const [visible, setVisible] = useState(
    document.visibilityState === "visible",
  );
  const cursor = cursors[cursors.length - 1];
  useEffect(() => {
    const onVisibility = () =>
      setVisible(document.visibilityState === "visible");
    document.addEventListener("visibilitychange", onVisibility);
    return () => document.removeEventListener("visibilitychange", onVisibility);
  }, []);
  const result = useQuery({
    queryKey: ["admin", "errors", "list", filters, cursor],
    queryFn: ({ signal }) =>
      api<ErrorPage>(`/errors${filterParameters(filters, cursor)}`, { signal }),
    retry: false,
    staleTime: 0,
    refetchOnWindowFocus: false,
    refetchInterval: automatic && visible && !cursor ? 10_000 : false,
    refetchIntervalInBackground: false,
  });
  function apply(next: Filters) {
    setFilters(next);
    setCursors([""]);
    setSelected(null);
    setFilterError("");
  }
  function submit(event: React.FormEvent) {
    event.preventDefault();
    const next = {
      ...draft,
      code: draft.code.trim(),
      requestId: draft.requestId.trim(),
    };
    if (next.requestId && !uuid.test(next.requestId)) {
      setFilterError("ID запроса должен быть в формате UUID.");
      return;
    }
    if (next.period === "custom") {
      const from = new Date(next.from).getTime();
      const to = new Date(next.to).getTime();
      if (!Number.isFinite(from) || !Number.isFinite(to) || from > to) {
        setFilterError(
          "Укажите начало и конец периода. Конец не может быть раньше начала.",
        );
        return;
      }
    }
    apply(next);
  }
  function refresh() {
    if (cursor) setCursors([""]);
    else void result.refetch();
  }
  const patch = (key: keyof Filters, value: string) =>
    setDraft((current) => ({ ...current, [key]: value }));
  const policy = result.data;
  return (
    <>
      <Heading
        title="Ошибки"
        subtitle="Технические ошибки сервисов и браузера."
      >
        <button
          className="button"
          onClick={refresh}
          disabled={result.isFetching}
        >
          <RefreshCw className={result.isFetching ? "spin" : ""} />
          Обновить
        </button>
      </Heading>
      <div className="metrics error-metrics">
        <Metric
          label="Групп ошибок за час"
          value={count(policy?.summary.lastHour)}
          icon={<Clock3 />}
        >
          По времени последнего события
        </Metric>
        <Metric
          label="Групп ошибок за сутки"
          value={count(policy?.summary.lastDay)}
          icon={<AlertTriangle />}
        >
          Повторяющиеся события объединяются
        </Metric>
        <Metric
          label="Всего групп в журнале"
          value={count(policy?.summary.total)}
          icon={<AlertTriangle />}
        >
          Хранение {policy?.retentionDays ?? 14} дней · до{" "}
          {count(policy?.maxRecords ?? 20_000)} записей
        </Metric>
      </div>
      <Panel className="errors-panel">
        <form className="error-filters" onSubmit={submit}>
          <div className="error-filter-grid">
            <label>
              <span>Сервис</span>
              <select
                aria-label="Сервис"
                value={draft.service}
                onChange={(event) => patch("service", event.target.value)}
              >
                <option value="all">Все сервисы</option>
                {Object.entries(services).map(([key, label]) => (
                  <option key={key} value={key}>
                    {label}
                  </option>
                ))}
              </select>
            </label>
            <label>
              <span>Уровень</span>
              <select
                aria-label="Уровень"
                value={draft.level}
                onChange={(event) => patch("level", event.target.value)}
              >
                <option value="all">Все уровни</option>
                <option value="error">Ошибки</option>
                <option value="warning">Предупреждения</option>
              </select>
            </label>
            <label>
              <span>Период</span>
              <select
                aria-label="Период"
                value={draft.period}
                onChange={(event) => patch("period", event.target.value)}
              >
                <option value="1h">Последний час</option>
                <option value="24h">Последние сутки</option>
                <option value="7d">Последние 7 дней</option>
                <option value="14d">Последние 14 дней</option>
                <option value="custom">Выбрать даты</option>
              </select>
            </label>
            <label>
              <span>Код ошибки</span>
              <input
                value={draft.code}
                maxLength={80}
                placeholder="Например, http.5xx"
                onChange={(event) => patch("code", event.target.value)}
              />
            </label>
            <label className="error-request-filter">
              <span>ID последнего запроса</span>
              <input
                value={draft.requestId}
                maxLength={36}
                placeholder="UUID запроса"
                onChange={(event) => patch("requestId", event.target.value)}
              />
            </label>
            {draft.period === "custom" && (
              <>
                <label>
                  <span>Начало периода</span>
                  <input
                    type="datetime-local"
                    value={draft.from}
                    onChange={(event) => patch("from", event.target.value)}
                  />
                </label>
                <label>
                  <span>Конец периода</span>
                  <input
                    type="datetime-local"
                    value={draft.to}
                    onChange={(event) => patch("to", event.target.value)}
                  />
                </label>
              </>
            )}
          </div>
          {filterError && (
            <p className="error-filter-validation" role="alert">
              {filterError}
            </p>
          )}
          <div className="error-filter-actions">
            <div className="actions">
              <button className="button" type="submit">
                <Search />
                Применить
              </button>
              <button
                className="button"
                type="button"
                onClick={() => {
                  setDraft(defaults);
                  apply(defaults);
                }}
              >
                Сбросить
              </button>
            </div>
            <label className="errors-auto">
              <input
                type="checkbox"
                checked={automatic}
                onChange={(event) => setAutomatic(event.target.checked)}
              />
              <span>Автообновление · 10 с</span>
            </label>
          </div>
        </form>
        {result.isPending ? (
          <Skeleton />
        ) : result.isError ? (
          <ErrorState
            error={result.error}
            retry={() => void result.refetch()}
          />
        ) : !result.data?.items.length ? (
          <EmptyState
            title="Ошибок не найдено"
            message="За выбранный период и с этими фильтрами технические ошибки не зарегистрированы."
          />
        ) : (
          <Table
            headings={[
              "Последнее событие",
              "Сервис / уровень",
              "Ошибка",
              "Событий",
              "",
            ]}
          >
            {result.data.items.map((event) => (
              <tr key={event.id} data-error-id={event.id}>
                <td className="muted">{date(event.lastSeen)}</td>
                <td>
                  <div className="cell-title">
                    {services[event.service] || event.service}
                  </div>
                  <div className="cell-sub">{levelBadge(event.level)}</div>
                </td>
                <td className="error-message-cell">
                  <div className="cell-title">{event.message}</div>
                  <div className="cell-sub mono">
                    {event.code}
                    {event.route ? ` · ${event.route}` : ""}
                  </div>
                </td>
                <td className="mono">{count(event.count)}</td>
                <td>
                  <button
                    className="icon-button"
                    aria-label="Подробности ошибки"
                    onClick={() => setSelected(event.id)}
                  >
                    <ArrowUpRight />
                  </button>
                </td>
              </tr>
            ))}
          </Table>
        )}
        {result.data && (
          <div className="table-footer">
            <span role="status">
              {cursor
                ? "Автообновление приостановлено при просмотре истории"
                : automatic
                  ? "Автообновление каждые 10 секунд"
                  : "Автообновление выключено"}
            </span>
            <div className="pagination">
              <button
                className="icon-button"
                disabled={cursors.length === 1 || result.isFetching}
                aria-label="Предыдущая страница"
                onClick={() => setCursors((current) => current.slice(0, -1))}
              >
                <ChevronLeft />
              </button>
              <span className="page-current">
                Страница {count(cursors.length)}
              </span>
              <button
                className="icon-button"
                disabled={!result.data.nextCursor || result.isFetching}
                aria-label="Следующая страница"
                onClick={() => {
                  const next = result.data?.nextCursor;
                  if (next) setCursors((current) => [...current, next]);
                }}
              >
                <ChevronRight />
              </button>
            </div>
          </div>
        )}
      </Panel>
      <p className="page-footnote">
        Время отображается в часовом поясе вашего устройства. Повторения одной
        ошибки объединяются в группу.
      </p>
      {selected && (
        <ErrorDetails id={selected} close={() => setSelected(null)} />
      )}
    </>
  );
}
