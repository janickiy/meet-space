import { PhoneIncoming, PhoneOutgoing } from "lucide-react";
import { useAuth } from "../auth";
import type { DirectCallHistory } from "../types";
import "./call-history.css";

export function callHistoryResult(call: DirectCallHistory, outgoing: boolean) {
  switch (call.status) {
    case "ringing":
      return outgoing ? "Вы звоните…" : "Входящий вызов";
    case "accepted":
      return "Разговор идёт";
    case "declined":
      return outgoing ? "Вызов отклонён" : "Вы отклонили вызов";
    case "cancelled":
      return outgoing ? "Вы отменили вызов" : "Пропущенный вызов";
    case "missed":
      return outgoing ? "Нет ответа" : "Пропущенный вызов";
    case "ended": {
      const seconds = Math.max(0, Math.floor(call.durationSec || 0));
      const minutes = Math.floor(seconds / 60);
      return `Завершён · ${minutes}:${String(seconds % 60).padStart(2, "0")}`;
    }
  }
}

/** Incoming/outgoing is viewer-relative; a status update never adds a duplicate row. */
export function CallHistoryMessage({ call }: { call: DirectCallHistory }) {
  const { user } = useAuth();
  const outgoing = call.callerUserId === user?.id;
  const missed = !outgoing && ["missed", "cancelled"].includes(call.status);
  const Icon = outgoing ? PhoneOutgoing : PhoneIncoming;
  return (
    <div
      className={`call-history-message${missed ? " call-history-missed" : ""}`}
    >
      <Icon size={23} aria-hidden="true" />
      <span>
        <strong>{outgoing ? "Исходящий звонок" : "Входящий звонок"}</strong>
        <span>{callHistoryResult(call, outgoing)}</span>
      </span>
    </div>
  );
}
