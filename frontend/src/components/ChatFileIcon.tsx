import { useEffect, useRef, useState } from "react";
import { FileText } from "lucide-react";
import { canPreviewChatAttachment, type ChatTransport } from "../api";
import { loadPrivateImage } from "../groupAvatarLoader";
import type { ChatAttachment } from "../types";

async function thumbnail(source: Blob, pixels: number, signal: AbortSignal) {
  signal.throwIfAborted();
  const image = await createImageBitmap(source, {
    imageOrientation: "from-image",
  });
  const canvas = document.createElement("canvas");
  canvas.width = canvas.height = pixels;
  try {
    signal.throwIfAborted();
    if (image.width * image.height > 40_000_000)
      throw new Error("Image dimensions exceed the preview limit");
    const size = Math.min(image.width, image.height);
    const context = canvas.getContext("2d");
    if (!context) throw new Error("Canvas is unavailable");
    context.drawImage(
      image,
      (image.width - size) / 2,
      (image.height - size) / 2,
      size,
      size,
      0,
      0,
      canvas.width,
      canvas.height,
    );
  } finally {
    image.close();
  }
  return new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob) resolve(blob);
      else reject(new Error("Could not encode attachment preview"));
    });
  });
}

export function ChatFileIcon({
  file,
  scopeId,
  transport,
  authenticated,
  visible,
  tile = false,
}: {
  file: ChatAttachment;
  scopeId: string;
  transport: ChatTransport;
  authenticated: boolean;
  visible: boolean;
  tile?: boolean;
}) {
  const icon = useRef<HTMLSpanElement>(null);
  const [url, setURL] = useState<string>();
  const previewable = canPreviewChatAttachment(file);
  const { id, mimeType, size, status } = file;

  useEffect(() => {
    const element = icon.current;
    if (
      !previewable ||
      !visible ||
      !element ||
      typeof IntersectionObserver === "undefined" ||
      typeof createImageBitmap === "undefined"
    )
      return;
    const controller = new AbortController();
    const { signal } = controller;
    let objectURL: string | undefined;
    const load = () => {
      void loadPrivateImage(async () => {
        const source = await transport.attachmentPreview(
          scopeId,
          { id, mimeType, size, status },
          authenticated,
          signal,
        );
        return thumbnail(source, tile ? 192 : 96, signal);
      }, signal)
        .then((blob) => {
          if (signal.aborted) return;
          objectURL = URL.createObjectURL(blob);
          setURL(objectURL);
        })
        .catch(() => {});
    };
    const observer = new IntersectionObserver((entries) => {
      if (signal.aborted || !entries.some((entry) => entry.isIntersecting))
        return;
      observer.disconnect();
      load();
    });
    observer.observe(element);
    return () => {
      observer.disconnect();
      controller.abort();
      if (objectURL) URL.revokeObjectURL(objectURL);
      setURL(undefined);
    };
  }, [
    authenticated,
    id,
    mimeType,
    previewable,
    scopeId,
    size,
    status,
    tile,
    transport,
    visible,
  ]);

  return (
    <span
      ref={icon}
      className={`chat-file-icon${previewable ? " chat-file-preview" : ""}`}
      aria-hidden="true"
    >
      {url ? (
        <img src={url} alt="" onError={() => setURL(undefined)} />
      ) : (
        <FileText size={24} />
      )}
    </span>
  );
}
