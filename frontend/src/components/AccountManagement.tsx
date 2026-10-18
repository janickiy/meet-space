import { useState } from "react";
import type { FormEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { Check, Plus, Trash2, UserRound } from "lucide-react";
import { useNavigate } from "react-router";
import { api } from "../api";
import { useAuth } from "../auth";
import { initials, passwordLength } from "../utils";
import { Button, ErrorNotice, Loading } from "./ui";
import "./account-management.css";

export function AccountManagement() {
  const auth = useAuth();
  const navigate = useNavigate();
  const accounts = useQuery({
    queryKey: ["browser-accounts", auth.user?.id],
    queryFn: ({ signal }) => api.accounts(signal),
    staleTime: 0,
  });
  const [mode, setMode] = useState<"login" | "register" | "delete" | null>(
    null,
  );
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [validation, setValidation] = useState("");
  const items = accounts.data?.items || [];
  // Учитывайте текущую учетную запись, даже если происходит обновление уже существующего
  // cookie-файла.
  const visible =
    auth.user && !items.some((item) => item.id === auth.user!.id)
      ? [auth.user, ...items]
      : items;
  function choose(next: typeof mode) {
    setMode(next);
    setPassword("");
    setError(undefined);
    setValidation("");
    setConfirmed(false);
  }
  async function changeAccount(id: string) {
    setBusy(true);
    setError(undefined);
    try {
      await auth.switchAccount(id);
      navigate("/app/settings#accounts", { replace: true });
    } catch (error) {
      setError(error);
    } finally {
      setBusy(false);
    }
  }
  async function submit(event: FormEvent) {
    event.preventDefault();
    setError(undefined);
    setValidation("");
    if (
      mode === "register" &&
      (passwordLength(password) < 8 || passwordLength(password) > 128)
    ) {
      setValidation("Пароль должен содержать от 8 до 128 символов.");
      return;
    }
    if (mode === "delete" && !confirmed) return;
    setBusy(true);
    try {
      if (mode === "delete") await auth.deleteAccount(password);
      else {
        if (mode === "register") await api.register(email, password, name);
        await auth.login(email, password);
      }
      setPassword("");
      navigate("/app/settings#accounts", { replace: true });
    } catch (error) {
      setError(error);
      setPassword("");
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="account-management" aria-labelledby="accounts-heading">
      <h2 id="accounts-heading">Аккаунты</h2>
      <p className="field-hint">
        Подключите до 5 аккаунтов и переключайтесь между ними.
      </p>
      {accounts.isPending && <Loading />}
      <ErrorNotice error={accounts.error} />
      {accounts.isError && (
        <Button variant="secondary" onClick={() => void accounts.refetch()}>
          Повторить
        </Button>
      )}
      <ul className="connected-accounts">
        {visible.map((item) => (
          <li key={item.id}>
            <span className="connected-account-avatar" aria-hidden="true">
              {initials(item.displayName || item.email)}
            </span>
            <span className="connected-account-name">
              <strong>{item.displayName || item.email}</strong>
              <small>{item.email}</small>
            </span>
            {item.id === auth.user?.id ? (
              <span className="connected-account-active">
                <Check size={16} aria-hidden="true" /> Текущий
              </span>
            ) : (
              <Button
                variant="secondary"
                disabled={busy}
                onClick={() => void changeAccount(item.id)}
              >
                Переключиться
              </Button>
            )}
          </li>
        ))}
      </ul>
      <ErrorNotice error={error} />
      <ErrorNotice>{validation || null}</ErrorNotice>
      {!mode && (
        <div className="account-management-actions">
          <Button
            variant="secondary"
            disabled={busy || visible.length >= 5}
            onClick={() => choose("login")}
          >
            <Plus size={18} aria-hidden="true" /> Добавить аккаунт
          </Button>
          <Button
            variant="danger"
            disabled={busy}
            onClick={() => choose("delete")}
          >
            <Trash2 size={18} aria-hidden="true" /> Удалить текущий аккаунт
          </Button>
        </div>
      )}
      {mode && (
        <form
          className="account-management-form"
          onSubmit={(event) => void submit(event)}
        >
          <h3>
            {mode === "delete"
              ? "Удалить аккаунт с сервера"
              : mode === "register"
                ? "Создать новый аккаунт"
                : "Войти в другой аккаунт"}
          </h3>
          {mode !== "delete" && (
            <>
              <div className="account-add-modes">
                <Button
                  variant="secondary"
                  disabled={busy || mode === "login"}
                  onClick={() => choose("login")}
                >
                  <UserRound size={16} aria-hidden="true" /> Войти
                </Button>
                <Button
                  variant="secondary"
                  disabled={busy || mode === "register"}
                  onClick={() => choose("register")}
                >
                  Создать аккаунт
                </Button>
              </div>
              {mode === "register" && (
                <label className="field">
                  <span>Имя</span>
                  <input
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    maxLength={100}
                    autoComplete="name"
                    disabled={busy}
                  />
                </label>
              )}
              <label className="field">
                <span>Email</span>
                <input
                  type="email"
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  required
                  maxLength={254}
                  autoComplete="username"
                  disabled={busy}
                />
              </label>
            </>
          )}
          {mode === "delete" && (
            <>
              <p>
                Аккаунт <strong>{auth.user?.email}</strong> будет удалён. Данные
                входа и личные настройки будут очищены, все сессии завершены.
                Общая история встреч и переписки останется с именем «Удалённый
                аккаунт».
              </p>
              <p className="field-hint">
                Перед удалением передайте владение группами или удалите их,
                завершите или отмените созданные встречи и выйдите из звонков.
              </p>
            </>
          )}
          <label className="field">
            <span>
              {mode === "delete" ? "Пароль текущего аккаунта" : "Пароль"}
            </span>
            <input
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              required
              maxLength={128}
              autoComplete={
                mode === "register" ? "new-password" : "current-password"
              }
              disabled={busy}
            />
          </label>
          {mode === "delete" && (
            <label className="account-delete-confirm">
              <input
                type="checkbox"
                checked={confirmed}
                onChange={(event) => setConfirmed(event.target.checked)}
                disabled={busy}
              />{" "}
              Я понимаю, что удаление нельзя отменить
            </label>
          )}
          <div className="account-management-actions">
            <Button
              variant={mode === "delete" ? "danger" : "primary"}
              type="submit"
              busy={busy}
              disabled={mode === "delete" && !confirmed}
            >
              {mode === "delete"
                ? "Удалить аккаунт навсегда"
                : mode === "register"
                  ? "Создать и войти"
                  : "Войти и добавить"}
            </Button>
            <Button
              variant="secondary"
              disabled={busy}
              onClick={() => choose(null)}
            >
              Отмена
            </Button>
          </div>
        </form>
      )}
    </section>
  );
}
