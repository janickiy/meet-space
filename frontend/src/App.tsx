import { lazy, startTransition, Suspense } from "react";
import {
  Navigate,
  Outlet,
  Route,
  Routes,
  useLocation,
  useNavigate,
} from "react-router";
import { useAuth } from "./auth";
import { Brand, Button, ErrorNotice, Loading } from "./components/ui";
import { Layout } from "./components/Layout";
import { Landing } from "./pages/Landing";
import { AuthPage, RegistrationSuccess } from "./pages/AuthPages";
import { Link } from "react-router";
import { CalendarCallback } from "./components/IntegrationsSettings";
import { AccountSettingsModal } from "./components/AccountSettingsModal";
import { PAST_CONFERENCES_PATH } from "./conferenceNavigation";

const PersonalPage = lazy(() =>
  import("./pages/PersonalPage").then((module) => ({
    default: module.PersonalPage,
  })),
);

const FoldersPage = lazy(() =>
  import("./pages/FoldersPage").then((module) => ({
    default: module.FoldersPage,
  })),
);

const Dashboard = lazy(() =>
  import("./pages/Dashboard").then((module) => ({ default: module.Dashboard })),
);
const CalendarPage = lazy(() =>
  import("./pages/CalendarPage").then((module) => ({
    default: module.CalendarPage,
  })),
);
const AnalyticsPage = lazy(() =>
  import("./pages/AnalyticsPage").then((module) => ({
    default: module.AnalyticsPage,
  })),
);
const ConferencePage = lazy(() =>
  import("./pages/ConferencePage").then((module) => ({
    default: module.ConferencePage,
  })),
);
const InvitePage = lazy(() =>
  import("./pages/InvitePage").then((module) => ({
    default: module.InvitePage,
  })),
);
const RecordingsPage = lazy(() =>
  import("./pages/RecordingsPage").then((module) => ({
    default: module.RecordingsPage,
  })),
);
const RecordingDetailPage = lazy(() =>
  import("./pages/RecordingDetailPage").then((module) => ({
    default: module.RecordingDetailPage,
  })),
);
const NotificationsPage = lazy(() =>
  import("./pages/NotificationsPage").then((module) => ({
    default: module.NotificationsPage,
  })),
);
const AdminPage = lazy(() =>
  import("./pages/AdminPage").then((module) => ({ default: module.AdminPage })),
);
const PreJoinPage = lazy(() =>
  import("./pages/PreJoinPage").then((module) => ({
    default: module.PreJoinPage,
  })),
);

/** Поддерживает прямые ссылки на настройки: за диалогом находится главная страница.
 * @return Диалог аккаунта; при закрытии прямой адрес заменяется адресом кабинета.
 */
function SettingsRoute() {
  const navigate = useNavigate();
  return (
    <>
      <Dashboard />
      <AccountSettingsModal
        onClose={() => navigate("/", { replace: true })}
        returnFocus={() => document.getElementById("workspace-main")}
      />
    </>
  );
}

/**
 * Protected выбирает публичную главную либо кабинет после восстановления сессии.
 * Остальные защищённые страницы без авторизации перенаправляются на вход.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
function Protected() {
  const auth = useAuth();
  const location = useLocation();
  const navigate = useNavigate();
  if (auth.loading)
    return (
      <div className="page-center">
        <Loading />
      </div>
    );
  if (auth.startupError)
    return (
      <div className="page-center">
        <div className="content-card">
          <h1>Не удалось проверить сессию</h1>
          <ErrorNotice>
            Сервер недоступен. Ваш токен не удалён — попробуйте снова.
          </ErrorNotice>
          <Button onClick={auth.retry}>Попробовать снова</Button>
          <Button
            variant="secondary"
            onClick={() => {
              startTransition(() => {
                void auth.logout().catch(() => {});
                navigate("/", { replace: true });
              });
            }}
          >
            Выйти
          </Button>
        </div>
      </div>
    );
  if (!auth.user)
    return location.pathname === "/" ? (
      <Landing />
    ) : (
      <Navigate
        to={`/login?next=${encodeURIComponent(location.pathname + location.search)}`}
        replace
      />
    );
  if (
    auth.user.guestConferenceId &&
    !["/conferences/", "/meetings/"].some(
      (prefix) =>
        location.pathname === prefix + auth.user!.guestConferenceId ||
        location.pathname === prefix + auth.user!.guestConferenceId + "/join",
    )
  )
    return (
      <Navigate to={`/conferences/${auth.user.guestConferenceId}`} replace />
    );
  return <Outlet />;
}

/** Сохраняет параметры старых ссылок на кабинет, заменяя адрес на общую главную. */
function HomeRedirect() {
  const { search, hash } = useLocation();
  return <Navigate to={{ pathname: "/", search, hash }} replace />;
}
/**
 * NotFound показывает страницу неизвестного маршрута с переходом к приложению.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
function NotFound() {
  return (
    <div className="page-center">
      <div className="content-card not-found">
        <Brand />
        <span className="not-found-number">404</span>
        <h1>Кажется, мы разминулись</h1>
        <p>Такой страницы не существует.</p>
        <Link className="button button-primary" to="/">
          На главную
        </Link>
      </div>
    </div>
  );
}
/**
 * App собирает публичные и защищённые маршруты Meet и провайдеры приложения.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function App() {
  return (
    <Routes>
      <Route path="/app" element={<HomeRedirect />} />
      <Route path="/login" element={<AuthPage />} />
      <Route path="/register" element={<AuthPage register />} />
      <Route path="/register/success" element={<RegistrationSuccess />} />
      <Route
        path="/i/:code"
        element={
          <Suspense
            fallback={
              <div className="page-center">
                <Loading />
              </div>
            }
          >
            <InvitePage />
          </Suspense>
        }
      />
      <Route
        path="/app/settings/calendar/:provider/callback"
        element={<CalendarCallback />}
      />
      <Route element={<Protected />}>
        <Route element={<Layout />}>
          <Route index element={<Dashboard />} />
          <Route path="/folders" element={<FoldersPage />} />
          <Route path="/folders/:id" element={<FoldersPage />} />
          <Route path="/personal" element={<PersonalPage />} />
          <Route path="/personal/:id" element={<PersonalPage />} />
          <Route path="/calendar" element={<CalendarPage />} />
          <Route path="/analytics" element={<AnalyticsPage />} />
          <Route path="/meetings" element={<Dashboard all />} />
          <Route path="/meetings/new" element={<Dashboard create />} />
          <Route path="/meetings/:id" element={<ConferencePage />} />
          <Route path="/meetings/:id/join" element={<PreJoinPage />} />
          <Route path="/conferences" element={<Dashboard all />} />
          <Route path="/conferences/new" element={<Dashboard create />} />
          <Route path="/conferences/:id" element={<ConferencePage />} />
          <Route path="/conferences/:id/join" element={<PreJoinPage />} />
          <Route path="/app/settings" element={<SettingsRoute />} />
          <Route path="/settings" element={<SettingsRoute />} />
          <Route path="/app/recordings" element={<RecordingsPage />} />
          <Route path="/recordings" element={<RecordingsPage />} />
          <Route path="/recordings/:id" element={<RecordingDetailPage />} />
          <Route
            path="/history/*"
            element={<Navigate to={PAST_CONFERENCES_PATH} replace />}
          />
          <Route path="/notifications" element={<NotificationsPage />} />
          <Route path="/app/search" element={<Navigate to="/" replace />} />
          <Route path="/search" element={<Navigate to="/" replace />} />
          <Route path="/admin" element={<AdminPage />} />
        </Route>
      </Route>
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}
