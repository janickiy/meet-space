import { useEffect, useId, useRef, useState } from "react";
import type { SubmitEvent } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Info, Mail, Search, Users, Video, X } from "lucide-react";
import { api } from "../api";
import type { Conference, InvitationUser } from "../types";
import { Button, ErrorNotice, Modal } from "./ui";
import { MeetingInvitationLink } from "./MeetingInvitationLink";
import "./conference-invitations.css";

const maxRecipients = 20;

/**
 * Share a meeting link and open a separate recipient dialog when requested.
 * @param conference The meeting to share with participants.
 * @param canInvite Whether the current user may send invitations through the server.
 * @param onBusyChange Notify the parent while an invitation request is in progress.
 */
export function ConferenceInviteContent({
  conference,
  canInvite,
  onBusyChange,
}: {
  conference: Conference;
  canInvite: boolean;
  onBusyChange?: (busy: boolean) => void;
}) {
  const [inviting, setInviting] = useState(false);
  return (
    <>
      <div className="conference-invitations">
        <p className="conference-invitation-title">{conference.title}</p>
        <MeetingInvitationLink inviteCode={conference.inviteCode} />
        {canInvite && (
          <div className="conference-invite-actions">
            <Button
              type="button"
              aria-haspopup="dialog"
              onClick={() => setInviting(true)}
            >
              <Mail size={18} aria-hidden="true" />
              Пригласить участников
            </Button>
          </div>
        )}
      </div>
      {canInvite && inviting && (
        <ConferenceInvitationModal
          key={conference.id}
          conference={conference}
          onClose={() => setInviting(false)}
          onBusyChange={onBusyChange}
        />
      )}
    </>
  );
}

/**
 * Invite recipients in a dedicated dialog and retain submission results until closed.
 * @param conference The meeting to invite recipients to.
 * @param onClose Return to the sharing dialog after closing or cancelling.
 * @param onBusyChange Keep the underlying dialog open while a request is pending.
 */
export function ConferenceInvitationModal({
  conference,
  onClose,
  onBusyChange,
}: {
  conference: Conference;
  onClose: () => void;
  onBusyChange?: (busy: boolean) => void;
}) {
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    onBusyChange?.(busy);
    return () => onBusyChange?.(false);
  }, [busy, onBusyChange]);

  return (
    <Modal
      title="Пригласить участников"
      className="conference-invite-modal conference-recipient-modal"
      onClose={() => {
        if (!busy) onClose();
      }}
    >
      <div className="invitation-meeting-context">
        <span className="invitation-meeting-icon">
          <Video size={20} aria-hidden="true" />
        </span>
        <div>
          <span>Приглашение на встречу</span>
          <strong>{conference.title}</strong>
        </div>
      </div>
      <ConferenceInvitationForm
        key={conference.id}
        conferenceId={conference.id}
        onBusyChange={setBusy}
        onCancel={onClose}
      />
    </Modal>
  );
}

/**
 * Collect email addresses and selected users, keeping input until invitations are accepted.
 * @param conferenceId The meeting that receives the selected participants.
 * @param onBusyChange Notify the parent while the invitation submission is pending.
 * @param onCancel Return to the sharing dialog without sending invitations.
 */
export function ConferenceInvitationForm({
  conferenceId,
  onBusyChange,
  onCancel,
}: {
  conferenceId: string;
  onBusyChange?: (busy: boolean) => void;
  onCancel?: () => void;
}) {
  const id = useId();
  const [emailsText, setEmailsText] = useState("");
  const [queryText, setQueryText] = useState("");
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<InvitationUser[]>([]);
  const [validation, setValidation] = useState("");
  const submitting = useRef(false);
  useEffect(() => {
    const timer = window.setTimeout(() => setSearch(queryText.trim()), 300);
    return () => window.clearTimeout(timer);
  }, [queryText]);
  const query = useQuery({
    queryKey: ["invitation-users", conferenceId, search],
    queryFn: ({ signal }) => api.invitationUsers(conferenceId, search, signal),
    enabled: Array.from(search).length >= 2,
    retry: false,
    staleTime: 30_000,
  });
  const invitation = useMutation({
    mutationFn: (recipients: { emails: string[]; userIds: string[] }) =>
      api.inviteParticipants(conferenceId, recipients),
    onSuccess: () => {
      setEmailsText("");
      setSelected([]);
      setQueryText("");
    },
    onSettled: () => {
      submitting.current = false;
    },
  });
  useEffect(() => {
    onBusyChange?.(invitation.isPending);
    return () => onBusyChange?.(false);
  }, [invitation.isPending, onBusyChange]);

  function edit() {
    setValidation("");
    if (!invitation.isPending) invitation.reset();
  }

  function toggle(user: InvitationUser) {
    edit();
    if (selected.some((item) => item.id === user.id)) {
      setSelected(selected.filter((item) => item.id !== user.id));
      return;
    }
    if (selected.length >= maxRecipients) {
      setValidation(
        `За один раз можно пригласить не больше ${maxRecipients} человек.`,
      );
      return;
    }
    setSelected([...selected, user]);
  }

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting.current) return;
    setValidation("");
    invitation.reset();
    const entered = [
      ...new Set(
        emailsText
          .split(/[\s,;]+/)
          .filter(Boolean)
          .map((email) => email.toLowerCase()),
      ),
    ];
    if (
      entered.some(
        (email) =>
          email.length > 254 || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email),
      )
    ) {
      setValidation("Проверьте адреса email. Например: colleague@example.com.");
      return;
    }
    const selectedEmails = new Set(
      selected.map((user) => user.email.toLowerCase()),
    );
    const emails = entered.filter((email) => !selectedEmails.has(email));
    const total = emails.length + selected.length;
    if (!total) {
      setValidation("Укажите email или выберите пользователя системы.");
      return;
    }
    if (total > maxRecipients) {
      setValidation(
        `За один раз можно пригласить не больше ${maxRecipients} человек.`,
      );
      return;
    }
    submitting.current = true;
    invitation.mutate({ emails, userIds: selected.map((user) => user.id) });
  }

  const currentSearch = queryText.trim() === search;
  const hasSearch = Array.from(queryText.trim()).length >= 2;
  return (
    <form className="conference-invitation-form" onSubmit={submit} noValidate>
      <div className="conference-invitation-fields">
        <div className="invitation-email-section">
          <label className="field" htmlFor={`${id}-emails`}>
            <span className="invitation-field-label">
              <Mail size={18} aria-hidden="true" />
              По email
            </span>
            <textarea
              id={`${id}-emails`}
              data-autofocus
              placeholder="colleague@example.com"
              autoComplete="off"
              rows={2}
              maxLength={6000}
              value={emailsText}
              disabled={invitation.isPending}
              aria-label="По email — адреса участников"
              aria-describedby={`${id}-email-hint`}
              onChange={(event) => {
                edit();
                setEmailsText(event.target.value);
              }}
            />
          </label>
          <p className="field-hint" id={`${id}-email-hint`}>
            Разделяйте адреса запятой или новой строкой.
          </p>
        </div>
        <div className="invitation-people-section">
          <label className="field" htmlFor={`${id}-search`}>
            <span className="invitation-field-label">
              <Users size={18} aria-hidden="true" />
              Пользователи MeetSpace
            </span>
            <span className="invitation-search-field">
              <Search size={17} aria-hidden="true" />
              <input
                id={`${id}-search`}
                type="search"
                placeholder="Имя или email"
                autoComplete="off"
                maxLength={100}
                value={queryText}
                disabled={invitation.isPending}
                aria-label="Пользователи MeetSpace — поиск по имени или email"
                aria-describedby={`${id}-search-hint`}
                onChange={(event) => setQueryText(event.target.value)}
              />
            </span>
          </label>
          <p className="field-hint" id={`${id}-search-hint`}>
            Найдите коллегу по имени или email — от 2 символов.
          </p>
          {selected.length > 0 && (
            <ul
              className="invitation-selected"
              aria-label="Выбранные пользователи"
            >
              {selected.map((user) => (
                <li key={user.id}>
                  <span>{user.displayName || user.email}</span>
                  <button
                    type="button"
                    aria-label={`Убрать ${user.email}`}
                    disabled={invitation.isPending}
                    onClick={() => toggle(user)}
                  >
                    <X size={15} aria-hidden="true" />
                  </button>
                </li>
              ))}
            </ul>
          )}
          {hasSearch && (!currentSearch || query.isFetching) && (
            <p className="field-hint" role="status">
              Ищем пользователей…
            </p>
          )}
          {hasSearch && currentSearch && query.isError && (
            <div className="invitation-search-error">
              <ErrorNotice error={query.error} />
              <Button
                type="button"
                variant="outline"
                onClick={() => void query.refetch()}
              >
                Повторить поиск
              </Button>
            </div>
          )}
          {hasSearch &&
            currentSearch &&
            query.isSuccess &&
            !query.isFetching &&
            (query.data.items.length ? (
              <ul
                className="invitation-user-results"
                aria-label="Пользователи системы"
              >
                {query.data.items.map((user) => (
                  <li key={user.id}>
                    <label>
                      <input
                        type="checkbox"
                        aria-label={
                          user.displayName
                            ? `${user.displayName}, ${user.email}`
                            : user.email
                        }
                        checked={selected.some((item) => item.id === user.id)}
                        disabled={invitation.isPending}
                        onChange={() => toggle(user)}
                      />
                      <span>
                        <strong>{user.displayName || user.email}</strong>
                        {user.displayName && <small>{user.email}</small>}
                      </span>
                    </label>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="field-hint" role="status">
                Пользователи не найдены. Укажите email в поле приглашения.
              </p>
            ))}
        </div>
      </div>
      <p className="field-hint invitation-delivery-note">
        <Info size={17} aria-hidden="true" />
        <span>
          Отправим ссылку на встречу по email. Пользователи MeetSpace также
          увидят приглашение в личном кабинете.
        </span>
      </p>
      <ErrorNotice error={invitation.error}>{validation || null}</ErrorNotice>
      {invitation.data && (
        <div className="invitation-result" role="status" aria-live="polite">
          <ul>
            {invitation.data.items.map((item) => (
              <li key={item.id}>
                <strong>{item.email}</strong>
                <span>
                  {item.status === "queued"
                    ? "Приглашение поставлено в очередь"
                    : item.status === "left_chat"
                      ? "Пользователь покинул чат. Он может вернуться по ссылке на встречу."
                      : "Приглашение уже отправлялось"}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
      <footer className="invitation-form-footer">
        <p>
          <Users size={16} aria-hidden="true" />
          До {maxRecipients} участников
        </p>
        {onCancel && (
          <Button
            type="button"
            variant="secondary"
            disabled={invitation.isPending}
            onClick={onCancel}
          >
            Назад
          </Button>
        )}
        <Button type="submit" busy={invitation.isPending}>
          <Mail size={18} aria-hidden="true" />
          Отправить приглашения
        </Button>
      </footer>
    </form>
  );
}
