import { useEffect, useRef, useState } from "react";
import { useAuth } from "../auth";
import { AvatarCropDialog } from "./AvatarCropDialog";
import { UserAvatar } from "./UserAvatar";
import { Button, ErrorNotice } from "./ui";
import "./avatar.css";

/** Save or remove the current user's photo independently from the name form. */
export function ProfileAvatarEditor() {
  const { user, updateAvatar } = useAuth();
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const [saved, setSaved] = useState("");
  const pending = useRef<AbortController | null>(null);
  useEffect(() => () => pending.current?.abort(), []);
  if (!user) return null;

  function edit() {
    setError(undefined);
    setSaved("");
    setEditing(true);
  }

  async function remove() {
    if (pending.current) return;
    const controller = new AbortController();
    pending.current = controller;
    setBusy(true);
    setError(undefined);
    setSaved("");
    try {
      await updateAvatar(null, controller.signal);
      if (!controller.signal.aborted) setSaved("Фото удалено.");
    } catch (error) {
      if (!controller.signal.aborted) setError(error);
    } finally {
      if (!controller.signal.aborted) {
        pending.current = null;
        setBusy(false);
      }
    }
  }
  return (
    <section className="profile-avatar-editor" aria-label="Фото профиля">
      <div className="avatar-file-field">
        <button
          type="button"
          className="avatar-file-picker"
          aria-label="Редактировать аватар профиля"
          disabled={busy}
          onClick={edit}
        >
          <UserAvatar
            id={user.id}
            name={user.displayName || user.email || ""}
            version={user.avatarVersion}
            className=""
          />
        </button>
        <div className="avatar-file-copy">
          <Button
            type="button"
            variant="outline"
            disabled={busy}
            onClick={edit}
          >
            {user.avatarVersion ? "Изменить фото" : "Добавить фото"}
          </Button>
          <span>
            Фото видно другим зарегистрированным пользователям MeetSpace.
          </span>
        </div>
      </div>
      {user.avatarVersion && (
        <div className="profile-avatar-actions">
          <Button
            type="button"
            variant="outline"
            busy={busy}
            onClick={() => void remove()}
          >
            Удалить фото
          </Button>
        </div>
      )}
      <ErrorNotice error={error} />
      {saved && <p role="status">{saved}</p>}
      {editing && (
        <AvatarCropDialog
          userId={user.id}
          hasAvatar={!!user.avatarVersion}
          onSave={updateAvatar}
          onSaved={() => {
            setSaved("Фото профиля сохранено.");
            setEditing(false);
          }}
          onClose={() => setEditing(false)}
        />
      )}
    </section>
  );
}
