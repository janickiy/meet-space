import type { AdminSession } from "./types.ts";

const base = "/api/v1/admin";
let csrfToken: string | null = null;
let administratorID: string | null = null;
let sessionGeneration = 0;
const pendingRequests = new Set<AbortController>();
let onUnauthorized: (() => void) | undefined;

export class APIError extends Error {
  readonly status: number;
  readonly code: string;
  constructor(status: number, message: string, code = "") {
    super(message);
    this.status = status;
    this.code = code;
  }
}

export function configureSession(
  session: AdminSession | null,
  callback?: () => void,
) {
  const nextToken = session?.csrfToken || null;
  const nextID = session?.administrator.id || null;
  if (csrfToken !== nextToken || administratorID !== nextID) {
    sessionGeneration++;
    for (const controller of pendingRequests) controller.abort();
    pendingRequests.clear();
  }
  csrfToken = nextToken;
  administratorID = nextID;
  onUnauthorized = callback;
}

export function hasAdministrativeSession(): boolean {
  return administratorID !== null && csrfToken !== null;
}

export function queryString(
  values: Record<string, string | number | undefined>,
): string {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(values)) {
    if (value !== undefined && value !== "" && value !== "all")
      query.set(key, String(value));
  }
  const encoded = query.toString();
  return encoded ? `?${encoded}` : "";
}

export async function api<T>(
  path: string,
  options: { method?: string; body?: unknown; signal?: AbortSignal } = {},
): Promise<T> {
  const pathname = path.split(/[?#]/, 1)[0];
  if (
    !path.startsWith("/") ||
    path.startsWith("//") ||
    path.includes("://") ||
    pathname.includes("\\") ||
    /%(2f|5c)/i.test(pathname) ||
    decodeURIComponent(pathname)
      .split("/")
      .some((part) => part === "." || part === "..")
  )
    throw new Error("Invalid API path");
  const generation = sessionGeneration;
  const controller = new AbortController();
  const forwardAbort = () => controller.abort(options.signal?.reason);
  if (options.signal?.aborted) forwardAbort();
  else options.signal?.addEventListener("abort", forwardAbort, { once: true });
  pendingRequests.add(controller);
  const timeout = setTimeout(
    () =>
      controller.abort(new DOMException("Request timed out", "TimeoutError")),
    30_000,
  );
  const assertCurrent = () => {
    if (generation !== sessionGeneration || controller.signal.aborted) {
      if (controller.signal.reason?.name === "TimeoutError")
        throw new APIError(0, "Сервер не ответил вовремя. Повторите попытку.");
      throw new DOMException(
        "Request belongs to an inactive session",
        "AbortError",
      );
    }
  };
  const method = (options.method || "GET").toUpperCase();
  const headers: Record<string, string> = { Accept: "application/json" };
  const mutation = !["GET", "HEAD", "OPTIONS"].includes(method);
  if (options.body !== undefined || mutation)
    headers["Content-Type"] = "application/json";
  if (csrfToken && !["GET", "HEAD", "OPTIONS"].includes(method))
    headers["X-CSRF-Token"] = csrfToken;
  try {
    let response: Response;
    try {
      assertCurrent();
      response = await fetch(base + path, {
        method,
        credentials: "same-origin",
        headers,
        body:
          options.body !== undefined
            ? JSON.stringify(options.body)
            : mutation
              ? "{}"
              : undefined,
        signal: controller.signal,
        cache: "no-store",
        redirect: "error",
      });
    } catch (error) {
      assertCurrent();
      if (error instanceof Error && error.name === "AbortError") throw error;
      throw new APIError(
        0,
        "Не удалось связаться с сервером. Проверьте соединение и повторите попытку.",
      );
    }
    assertCurrent();
    let data = null;
    if (response.status !== 204) {
      try {
        data = await response.json();
      } catch (error) {
        assertCurrent();
        if (error instanceof Error && error.name === "AbortError") throw error;
        if (response.ok)
          throw new APIError(
            502,
            "Сервер вернул некорректный ответ. Повторите попытку.",
          );
      }
    }
    assertCurrent();
    if (!response.ok) {
      if (response.status === 401 && path !== "/auth/login") onUnauthorized?.();
      const detail = data?.error;
      const code =
        typeof detail === "object"
          ? detail?.code
          : data?.code || (typeof detail === "string" ? detail : "");
      const message =
        typeof detail === "object" ? detail?.message : data?.message;
      throw new APIError(
        response.status,
        message || errorMessage(response.status),
        code || "",
      );
    }
    return data as T;
  } finally {
    clearTimeout(timeout);
    options.signal?.removeEventListener("abort", forwardAbort);
    pendingRequests.delete(controller);
  }
}

export function errorMessage(status: number): string {
  if (status === 401) return "Сессия завершена. Войдите снова.";
  if (status === 403) return "Недостаточно прав для выполнения действия.";
  if (status === 404) return "Запрошенный объект не найден.";
  if (status === 409)
    return "Действие недоступно: состояние объекта изменилось. Обновите страницу.";
  if (status === 429)
    return "Слишком много попыток. Подождите и попробуйте снова.";
  if (status >= 500)
    return "Сервер временно недоступен. Повторите попытку позже.";
  return "Не удалось выполнить действие. Проверьте введённые данные.";
}
