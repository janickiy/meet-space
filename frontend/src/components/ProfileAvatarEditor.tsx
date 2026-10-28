import { useEffect, useRef, useState } from "react";
import { useAuth } from "../auth";
import { AvatarPicker } from "./AvatarPicker";
import { UserAvatar } from "./UserAvatar";
import { Button, ErrorNotice } from "./ui";

/** Save or remove the current user's photo independently from the name form. */
export function ProfileAvatarEditor() {
  const { user, updateAvatar } = useAuth();
  const [file, setFile] = useState<File>();
  const [busy, setBusy] = useState(false);
  const [preparing, setPreparing] = useState(false);
  const [error, setError] = useState<unknown>();
  const [saved, setSaved] = useState("");
  const pending = useRef<AbortController | null>(null);
  useEffect(() => () => pending.current?.abort(), []);
  async function save(remove = false) {
    if (busy || preparing) return;
    const controller = new AbortController();
    pending.current = controller;
    setBusy(true);
    setError(undefined);
    setSaved("");
    try {
      await updateAvatar(remove ? null : file!, controller.signal);
      if (!controller.signal.aborted) {
        setFile(undefined);
        setSaved(remove ? "Фото удалено." : "Фото профиля сохранено.");
      }
    } catch (error) {
      if (!controller.signal.aborted) setError(error);
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  return (
    <section className="profile-avatar-editor" aria-label="Фото профиля">
      <AvatarPicker
        file={file}
        onChange={(file) => {
          setFile(file);
          setSaved("");
          setError(undefined);
        }}
        disabled={busy}
        onPreparing={setPreparing}
        label="Выбрать аватар профиля"
        fallback={
          <UserAvatar
            id={user?.id}
            name={user?.displayName || user?.email || ""}
            version={user?.avatarVersion}
            className=""
          />
        }
      />
      <p className="field-hint">
        Фото видно другим зарегистрированным пользователям MeetSpace.
      </p>
      <div className="profile-avatar-actions">
        {file && (
          <Button
            type="button"
            busy={busy}
            disabled={preparing}
            onClick={() => void save()}
          >
            Сохранить фото
          </Button>
        )}
        {user?.avatarVersion && (
          <Button
            type="button"
            variant="outline"
            disabled={busy || preparing}
            onClick={() => void save(true)}
          >
            Удалить фото
          </Button>
        )}
      </div>
      <ErrorNotice error={error} />
      {saved && <p role="status">{saved}</p>}
    </section>
  );
}
