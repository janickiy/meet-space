import { useState } from "react";
import type { ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import {
  Activity,
  ArrowRight,
  ArrowUpRight,
  CheckCheck,
  Cpu,
  Disc3,
  FileVideo,
  Folder,
  HardDrive,
  Info,
  LockKeyhole,
  MemoryStick,
  MessageCircle,
  MessagesSquare,
  Plus,
  RefreshCw,
  SearchX,
  Settings2,
  ShieldCheck,
  Trash2,
  Users,
  Video,
} from "lucide-react";
import { api } from "./api";
import {
  Badge,
  EmptyState,
  ErrorState,
  Heading,
  Metric,
  Note,
  Pagination,
  Panel,
  Person,
  SearchBox,
  Skeleton,
  StatusBadge,
  Table,
} from "./components";
import { useAdmin } from "./context";
import {
  bytes,
  canDeleteRecording,
  count,
  date,
  duration,
  percent,
} from "./format";
import { useList } from "./hooks";
import type {
  Administrator,
  AuditEvent,
  ChatSummary,
  Conference,
  Dashboard,
  FileSummary,
  Recording,
  ServiceHealth,
  Settings,
  SystemSummary,
  User,
} from "./types";

function Refresh({ refresh, busy }: { refresh: () => void; busy?: boolean }) {
  return (
    <button className="button" onClick={refresh} disabled={busy}>
      <RefreshCw className={busy ? "spin" : ""} />
      Обновить
    </button>
  );
}
function NoResults({ reset }: { reset: () => void }) {
  return (
    <EmptyState
      title="Ничего не найдено"
      message="Попробуйте изменить запрос или фильтры."
    >
      <button className="button" onClick={reset}>
        <SearchX />
        Сбросить фильтры
      </button>
    </EmptyState>
  );
}
function SortControls({
  sort,
  setSort,
  order,
  setOrder,
  options,
}: {
  sort: string;
  setSort: (value: string) => void;
  order: string;
  setOrder: (value: string) => void;
  options: [string, string][];
}) {
  return (
    <>
      <select
        aria-label="Сортировать по"
        value={sort}
        onChange={(event) => setSort(event.target.value)}
      >
        {options.map(([value, label]) => (
          <option value={value} key={value}>
            {label}
          </option>
        ))}
      </select>
      <select
        aria-label="Порядок сортировки"
        value={order}
        onChange={(event) => setOrder(event.target.value)}
      >
        <option value="desc">По убыванию</option>
        <option value="asc">По возрастанию</option>
      </select>
    </>
  );
}
function Services({ services }: { services: ServiceHealth[] }) {
  if (!services?.length)
    return <EmptyState message="Проверки сервисов пока недоступны." />;
  return (
    <div className="health-list">
      {services.map((service) => (
        <div className="service" key={service.name}>
          <span className="service-name">
            <span
              className={`dot ${["ok", "up", "healthy", "available"].includes(service.status) ? "good-text" : ["down", "error", "unavailable", "unhealthy"].includes(service.status) ? "bad-text" : "muted"}`}
            />
            {service.name}
          </span>
          <span className="service-time">
            {service.latencyMs != null &&
            ["ok", "up", "healthy", "available"].includes(service.status) ? (
              <>
                <span className="sr-only">Доступен, </span>
                {count(service.latencyMs)} мс
              </>
            ) : (
              <StatusBadge status={service.status} />
            )}
          </span>
        </div>
      ))}
    </div>
  );
}
function ActivityChart({ activity }: { activity: Dashboard["activity"] }) {
  if (!activity?.length)
    return (
      <EmptyState
        title="Нет данных об активности"
        message="История конференций пока недоступна."
      />
    );
  const max = Math.max(1, ...activity.map((item) => item.count));
  return (
    <div className="activity-chart">
      <div className="chart-summary">
        <strong>
          {count(activity.reduce((sum, item) => sum + item.count, 0))}
        </strong>
        <span>конференций за 7 дней</span>
      </div>
      <div
        className="bar-chart"
        role="img"
        aria-label={activity
          .map((item) => `${date(item.date, false)}: ${item.count} конференций`)
          .join("; ")}
      >
        <div className="chart-y">
          <span>{count(max)}</span>
          {max > 1 && <span>{count(Math.round(max / 2))}</span>}
          <span>0</span>
        </div>
        <div className="chart-columns">
          {activity.map((item) => (
            <div className="chart-column" key={item.date}>
              <div className="bar-area">
                <span
                  className="chart-bar"
                  style={{ height: `${(item.count / max) * 100}%` }}
                  title={`${date(item.date, false)}: ${count(item.count)}`}
                />
              </div>
              <span className="chart-date">
                {new Intl.DateTimeFormat("ru-RU", {
                  day: "numeric",
                  month: "short",
                }).format(new Date(item.date))}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
function ConferenceRows({ items }: { items: Conference[] }) {
  const { openDialog } = useAdmin();
  return (
    <>
      {items.map((item) => (
        <tr key={item.id}>
          <td>
            <div className="person">
              <span className="table-icon">
                <Video />
              </span>
              <div>
                <div className="cell-title">{item.title || "Без названия"}</div>
                <div className="cell-sub">
                  {item.ownerName || "Организатор не указан"}
                </div>
              </div>
            </div>
          </td>
          <td>
            {item.status === "active" ? (
              <Badge kind="good">В эфире</Badge>
            ) : (
              <StatusBadge status={item.status} />
            )}
          </td>
          <td>{count(item.participantCount)}</td>
          <td>
            <StatusBadge status={item.recordingStatus} />
          </td>
          <td className="muted">{date(item.startedAt || item.createdAt)}</td>
          <td>
            <button
              className="icon-button"
              aria-label={`Подробнее о конференции ${item.title}`}
              onClick={() =>
                openDialog({ kind: "conference", conference: item })
              }
            >
              <ArrowUpRight />
            </button>
          </td>
        </tr>
      ))}
    </>
  );
}

export function DashboardPage() {
  const result = useQuery({
    queryKey: ["admin", "dashboard"],
    queryFn: ({ signal }) => api<Dashboard>("/dashboard", { signal }),
    refetchInterval: 30_000,
  });
  const active = useQuery({
    queryKey: ["admin", "active-conferences"],
    queryFn: ({ signal }) =>
      api<{ items: Conference[] }>(
        "/conferences?status=active&page=1&pageSize=4&sort=createdAt&order=desc",
        { signal },
      ),
    refetchInterval: 30_000,
  });
  const data = result.data;
  return (
    <>
      <Heading
        title="Обзор платформы"
        subtitle="Всё, что происходит в вашем MeetSpace."
      >
        <span className="heading-date">
          {new Intl.DateTimeFormat("ru-RU", {
            day: "numeric",
            month: "long",
            weekday: "long",
          }).format(new Date())}
        </span>
        <Refresh
          refresh={() => {
            void result.refetch();
            void active.refetch();
          }}
          busy={result.isFetching}
        />
      </Heading>
      {result.isPending ? (
        <Skeleton />
      ) : result.isError ? (
        <Panel>
          <ErrorState
            error={result.error}
            retry={() => void result.refetch()}
          />
        </Panel>
      ) : (
        data && (
          <>
            <div className="metrics">
              <Metric
                label="Пользователи"
                value={count(data.users?.total)}
                icon={<Users />}
              >
                {count(data.users?.active)} активны ·{" "}
                {count(data.users?.blocked)} блок.
                <br />
                {count(data.users?.new)} новых за 7 дней
              </Metric>
              <Metric
                label="Конференции в эфире"
                value={count(data.conferences?.active)}
                icon={<Video />}
              >
                {count(data.conferences?.participants)} участников сейчас
              </Metric>
              <Metric
                label="Записи"
                value={count(data.recordings?.total)}
                icon={<Disc3 />}
              >
                {count(data.recordings?.active)} в процессе ·{" "}
                {bytes(data.recordings?.bytes)}
              </Metric>
              <Metric
                label="Хранилище"
                value={bytes(data.files?.bytes)}
                icon={<HardDrive />}
              >
                {data.files?.capacityBytes != null
                  ? `из ${bytes(data.files.capacityBytes)}`
                  : `${count(data.files?.count)} файлов`}
              </Metric>
            </div>
            <div className="dashboard-grid">
              <Panel
                title="Активность конференций"
                subtitle="Количество новых конференций по дням"
                actions={<span className="period-label">7 дней</span>}
              >
                <ActivityChart activity={data.activity} />
              </Panel>
              <Panel
                title="Состояние сервисов"
                actions={<Activity className="muted" />}
              >
                <Services services={data.services} />
                <div className="panel-bottom">
                  <span>Проверки сервисов</span>
                  <Link className="text-link" to="/system">
                    Подробнее <ArrowUpRight />
                  </Link>
                </div>
              </Panel>
            </div>
            <Panel
              title="Сейчас в эфире"
              actions={
                <Link className="text-link" to="/conferences">
                  Все конференции <ArrowRight />
                </Link>
              }
            >
              {active.isPending ? (
                <Skeleton rows={3} />
              ) : active.isError ? (
                <ErrorState
                  error={active.error}
                  retry={() => void active.refetch()}
                />
              ) : active.data?.items.length ? (
                <Table
                  headings={[
                    "Конференция / организатор",
                    "Статус",
                    "Участники",
                    "Запись",
                    "Начало",
                    "",
                  ]}
                >
                  <ConferenceRows items={active.data.items} />
                </Table>
              ) : (
                <EmptyState
                  title="Активных конференций нет"
                  message="Когда участники начнут встречу, она появится здесь."
                />
              )}
            </Panel>
            <div className="dashboard-totals">
              <span>
                Всего конференций:{" "}
                <strong>{count(data.conferences?.total)}</strong>
              </span>
              <span>
                Завершено: <strong>{count(data.conferences?.completed)}</strong>
              </span>
              <span>Данные обновляются каждые 30 секунд</span>
            </div>
          </>
        )
      )}
    </>
  );
}

export function UsersPage() {
  const list = useList<User>("users");
  const { openDialog } = useAdmin();
  function changeStatus(user: User) {
    const block = user.status === "active";
    openDialog({
      kind: "confirm",
      title: block ? "Заблокировать пользователя?" : "Восстановить доступ?",
      label: block ? "Заблокировать" : "Разблокировать",
      danger: block,
      body: (
        <>
          <Person name={user.name} description={user.email} />
          <p>
            {block
              ? "Вход в платформу будет запрещён, текущие сессии отозваны. Сообщения, файлы и записи сохранятся."
              : "Пользователь снова сможет войти в MeetSpace. Потребуется повторная авторизация."}
          </p>
        </>
      ),
      run: async (signal) => {
        await api(`/users/${encodeURIComponent(user.id)}`, {
          method: "PATCH",
          signal,
          body: { status: block ? "blocked" : "active" },
        });
      },
      success: block
        ? "Пользователь заблокирован"
        : "Доступ пользователя восстановлен",
    });
  }
  return (
    <>
      <Heading
        title="Пользователи"
        subtitle="Управляйте доступом участников к платформе."
      >
        <Refresh refresh={() => void list.refetch()} busy={list.isFetching} />
      </Heading>
      <Panel>
        <div className="toolbar">
          <SearchBox
            value={list.query}
            onChange={list.setQuery}
            placeholder="Имя, email или ID"
          />
          <select
            aria-label="Статус пользователя"
            value={list.status}
            onChange={(event) => list.setStatus(event.target.value)}
          >
            <option value="all">Все статусы</option>
            <option value="active">Активные</option>
            <option value="blocked">Заблокированные</option>
          </select>
          <SortControls
            {...list}
            options={[
              ["createdAt", "Дата регистрации"],
              ["name", "Имя"],
              ["email", "Email"],
            ]}
          />
        </div>
        {list.isPending ? (
          <Skeleton />
        ) : list.isError ? (
          <ErrorState error={list.error} retry={() => void list.refetch()} />
        ) : !list.data?.items.length ? (
          <NoResults reset={list.reset} />
        ) : (
          <Table
            headings={[
              "Пользователь",
              "ID",
              "Статус",
              "Регистрация",
              "Действие",
            ]}
          >
            {list.data.items.map((user) => (
              <tr key={user.id}>
                <td>
                  <Person
                    name={user.name}
                    description={user.email || "Email не указан"}
                  />
                </td>
                <td className="mono id-cell" title={user.id}>
                  {user.id}
                </td>
                <td>
                  <StatusBadge status={user.status} />
                </td>
                <td className="muted">{date(user.createdAt, false)}</td>
                <td>
                  <button
                    className={`button ${user.status === "active" ? "danger-quiet" : ""}`}
                    onClick={() => changeStatus(user)}
                  >
                    {user.status === "active"
                      ? "Блокировать"
                      : "Разблокировать"}
                  </button>
                </td>
              </tr>
            ))}
          </Table>
        )}
        {list.data && <Pagination {...list} total={list.data.total} />}
      </Panel>
    </>
  );
}

export function AdministratorsPage() {
  const list = useList<Administrator>("administrators");
  const { openDialog, session } = useAdmin();
  function status(administrator: Administrator) {
    const block = administrator.status === "active";
    openDialog({
      kind: "confirm",
      title: block ? "Заблокировать администратора?" : "Восстановить доступ?",
      label: block ? "Заблокировать" : "Разблокировать",
      danger: block,
      body: (
        <>
          <Person
            name={administrator.name}
            description={administrator.username}
          />
          <p>
            {block
              ? "Администратор потеряет доступ к панели. Все его сессии будут отозваны."
              : "Администратор сможет снова войти с логином и паролем."}
          </p>
        </>
      ),
      run: async (signal) => {
        await api(`/administrators/${encodeURIComponent(administrator.id)}`, {
          method: "PATCH",
          signal,
          body: { status: block ? "blocked" : "active" },
        });
      },
      success: block
        ? "Администратор заблокирован"
        : "Доступ администратора восстановлен",
    });
  }
  function remove(administrator: Administrator) {
    openDialog({
      kind: "confirm",
      title: "Удалить администратора?",
      label: "Удалить",
      danger: true,
      body: (
        <>
          <Person
            name={administrator.name}
            description={administrator.username}
          />
          <p>
            Учётная запись будет удалена, сессии отозваны. Записи в журнале
            аудита сохранятся.
          </p>
        </>
      ),
      run: async (signal) => {
        await api(`/administrators/${encodeURIComponent(administrator.id)}`, {
          method: "DELETE",
          signal,
        });
      },
      success: "Администратор удалён",
    });
  }
  return (
    <>
      <Heading
        title="Администраторы"
        subtitle="Отдельные учётные записи для управления платформой."
      >
        <button
          className="button primary"
          onClick={() => openDialog({ kind: "administrator" })}
        >
          <Plus />
          Добавить
        </button>
      </Heading>
      <Note icon={<ShieldCheck />}>
        Вход по логину и паролю. Собственную учётную запись и последнего
        активного администратора нельзя удалить или заблокировать.
      </Note>
      <Panel>
        <div className="toolbar">
          <SearchBox
            value={list.query}
            onChange={list.setQuery}
            placeholder="Имя или логин администратора"
          />
          <select
            aria-label="Статус администратора"
            value={list.status}
            onChange={(event) => list.setStatus(event.target.value)}
          >
            <option value="all">Все статусы</option>
            <option value="active">Активные</option>
            <option value="blocked">Заблокированные</option>
          </select>
          <SortControls
            {...list}
            options={[
              ["createdAt", "Дата создания"],
              ["username", "Логин"],
              ["name", "Имя"],
            ]}
          />
        </div>
        {list.isPending ? (
          <Skeleton />
        ) : list.isError ? (
          <ErrorState error={list.error} retry={() => void list.refetch()} />
        ) : !list.data?.items.length ? (
          <NoResults reset={list.reset} />
        ) : (
          <Table
            headings={[
              "Администратор",
              "Логин",
              "Статус",
              "Последний вход",
              "Действия",
            ]}
          >
            {list.data.items.map((administrator) => (
              <tr key={administrator.id}>
                <td>
                  <Person
                    name={administrator.name || administrator.username}
                    description={
                      administrator.id === session.administrator.id
                        ? "Это вы"
                        : administrator.email || "Email не указан"
                    }
                  />
                </td>
                <td className="mono">{administrator.username}</td>
                <td>
                  <StatusBadge status={administrator.status} />
                </td>
                <td className="muted">
                  {administrator.lastLoginAt
                    ? date(administrator.lastLoginAt)
                    : "Ещё не входил"}
                </td>
                <td>
                  <div className="row-actions">
                    <button
                      className="icon-button"
                      aria-label={`Редактировать ${administrator.username}`}
                      onClick={() =>
                        openDialog({ kind: "administrator", administrator })
                      }
                    >
                      <Settings2 />
                    </button>
                    <button
                      className="icon-button"
                      aria-label={`Сменить пароль ${administrator.username}`}
                      onClick={() =>
                        openDialog({ kind: "password", administrator })
                      }
                    >
                      <LockKeyhole />
                    </button>
                    {administrator.id !== session.administrator.id && (
                      <>
                        <button
                          className="button"
                          onClick={() => status(administrator)}
                        >
                          {administrator.status === "active"
                            ? "Блокировать"
                            : "Разблокировать"}
                        </button>
                        <button
                          className="icon-button danger-quiet"
                          aria-label={`Удалить ${administrator.username}`}
                          onClick={() => remove(administrator)}
                        >
                          <Trash2 />
                        </button>
                      </>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </Table>
        )}
        {list.data && <Pagination {...list} total={list.data.total} />}
      </Panel>
    </>
  );
}

export function ConferencesPage() {
  const list = useList<Conference>("conferences");
  return (
    <>
      <Heading
        title="Конференции"
        subtitle="Активные и завершённые встречи в одном месте."
      >
        <Badge>Только просмотр</Badge>
        <Refresh refresh={() => void list.refetch()} busy={list.isFetching} />
      </Heading>
      <Panel>
        <div className="toolbar">
          <SearchBox
            value={list.query}
            onChange={list.setQuery}
            placeholder="Название или организатор"
          />
          <select
            aria-label="Статус конференции"
            value={list.status}
            onChange={(event) => list.setStatus(event.target.value)}
          >
            <option value="all">Все конференции</option>
            <option value="active">В эфире</option>
            <option value="finished">Завершённые</option>
            <option value="scheduled">Запланированные</option>
            <option value="created">Созданные</option>
            <option value="cancelled">Отменённые</option>
          </select>
          <SortControls
            {...list}
            options={[
              ["createdAt", "Дата создания"],
              ["title", "Название"],
            ]}
          />
        </div>
        {list.isPending ? (
          <Skeleton />
        ) : list.isError ? (
          <ErrorState error={list.error} retry={() => void list.refetch()} />
        ) : !list.data?.items.length ? (
          <NoResults reset={list.reset} />
        ) : (
          <Table
            headings={[
              "Конференция / организатор",
              "Статус",
              "Участники",
              "Запись",
              "Начало",
              "",
            ]}
          >
            <ConferenceRows items={list.data.items} />
          </Table>
        )}
        {list.data && <Pagination {...list} total={list.data.total} />}
      </Panel>
    </>
  );
}

export function RecordingsPage() {
  const list = useList<Recording>("recordings");
  const { openDialog } = useAdmin();
  function remove(recording: Recording) {
    openDialog({
      kind: "confirm",
      title: "Удалить запись?",
      label: "Удалить запись",
      danger: true,
      body: (
        <>
          <h3>
            {recording.title || recording.conferenceTitle || recording.id}
          </h3>
          <p>
            {date(recording.createdAt)} · {bytes(recording.sizeBytes)}
          </p>
          <p>
            Файл записи и связанные метаданные будут удалены. Восстановить
            запись не получится.
          </p>
        </>
      ),
      run: async (signal) => {
        await api(`/recordings/${encodeURIComponent(recording.id)}`, {
          method: "DELETE",
          signal,
        });
      },
      success: "Запись удалена",
    });
  }
  return (
    <>
      <Heading
        title="Записи"
        subtitle="Записи конференций и состояние обработки."
      >
        <Refresh refresh={() => void list.refetch()} busy={list.isFetching} />
      </Heading>
      <Panel>
        <div className="toolbar">
          <SearchBox
            value={list.query}
            onChange={list.setQuery}
            placeholder="Название конференции или ID"
          />
          <select
            aria-label="Статус записи"
            value={list.status}
            onChange={(event) => list.setStatus(event.target.value)}
          >
            <option value="all">Все статусы</option>
            <option value="starting">Запускается</option>
            <option value="recording">Идёт запись</option>
            <option value="degraded">Запись с ошибками</option>
            <option value="stopping">Завершается</option>
            <option value="ready">Готова</option>
            <option value="partial_ready">Частично готова</option>
            <option value="finalizing">Обработка</option>
            <option value="uploading">Загрузка</option>
            <option value="failed">Ошибка</option>
            <option value="cancelled">Отменена</option>
          </select>
          <SortControls
            {...list}
            options={[
              ["createdAt", "Дата создания"],
              ["sizeBytes", "Размер"],
            ]}
          />
        </div>
        {list.isPending ? (
          <Skeleton />
        ) : list.isError ? (
          <ErrorState error={list.error} retry={() => void list.refetch()} />
        ) : !list.data?.items.length ? (
          <NoResults reset={list.reset} />
        ) : (
          <Table
            headings={[
              "Запись / конференция",
              "Статус",
              "Длительность",
              "Размер",
              "Действия",
            ]}
          >
            {list.data.items.map((recording) => (
              <tr key={recording.id}>
                <td>
                  <div className="person">
                    <span className="table-icon">
                      <FileVideo />
                    </span>
                    <div>
                      <div className="cell-title">
                        {recording.title ||
                          recording.conferenceTitle ||
                          "Без названия"}
                      </div>
                      <div className="cell-sub">
                        {date(recording.createdAt)}
                      </div>
                    </div>
                  </div>
                </td>
                <td>
                  <StatusBadge status={recording.status} />
                </td>
                <td className="mono">{duration(recording.durationSeconds)}</td>
                <td className="muted">{bytes(recording.sizeBytes)}</td>
                <td>
                  <div className="row-actions">
                    <button
                      className="icon-button"
                      aria-label="Информация о записи"
                      onClick={() =>
                        openDialog({ kind: "recording", recording })
                      }
                    >
                      <Info />
                    </button>
                    <button
                      className="icon-button danger-quiet"
                      aria-label="Удалить запись"
                      disabled={
                        !canDeleteRecording(
                          recording.status,
                          recording.canDelete,
                        )
                      }
                      title={
                        !canDeleteRecording(
                          recording.status,
                          recording.canDelete,
                        )
                          ? "Удаление недоступно во время записи или обработки"
                          : "Удалить запись"
                      }
                      onClick={() => remove(recording)}
                    >
                      <Trash2 />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </Table>
        )}
        {list.data && <Pagination {...list} total={list.data.total} />}
      </Panel>
    </>
  );
}

export function ChatsPage() {
  const result = useQuery({
    queryKey: ["admin", "chats"],
    queryFn: ({ signal }) => api<ChatSummary>("/chats", { signal }),
  });
  const data = result.data;
  return (
    <>
      <Heading title="Чаты" subtitle="Общая статистика общения на платформе.">
        <Refresh
          refresh={() => void result.refetch()}
          busy={result.isFetching}
        />
      </Heading>
      {result.isPending ? (
        <Skeleton />
      ) : result.isError ? (
        <Panel>
          <ErrorState
            error={result.error}
            retry={() => void result.refetch()}
          />
        </Panel>
      ) : (
        data && (
          <>
            <div className="metrics">
              <Metric
                label="Личные чаты"
                value={count(data.directChats)}
                icon={<MessageCircle />}
              >
                Приватные диалоги
              </Metric>
              <Metric
                label="Групповые чаты"
                value={count(data.groupChats)}
                icon={<Users />}
              >
                Совместное общение
              </Metric>
              <Metric
                label="Чаты конференций"
                value={count(data.conferenceChats)}
                icon={<Video />}
              >
                Обсуждения встреч
              </Metric>
              <Metric
                label="Сообщения"
                value={count(data.messages)}
                icon={<MessagesSquare />}
              >
                Общее количество
              </Metric>
            </div>
            <Panel title="Общая статистика">
              <div className="detail-list">
                <Detail label="Личные чаты" value={count(data.directChats)} />
                <Detail label="Групповые чаты" value={count(data.groupChats)} />
                <Detail
                  label="Чаты конференций"
                  value={count(data.conferenceChats)}
                />
                <Detail label="Сообщения" value={count(data.messages)} />
                <Detail label="Вложения" value={count(data.attachments)} />
              </div>
            </Panel>
          </>
        )
      )}
    </>
  );
}

export function FilesPage() {
  const result = useQuery({
    queryKey: ["admin", "files"],
    queryFn: ({ signal }) => api<FileSummary>("/files", { signal }),
  });
  const data = result.data,
    used = percent(data?.bytes, data?.capacityBytes);
  return (
    <>
      <Heading
        title="Файлы и хранилище"
        subtitle="Объём данных и доступное место."
      >
        <Refresh
          refresh={() => void result.refetch()}
          busy={result.isFetching}
        />
      </Heading>
      {result.isPending ? (
        <Skeleton />
      ) : result.isError ? (
        <Panel>
          <ErrorState
            error={result.error}
            retry={() => void result.refetch()}
          />
        </Panel>
      ) : (
        data && (
          <>
            <div className="grid-two">
              <Panel className="storage-summary">
                <div className="metric-label">
                  Использование хранилища
                  <HardDrive />
                </div>
                <div className="storage-value">{bytes(data.bytes)}</div>
                <p className="muted">
                  {data.capacityBytes != null
                    ? `из ${bytes(data.capacityBytes)}`
                    : "Общая ёмкость хранилища недоступна"}
                </p>
                {used != null && (
                  <>
                    <div
                      className="progress"
                      role="progressbar"
                      aria-label="Занято хранилища"
                      aria-valuemin={0}
                      aria-valuemax={100}
                      aria-valuenow={used}
                    >
                      <span style={{ width: `${used}%` }} />
                    </div>
                    <div className="storage-note">
                      Занято {used.toFixed(1).replace(".", ",")}% · свободно{" "}
                      {bytes(
                        Math.max(
                          0,
                          (data.capacityBytes || 0) - (data.bytes || 0),
                        ),
                      )}
                    </div>
                  </>
                )}
              </Panel>
              <Panel title="Общая статистика">
                <div className="detail-list">
                  <Detail label="Количество файлов" value={count(data.count)} />
                  <Detail label="Занимаемый объём" value={bytes(data.bytes)} />
                  <Detail
                    label="Общая ёмкость"
                    value={bytes(data.capacityBytes)}
                  />
                  <Detail
                    label="Категории"
                    value={count(data.categories?.length)}
                  />
                </div>
              </Panel>
            </div>
            {data.scope && <Note icon={<Info />}>{data.scope}</Note>}
            <Panel title="Категории файлов">
              {data.categories?.length ? (
                <Table
                  headings={["Категория", "Количество", "Объём", "Доля объёма"]}
                >
                  {data.categories.map((category) => (
                    <tr key={category.name}>
                      <td>
                        <div className="person">
                          <span className="table-icon">
                            <Folder />
                          </span>
                          {category.name}
                        </div>
                      </td>
                      <td>{count(category.count)}</td>
                      <td>{bytes(category.bytes)}</td>
                      <td>
                        {percent(category.bytes, data.bytes) != null
                          ? `${percent(category.bytes, data.bytes)!.toFixed(1).replace(".", ",")}%`
                          : "—"}
                      </td>
                    </tr>
                  ))}
                </Table>
              ) : (
                <EmptyState message="Распределение файлов по категориям недоступно." />
              )}
            </Panel>
          </>
        )
      )}
    </>
  );
}

export function SystemPage() {
  const result = useQuery({
    queryKey: ["admin", "system"],
    queryFn: ({ signal }) => api<SystemSummary>("/system", { signal }),
    refetchInterval: 30_000,
  });
  const data = result.data,
    resources = data?.resources;
  const healthy = data?.services?.filter((service) =>
    ["healthy", "ok", "available", "up"].includes(service.status),
  ).length;
  return (
    <>
      <Heading
        title="Состояние системы"
        subtitle="Сервисы, ресурсы и доступность платформы."
      >
        <Refresh
          refresh={() => void result.refetch()}
          busy={result.isFetching}
        />
      </Heading>
      {result.isPending ? (
        <Skeleton />
      ) : result.isError ? (
        <Panel>
          <ErrorState
            error={result.error}
            retry={() => void result.refetch()}
          />
        </Panel>
      ) : (
        data && (
          <>
            <div className="metrics">
              <Metric
                label="Доступные сервисы"
                value={`${count(healthy)} / ${count(data.services?.length)}`}
                icon={<CheckCheck />}
              >
                По последней проверке
              </Metric>
              <Metric
                label="Процессор"
                value={
                  resources?.cpuPercent != null
                    ? `${count(resources.cpuPercent)}%`
                    : "Нет данных"
                }
                icon={<Cpu />}
              >
                Загрузка процессора
              </Metric>
              <Metric
                label="Оперативная память"
                value={bytes(resources?.memoryUsedBytes)}
                icon={<MemoryStick />}
              >
                {resources?.memoryTotalBytes != null
                  ? `из ${bytes(resources.memoryTotalBytes)}`
                  : "Общий объём недоступен"}
              </Metric>
              <Metric
                label="Диск"
                value={bytes(resources?.diskUsedBytes)}
                icon={<HardDrive />}
              >
                {resources?.diskTotalBytes != null
                  ? `из ${bytes(resources.diskTotalBytes)}`
                  : "Общий объём недоступен"}
              </Metric>
            </div>
            <Panel
              title="Сервисы платформы"
              subtitle="Автоматическая проверка каждые 30 секунд"
            >
              {data.services?.length ? (
                <Table
                  headings={[
                    "Сервис",
                    "Состояние",
                    "Отклик",
                    "Версия",
                    "Последняя проверка",
                  ]}
                >
                  {data.services.map((service) => (
                    <tr key={service.name}>
                      <td>
                        <div className="cell-title">{service.name}</div>
                        {service.message && (
                          <div className="cell-sub service-message">
                            {service.message}
                          </div>
                        )}
                      </td>
                      <td>
                        <StatusBadge status={service.status} />
                      </td>
                      <td className="mono">
                        {service.latencyMs != null
                          ? `${count(service.latencyMs)} мс`
                          : "—"}
                      </td>
                      <td className="mono">{service.version || "—"}</td>
                      <td className="muted">{date(service.checkedAt)}</td>
                    </tr>
                  ))}
                </Table>
              ) : (
                <EmptyState message="Проверки сервисов пока недоступны." />
              )}
            </Panel>
            <Note icon={<Info />}>
              Отсутствующие метрики отображаются как «Нет данных». Это не
              означает, что сервис недоступен.
            </Note>
          </>
        )
      )}
    </>
  );
}

export function SettingsPage() {
  const result = useQuery({
    queryKey: ["admin", "settings"],
    queryFn: ({ signal }) => api<Settings>("/settings", { signal }),
  });
  const [group, setGroup] = useState("");
  const data = result.data;
  const groups = [
    ...new Set(data?.items.map((item) => item.group || "Основные") || []),
  ];
  const selected = groups.includes(group) ? group : groups[0];
  const groupNames: Record<string, string> = {
    general: "Основные",
    limits: "Лимиты",
    smtp: "Почта",
    mail: "Почта",
    recording: "Запись и хранение",
    storage: "Хранилище",
    media: "Медиа",
    network: "Сеть",
    security: "Безопасность",
  };
  return (
    <>
      <Heading title="Настройки" subtitle="Действующие параметры платформы.">
        <Badge>Конфигурация сервера</Badge>
        <Refresh
          refresh={() => void result.refetch()}
          busy={result.isFetching}
        />
      </Heading>
      <Note icon={<ShieldCheck />}>
        Настройки этой установки управляются на сервере. Изменения выполняются в
        конфигурации с последующим перезапуском соответствующего сервиса.
        Значения секретов не показываются.
      </Note>
      {result.isPending ? (
        <Skeleton />
      ) : result.isError ? (
        <Panel>
          <ErrorState
            error={result.error}
            retry={() => void result.refetch()}
          />
        </Panel>
      ) : !data?.items.length ? (
        <Panel>
          <EmptyState message="Сервер не предоставил доступные параметры." />
        </Panel>
      ) : (
        <>
          <div className="tabs" role="tablist" aria-label="Группы настроек">
            {groups.map((value) => (
              <button
                role="tab"
                aria-selected={selected === value}
                aria-controls="settings-content"
                id={`settings-tab-${value}`}
                className={selected === value ? "active" : ""}
                key={value}
                onClick={() => setGroup(value)}
              >
                {groupNames[value] || value}
              </button>
            ))}
          </div>
          <Panel title={groupNames[selected] || selected}>
            <div
              className="settings-list"
              role="tabpanel"
              id="settings-content"
              aria-labelledby={`settings-tab-${selected}`}
            >
              {data.items
                .filter((item) => (item.group || "Основные") === selected)
                .map((item) => (
                  <div className="setting-row" key={item.key}>
                    <div>
                      <h3>{item.label}</h3>
                      <p>{item.description}</p>
                    </div>
                    <div className="setting-value">
                      {item.secret || item.type === "secret" ? (
                        <Badge kind={item.configured ? "good" : "neutral"}>
                          {item.configured ? "Секрет настроен" : "Не настроен"}
                        </Badge>
                      ) : typeof item.value === "boolean" ? (
                        <Badge kind={item.value ? "good" : "neutral"}>
                          {item.value ? "Включено" : "Выключено"}
                        </Badge>
                      ) : (
                        <span>
                          {item.value == null || item.value === ""
                            ? "Не задано"
                            : String(item.value)}
                        </span>
                      )}
                      <span className="server-label">
                        <LockKeyhole />
                        На сервере
                      </span>
                    </div>
                  </div>
                ))}
            </div>
          </Panel>
        </>
      )}
    </>
  );
}

export const auditLabels: Record<string, string> = {
  login: "Успешный вход",
  "auth.login": "Успешный вход",
  "auth.login_success": "Успешный вход",
  "auth.login_failed": "Неуспешный вход",
  login_failed: "Неуспешный вход",
  logout: "Выход",
  "auth.logout": "Выход",
  "administrator.bootstrap": "Первоначальный администратор",
  "administrator.recover": "Восстановление доступа",
  "administrator.create": "Создание администратора",
  "administrator.update": "Изменение администратора",
  "administrator.delete": "Удаление администратора",
  "administrator.block": "Блокировка администратора",
  "administrator.unblock": "Разблокировка администратора",
  "administrator.password": "Смена пароля",
  "administrator.password_reset": "Сброс пароля",
  "user.block": "Блокировка пользователя",
  "user.unblock": "Разблокировка пользователя",
  "user.update": "Изменение пользователя",
  "recording.delete": "Удаление записи",
  "settings.update": "Изменение настроек",
};
export function AuditPage() {
  const [objectType, setObjectType] = useState("all");
  const list = useList<AuditEvent>("audit", { objectType });
  const { openDialog } = useAdmin();
  return (
    <>
      <Heading
        title="Журнал аудита"
        subtitle="История действий администраторов и событий входа."
      >
        <Badge>Только чтение</Badge>
        <Refresh refresh={() => void list.refetch()} busy={list.isFetching} />
      </Heading>
      <Panel>
        <div className="toolbar">
          <SearchBox
            value={list.query}
            onChange={list.setQuery}
            placeholder="Действие, логин, объект или IP"
          />
          <select
            aria-label="Тип объекта"
            value={objectType}
            onChange={(event) => {
              setObjectType(event.target.value);
              list.setPage(1);
            }}
          >
            <option value="all">Все объекты</option>
            <option value="administrator">Администраторы</option>
            <option value="user">Пользователи</option>
            <option value="recording">Записи</option>
            <option value="settings">Настройки</option>
            <option value="session">Сессии</option>
          </select>
          <SortControls {...list} options={[["createdAt", "Время события"]]} />
        </div>
        {list.isPending ? (
          <Skeleton />
        ) : list.isError ? (
          <ErrorState error={list.error} retry={() => void list.refetch()} />
        ) : !list.data?.items.length ? (
          <NoResults
            reset={() => {
              setObjectType("all");
              list.reset();
            }}
          />
        ) : (
          <Table
            headings={[
              "Время",
              "Администратор",
              "Действие / объект",
              "IP-адрес",
              "",
            ]}
          >
            {list.data.items.map((event) => (
              <tr key={event.id}>
                <td className="muted">{date(event.createdAt)}</td>
                <td className="mono">{event.administratorUsername || "—"}</td>
                <td>
                  <div className="cell-title">
                    {auditLabels[event.action] || event.action}
                  </div>
                  <div className="cell-sub">
                    {event.objectType}
                    {event.objectId ? ` · ${event.objectId}` : ""}
                  </div>
                </td>
                <td className="mono">{event.ip || "—"}</td>
                <td>
                  <button
                    className="icon-button"
                    aria-label="Подробности события"
                    onClick={() => openDialog({ kind: "audit", event })}
                  >
                    <ArrowUpRight />
                  </button>
                </td>
              </tr>
            ))}
          </Table>
        )}
        {list.data && <Pagination {...list} total={list.data.total} />}
      </Panel>
      <p className="page-footnote">
        Журнал нельзя изменить или удалить. Время отображается в часовом поясе
        вашего устройства.
      </p>
    </>
  );
}

export function Detail({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="detail-row">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}
