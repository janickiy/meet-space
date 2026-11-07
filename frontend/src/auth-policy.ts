import type { AuthProviders } from "./types";

/** Неизвестная или неполная политика не открывает регистрацию и восстановление. */
export function validateAuthProviders(value: unknown): AuthProviders {
  const policy = value as Partial<AuthProviders> | null;
  if (
    !policy ||
    !["local", "ldap"].includes(String(policy.mode)) ||
    typeof policy.registrationEnabled !== "boolean" ||
    typeof policy.passwordRecoveryEnabled !== "boolean" ||
    typeof policy.ldap?.enabled !== "boolean" ||
    typeof policy.ldap?.label !== "string" ||
    policy.ldap.enabled !== (policy.mode === "ldap") ||
    (policy.mode === "ldap" &&
      (policy.registrationEnabled || policy.passwordRecoveryEnabled))
  ) {
    throw new Error(
      "Не удалось загрузить настройки входа. Попробуйте ещё раз.",
    );
  }
  return policy as AuthProviders;
}
