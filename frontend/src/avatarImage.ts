export const MAX_AVATAR_SOURCE_BYTES = 10 * 1024 * 1024;

/** Validate a source photo before decoding; SVG and GIF are excluded. */
export function avatarSourceError(
  file: Pick<File, "size" | "type">,
): string | undefined {
  if (!file.size || file.size > MAX_AVATAR_SOURCE_BYTES)
    return "Выберите изображение размером до 10 МБ.";
  if (!["image/png", "image/jpeg", "image/webp"].includes(file.type))
    return "Для аватара подходят PNG, JPEG и WebP.";
}

/** Crop file to a bounded square JPEG; signal cancels stale selection results. */
export async function prepareAvatar(
  file: File,
  signal: AbortSignal,
): Promise<File> {
  const error = avatarSourceError(file);
  if (error) throw new Error(error);
  signal.throwIfAborted();
  const image = await createImageBitmap(file, {
    imageOrientation: "from-image",
  });
  try {
    signal.throwIfAborted();
    if (
      !image.width ||
      !image.height ||
      image.width * image.height > 40_000_000
    )
      throw new Error(
        "Изображение слишком большое. Выберите фото до 40 мегапикселей.",
      );
    const side = Math.min(image.width, image.height);
    const size = Math.min(side, 512);
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = size;
    const context = canvas.getContext("2d", { alpha: false });
    if (!context) throw new Error("Не удалось подготовить изображение.");
    context.fillStyle = "#ffffff";
    context.fillRect(0, 0, size, size);
    context.drawImage(
      image,
      (image.width - side) / 2,
      (image.height - side) / 2,
      side,
      side,
      0,
      0,
      size,
      size,
    );
    const blob = await new Promise<Blob>((resolve, reject) =>
      canvas.toBlob(
        (value) =>
          value
            ? resolve(value)
            : reject(new Error("Не удалось обработать фото.")),
        "image/jpeg",
        0.85,
      ),
    );
    signal.throwIfAborted();
    if (blob.size > 2 * 1024 * 1024)
      throw new Error("Не удалось уменьшить фото до 2 МБ.");
    return new File([blob], "avatar.jpg", { type: "image/jpeg" });
  } finally {
    image.close();
  }
}
