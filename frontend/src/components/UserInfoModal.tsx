import type { ReactNode } from "react";
import type { PersonalPeer } from "../types";
import { initials } from "../utils";
import { DirectCallButton } from "./DirectCallButton";
import { Button, Modal } from "./ui";

/** Общая карточка пользователя для личной переписки и списка участников группы. */
export function UserInfoModal({
  person,
  subtitle,
  online,
  presencePending = false,
  onClose,
  onCallStart = onClose,
  returnFocus,
  actions,
}: {
  person: PersonalPeer;
  subtitle: string;
  online?: boolean;
  presencePending?: boolean;
  onClose: () => void;
  onCallStart?: () => void;
  returnFocus?: () => HTMLElement | null;
  actions?: ReactNode;
}) {
  const statusText = presencePending
    ? "Проверяем статус…"
    : online === true
      ? "В сети"
      : online === false
        ? "не в сети."
        : "Статус временно недоступен.";
  return (
    <Modal
      title="Информация о пользователе"
      className="personal-info-modal"
      onClose={onClose}
      returnFocus={returnFocus}
    >
      <div className="personal-user-info">
        <span className="personal-avatar group-avatar-large" aria-hidden="true">
          {initials(person.displayName)}
        </span>
        <h3>{person.displayName}</h3>
        <p>{subtitle}</p>
        <p
          className={`personal-user-presence${online === true ? " personal-user-presence-online" : ""}`}
          role="status"
          aria-live="polite"
        >
          <span className="personal-user-presence-dot" aria-hidden="true" />
          {statusText}
        </p>
        <dl>
          <dt>Имя</dt>
          <dd>{person.displayName}</dd>
          <dt>Идентификатор пользователя</dt>
          <dd className="personal-user-id">{person.id}</dd>
        </dl>
      </div>
      <div className="personal-confirm-actions">
        {actions}
        <DirectCallButton
          userId={person.id}
          name={person.displayName}
          label
          onStart={onCallStart}
        />
        <Button variant="outline" onClick={onClose}>
          Закрыть
        </Button>
      </div>
    </Modal>
  );
}
