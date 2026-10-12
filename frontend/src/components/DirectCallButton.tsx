import { Phone } from "lucide-react";
import { useAuth } from "../auth";
import { useDirectCalls } from "./DirectCalls";

/** One entry point for a private call from chat headers and participant profiles. */
export function DirectCallButton({
  userId,
  name,
  label = false,
  onStart,
}: {
  userId: string;
  name: string;
  label?: boolean;
  onStart?: () => void;
}) {
  const { user } = useAuth();
  const calls = useDirectCalls();
  if (!user || user.guestConferenceId || user.id === userId) return null;
  return (
    <button
      type="button"
      className={label ? "button button-outline" : "icon-button"}
      aria-label={`Позвонить: ${name}`}
      title={calls.busy ? "У вас уже есть звонок" : `Позвонить: ${name}`}
      disabled={calls.busy}
      onClick={() => {
        onStart?.();
        void calls.startCall(userId);
      }}
    >
      <Phone size={22} aria-hidden="true" />
      {label && "Позвонить"}
    </button>
  );
}
