import { useEffect, useId, useRef, useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import {
  Download,
  FileText,
  Image,
  Link2,
  MessageCircle,
  Play,
} from "lucide-react";
import { api, ApiError, personalChatAPI } from "../api";
import { useAuth } from "../auth";
import { formatBytes } from "../collaboration";
import {
  materialMonth,
  sharedMaterialURL,
  visibleConversationMaterials,
} from "../conversationMaterials";
import { safeRecordingUrl } from "../recordingPresentation";
import type {
  ChatAttachment,
  ConferenceChatMaterial,
  PersonalConversation,
} from "../types";
import { ChatFileIcon } from "./ChatFileIcon";
import { Button, ErrorNotice, Loading, Modal } from "./ui";
import "./conversation-materials.css";

type MaterialKind = "media" | "file" | "link";
const tabs = [
  { kind: "media", label: "Медиа", empty: "В этом чате пока нет медиа." },
  { kind: "file", label: "Файлы", empty: "В этом чате пока нет файлов." },
  { kind: "link", label: "Ссылки", empty: "В этом чате пока нет ссылок." },
] as const;

/** Материалы запрашиваются отдельно от ленты и включают доступную историю переписки. */
export function ConversationMaterials({
  conversation,
  onMessage,
  onAccessDenied,
}: {
  conversation: PersonalConversation;
  onMessage: (id: string) => void;
  onAccessDenied: () => void;
}) {
  const { user } = useAuth();
  return (
    <MaterialsContent
      key={`${user?.id}:${conversation.id}:${conversation.type === "direct" ? conversation.historyClearedThrough || 0 : 0}`}
      conversation={conversation}
      onMessage={onMessage}
      onAccessDenied={onAccessDenied}
    />
  );
}

function MaterialsContent({
  conversation,
  onMessage,
  onAccessDenied,
}: Parameters<typeof ConversationMaterials>[0]) {
  const { user } = useAuth();
  const id = useId();
  const [kind, setKind] = useState<MaterialKind>("media");
  const [preview, setPreview] = useState<ConferenceChatMaterial>();
  const tabButtons = useRef<Array<HTMLButtonElement | null>>([]);
  const query = useInfiniteQuery({
    queryKey: ["personal-materials", conversation.id, user?.id, kind],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      api.personalMaterials(conversation.id, kind, pageParam, signal),
    getNextPageParam: (page) => page.nextCursor || undefined,
    retry: false,
    enabled: !!user,
  });
  const denied =
    query.error instanceof ApiError && [403, 404].includes(query.error.status);
  const accessDenied = useRef(onAccessDenied);
  accessDenied.current = onAccessDenied;
  useEffect(() => {
    if (denied) accessDenied.current();
  }, [denied]);
  const items = visibleConversationMaterials(
    query.data?.pages || [],
    conversation.type === "direct"
      ? conversation.historyClearedThrough || 0
      : 0,
  );
  const months = new Map<string, ConferenceChatMaterial[]>();
  for (const item of items) {
    const month = materialMonth(item.message.createdAt);
    if (!months.has(month)) months.set(month, []);
    months.get(month)!.push(item);
  }
  const previewVisible =
    preview &&
    items.some(
      (item) =>
        item.message.id === preview.message.id &&
        item.attachment?.id === preview.attachment?.id,
    );
  if (denied) return null;
  return (
    <section
      className="conversation-materials"
      aria-label="Материалы переписки"
    >
      <div
        className="conversation-material-tabs"
        role="tablist"
        aria-label="Материалы переписки"
      >
        {tabs.map((tab, index) => (
          <button
            key={tab.kind}
            ref={(element) => {
              tabButtons.current[index] = element;
            }}
            type="button"
            role="tab"
            id={`${id}-${tab.kind}`}
            aria-selected={kind === tab.kind}
            aria-controls={`${id}-panel`}
            tabIndex={kind === tab.kind ? 0 : -1}
            onClick={() => {
              setPreview(undefined);
              setKind(tab.kind);
            }}
            onKeyDown={(event) => {
              let next: number;
              if (event.key === "ArrowRight") next = (index + 1) % tabs.length;
              else if (event.key === "ArrowLeft")
                next = (index + tabs.length - 1) % tabs.length;
              else if (event.key === "Home") next = 0;
              else if (event.key === "End") next = tabs.length - 1;
              else return;
              event.preventDefault();
              setPreview(undefined);
              setKind(tabs[next].kind);
              tabButtons.current[next]?.focus();
            }}
          >
            {tab.label}
          </button>
        ))}
      </div>
      <div
        id={`${id}-panel`}
        className="conversation-material-panel"
        role="tabpanel"
        aria-labelledby={`${id}-${kind}`}
        aria-busy={query.isFetching}
        tabIndex={0}
      >
        <ErrorNotice error={query.error} />
        {query.isPending && <Loading />}
        {!query.isPending && !query.isError && !items.length && (
          <div className="conversation-material-empty">
            {kind === "media" ? (
              <Image size={30} aria-hidden="true" />
            ) : kind === "file" ? (
              <FileText size={30} aria-hidden="true" />
            ) : (
              <Link2 size={30} aria-hidden="true" />
            )}
            <p>{tabs.find((tab) => tab.kind === kind)!.empty}</p>
          </div>
        )}
        {[...months].map(([month, materials]) => (
          <section
            className="conversation-material-month"
            key={month}
            aria-label={month}
          >
            <h3>{month}</h3>
            <ul
              className={
                kind === "media"
                  ? "conversation-media-grid"
                  : "conversation-material-list"
              }
            >
              {materials.map((item) => (
                <li
                  key={`${item.message.id}:${item.attachment?.id || item.url}`}
                >
                  {kind === "media" && item.attachment ? (
                    <button
                      className="conversation-media-tile"
                      type="button"
                      onClick={() => setPreview(item)}
                      aria-label={`Открыть ${item.attachment.filename}`}
                    >
                      {item.attachment.mimeType.startsWith("video/") ? (
                        <span className="conversation-video-icon">
                          <Play size={30} aria-hidden="true" />
                        </span>
                      ) : (
                        <ChatFileIcon
                          file={item.attachment}
                          scopeId={conversation.id}
                          transport={personalChatAPI}
                          authenticated={conversation.type === "group"}
                          visible={!!user}
                          tile
                        />
                      )}
                      <span className="conversation-media-caption">
                        {item.attachment.filename}
                      </span>
                    </button>
                  ) : item.attachment ? (
                    <div className="conversation-material-file">
                      <button
                        type="button"
                        className="conversation-material-message"
                        onClick={() => onMessage(item.message.id)}
                        aria-label={`Показать в чате: ${item.attachment.filename}`}
                      >
                        <span className="conversation-material-file-icon">
                          <FileText size={24} aria-hidden="true" />
                        </span>
                        <span className="conversation-material-copy">
                          <strong>{item.attachment.filename}</strong>
                          <small>
                            {formatBytes(item.attachment.size)} ·{" "}
                            {item.message.senderName}
                          </small>
                        </span>
                      </button>
                      <MaterialDownload
                        conversation={conversation}
                        file={item.attachment}
                        onAccessDenied={onAccessDenied}
                      />
                    </div>
                  ) : item.url && sharedMaterialURL(item.url) ? (
                    <div className="conversation-material-link">
                      <span className="conversation-material-file-icon">
                        <Link2 size={23} aria-hidden="true" />
                      </span>
                      <span className="conversation-material-copy">
                        <a
                          href={item.url}
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          {item.url}
                        </a>
                        <small>{item.message.senderName}</small>
                      </span>
                      <button
                        type="button"
                        className="icon-button"
                        onClick={() => onMessage(item.message.id)}
                        aria-label={`Показать в чате: ${item.url}`}
                        title="Показать в чате"
                      >
                        <MessageCircle size={19} aria-hidden="true" />
                      </button>
                    </div>
                  ) : null}
                </li>
              ))}
            </ul>
          </section>
        ))}
        {query.isError && (
          <Button
            variant="outline"
            onClick={() =>
              void (query.isFetchNextPageError
                ? query.fetchNextPage()
                : query.refetch())
            }
          >
            Повторить загрузку
          </Button>
        )}
        {query.hasNextPage && !query.isError && (
          <Button
            className="conversation-material-more"
            variant="outline"
            busy={query.isFetchingNextPage}
            onClick={() => void query.fetchNextPage()}
          >
            Показать ещё
          </Button>
        )}
      </div>
      {previewVisible && preview.attachment && (
        <MaterialPreview
          key={preview.attachment.id}
          conversation={conversation}
          item={preview}
          onClose={() => setPreview(undefined)}
          onMessage={() => {
            setPreview(undefined);
            onMessage(preview.message.id);
          }}
          onAccessDenied={onAccessDenied}
        />
      )}
    </section>
  );
}

/** Проверяет серверную ссылку перед получением приватного файла или открытием подписанного URL. */
async function materialDownload(
  conversation: PersonalConversation,
  file: ChatAttachment,
  signal: AbortSignal,
) {
  const result = await personalChatAPI.attachmentDownload(
    conversation.id,
    file.id,
    signal,
  );
  signal.throwIfAborted();
  const safeURL = safeRecordingUrl(result.url);
  if (!safeURL) throw new Error("Не удалось получить ссылку на файл.");
  const url = new URL(safeURL, window.location.origin);
  if (conversation.type === "group" && result.authenticated !== true)
    throw new Error("Не удалось подтвердить доступ к файлу группы.");
  if (result.authenticated) {
    const path = `/api/v1/conversations/${encodeURIComponent(conversation.id)}/attachments/${encodeURIComponent(file.id)}/content`;
    if (
      url.origin !== window.location.origin ||
      url.pathname !== path ||
      url.search ||
      url.hash
    )
      throw new Error("Не удалось получить ссылку на файл.");
    const blob = await personalChatAPI.attachmentContent(
      conversation.id,
      file.id,
      signal,
    );
    signal.throwIfAborted();
    return URL.createObjectURL(blob);
  }
  return url.toString();
}

function MaterialDownload({
  conversation,
  file,
  onAccessDenied,
}: {
  conversation: PersonalConversation;
  file: ChatAttachment;
  onAccessDenied: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const controller = useRef<AbortController | undefined>(undefined);
  const objectURLs = useRef(new Map<string, ReturnType<typeof setTimeout>>());
  useEffect(
    () => () => {
      controller.current?.abort();
      objectURLs.current.forEach((timer, url) => {
        clearTimeout(timer);
        URL.revokeObjectURL(url);
      });
      objectURLs.current.clear();
    },
    [],
  );
  const download = async () => {
    if (busy) return;
    controller.current?.abort();
    const current = new AbortController();
    controller.current = current;
    setBusy(true);
    setError(undefined);
    try {
      const url = await materialDownload(conversation, file, current.signal);
      if (current.signal.aborted && url.startsWith("blob:"))
        URL.revokeObjectURL(url);
      current.signal.throwIfAborted();
      if (url.startsWith("blob:"))
        objectURLs.current.set(
          url,
          setTimeout(() => {
            URL.revokeObjectURL(url);
            objectURLs.current.delete(url);
          }, 60_000),
        );
      const link = document.createElement("a");
      link.href = url;
      link.download = file.filename;
      link.rel = "noopener noreferrer";
      link.target = "_blank";
      document.body.append(link);
      link.click();
      link.remove();
    } catch (failure) {
      if (current.signal.aborted) return;
      if (failure instanceof ApiError && failure.status === 403)
        onAccessDenied();
      setError(failure);
    } finally {
      if (!current.signal.aborted) setBusy(false);
    }
  };
  return (
    <div className="conversation-material-download">
      <button
        type="button"
        className="icon-button"
        aria-label={`Скачать ${file.filename}`}
        title="Скачать файл"
        disabled={busy}
        aria-busy={busy}
        onClick={() => void download()}
      >
        <Download size={20} aria-hidden="true" />
      </button>
      <ErrorNotice error={error} />
    </div>
  );
}

function MaterialPreview({
  conversation,
  item,
  onClose,
  onMessage,
  onAccessDenied,
}: {
  conversation: PersonalConversation;
  item: ConferenceChatMaterial;
  onClose: () => void;
  onMessage: () => void;
  onAccessDenied: () => void;
}) {
  const file = item.attachment!;
  const [url, setURL] = useState<string>();
  const [error, setError] = useState<unknown>();
  const accessDenied = useRef(onAccessDenied);
  accessDenied.current = onAccessDenied;
  const video = ["video/mp4", "video/webm", "video/ogg"].includes(
    file.mimeType,
  );
  const image = ["image/jpeg", "image/png", "image/webp"].includes(
    file.mimeType,
  );
  useEffect(() => {
    if (!video && !image) return;
    const controller = new AbortController();
    let objectURL: string | undefined;
    void materialDownload(conversation, file, controller.signal)
      .then((url) => {
        if (controller.signal.aborted) {
          if (url.startsWith("blob:")) URL.revokeObjectURL(url);
          return;
        }
        objectURL = url.startsWith("blob:") ? url : undefined;
        setURL(url);
      })
      .catch((failure) => {
        if (controller.signal.aborted) return;
        if (failure instanceof ApiError && failure.status === 403)
          accessDenied.current();
        setError(failure);
      });
    return () => {
      controller.abort();
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [
    conversation.id,
    conversation.type,
    file.id,
    file.mimeType,
    image,
    video,
  ]);
  return (
    <Modal
      title={file.filename}
      className="conversation-media-preview"
      onClose={onClose}
    >
      <ErrorNotice error={error} />
      {!url && !error && (video || image) && <Loading />}
      {url && video && (
        <video
          src={url}
          controls
          playsInline
          preload="metadata"
          onError={() =>
            setError(
              new Error("Не удалось воспроизвести видео. Можно скачать файл."),
            )
          }
        />
      )}
      {url && image && (
        <img
          src={url}
          alt={file.filename}
          referrerPolicy="no-referrer"
          onError={() =>
            setError(
              new Error("Не удалось открыть изображение. Можно скачать файл."),
            )
          }
        />
      )}
      {!video && !image && (
        <p className="muted">Предпросмотр этого файла недоступен.</p>
      )}
      <div className="conversation-media-preview-actions">
        <Button variant="outline" onClick={onMessage}>
          <MessageCircle size={18} aria-hidden="true" />
          Показать в чате
        </Button>
        <MaterialDownload
          conversation={conversation}
          file={file}
          onAccessDenied={onAccessDenied}
        />
      </div>
    </Modal>
  );
}
