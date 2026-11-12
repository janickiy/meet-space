import { useCallback, useEffect, useId, useRef, useState } from "react";
import type { PointerEvent } from "react";
import { Check, ImagePlus, Upload } from "lucide-react";
import {
  avatarCropBounds,
  decodeAvatar,
  DEFAULT_AVATAR_CROP,
  encodeAvatar,
  type AvatarCrop,
} from "../avatarImage";
import { Button, ErrorNotice, Modal } from "./ui";
import "./avatar-crop-dialog.css";

const previewSize = 512;
const sliders = [
  { key: "zoom", label: "Масштаб", min: 100, max: 300 },
  { key: "x", label: "По горизонтали", min: -100, max: 100 },
  { key: "y", label: "По вертикали", min: -100, max: 100 },
] as const;

type Pan = {
  pointerId: number;
  clientX: number;
  clientY: number;
  crop: AvatarCrop;
  xPerPixel: number;
  yPerPixel: number;
};

function positionPercent(value: number) {
  return Math.round(Math.min(100, Math.max(-100, value)));
}

/** Edits a draft locally and passes only the final square photo to its owner. */
export function AvatarCropDialog({
  title = "Фото профиля",
  loadPhoto,
  onSave,
  onSaved,
  onClose,
}: {
  title?: string;
  loadPhoto?: (signal: AbortSignal) => Blob | Promise<Blob>;
  onSave: (file: File, signal: AbortSignal) => Promise<unknown>;
  onSaved: () => void;
  onClose: () => void;
}) {
  const sliderId = useId();
  const fileInput = useRef<HTMLInputElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const bitmap = useRef<ImageBitmap | null>(null);
  const pending = useRef<AbortController | null>(null);
  const pan = useRef<Pan | null>(null);
  const [image, setImage] = useState<ImageBitmap>();
  const [crop, setCrop] = useState<AvatarCrop>(DEFAULT_AVATAR_CROP);
  // Each opening owns its draft; parent refreshes must not reset an active crop.
  const [initialPhoto] = useState(() => loadPhoto);
  const [loading, setLoading] = useState(!!initialPhoto);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<unknown>();
  const [imageError, setImageError] = useState("");

  const load = useCallback(
    async (read: (signal: AbortSignal) => Blob | Promise<Blob>) => {
      pending.current?.abort();
      const controller = new AbortController();
      pending.current = controller;
      const { signal } = controller;
      setLoading(true);
      setError(undefined);
      setImageError("");
      try {
        const source = await read(signal);
        let next: ImageBitmap;
        try {
          next = await decodeAvatar(source, signal);
        } catch (error) {
          if (!signal.aborted)
            setImageError(
              error instanceof Error && error.name === "Error"
                ? error.message
                : "Не удалось открыть изображение. Выберите другое фото.",
            );
          return;
        }
        if (signal.aborted) {
          next.close();
          return;
        }
        bitmap.current?.close();
        bitmap.current = next;
        setImage(next);
        setCrop(DEFAULT_AVATAR_CROP);
      } catch (error) {
        if (!signal.aborted) setError(error);
      } finally {
        if (!signal.aborted) {
          pending.current = null;
          setLoading(false);
        }
      }
    },
    [],
  );
  useEffect(() => {
    if (initialPhoto) void load(initialPhoto);
    return () => {
      pending.current?.abort();
      bitmap.current?.close();
      bitmap.current = null;
    };
  }, [load, initialPhoto]);

  const bounds = image && avatarCropBounds(image.width, image.height, crop);
  const travelX = image && bounds ? image.width - bounds.size : 0;
  const travelY = image && bounds ? image.height - bounds.size : 0;
  const canPan = !loading && !saving && (travelX > 0 || travelY > 0);

  useEffect(() => {
    if (!image) return;
    const context = canvas.current?.getContext("2d", { alpha: false });
    if (!context) return;
    const { x, y, size } = avatarCropBounds(image.width, image.height, crop);
    context.fillStyle = "#fff";
    context.fillRect(0, 0, previewSize, previewSize);
    context.drawImage(image, x, y, size, size, 0, 0, previewSize, previewSize);
  }, [crop, image]);

  function startPan(event: PointerEvent<HTMLDivElement>) {
    if (!canPan || !bounds || !event.isPrimary || event.button !== 0) return;
    const width = event.currentTarget.getBoundingClientRect().width;
    if (!width) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    const movement = (200 * bounds.size) / width;
    pan.current = {
      pointerId: event.pointerId,
      clientX: event.clientX,
      clientY: event.clientY,
      crop,
      xPerPixel: travelX ? movement / travelX : 0,
      yPerPixel: travelY ? movement / travelY : 0,
    };
  }

  function movePan(event: PointerEvent<HTMLDivElement>) {
    const start = pan.current;
    if (!canPan || !start || start.pointerId !== event.pointerId) return;
    setCrop({
      ...start.crop,
      x: positionPercent(
        start.crop.x + (event.clientX - start.clientX) * start.xPerPixel,
      ),
      y: positionPercent(
        start.crop.y + (event.clientY - start.clientY) * start.yPerPixel,
      ),
    });
  }

  async function save() {
    if (!image || pending.current) return;
    const controller = new AbortController();
    pending.current = controller;
    const { signal } = controller;
    setSaving(true);
    setError(undefined);
    setImageError("");
    try {
      const file = await encodeAvatar(image, crop, signal);
      await onSave(file, signal);
      if (!signal.aborted) onSaved();
    } catch (error) {
      if (!signal.aborted) setError(error);
    } finally {
      if (!signal.aborted) {
        pending.current = null;
        setSaving(false);
      }
    }
  }

  return (
    <Modal
      title={title}
      className="avatar-crop-dialog"
      onClose={() => {
        if (!saving) onClose();
      }}
    >
      <div className="avatar-crop-grid">
        <div className="avatar-crop-preview">
          <div
            className={
              canPan ? "avatar-crop-stage is-pannable" : "avatar-crop-stage"
            }
            onPointerDown={startPan}
            onPointerMove={movePan}
            onLostPointerCapture={() => {
              pan.current = null;
            }}
            onDragOver={(event) => {
              if (event.dataTransfer.types.includes("Files"))
                event.preventDefault();
            }}
            onDrop={(event) => {
              event.preventDefault();
              const file = event.dataTransfer.files[0];
              if (file && !saving) void load(() => file);
            }}
          >
            {image ? (
              <canvas
                ref={canvas}
                width={previewSize}
                height={previewSize}
                role="img"
                aria-label={`Предпросмотр: ${title.toLocaleLowerCase("ru")}`}
              />
            ) : (
              <div className="avatar-crop-empty">
                <ImagePlus size={32} aria-hidden="true" />
                {loading ? "Открываем фото…" : "Выберите или перетащите фото"}
              </div>
            )}
          </div>
          {image && (
            <p className="avatar-crop-hint">
              Перетащите фото или используйте ползунки.
            </p>
          )}
        </div>
        <div className="avatar-crop-controls" aria-busy={loading || saving}>
          {image &&
            sliders.map(({ key, label, min, max }) => {
              const unavailable =
                (key === "x" && !travelX) || (key === "y" && !travelY);
              const value = unavailable ? 0 : crop[key];
              return (
                <label
                  className="avatar-crop-slider"
                  htmlFor={`${sliderId}-${key}`}
                  key={key}
                >
                  <span className="avatar-crop-slider-label">
                    {label}
                    <output aria-hidden="true">{value}%</output>
                  </span>
                  <input
                    id={`${sliderId}-${key}`}
                    type="range"
                    min={min}
                    max={max}
                    value={value}
                    aria-valuetext={`${value}%`}
                    disabled={loading || saving || unavailable}
                    onChange={(event) =>
                      setCrop({ ...crop, [key]: Number(event.target.value) })
                    }
                  />
                </label>
              );
            })}
          <div className="avatar-crop-tools">
            {image && (
              <Button
                type="button"
                variant="outline"
                disabled={loading || saving}
                onClick={() => setCrop(DEFAULT_AVATAR_CROP)}
              >
                Сбросить
              </Button>
            )}
            <Button
              type="button"
              variant="secondary"
              className="avatar-crop-replace"
              disabled={saving}
              onClick={() => fileInput.current?.click()}
            >
              <Upload size={18} aria-hidden="true" />
              {image ? "Заменить фото" : "Выбрать фото"}
            </Button>
            {!image && (
              <p className="avatar-crop-hint">PNG, JPEG или WebP до 10 МБ.</p>
            )}
            <input
              ref={fileInput}
              hidden
              type="file"
              accept="image/png,image/jpeg,image/webp"
              aria-label="Файл фотографии"
              disabled={saving}
              onChange={(event) => {
                const file = event.target.files?.[0];
                event.target.value = "";
                if (file) void load(() => file);
              }}
            />
          </div>
          {(error !== undefined || imageError) && (
            <div className="avatar-crop-error">
              <ErrorNotice error={error}>{imageError || null}</ErrorNotice>
            </div>
          )}
        </div>
      </div>
      <footer className="avatar-crop-footer">
        <Button
          type="button"
          variant="outline"
          disabled={saving}
          onClick={onClose}
        >
          Отмена
        </Button>
        <Button
          type="button"
          busy={saving}
          disabled={!image || loading}
          onClick={() => void save()}
        >
          <Check size={18} aria-hidden="true" />
          Сохранить фото
        </Button>
      </footer>
    </Modal>
  );
}
