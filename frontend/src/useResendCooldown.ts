import { useEffect, useState } from "react";

const COOLDOWN_MS = 2 * 60 * 1000;

function readDeadline(storageKey: string) {
  try {
    const deadline = Number(sessionStorage.getItem(storageKey));
    const now = Date.now();
    if (
      Number.isFinite(deadline) &&
      deadline > now &&
      deadline <= now + COOLDOWN_MS
    )
      return deadline;
    sessionStorage.removeItem(storageKey);
  } catch {
    // An in-memory pause still works when browser storage is unavailable.
  }
  return 0;
}

function saveDeadline(storageKey: string, deadline: number) {
  try {
    if (deadline) sessionStorage.setItem(storageKey, String(deadline));
    else sessionStorage.removeItem(storageKey);
  } catch {
    // Persist only a timestamp, never an email address, password or link token.
  }
}

/** A two-minute resend pause that survives reloads in the current browser tab. */
export function useResendCooldown(storageKey: string) {
  const [deadline, setDeadline] = useState(() => readDeadline(storageKey));
  const [now, setNow] = useState(Date.now);
  const remainingSeconds = Math.max(0, Math.ceil((deadline - now) / 1000));
  const time = `${Math.floor(remainingSeconds / 60)
    .toString()
    .padStart(2, "0")}:${(remainingSeconds % 60).toString().padStart(2, "0")}`;

  useEffect(() => {
    if (!deadline) return;
    const update = () => {
      const current = Date.now();
      setNow(current);
      if (current >= deadline) {
        setDeadline(0);
        saveDeadline(storageKey, 0);
      }
    };
    update();
    const interval = window.setInterval(update, 1000);
    window.addEventListener("focus", update);
    document.addEventListener("visibilitychange", update);
    return () => {
      window.clearInterval(interval);
      window.removeEventListener("focus", update);
      document.removeEventListener("visibilitychange", update);
    };
  }, [deadline, storageKey]);

  function start() {
    const current = Date.now();
    const next = current + COOLDOWN_MS;
    setNow(current);
    setDeadline(next);
    saveDeadline(storageKey, next);
  }

  return {
    remainingSeconds,
    time,
    start,
    isActive: () => Date.now() < deadline,
  };
}
