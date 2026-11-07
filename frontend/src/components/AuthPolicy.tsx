import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import { api } from "../api";
import { Brand, Button, ErrorNotice, Loading } from "./ui";

export function useAuthPolicy() {
  const query = useQuery({
    queryKey: ["auth-providers"],
    queryFn: ({ signal }) => api.authProviders(signal),
    staleTime: 0,
    refetchOnMount: "always",
    retry: false,
  });
  const ready = query.isSuccess && !query.isFetching;
  return {
    ...query,
    ready,
    directoryOnly: ready && query.data.mode === "ldap",
    registrationAllowed: ready && query.data.registrationEnabled,
    recoveryAllowed: ready && query.data.passwordRecoveryEnabled,
  };
}

export function AuthPolicyStatus({
  policy,
}: {
  policy: ReturnType<typeof useAuthPolicy>;
}) {
  return policy.isError ? (
    <div role="status">
      <ErrorNotice>Не удалось загрузить настройки входа.</ErrorNotice>
      <Button variant="secondary" onClick={() => void policy.refetch()}>
        Повторить
      </Button>
    </div>
  ) : (
    <Loading />
  );
}

export function AuthPolicyPage({
  policy,
}: {
  policy: ReturnType<typeof useAuthPolicy>;
}) {
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
          aria-label="Настройки входа"
        >
          <AuthPolicyStatus policy={policy} />
        </section>
      </main>
    </div>
  );
}
