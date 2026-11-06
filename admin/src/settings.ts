import type { Setting, Settings } from "./types.ts";

export type SettingsDraft = Record<string, string | boolean>;
export type SettingsChanges = Record<string, string | number | boolean>;
export const mebibyte = 1024 * 1024;

export function isSecretSetting(item: Setting): boolean {
  return item.secret || item.type === "secret" || item.key === "smtpPassword";
}

// Even an erroneous backend response must not put a write-only secret in cache.
export function sanitizeSettings(data: Settings): Settings {
  return {
    ...data,
    items: data.items.map((item) =>
      isSecretSetting(item) ? { ...item, value: null } : item,
    ),
  };
}

export function settingScale(item: Setting): number {
  return item.key === "attachmentMaxBytes" ? mebibyte : 1;
}

export function settingInput(item: Setting): string | boolean {
  if (isSecretSetting(item)) return "";
  if (item.type === "boolean") return item.value === true;
  if (item.type === "number" && typeof item.value === "number")
    return String(item.value / settingScale(item));
  return item.value == null ? "" : String(item.value);
}

// Only saved values count. Draft SMTP data must be saved successfully before
// it can unlock the registration policy in another group.
export function smtpConfigured(data: Settings): boolean {
  const values = Object.fromEntries(
    data.items.map((item) => [item.key, item.value]),
  );
  return (
    values.smtpEnabled === true &&
    ["smtpHost", "smtpUsername", "smtpFrom"].every(
      (key) => typeof values[key] === "string" && values[key].trim() !== "",
    ) &&
    typeof values.smtpPort === "number" &&
    Number.isInteger(values.smtpPort) &&
    values.smtpPort >= 1 &&
    values.smtpPort <= 65535 &&
    ["tls", "starttls"].includes(String(values.smtpTLSMode)) &&
    data.items.some((item) => item.key === "smtpPassword" && item.configured)
  );
}

export function buildSettingsChanges(
  data: Settings,
  drafts: SettingsDraft,
  group: string,
): { changes: SettingsChanges; errors: Record<string, string> } {
  const changes: SettingsChanges = {};
  const errors: Record<string, string> = {};
  for (const item of data.items) {
    if (
      !item.editable ||
      (item.group || "general") !== group ||
      !(item.key in drafts)
    )
      continue;
    const draft = drafts[item.key];
    if (isSecretSetting(item)) {
      if (draft !== "") changes[item.key] = String(draft);
      continue;
    }
    if (item.type === "number") {
      const value =
        typeof draft === "string" && draft.trim() !== ""
          ? Number(draft) * settingScale(item)
          : NaN;
      if (!Number.isSafeInteger(value))
        errors[item.key] = "Укажите корректное число.";
      else if (item.min != null && value < item.min)
        errors[item.key] =
          `Минимальное значение: ${item.min / settingScale(item)}.`;
      else if (item.max != null && value > item.max)
        errors[item.key] =
          `Максимальное значение: ${item.max / settingScale(item)}.`;
      else if (
        item.step != null &&
        item.step > 0 &&
        (value - (item.min || 0)) % item.step !== 0
      )
        errors[item.key] = `Шаг значения: ${item.step / settingScale(item)}.`;
      else if (value !== item.value) changes[item.key] = value;
    } else if (item.type === "boolean") {
      if (typeof draft !== "boolean")
        errors[item.key] = "Выберите состояние переключателя.";
      else if (draft !== item.value) changes[item.key] = draft;
    } else {
      const value = String(draft).trim();
      if (item.options?.length && !item.options.includes(value))
        errors[item.key] = "Выберите значение из списка.";
      else if (value !== item.value) changes[item.key] = value;
    }
  }

  if ("emailVerificationRequired" in changes && !smtpConfigured(data))
    errors.emailVerificationRequired =
      "Для изменения подтверждения email сначала настройте и сохраните SMTP во вкладке «Почта».";

  if (group === "mail" || group === "smtp") {
    const values = Object.fromEntries(
      data.items.map((item) => [item.key, item.value]),
    );
    Object.assign(values, changes);
    if (
      values.emailVerificationRequired === true &&
      values.smtpEnabled !== true
    )
      errors.smtpEnabled =
        "Сначала отключите подтверждение email во вкладке «Основные».";
    if (values.smtpEnabled === true) {
      for (const key of ["smtpHost", "smtpUsername", "smtpFrom"])
        if (typeof values[key] !== "string" || !values[key].trim())
          errors[key] = "Заполните поле, чтобы включить почту.";
      const password = data.items.find((item) => item.key === "smtpPassword");
      if (!password?.configured && !changes.smtpPassword)
        errors.smtpPassword = "Введите пароль, чтобы включить почту.";
    }
    const host = String(values.smtpHost || "");
    if (host && (host.includes("://") || /[\s/@\r\n\0]/.test(host)))
      errors.smtpHost =
        "Укажите имя SMTP-сервера без протокола, пути и логина.";
    for (const key of ["smtpHost", "smtpUsername", "smtpFrom"]) {
      const value = String(values[key] || "");
      if (/[\r\n\0]/.test(value))
        errors[key] = "Значение не должно содержать переносы строк.";
      if (
        new TextEncoder().encode(value).length >
        (key === "smtpHost" ? 253 : 512)
      )
        errors[key] = "Значение слишком длинное.";
    }
    const from = String(values.smtpFrom || "");
    const address = from.match(/<([^<>]+)>$/)?.[1] || from;
    if (
      from &&
      (!/^[\x21-\x7e]+$/.test(address) ||
        !/^[^\s@<>]+@[^\s@<>]+$/.test(address))
    )
      errors.smtpFrom =
        "Укажите адрес отправителя, например MeetSpace <team@example.com>.";
    if (
      typeof changes.smtpPassword === "string" &&
      new TextEncoder().encode(changes.smtpPassword).length > 4096
    )
      errors.smtpPassword = "Пароль слишком длинный.";
    if (
      typeof changes.smtpPassword === "string" &&
      changes.smtpPassword.includes("\0")
    )
      errors.smtpPassword = "Пароль содержит недопустимый символ.";
  }
  return { changes, errors };
}

export function settingsFailureMessage(
  error: unknown,
  drafts: SettingsDraft,
): string {
  let message =
    error instanceof Error
      ? error.message
      : "Не удалось сохранить настройки. Повторите попытку.";
  // API errors should never echo secrets; redact defensively before rendering one.
  const password = drafts.smtpPassword;
  if (typeof password === "string" && password)
    message = message.replaceAll(password, "••••••••");
  return message;
}
