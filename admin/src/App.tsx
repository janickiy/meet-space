import { useCallback, useEffect, useRef, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Link,
  NavLink,
  Navigate,
  Route,
  Routes,
  useLocation,
} from "react-router";
import {
  Activity,
  AlertTriangle,
  ArrowRight,
  ChevronsUpDown,
  Disc3,
  Folder,
  LayoutDashboard,
  LogOut,
  Menu,
  MessagesSquare,
  Moon,
  ScrollText,
  ShieldCheck,
  SlidersHorizontal,
  Sun,
  UserRound,
  Users,
  Video,
  X,
} from "lucide-react";
import { ErrorsPage } from "./ErrorsPage";
import { api, APIError, configureSession } from "./api";
import {
  clearAdministrativeCache,
  readSignedOut,
  rememberSignedOut,
} from "./sessionState";
import { AdminContext, useAdmin } from "./context";
import type { DialogRequest } from "./context";
import {
  Badge,
  ErrorState,
  Modal,
  Person,
  Skeleton,
  StatusBadge,
} from "./components";
import {
  AdministratorsPage,
  AuditPage,
  auditLabels,
  ChatsPage,
  ConferencesPage,
  DashboardPage,
  Detail,
  FilesPage,
  RecordingsPage,
  SettingsPage,
  SystemPage,
  UsersPage,
} from "./pages";
import {
  bytes,
  date,
  duration,
  initials,
  validateAdministrator,
  validatePassword,
} from "./format";
import type { AdminSession, Administrator } from "./types";

const sections = [
  { path: "/", label: "Главная", Icon: LayoutDashboard },
  { path: "/users", label: "Пользователи", Icon: Users },
  { path: "/administrators", label: "Администраторы", Icon: ShieldCheck },
  { path: "/conferences", label: "Конференции", Icon: Video },
  { path: "/chats", label: "Чаты", Icon: MessagesSquare },
  { path: "/recordings", label: "Записи", Icon: Disc3 },
  { path: "/files", label: "Файлы", Icon: Folder },
  { path: "/system", label: "Система", Icon: Activity },
  { path: "/settings", label: "Настройки", Icon: SlidersHorizontal },
  { path: "/audit", label: "Журнал аудита", Icon: ScrollText },
  { path: "/errors", label: "Ошибки", Icon: AlertTriangle },
];

function initialTheme(): "dark" | "light" {
  try {
    return localStorage.getItem("meetspace-admin-theme") === "light"
      ? "light"
      : "dark";
  } catch {
    return "dark";
  }
}
export default function App() {
  const queryClient = useQueryClient();
  const [signedOut, setSignedOut] = useState(readSignedOut);
  const [sessionNotice, setSessionNotice] = useState("");
  const currentIdentity = useRef<string | null>(null);
  const [theme, setTheme] = useState(initialTheme);
  const [dialog, setDialog] = useState<DialogRequest | null>(null);
  const [notification, setNotification] = useState("");
  const [menuOpen, setMenuOpen] = useState(false);
  const location = useLocation();
  const endSession = useCallback(() => {
    rememberSignedOut(true);
    setSignedOut(true);
    currentIdentity.current = null;
    configureSession(null);
    clearAdministrativeCache(queryClient);
    setDialog(null);
    setNotification("");
    setMenuOpen(false);
  }, [queryClient]);
  const acceptSession = useCallback(
    (value: AdminSession) => {
      configureSession(value, endSession);
      if (currentIdentity.current !== value.administrator.id) {
        void queryClient.cancelQueries({ queryKey: ["admin"] });
        queryClient.removeQueries({ queryKey: ["admin"] });
        setDialog(null);
        setNotification("");
      }
      currentIdentity.current = value.administrator.id;
      return value;
    },
    [endSession, queryClient],
  );
  const session = useQuery<AdminSession | null>({
    queryKey: ["admin-session"],
    queryFn: async ({ signal }) =>
      acceptSession(await api<AdminSession>("/auth/me", { signal })),
    enabled: !signedOut,
    retry: false,
    staleTime: 60_000,
  });
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    try {
      localStorage.setItem("meetspace-admin-theme", theme);
    } catch {
      /* Theme is optional. */
    }
  }, [theme]);
  useEffect(() => {
    setMenuOpen(false);
  }, [location.pathname]);
  useEffect(() => {
    if (!notification) return;
    const timer = setTimeout(() => setNotification(""), 5000);
    return () => clearTimeout(timer);
  }, [notification]);
  const logout = async (signal?: AbortSignal) => {
    let unconfirmed = false;
    try {
      await api("/auth/logout", { method: "POST", signal });
    } catch (failure) {
      unconfirmed = !(failure instanceof APIError && failure.status === 401);
    } finally {
      endSession();
      setSessionNotice(
        unconfirmed
          ? "Локальные данные очищены. Сервер не подтвердил отзыв сессии: проверьте соединение перед повторным входом."
          : "",
      );
    }
  };
  const unauthorized =
    session.error instanceof APIError && session.error.status === 401;
  if (session.isPending && !signedOut)
    return (
      <div className="auth-loading">
        <Brand />
        <Skeleton rows={3} />
      </div>
    );
  if (session.isError && !unauthorized && !signedOut)
    return (
      <div className="auth-loading">
        <Brand />
        <ErrorState
          error={session.error}
          retry={() => void session.refetch()}
        />
      </div>
    );
  if (!session.data || unauthorized || signedOut)
    return (
      <Login
        onLogin={(value) => {
          void queryClient.cancelQueries({ queryKey: ["admin-session"] });
          acceptSession(value);
          queryClient.setQueryData(["admin-session"], value);
          rememberSignedOut(false);
          setSignedOut(false);
          setSessionNotice("");
        }}
        notice={sessionNotice}
        theme={theme}
        toggleTheme={() => setTheme(theme === "dark" ? "light" : "dark")}
      />
    );
  const administrator = session.data.administrator;
  const current =
    sections.find((section) => section.path === location.pathname) ||
    sections[0];
  return (
    <AdminContext.Provider
      key={administrator.id}
      value={{
        session: session.data,
        openDialog: setDialog,
        notify: setNotification,
        logout,
      }}
    >
      <div className="app-shell" inert={dialog !== null}>
        <a className="skip-link" href="#main-content">
          Перейти к содержимому
        </a>
        {menuOpen && (
          <button
            className="nav-backdrop"
            aria-label="Закрыть навигацию"
            onClick={() => setMenuOpen(false)}
          />
        )}
        <aside
          className={`sidebar ${menuOpen ? "open" : ""}`}
          aria-label="Основная навигация"
        >
          <div className="brand-group">
            <Brand />
            <span className="brand-caption">ADMIN CONSOLE</span>
          </div>
          <nav>
            <p className="nav-caption">Платформа</p>
            {sections.map(({ path, label, Icon }, index) => (
              <div key={path}>
                {index === 7 && (
                  <>
                    <div className="nav-divider" />
                    <p className="nav-caption">Управление</p>
                  </>
                )}
                <NavLink
                  to={path}
                  onClick={() => setMenuOpen(false)}
                  end={path === "/"}
                  className={({ isActive }) =>
                    `nav-item ${isActive ? "active" : ""}`
                  }
                >
                  <Icon />
                  <span>{label}</span>
                </NavLink>
              </div>
            ))}
          </nav>
          <div className="sidebar-bottom">
            <div className="instance">
              <div>
                <ShieldCheck />
                Административный доступ
              </div>
              <p>{window.location.host}</p>
            </div>
            <button
              className="profile-link"
              onClick={() => setDialog({ kind: "profile" })}
            >
              <span className="avatar admin-avatar">
                {initials(administrator.name || administrator.username)}
              </span>
              <span>
                <strong>{administrator.name || administrator.username}</strong>
                <small>Администратор</small>
              </span>
              <ChevronsUpDown />
            </button>
          </div>
        </aside>
        <div className="main-column">
          <header className="topbar">
            <button
              className="icon-button mobile-menu"
              aria-label={menuOpen ? "Закрыть навигацию" : "Открыть навигацию"}
              aria-expanded={menuOpen}
              onClick={() => setMenuOpen(!menuOpen)}
            >
              {menuOpen ? <X /> : <Menu />}
            </button>
            <div className="breadcrumb">
              <span>Рабочее пространство</span>
              <span>/</span>
              <strong>{current.label}</strong>
            </div>
            <div className="topbar-actions">
              <button
                className="icon-button"
                aria-label={theme === "dark" ? "Светлая тема" : "Тёмная тема"}
                onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
              >
                {theme === "dark" ? <Sun /> : <Moon />}
              </button>
              <button
                className="topbar-profile"
                aria-label="Мой профиль"
                onClick={() => setDialog({ kind: "profile" })}
              >
                <UserRound />
                <span>{administrator.username}</span>
              </button>
            </div>
          </header>
          <main id="main-content" className="main-content">
            <Routes>
              <Route path="/" element={<DashboardPage />} />
              <Route path="/users" element={<UsersPage />} />
              <Route path="/administrators" element={<AdministratorsPage />} />
              <Route path="/conferences" element={<ConferencesPage />} />
              <Route path="/chats" element={<ChatsPage />} />
              <Route path="/recordings" element={<RecordingsPage />} />
              <Route path="/files" element={<FilesPage />} />
              <Route path="/system" element={<SystemPage />} />
              <Route path="/settings" element={<SettingsPage />} />
              <Route path="/audit" element={<AuditPage />} />
              <Route path="/errors" element={<ErrorsPage />} />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
            <footer className="app-footer">
              <span>MeetSpace / Admin console</span>
              <span>Данные вашей платформы</span>
            </footer>
          </main>
        </div>
      </div>
      {dialog && (
        <Dialog
          key={dialog.kind}
          dialog={dialog}
          close={() => setDialog(null)}
        />
      )}
      {notification && (
        <div className="toast" role="status">
          {notification}
          <button
            aria-label="Закрыть уведомление"
            onClick={() => setNotification("")}
          >
            <X />
          </button>
        </div>
      )}
    </AdminContext.Provider>
  );
}

function Brand() {
  return (
    <Link className="brand" to="/" aria-label="MeetSpace, главная">
      <span className="logo">
        <Video />
      </span>
      <span>
        meetspace<span className="brand-dot">.</span>
      </span>
    </Link>
  );
}

function Login({
  onLogin,
  notice,
  theme,
  toggleTheme,
}: {
  onLogin: (value: AdminSession) => void;
  notice: string;
  theme: string;
  toggleTheme: () => void;
}) {
  const [username, setUsername] = useState(""),
    [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [help, setHelp] = useState(false);
  const pending = useRef<AbortController | null>(null);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      pending.current?.abort();
    };
  }, []);
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (pending.current) return;
    setBusy(true);
    setError("");
    const controller = new AbortController();
    pending.current = controller;
    try {
      const value = await api<AdminSession>("/auth/login", {
        method: "POST",
        body: { username: username.trim(), password },
        signal: controller.signal,
      });
      if (!mounted.current || controller.signal.aborted) return;
      setPassword("");
      onLogin(value);
    } catch (failure) {
      if (!mounted.current || controller.signal.aborted) return;
      setPassword("");
      setError(
        failure instanceof APIError && failure.status === 401
          ? "Неверный логин или пароль."
          : failure instanceof Error
            ? failure.message
            : "Не удалось войти. Повторите попытку.",
      );
    } finally {
      pending.current = null;
      if (mounted.current) setBusy(false);
    }
  }
  return (
    <div className="login-page">
      <div className="login-story">
        <Brand />
        <div className="login-hero">
          <div className="eyebrow">ADMIN CONSOLE</div>
          <h1>
            Ваше пространство.
            <br />
            Под контролем.
          </h1>
          <p>Люди, встречи и инфраструктура — в едином центре управления.</p>
          <div className="orbit" aria-hidden="true">
            <span className="logo">
              <Video />
            </span>
            <div className="orbit-label">
              <ShieldCheck />
              Защищённый доступ
            </div>
          </div>
        </div>
        <p className="login-copyright">MeetSpace · Self-hosted collaboration</p>
      </div>
      <div className="login-main">
        <button
          className="icon-button login-theme"
          onClick={toggleTheme}
          aria-label={theme === "dark" ? "Светлая тема" : "Тёмная тема"}
        >
          {theme === "dark" ? <Sun /> : <Moon />}
        </button>
        <div className="login-form">
          <div className="logo login-shield">
            <ShieldCheck />
          </div>
          <h1>Вход в админку</h1>
          <p>Используйте учётную запись администратора.</p>
          <form onSubmit={submit}>
            {notice && (
              <p className="form-error" role="status">
                {notice}
              </p>
            )}
            <div className="form-error" role="alert" hidden={!error}>
              {error}
            </div>
            <label className="field">
              Логин администратора
              <input
                name="username"
                autoComplete="username"
                autoCapitalize="none"
                spellCheck={false}
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                placeholder="admin или email"
                required
                maxLength={40}
                disabled={busy}
              />
            </label>
            <label className="field">
              Пароль
              <input
                type="password"
                name="password"
                autoComplete="current-password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                placeholder="Введите пароль"
                required
                maxLength={512}
                disabled={busy}
              />
            </label>
            <button
              type="submit"
              className="button primary login-submit"
              disabled={busy}
            >
              {busy ? (
                "Выполняется вход…"
              ) : (
                <>
                  Войти <ArrowRight />
                </>
              )}
            </button>
          </form>
          <p className="login-note">
            Учётная запись пользователя платформы
            <br />
            не подходит для входа в эту панель.
          </p>
          <button
            className="text-link login-recovery"
            onClick={() => setHelp(true)}
          >
            Нет доступа к аккаунту?
          </button>
        </div>
      </div>
      {help && (
        <Modal
          title="Восстановление доступа"
          close={() => setHelp(false)}
          footer={
            <button className="button" onClick={() => setHelp(false)}>
              Понятно
            </button>
          }
        >
          <p>
            Обратитесь к администратору сервера MeetSpace. Восстановление
            первоначального доступа выполняется на сервере.
          </p>
          <p>
            Другой действующий администратор может сбросить ваш пароль в разделе
            «Администраторы».
          </p>
        </Modal>
      )}
    </div>
  );
}

function useSave(close: () => void) {
  const { notify } = useAdmin();
  const queryClient = useQueryClient();
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const pending = useRef<AbortController | null>(null);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      pending.current?.abort();
    };
  }, []);
  async function run(
    callback: (signal: AbortSignal) => Promise<void>,
    message: string,
  ) {
    if (pending.current) return;
    const controller = new AbortController();
    pending.current = controller;
    setBusy(true);
    setError("");
    try {
      await callback(controller.signal);
      if (!mounted.current || controller.signal.aborted) return;
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["admin"] }),
        queryClient.invalidateQueries({ queryKey: ["admin-session"] }),
      ]);
      if (!mounted.current || controller.signal.aborted) return;
      close();
      notify(message);
    } catch (failure) {
      if (!mounted.current || controller.signal.aborted) return;
      setError(
        failure instanceof Error
          ? failure.message
          : "Не удалось сохранить изменения.",
      );
    } finally {
      pending.current = null;
      if (mounted.current) setBusy(false);
    }
  }
  return { busy, error, setError, run };
}

function Dialog({
  dialog,
  close,
}: {
  dialog: DialogRequest;
  close: () => void;
}) {
  const { session, openDialog, logout } = useAdmin();
  const saving = useSave(close);
  if (dialog.kind === "administrator")
    return (
      <AdministratorForm administrator={dialog.administrator} close={close} />
    );
  if (dialog.kind === "password")
    return <PasswordForm administrator={dialog.administrator} close={close} />;
  if (dialog.kind === "confirm")
    return (
      <Modal
        title={dialog.title}
        close={close}
        busy={saving.busy}
        footer={
          <>
            <button className="button" disabled={saving.busy} onClick={close}>
              Отмена
            </button>
            <button
              className={`button ${dialog.danger ? "danger" : "primary"}`}
              disabled={saving.busy}
              onClick={() => void saving.run(dialog.run, dialog.success)}
            >
              {saving.busy ? "Выполняется…" : dialog.label}
            </button>
          </>
        }
      >
        <div className="form-error" role="alert" hidden={!saving.error}>
          {saving.error}
        </div>
        {dialog.body}
      </Modal>
    );
  if (dialog.kind === "profile")
    return (
      <Modal
        title="Мой профиль"
        close={close}
        busy={saving.busy}
        footer={
          <>
            <button
              className="button"
              disabled={saving.busy}
              onClick={() =>
                openDialog({
                  kind: "administrator",
                  administrator: session.administrator,
                })
              }
            >
              Редактировать
            </button>
            <button
              className="button"
              disabled={saving.busy}
              onClick={() =>
                openDialog({
                  kind: "password",
                  administrator: session.administrator,
                })
              }
            >
              Сменить пароль
            </button>
            <button
              className="button"
              disabled={saving.busy}
              onClick={() =>
                void saving.run(logout, "Вы вышли из административной панели")
              }
            >
              <LogOut />
              Выйти
            </button>
          </>
        }
      >
        <div className="form-error" role="alert" hidden={!saving.error}>
          {saving.error}
        </div>
        <Person
          name={session.administrator.name || session.administrator.username}
          description={session.administrator.username}
        />
        <div className="modal-details">
          <Detail
            label="Email"
            value={session.administrator.email || "Не указан"}
          />
          <Detail label="Уровень доступа" value="Администратор" />
          <Detail
            label="Последний вход"
            value={date(session.administrator.lastLoginAt)}
          />
        </div>
      </Modal>
    );
  let title = "Подробности",
    content: ReactNode;
  if (dialog.kind === "conference") {
    const item = dialog.conference;
    title = item.title || "Конференция";
    content = (
      <>
        <StatusBadge status={item.status} />
        <div className="modal-details">
          <Detail label="ID" value={<span className="mono">{item.id}</span>} />
          <Detail label="Организатор" value={item.ownerName || "—"} />
          <Detail label="Создана" value={date(item.createdAt)} />
          <Detail label="Начало" value={date(item.startedAt)} />
          <Detail label="Завершение" value={date(item.endedAt)} />
          <Detail
            label="Участники"
            value={item.participantCount ?? "Нет данных"}
          />
          <Detail
            label="Запись"
            value={<StatusBadge status={item.recordingStatus} />}
          />
        </div>
        <p className="muted">Конференция доступна только для просмотра.</p>
      </>
    );
  } else if (dialog.kind === "recording") {
    const item = dialog.recording;
    title = "Информация о записи";
    content = (
      <>
        <h3>{item.title || item.conferenceTitle || "Без названия"}</h3>
        <div className="modal-details">
          <Detail label="ID" value={<span className="mono">{item.id}</span>} />
          <Detail
            label="Конференция"
            value={item.conferenceTitle || item.conferenceId || "—"}
          />
          <Detail label="Статус" value={<StatusBadge status={item.status} />} />
          <Detail label="Создана" value={date(item.createdAt)} />
          <Detail label="Длительность" value={duration(item.durationSeconds)} />
          <Detail label="Размер" value={bytes(item.sizeBytes)} />
        </div>
      </>
    );
  } else {
    const event = dialog.event;
    title = "Событие аудита";
    content = (
      <>
        <Badge>Только чтение</Badge>
        <div className="modal-details">
          <Detail label="Время" value={date(event.createdAt)} />
          <Detail
            label="Администратор"
            value={event.administratorUsername || "—"}
          />
          <Detail
            label="Действие"
            value={auditLabels[event.action] || event.action}
          />
          <Detail
            label="Объект"
            value={`${event.objectType}${event.objectId ? ` · ${event.objectId}` : ""}`}
          />
          <Detail
            label="IP-адрес"
            value={<span className="mono">{event.ip || "—"}</span>}
          />
          <Detail label="User agent" value={event.userAgent || "—"} />
        </div>
        {event.metadata && Object.keys(event.metadata).length > 0 && (
          <>
            <h3>Подробности</h3>
            <pre className="metadata">
              {JSON.stringify(event.metadata, null, 2)}
            </pre>
          </>
        )}
      </>
    );
  }
  return (
    <Modal
      title={title}
      close={close}
      footer={
        <button className="button" onClick={close}>
          Закрыть
        </button>
      }
    >
      {content}
    </Modal>
  );
}

function AdministratorForm({
  administrator,
  close,
}: {
  administrator?: Administrator;
  close: () => void;
}) {
  const [username, setUsername] = useState(administrator?.username || ""),
    [name, setName] = useState(administrator?.name || ""),
    [email, setEmail] = useState(administrator?.email || "");
  const [password, setPassword] = useState(""),
    [repeat, setRepeat] = useState("");
  const saving = useSave(close);
  async function submit(event: FormEvent) {
    event.preventDefault();
    const error = validateAdministrator(
      username,
      password,
      repeat,
      !administrator,
    );
    if (error) {
      saving.setError(error);
      return;
    }
    if (!name.trim()) {
      saving.setError("Укажите имя администратора.");
      return;
    }
    await saving.run(
      async (signal) => {
        await api(
          administrator
            ? `/administrators/${encodeURIComponent(administrator.id)}`
            : "/administrators",
          {
            method: administrator ? "PATCH" : "POST",
            signal,
            body: {
              username: username.trim().toLowerCase(),
              name: name.trim(),
              email: administrator ? email.trim() : email.trim() || null,
              ...(!administrator ? { password } : {}),
            },
          },
        );
        setPassword("");
        setRepeat("");
      },
      administrator ? "Администратор обновлён" : "Администратор создан",
    );
  }
  return (
    <Modal
      title={
        administrator ? "Редактировать администратора" : "Новый администратор"
      }
      close={close}
      busy={saving.busy}
      footer={
        <>
          <button className="button" disabled={saving.busy} onClick={close}>
            Отмена
          </button>
          <button
            className="button primary"
            form="administrator-form"
            type="submit"
            disabled={saving.busy}
          >
            {saving.busy
              ? "Сохраняем…"
              : administrator
                ? "Сохранить"
                : "Создать"}
          </button>
        </>
      }
    >
      <form id="administrator-form" onSubmit={submit}>
        <div className="form-error" role="alert" hidden={!saving.error}>
          {saving.error}
        </div>
        <label className="field">
          Логин
          <input
            name="username"
            autoComplete="off"
            autoCapitalize="none"
            spellCheck={false}
            value={username}
            onChange={(event) => setUsername(event.target.value)}
            required
            minLength={3}
            maxLength={40}
            disabled={saving.busy}
          />
          <span className="field-help">
            Используется для входа в административную панель
          </span>
        </label>
        <label className="field">
          Имя
          <input
            name="name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            required
            maxLength={160}
            disabled={saving.busy}
          />
        </label>
        <label className="field">
          Email <span className="muted">необязательно</span>
          <input
            name="email"
            type="email"
            autoComplete="off"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            maxLength={254}
            disabled={saving.busy}
          />
        </label>
        {!administrator && (
          <>
            <label className="field">
              Пароль
              <input
                type="password"
                name="password"
                autoComplete="new-password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                required
                maxLength={512}
                disabled={saving.busy}
              />
              <span className="field-help">8–128 символов</span>
            </label>
            <label className="field">
              Повторите пароль
              <input
                type="password"
                name="passwordConfirmation"
                autoComplete="new-password"
                value={repeat}
                onChange={(event) => setRepeat(event.target.value)}
                required
                maxLength={512}
                disabled={saving.busy}
              />
            </label>
          </>
        )}
      </form>
    </Modal>
  );
}

function PasswordForm({
  administrator,
  close,
}: {
  administrator: Administrator;
  close: () => void;
}) {
  const { session, logout } = useAdmin();
  const own = administrator.id === session.administrator.id;
  const [current, setCurrent] = useState(""),
    [password, setPassword] = useState(""),
    [repeat, setRepeat] = useState("");
  const saving = useSave(close);
  async function submit(event: FormEvent) {
    event.preventDefault();
    const error = validatePassword(password);
    if (error) {
      saving.setError(error);
      return;
    }
    if (password !== repeat) {
      saving.setError("Пароли не совпадают.");
      return;
    }
    await saving.run(
      async (signal) => {
        await api(
          own
            ? "/auth/password"
            : `/administrators/${encodeURIComponent(administrator.id)}/password`,
          {
            method: "POST",
            signal,
            body: { password, ...(own ? { currentPassword: current } : {}) },
          },
        );
        setCurrent("");
        setPassword("");
        setRepeat("");
        if (own) {
          try {
            await logout(signal);
          } catch (failure) {
            if (!(failure instanceof APIError && failure.status === 401))
              throw failure;
          }
        }
      },
      own ? "Пароль изменён. Войдите снова." : "Пароль администратора изменён",
    );
  }
  return (
    <Modal
      title={own ? "Сменить пароль" : "Сбросить пароль"}
      close={close}
      busy={saving.busy}
      footer={
        <>
          <button className="button" onClick={close} disabled={saving.busy}>
            Отмена
          </button>
          <button
            className="button primary"
            type="submit"
            form="password-form"
            disabled={saving.busy}
          >
            {saving.busy ? "Сохраняем…" : "Сохранить пароль"}
          </button>
        </>
      }
    >
      <p>
        Для <strong>{administrator.username}</strong>. Все сессии этой учётной
        записи будут отозваны. Потребуется повторный вход.
      </p>
      <form id="password-form" onSubmit={submit}>
        <div className="form-error" role="alert" hidden={!saving.error}>
          {saving.error}
        </div>
        {own && (
          <label className="field">
            Текущий пароль
            <input
              type="password"
              autoComplete="current-password"
              required
              value={current}
              onChange={(event) => setCurrent(event.target.value)}
              disabled={saving.busy}
            />
          </label>
        )}
        <label className="field">
          Новый пароль
          <input
            type="password"
            autoComplete="new-password"
            required
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            maxLength={512}
            disabled={saving.busy}
          />
          <span className="field-help">8–128 символов</span>
        </label>
        <label className="field">
          Повторите новый пароль
          <input
            type="password"
            autoComplete="new-password"
            required
            value={repeat}
            onChange={(event) => setRepeat(event.target.value)}
            maxLength={512}
            disabled={saving.busy}
          />
        </label>
      </form>
    </Modal>
  );
}
