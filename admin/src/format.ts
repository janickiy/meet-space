export function count(value: number | null | undefined): string {
  return value == null || !Number.isFinite(value)
    ? "Нет данных"
    : new Intl.NumberFormat("ru-RU").format(value);
}
export function bytes(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return "Нет данных";
  if (value === 0) return "0 Б";
  const units = ["Б", "КБ", "МБ", "ГБ", "ТБ"];
  const rank = Math.min(
    Math.floor(Math.log(Math.max(value, 1)) / Math.log(1024)),
    units.length - 1,
  );
  return `${new Intl.NumberFormat("ru-RU", { maximumFractionDigits: rank === 0 ? 0 : 1 }).format(value / 1024 ** rank)} ${units[rank]}`;
}
export function date(
  value: string | null | undefined,
  withTime = true,
): string {
  if (!value) return "—";
  const parsed = new Date(value);
  if (!Number.isFinite(parsed.getTime())) return "—";
  return new Intl.DateTimeFormat("ru-RU", {
    day: "2-digit",
    month: "short",
    year: "numeric",
    ...(withTime ? { hour: "2-digit", minute: "2-digit" } : {}),
  }).format(parsed);
}
export function duration(seconds: number | null | undefined): string {
  if (seconds == null || !Number.isFinite(seconds)) return "—";
  const rounded = Math.max(0, Math.floor(seconds));
  const h = Math.floor(rounded / 3600),
    m = Math.floor((rounded % 3600) / 60),
    s = rounded % 60;
  return h
    ? `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`
    : `${m}:${String(s).padStart(2, "0")}`;
}
export function initials(name: string): string {
  return (
    name
      .trim()
      .split(/\s+/)
      .map((part) => part[0])
      .slice(0, 2)
      .join("")
      .toUpperCase() || "А"
  );
}
export function percent(
  used: number | null | undefined,
  total: number | null | undefined,
): number | null {
  return used == null || total == null || total <= 0
    ? null
    : Math.max(0, Math.min(100, (used / total) * 100));
}
export function canDeleteRecording(
  status: string,
  canDelete: boolean,
): boolean {
  return (
    canDelete === true &&
    ["ready", "partial_ready", "failed", "cancelled"].includes(status)
  );
}
export function validateAdministrator(
  username: string,
  password: string,
  confirmation: string,
  creating: boolean,
): string | null {
  if (!/^[a-zA-Z0-9][a-zA-Z0-9.@+_-]{2,39}$/.test(username.trim()))
    return "Логин: 3–40 символов, начинается с латинской буквы или цифры; допустимы латинские буквы, цифры, . - _ @ +.";
  if (creating && validatePassword(password)) return validatePassword(password);
  if (creating && password !== confirmation) return "Пароли не совпадают.";
  return null;
}
export function validatePassword(password: string): string | null {
  if (
    [...password].length < 8 ||
    [...password].length > 128 ||
    new TextEncoder().encode(password).length > 512
  )
    return "Пароль должен содержать от 8 до 128 символов.";
  return null;
}
