import type { QueryClient } from "@tanstack/react-query";

// This flag contains no identity or credential. It prevents an unconfirmed
// logout from silently restoring the HttpOnly-cookie session on reload/focus.
const signedOutKey = "meetspace-admin-signed-out";

export function readSignedOut(storage?: Pick<Storage, "getItem">): boolean {
  try {
    return (storage ?? window.sessionStorage).getItem(signedOutKey) === "1";
  } catch {
    return false;
  }
}

export function rememberSignedOut(
  value: boolean,
  storage?: Pick<Storage, "setItem" | "removeItem">,
) {
  try {
    const target = storage ?? window.sessionStorage;
    if (value) target.setItem(signedOutKey, "1");
    else target.removeItem(signedOutKey);
  } catch {
    /* Private-mode storage can be unavailable; the in-memory guard remains. */
  }
}

export function clearAdministrativeCache(client: QueryClient) {
  void client.cancelQueries({ queryKey: ["admin"] });
  void client.cancelQueries({ queryKey: ["admin-session"] });
  client.removeQueries({ queryKey: ["admin"] });
  client.setQueryData(["admin-session"], null);
}
