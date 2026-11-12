export const MAX_AVATAR_SOURCE_BYTES = 10 * 1024 * 1024;

export type AvatarCrop = { zoom: number; x: number; y: number };

export const DEFAULT_AVATAR_CROP: Readonly<AvatarCrop> = {
  zoom: 100,
  x: 0,
  y: 0,
};

/** Compute the square in source pixels. Positive offsets move the photo right or down. */
export function avatarCropBounds(
  width: number,
  height: number,
  { zoom, x, y }: AvatarCrop,
) {
  const scale = Math.max(100, Math.min(300, zoom)) / 100;
  const size = Math.min(width, height) / scale;
  return {
    x: ((width - size) * (100 - Math.max(-100, Math.min(100, x)))) / 200,
    y: ((height - size) * (100 - Math.max(-100, Math.min(100, y)))) / 200,
    size,
  };
}

/** Validate a source photo before decoding; SVG and GIF are excluded. */
export function avatarSourceError(
  file: Pick<File, "size" | "type">,
): string | undefined {
  if (!file.size || file.size > MAX_AVATAR_SOURCE_BYTES)
    return "Выберите изображение размером до 10 МБ.";
  if (!["image/png", "image/jpeg", "image/webp"].includes(file.type))
    return "Для аватара подходят PNG, JPEG и WebP.";
}

/** Decode the oriented photo. The caller owns and closes the returned bitmap. */
export async function decodeAvatar(
  file: Blob,
  signal: AbortSignal,
): Promise<ImageBitmap> {
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
    return image;
  } catch (error) {
    image.close();
    throw error;
  }
}

/** Export the selected crop as a metadata-free square JPEG. */
export async function encodeAvatar(
  image: ImageBitmap,
  crop: AvatarCrop,
  signal: AbortSignal,
): Promise<File> {
  signal.throwIfAborted();
  const bounds = avatarCropBounds(image.width, image.height, crop);
  const size = Math.max(1, Math.min(Math.floor(bounds.size), 512));
  const canvas = document.createElement("canvas");
  canvas.width = canvas.height = size;
  const context = canvas.getContext("2d", { alpha: false });
  if (!context) throw new Error("Не удалось подготовить изображение.");
  context.fillStyle = "#ffffff";
  context.fillRect(0, 0, size, size);
  context.drawImage(
    image,
    bounds.x,
    bounds.y,
    bounds.size,
    bounds.size,
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
}

/** Keep the centered crop for the group avatar picker. */
export async function prepareAvatar(
  file: File,
  signal: AbortSignal,
): Promise<File> {
  const image = await decodeAvatar(file, signal);
  try {
    return await encodeAvatar(image, DEFAULT_AVATAR_CROP, signal);
  } finally {
    image.close();
  }
}
