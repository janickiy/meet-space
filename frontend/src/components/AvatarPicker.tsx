import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { Camera, LoaderCircle } from "lucide-react";
import { prepareAvatar } from "../avatarImage";
import { ErrorNotice } from "./ui";
import "./avatar.css";

/** Pick and normalize a photo without uploading it until the parent saves. */
export function AvatarPicker({
  file,
  onChange,
  disabled = false,
  label,
  fallback,
  onPreparing,
}: {
  file?: File;
  onChange: (file?: File) => void;
  disabled?: boolean;
  label: string;
  fallback?: ReactNode;
  onPreparing?: (value: boolean) => void;
}) {
  const input = useRef<HTMLInputElement>(null);
  const pending = useRef<AbortController | null>(null);
  const [url, setURL] = useState<string>();
  const [error, setError] = useState("");
  const [preparing, setPreparing] = useState(false);
  useEffect(
    () => () => {
      pending.current?.abort();
      onPreparing?.(false);
    },
    [onPreparing],
  );
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
    <div className="avatar-file-field">
      <button
        type="button"
        className="avatar-file-picker"
        disabled={disabled || preparing}
        aria-label={label}
        onClick={() => input.current?.click()}
      >
        {preparing ? (
          <LoaderCircle size={24} className="spin" />
        ) : url ? (
          <img src={url} alt="" />
        ) : (
          fallback || <Camera size={25} />
        )}
      </button>
      <div className="avatar-file-copy">
        <button
          type="button"
          className="text-link"
          disabled={disabled || preparing}
          onClick={() => input.current?.click()}
        >
          {preparing
            ? "Подготавливаем фото…"
            : file
              ? "Выбрать другое фото"
              : "Выбрать фото"}
        </button>
        <span>
          PNG, JPEG или WebP до 10 МБ. Фото будет уменьшено и обрезано по
          центру.
        </span>
        {file && (
          <button
            type="button"
            className="text-link"
            disabled={disabled || preparing}
            onClick={() => {
              pending.current?.abort();
              setError("");
              onChange(undefined);
            }}
          >
            Отменить выбор фото
          </button>
        )}
      </div>
      <input
        ref={input}
        hidden
        type="file"
        accept="image/png,image/jpeg,image/webp"
        disabled={disabled || preparing}
        aria-label={`Файл: ${label.toLocaleLowerCase("ru")}`}
        onChange={async (event) => {
          const selected = event.target.files?.[0];
          event.target.value = "";
          if (!selected) return;
          pending.current?.abort();
          const controller = new AbortController();
          pending.current = controller;
          setError("");
          setPreparing(true);
          onPreparing?.(true);
          try {
            const normalized = await prepareAvatar(selected, controller.signal);
            if (!controller.signal.aborted) onChange(normalized);
          } catch (error) {
            if (!controller.signal.aborted)
              setError(
                error instanceof Error
                  ? error.message
                  : "Не удалось открыть изображение.",
              );
          } finally {
            if (!controller.signal.aborted) {
              setPreparing(false);
              onPreparing?.(false);
            }
          }
        }}
      />
      {error && <ErrorNotice>{error}</ErrorNotice>}
    </div>
  );
}
