import { useQuery } from "@tanstack/react-query";
import { api } from "../api";
import "./login-provider-choice.css";

export type LoginProvider = "local" | "ldap";

/** Недоступность каталога не мешает войти с локальной учётной записью. */
export function useLoginProviders(enabled = true) {
  return useQuery({
    queryKey: ["auth-providers"],
    queryFn: ({ signal }) => api.authProviders(signal),
    enabled,
    staleTime: 60_000,
    retry: false,
  });
}

export function LoginProviderChoice({
  value,
  change,
  disabled,
}: {
  value: LoginProvider;
  change: (value: LoginProvider) => void;
  disabled: boolean;
}) {
  return (
    <div
      className="login-provider-choice"
      role="group"
      aria-label="Способ входа"
    >
      {(["local", "ldap"] as const).map((provider) => (
        <button
          key={provider}
          type="button"
          disabled={disabled}
          aria-pressed={value === provider}
          onClick={() => change(provider)}
        >
          {provider === "local" ? "По email" : "Корпоративный вход"}
        </button>
      ))}
    </div>
  );
}
