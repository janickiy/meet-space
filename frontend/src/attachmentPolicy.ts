export const attachmentLimit = 10 * 1024 * 1024;

/** Allowed extensions mirror the server policy; server-side content checks remain authoritative. */
export const attachmentExtensions = [
  "jpg",
  "jpeg",
  "png",
  "webp",
  "pdf",
  "txt",
  "csv",
  "tsv",
  "md",
  "markdown",
  "rtf",
  "doc",
  "dot",
  "docx",
  "dotx",
  "docm",
  "dotm",
  "xls",
  "xlt",
  "xlsx",
  "xltx",
  "xlsm",
  "xlsb",
  "xltm",
  "ppt",
  "pot",
  "pps",
  "pptx",
  "potx",
  "ppsx",
  "pptm",
  "potm",
  "ppsm",
  "odt",
  "ods",
  "odp",
  "odg",
  "odf",
  "ott",
  "ots",
  "otp",
  "otg",
  "pages",
  "numbers",
  "key",
  "epub",
  "zip",
  "zipx",
  "7z",
  "rar",
  "tar",
  "gz",
  "gzip",
  "tgz",
  "bz2",
  "bzip2",
  "tbz",
  "tbz2",
  "xz",
  "txz",
  "zst",
  "zstd",
  "tzst",
  "lz4",
  "lz",
  "lzip",
  "lzma",
  "z",
  "taz",
  "cab",
  "cpio",
  "ar",
  "arj",
  "lha",
  "lzh",
] as const;

export const attachmentAccept = attachmentExtensions
  .map((ext) => `.${ext}`)
  .join(",");
const allowedExtensions = new Set<string>(attachmentExtensions);

/**
 * Validate an attachment before uploading without trusting its browser-supplied MIME type.
 * @param file Selected file name and size in bytes.
 * @returns A user-facing validation error, or null when the file can be uploaded.
 */
export function validateAttachment(
  file: Pick<File, "name" | "size">,
): string | null {
  if (
    !Number.isFinite(file.size) ||
    file.size < 1 ||
    file.size > attachmentLimit
  )
    return "Размер файла должен быть от 1 байта до 10 МБ.";
  const extension = file.name
    .slice(file.name.lastIndexOf(".") + 1)
    .toLowerCase();
  if (!file.name.includes(".") || !allowedExtensions.has(extension))
    return "Можно прикрепить изображения, PDF, офисные и текстовые документы, а также архивы.";
  return null;
}
