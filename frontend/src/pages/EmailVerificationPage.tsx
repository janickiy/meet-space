import { useEffect, useRef, useState } from "react";
import type { SubmitEvent } from "react";
import { Link, useLocation, useNavigate } from "react-router";
import { api, errorMessage } from "../api";
import { Brand, ErrorNotice, SuccessMark } from "../components/ui";
import { utf8Bytes } from "../utils";
import "./auth-pages.css";

export function EmailVerificationPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const [token, setToken] = useState(
    () => new URLSearchParams(location.hash.slice(1)).get("token") || "",
  );
  const [email, setEmail] = useState(() =>
    typeof location.state?.email === "string" ? location.state.email : "",
  );
  const [confirmed, setConfirmed] = useState(false);
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
    // The bearer link belongs in memory, never in server URLs or browser history.
    if (location.hash) {
      setToken(new URLSearchParams(location.hash.slice(1)).get("token") || "");
      navigate(location.pathname, { replace: true, state: location.state });
    }
  }, [location.hash, location.pathname, location.state, navigate]);
  async function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending.current) return;
    if (
      !token &&
      (!/^[^\s@]+@[^\s@]+$/.test(email.trim()) || utf8Bytes(email.trim()) > 254)
    ) {
      setError("Введите корректный email.");
      return;
    }
    pending.current = true;
    setBusy(true);
    setError("");
    controller.current = new AbortController();
    try {
      if (token) await api.verifyEmail(token, controller.current.signal);
      else
        await api.resendVerification(email.trim(), controller.current.signal);
      if (mounted.current) {
        setConfirmed(!!token);
        setSent(!token);
      }
    } catch (cause) {
      if (mounted.current && !controller.current.signal.aborted)
        setError(errorMessage(cause));
    } finally {
      pending.current = false;
      if (mounted.current) setBusy(false);
    }
  }
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
          aria-labelledby="verification-title"
        >
          {confirmed && <SuccessMark />}
          <h1 id="verification-title">
            {confirmed
              ? "Email подтверждён"
              : token
                ? "Подтвердите email"
                : "Проверьте почту"}
          </h1>
          <p>
            {confirmed
              ? "Теперь вы можете войти в свой аккаунт."
              : token
                ? "Нажмите кнопку, чтобы завершить подтверждение регистрации."
                : "Для входа подтвердите email по ссылке из письма. Ссылка действует 24 часа."}
          </p>
          <ErrorNotice>{error || null}</ErrorNotice>
          {sent && (
            <p role="status">
              Если аккаунт ожидает подтверждения, письмо будет отправлено.
              Повторная отправка доступна не чаще раза в минуту.
            </p>
          )}
          {!confirmed && (
            <form onSubmit={submit} noValidate>
              {!token && (
                <label className="field" htmlFor="verification-email">
                  Email
                  <input
                    id="verification-email"
                    type="email"
                    autoComplete="email"
                    value={email}
                    onChange={(event) => setEmail(event.target.value)}
                    disabled={busy}
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
                    ? "Подтвердить email"
                    : "Отправить письмо повторно"}
              </button>
            </form>
          )}
          {token && !confirmed && (
            <p>
              <Link
                to="/verify-email"
                onClick={() => {
                  setToken("");
                  setError("");
                }}
              >
                Запросить новую ссылку
              </Link>
            </p>
          )}
          {confirmed && (
            <Link className="button button-primary full-width" to="/login">
              Войти в аккаунт
            </Link>
          )}
        </section>
      </main>
    </div>
  );
}
