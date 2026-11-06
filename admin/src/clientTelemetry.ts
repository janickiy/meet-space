/** Коды описывают источник ошибки и никогда не содержат пользовательский текст. */
export type ClientErrorCode =
  "uncaught_error" | "unhandled_rejection" | "react_render_error";
export type BrowserFamily =
  "chromium" | "firefox" | "safari" | "edge" | "other";

/** Разрешённые поля сообщения; исходные ошибки и HTTP-запросы не сериализуются. */
export interface ClientErrorEvent {
  version: string;
  route: string;
  code: ClientErrorCode;
  browser: BrowserFamily;
  stack: { file: string; line: number; column: number }[];
}

const staticRoutes = new Set([
  "/",
  "/users",
  "/administrators",
  "/conferences",
  "/chats",
  "/recordings",
  "/files",
  "/system",
  "/settings",
  "/audit",
  "/errors",
]);

export function telemetryRoute(pathname: string): string {
  const path = pathname.split(/[?#]/, 1)[0];
  return staticRoutes.has(path) ? path : "unknown";
}

/**
 * Сводит User-Agent к семейству, исключая точную версию и характеристики устройства.
 * @args userAgent — строка браузера, используемая только локально.
 * @return одно из пяти разрешённых значений.
 */
export function browserFamily(userAgent: string): BrowserFamily {
  if (/Edg\//.test(userAgent)) return "edge";
  if (/Firefox\/|FxiOS\//.test(userAgent)) return "firefox";
  if (/Chrome\/|Chromium\/|CriOS\//.test(userAgent)) return "chromium";
  if (/Safari\//.test(userAgent)) return "safari";
  return "other";
}

/**
 * Оставляет не более пяти координат в JS-артефактах того же origin.
 * @args error — произвольный отказ; origin — origin самого приложения.
 * @return кадры без текста ошибки, функций, хоста, query, hash и внешних URL.
 */
export function sanitizedStack(
  error: unknown,
  origin: string,
): ClientErrorEvent["stack"] {
  const frames: ClientErrorEvent["stack"] = [];
  try {
    if (!(error instanceof Error) || typeof error.stack !== "string")
      return frames;
    for (const row of error.stack.slice(0, 16_384).split("\n")) {
      // Только форма реального кадра Chrome или Firefox, без первой строки сообщения.
      if (!/^\s*at\s/.test(row) && !/^[^\s@]*@https?:\/\//.test(row)) continue;
      const match = row.match(/(https?:\/\/[^\s()]+):(\d+):(\d+)\)?$/);
      if (!match) continue;
      const url = new URL(match[1]);
      const asset = url.pathname.match(
        /^\/assets\/([A-Za-z0-9_-]+-[A-Za-z0-9_-]{8,}\.js)$/,
      );
      const line = Number(match[2]);
      const column = Number(match[3]);
      if (
        url.origin !== origin ||
        url.username ||
        url.password ||
        url.search ||
        url.hash ||
        !asset ||
        asset[1].length > 160 ||
        !Number.isSafeInteger(line) ||
        !Number.isSafeInteger(column) ||
        line < 1 ||
        line > 10_000_000 ||
        column < 1 ||
        column > 10_000_000
      )
        continue;
      frames.push({ file: asset[1], line, column });
      if (frames.length === 5) break;
    }
  } catch {
    // Нестандартный объект ошибки не должен вызывать рекурсивный отказ телеметрии.
  }
  return frames;
}

export interface ReporterOptions {
  version: string;
  origin: string;
  pathname: () => string;
  userAgent: string;
  isActive: () => boolean;
  send: (event: ClientErrorEvent, signal: AbortSignal) => Promise<unknown>;
  now?: () => number;
}

// Reports are bounded and never persist raw errors, URLs, credentials or bodies.
export function createAdminErrorReporter(options: ReporterOptions) {
  const now = options.now || Date.now;
  const recent = new Map<string, number>();
  const pending = new Set<AbortController>();
  let windowStarted = now();
  let inWindow = 0;
  let total = 0;
  let closed = false;
  return {
    report(code: ClientErrorCode, error: unknown): void {
      try {
        if (closed || !options.isActive() || total >= 20) return;
        const timestamp = now();
        if (timestamp - windowStarted >= 60_000) {
          windowStarted = timestamp;
          inWindow = 0;
        }
        if (inWindow >= 5) return;
        const event: ClientErrorEvent = {
          version: /^[A-Za-z0-9][A-Za-z0-9._+-]{0,79}$/.test(options.version)
            ? options.version
            : "unknown",
          route: telemetryRoute(options.pathname()),
          code,
          browser: browserFamily(options.userAgent),
          stack: sanitizedStack(error, options.origin),
        };
        const body = JSON.stringify(event);
        const last = recent.get(body);
        if (last !== undefined && timestamp - last < 10_000) return;
        recent.set(body, timestamp);
        inWindow++;
        total++;
        const controller = new AbortController();
        pending.add(controller);
        try {
          Promise.resolve(options.send(event, controller.signal))
            .catch(() => {})
            .finally(() => pending.delete(controller));
        } catch {
          pending.delete(controller);
        }
      } catch {
        // Telemetry failure must not affect the application or recurse.
      }
    },
    close() {
      closed = true;
      for (const controller of pending) controller.abort();
      pending.clear();
      recent.clear();
    },
  };
}

let reporter: ReturnType<typeof createAdminErrorReporter> | undefined;
export function reportAdminError(code: ClientErrorCode, error: unknown) {
  reporter?.report(code, error);
}
export function installAdminTelemetry(options: ReporterOptions): () => void {
  if (reporter) return () => {};
  const current = createAdminErrorReporter(options);
  reporter = current;
  const onError = (event: ErrorEvent) =>
    current.report("uncaught_error", event.error);
  const onRejection = (event: PromiseRejectionEvent) =>
    current.report("unhandled_rejection", event.reason);
  window.addEventListener("error", onError);
  window.addEventListener("unhandledrejection", onRejection);
  return () => {
    window.removeEventListener("error", onError);
    window.removeEventListener("unhandledrejection", onRejection);
    current.close();
    if (reporter === current) reporter = undefined;
  };
}
