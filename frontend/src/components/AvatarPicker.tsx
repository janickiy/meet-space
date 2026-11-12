import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { Camera } from "lucide-react";
import { AvatarCropDialog } from "./AvatarCropDialog";
import "./avatar.css";

/** Edit a draft photo without uploading it until the parent form saves. */
export function AvatarPicker({
  file,
  onChange,
  disabled = false,
  label,
  title,
  fallback,
  loadPhoto,
  onPreparing,
}: {
  file?: File;
  onChange: (file?: File) => void;
  disabled?: boolean;
  label: string;
  title: string;
  fallback?: ReactNode;
  loadPhoto?: (signal: AbortSignal) => Blob | Promise<Blob>;
  onPreparing?: (value: boolean) => void;
}) {
  const [url, setURL] = useState<string>();
  const [editing, setEditing] = useState(false);
  useEffect(() => {
    onPreparing?.(editing);
    return () => onPreparing?.(false);
  }, [editing, onPreparing]);
  useEffect(() => {
    if (!file) {
      setURL(undefined);
      return;
    }
    const url = URL.createObjectURL(file);
    setURL(url);
    return () => URL.revokeObjectURL(url);
  }, [file]);
  return (
    <>
      <div className="avatar-file-field">
        <button
          type="button"
          className="avatar-file-picker"
          disabled={disabled}
          aria-label={label}
          onClick={() => setEditing(true)}
        >
          {url ? <img src={url} alt="" /> : fallback || <Camera size={25} />}
        </button>
        <div className="avatar-file-copy">
          <button
            type="button"
            className="text-link"
            disabled={disabled}
            onClick={() => setEditing(true)}
          >
            {file || loadPhoto ? "Изменить фото" : "Добавить фото"}
          </button>
          <span>
            PNG, JPEG или WebP до 10 МБ. Можно изменить масштаб и положение
            фото.
          </span>
          {file && (
            <button
              type="button"
              className="text-link"
              disabled={disabled}
              onClick={() => onChange(undefined)}
            >
              Отменить выбор фото
            </button>
          )}
        </div>
      </div>
      {editing && (
        <AvatarCropDialog
          title={title}
          loadPhoto={file ? () => file : loadPhoto}
          onSave={async (cropped, signal) => {
            signal.throwIfAborted();
            onChange(cropped);
          }}
          onSaved={() => setEditing(false)}
          onClose={() => setEditing(false)}
        />
      )}
    </>
  );
}
