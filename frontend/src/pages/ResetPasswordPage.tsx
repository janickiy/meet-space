import { AuthPolicyPage, useAuthPolicy } from "../components/AuthPolicy";
import { useEffect, useRef, useState } from "react";
import type { SubmitEvent } from "react";
import { Link, Navigate, useLocation, useNavigate } from "react-router";
import { api, errorMessage } from "../api";
import {
  Brand,
  ErrorNotice,
  PasswordInput,
  SuccessMark,
} from "../components/ui";
import { passwordLength, utf8Bytes } from "../utils";
import "./auth-pages.css";

/** Одноразовая ссылка остаётся только в памяти и не попадает в URL запросов. */
export function ResetPasswordPage() {
  const policy = useAuthPolicy();
  const location = useLocation();
  const navigate = useNavigate();
  const [token, setToken] = useState(
    () => new URLSearchParams(location.hash.slice(1)).get("token") || "",
  );
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [complete, setComplete] = useState(false);
  const [sent, setSent] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const pending = useRef(false);
  const mounted = useRef(true);
  const controller = useRef<AbortController | null>(null);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      controller.current?.abort();
    };
  }, []);
  useEffect(() => {
    if (location.hash) {
      setToken(new URLSearchParams(location.hash.slice(1)).get("token") || "");
      navigate(location.pathname, { replace: true });
    }
  }, [location.hash, location.pathname, navigate]);

  async function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending.current || !policy.recoveryAllowed) return;
    setError("");
    if (token) {
      const size = passwordLength(password);
      if (size < 8 || size > 128) {
        setError("Пароль должен содержать от 8 до 128 символов.");
        return;
      }
      if (password !== confirmation) {
        setError("Пароли не совпадают.");
        return;
      }
    } else if (
      !/^[^\s@]+@[^\s@]+$/.test(email.trim()) ||
      utf8Bytes(email.trim()) > 254
    ) {
      setError("Введите корректный email.");
      return;
    }
    pending.current = true;
    setBusy(true);
    const request = new AbortController();
    controller.current = request;
    try {
      if (token) await api.resetPassword(token, password, request.signal);
      else await api.recoverPassword(email.trim(), request.signal);
      if (mounted.current && !request.signal.aborted) {
        if (token) {
          setComplete(true);
          setToken("");
          setPassword("");
          setConfirmation("");
        } else setSent(true);
      }
    } catch (cause) {
      if (mounted.current && !request.signal.aborted)
        setError(errorMessage(cause));
    } finally {
      pending.current = false;
      if (mounted.current) setBusy(false);
    }
  }

  if (!policy.ready) return <AuthPolicyPage policy={policy} />;
  if (!policy.recoveryAllowed) return <Navigate to="/login" replace />;
  return (
    <div className="auth-page auth-business-page auth-business-success">
      <header className="auth-business-header">
        <Brand />
        <Link className="auth-back" to="/login">
          Войти
        </Link>
      </header>
      <main className="auth-success-layout">
        <section
          className="auth-card registration-success"
          aria-labelledby="reset-title"
        >
          {complete && <SuccessMark />}
          <h1 id="reset-title">
            {complete
              ? "Пароль изменён"
              : token
                ? "Новый пароль"
                : "Восстановление пароля"}
          </h1>
          <p>
            {complete
              ? "Войдите с новым паролем. Предыдущие сеансы вашего аккаунта завершены."
              : token
                ? "Задайте новый пароль для своего аккаунта. Ссылка действует 1 час и используется один раз."
                : "Укажите email своего аккаунта. Мы отправим ссылку для восстановления пароля."}
          </p>
          <ErrorNotice>{error || null}</ErrorNotice>
          {sent && (
            <p role="status">
              Если аккаунт существует, на указанный email придёт ссылка для
              восстановления пароля. Ссылка действует 1 час. Повторная отправка
              доступна не чаще раза в минуту.
            </p>
          )}
          {!complete && (
            <form onSubmit={submit} noValidate>
              {token ? (
                <>
                  <label className="field" htmlFor="reset-password">
                    Новый пароль
                    <PasswordInput
                      id="reset-password"
                      autoComplete="new-password"
                      placeholder="От 8 до 128 символов"
                      required
                      value={password}
                      onChange={(event) => setPassword(event.target.value)}
                      disabled={busy}
                    />
                  </label>
                  <label className="field" htmlFor="reset-confirmation">
                    Повторите пароль
                    <PasswordInput
                      id="reset-confirmation"
                      autoComplete="new-password"
                      required
                      value={confirmation}
                      onChange={(event) => setConfirmation(event.target.value)}
                      disabled={busy}
                    />
                  </label>
                </>
              ) : (
                <label className="field" htmlFor="reset-email">
                  Email
                  <input
                    id="reset-email"
                    type="email"
                    autoComplete="email"
                    value={email}
                    onChange={(event) => {
                      setEmail(event.target.value);
                      setSent(false);
                    }}
                    disabled={busy}
                    required
                  />
                </label>
              )}
              <button
                type="submit"
                className="button button-primary full-width"
                disabled={busy}
              >
                {busy
                  ? "Подождите…"
                  : token
                    ? "Сохранить новый пароль"
                    : "Отправить ссылку"}
              </button>
            </form>
          )}
          {token && !complete && (
            <p>
              <button
                type="button"
                className="text-button"
                disabled={busy}
                onClick={() => {
                  setToken("");
                  setPassword("");
                  setConfirmation("");
                  setError("");
                }}
              >
                Запросить новую ссылку
              </button>
            </p>
          )}
          {complete && (
            <Link
              reloadDocument
              className="button button-primary full-width"
              to="/login"
            >
              Войти с новым паролем
            </Link>
          )}
        </section>
      </main>
    </div>
  );
}
