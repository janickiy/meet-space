import type { ReactNode } from "react";
import type { PersonalPeer } from "../types";
import { UserAvatar } from "./UserAvatar";
import { DirectCallButton } from "./DirectCallButton";
import { Modal } from "./ui";
import "./user-info-modal.css";

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
      <div className="personal-info-profile">
        <UserAvatar
          id={person.id}
          name={person.displayName}
          className="personal-info-avatar"
        />
        <div className="personal-info-identity">
          <h3>{person.displayName}</h3>
          <p
            className={`personal-info-presence${!presencePending && online === true ? " personal-info-presence-online" : ""}`}
            role="status"
            aria-live="polite"
          >
            <span aria-hidden="true" />
            {statusText}
          </p>
          <p className="personal-info-subtitle">{subtitle}</p>
        </div>
      </div>
      <dl className="personal-info-details">
        <div>
          <dt>Идентификатор пользователя</dt>
          <dd>{person.id}</dd>
        </div>
      </dl>
      <div className="personal-info-actions">
        {actions}
        <div className="personal-info-call">
          <DirectCallButton
            userId={person.id}
            name={person.displayName}
            label
            onStart={onCallStart}
          />
        </div>
      </div>
    </Modal>
  );
}
