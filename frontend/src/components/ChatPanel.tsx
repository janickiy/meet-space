import { Fragment, useEffect, useRef, useState } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query";
import {
  Check,
  CheckSquare,
  Download,
  MessageCircle,
  Paperclip,
  Pencil,
  Pin,
  PinOff,
  Reply,
  Forward,
  SendHorizontal,
  Star,
  Trash2,
  X,
} from "lucide-react";
import {
  api,
  ApiError,
  canPreviewChatAttachment,
  errorMessage,
  type ChatTransport,
} from "../api";
import { useAuth } from "../auth";
import { useAppearance } from "../appearance";
import { mergeChatPages, formatBytes } from "../collaboration";
import { deleteChatMessages } from "../chatDeletion";
import {
  chatDayKey,
  chatDayLabel,
  chatTime,
  insertChatEmoji,
} from "../chatPresentation";
import { formatDate, initials } from "../utils";
import type {
  ChatAttachment,
  ChatMessage,
  ChatPage,
  Participant,
} from "../types";
import { AttachmentUploader } from "./AttachmentUploader";
import { ChatFileIcon } from "./ChatFileIcon";
import { EmojiPicker } from "./EmojiPicker";
import { ForwardMessagesDialog } from "./ForwardMessagesDialog";
import { ItemActions, type ItemAction } from "./ItemActions";
import { Button, ErrorNotice, Modal } from "./ui";
import { MessagingSkeleton } from "./MessagingSkeleton";
import { CallHistoryMessage } from "./CallHistoryMessage";
import { UserAvatar } from "./UserAvatar";
import { PersonalChatPins, usePersonalChatPins } from "./PersonalChatPins";
import "./chat-design.css";
import "./personal-messenger.css";
import "./conference-messenger.css";
import "./chat-selection.css";

function isAccessDenied(error: unknown) {
  return error instanceof ApiError && [403, 404].includes(error.status);
}

/**
 * Показывает историю чата и управляет отправкой, ответами, вложениями и правами на изменение сообщений.
 * @args conferenceId — идентификатор встречи; membership — роль текущего участника;
 * readOnly — запрет изменений; readOnlyReason — пояснение режима чтения.
 * @return Панель сообщений с редактором и диалогами изменения и удаления.
 */
export function ChatPanel({
  conferenceId,
  membership,
  readOnly,
  readOnlyReason,
  focusMessageId,
  onLatest,
}: {
  conferenceId: string;
  membership: Participant;
  readOnly: boolean;
  readOnlyReason?: string;
  focusMessageId?: string;
  onLatest?: () => void;
}) {
  return (
    <MessageThread
      scopeId={conferenceId}
      transport={api}
      mayModerate={["owner", "co_host"].includes(membership.role)}
      readOnly={readOnly}
      readOnlyReason={readOnlyReason}
      focusMessageId={focusMessageId}
      onLatest={onLatest}
    />
  );
}

export function MessageThread({
  scopeId,
  transport,
  mayModerate = false,
  readOnly = false,
  readOnlyReason,
  personal = false,
  composerPlaceholder = "Напишите сообщение…",
  requireAuthenticatedDownloads = false,
  onAccessDenied,
  focusMessageId,
  onLatest,
  visible = true,
  focusRequest = 0,
  onFocusMessage,
}: {
  scopeId: string;
  transport: ChatTransport;
  mayModerate?: boolean;
  readOnly?: boolean;
  readOnlyReason?: string;
  personal?: boolean;
  composerPlaceholder?: string;
  requireAuthenticatedDownloads?: boolean;
  onAccessDenied?: () => void;
  focusMessageId?: string;
  onLatest?: () => void;
  /** Скрытая боковая панель сохраняет черновик без измерения или маркировки прочитанных данных. */
  visible?: boolean;
  /** При повторном выборе можно вернуться к тому же сообщению, не удаляя черновик. */
  focusRequest?: number;
  onFocusMessage?: (messageId: string) => void;
}) {
  const historyKey = personal ? "personal-chat" : "chat";
  const readKey = personal ? "personal-chat-read" : "chat-read";
  const { user } = useAuth();
  const { textSize } = useAppearance();
  const mayBookmark = !personal && !!user && !user.guestConferenceId;
  const mayForward = !!user && !user.guestConferenceId;
  const focused = focusMessageId || undefined;
  const client = useQueryClient();
  const [open, setOpen] = useState(true);
  const [text, setText] = useState("");
  const [reply, setReply] = useState<ChatMessage | null>(null);
  const [editing, setEditing] = useState<ChatMessage | null>(null);
  const [deleting, setDeleting] = useState<ChatMessage | null>(null);
  const [forwardingMessages, setForwardingMessages] = useState<
    ChatMessage[] | null
  >(null);
  const [selectingMessages, setSelectingMessages] = useState(false);
  const [selectedMessages, setSelectedMessages] = useState<Set<string>>(
    new Set(),
  );
  const [confirmSelection, setConfirmSelection] = useState(false);
  const [selectionNotice, setSelectionNotice] = useState("");
  const [selectionError, setSelectionError] = useState("");
  const [deleteProgress, setDeleteProgress] = useState({ done: 0, total: 0 });
  const bulkController = useRef<AbortController | null>(null);
  const selectionToolbar = useRef<HTMLDivElement>(null);
  const wasSelectingMessages = useRef(false);
  const [editText, setEditText] = useState("");
  const [attachments, setAttachments] = useState<ChatAttachment[]>([]);
  const [uploadBusy, setUploadBusy] = useState(false);
  const [composerGeneration, setComposerGeneration] = useState(0);
  const [uploadDetailsTarget, setUploadDetailsTarget] =
    useState<HTMLDivElement | null>(null);
  const [validation, setValidation] = useState("");
  const [downloadError, setDownloadError] = useState("");
  const [download, setDownload] = useState<{
    id: string;
    url: string;
    expiresAt: string;
  } | null>(null);
  const composer = useRef<HTMLTextAreaElement>(null);
  const selection = useRef({ start: 0, end: 0 });
  const pendingCaret = useRef<number | null>(null);
  const retryRequest = useRef<{ signature: string; id: string } | null>(null);
  const sendActive = useRef(false);
  const mounted = useRef(true);
  const downloadController = useRef<AbortController | null>(null);
  const [pending, setPending] = useState<{
    clientRequestId: string;
    text: string;
    attachments: ChatAttachment[];
    replyName?: string;
    createdAt: string;
    state: "sending" | "failed";
  } | null>(null);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      downloadController.current?.abort();
    };
  }, []);
  const viewport = useRef<HTMLDivElement>(null);
  const lastMarker = useRef<HTMLSpanElement>(null);
  const markerVisible = useRef(false);
  const initialScroll = useRef(false);
  const previousLatest = useRef<string | undefined>(undefined);
  const olderAnchor = useRef<{ height: number; top: number } | null>(null);
  const readingAnchor = useRef<{ id: string; offset: number } | null>(null);
  const [hasNewMessages, setHasNewMessages] = useState(false);
  const newestVisible = useRef(false);
  const [readCandidate, setReadCandidate] = useState<string | null>(null);
  const lastRead = useRef<string | null>(null);

  // Ссылка на приватный файл перестаёт отображаться немного раньше её серверного срока действия.
  useEffect(() => {
    if (!download) return;
    const timer = setTimeout(
      () => setDownload(null),
      Math.max(0, Date.parse(download.expiresAt) - Date.now() - 5000),
    );
    return () => {
      clearTimeout(timer);
      if (download.url.startsWith("blob:")) URL.revokeObjectURL(download.url);
    };
  }, [download]);

  const query = useInfiniteQuery({
    queryKey: focused
      ? [historyKey, scopeId, user?.id, "context", focused]
      : [historyKey, scopeId, user?.id],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      focused && !pageParam
        ? transport.messageContext(scopeId, focused, signal)
        : transport.messages(scopeId, pageParam, signal),
    getNextPageParam: (last) => last.nextCursor || undefined,
    refetchInterval: 15000,
    retry: (failures, error) => !isAccessDenied(error) && failures < 3,
  });
  const read = useQuery({
    queryKey: [readKey, scopeId, user?.id],
    queryFn: ({ signal }) => transport.chatRead(scopeId, signal),
    refetchInterval: 15000,
    retry: (failures, error) => !isAccessDenied(error) && failures < 3,
  });
  const accessDenied =
    !!onAccessDenied &&
    ((isAccessDenied(query.error) &&
      !(
        focused &&
        query.error instanceof ApiError &&
        query.error.status === 404
      )) ||
      isAccessDenied(read.error));
  useEffect(() => {
    if (accessDenied) onAccessDenied?.();
  }, [accessDenied, onAccessDenied]);
  const historyUnavailable =
    isAccessDenied(query.error) || isAccessDenied(read.error);
  const pins = usePersonalChatPins(scopeId, personal && !historyUnavailable);
  const pinnedIds = new Set(pins.items.map((message) => message.id));
  const messages = historyUnavailable
    ? []
    : mergeChatPages(query.data?.pages || []);
  const canDeleteMessage = (message: ChatMessage) =>
    !readOnly &&
    !historyUnavailable &&
    !message.deletedAt &&
    !message.call &&
    (message.senderId === user?.id || mayModerate);
  const canSelectMessage = (message: ChatMessage) =>
    !historyUnavailable &&
    !message.deletedAt &&
    !message.call &&
    (mayForward || canDeleteMessage(message));
  const selectedItems = messages.filter(
    (message) => selectedMessages.has(message.id) && canSelectMessage(message),
  );
  const selectedIds = selectedItems.map((message) => message.id);
  const canDeleteSelection =
    selectedItems.length > 0 && selectedItems.every(canDeleteMessage);

  useEffect(() => {
    setSelectedMessages(new Set());
    setSelectingMessages(false);
    setConfirmSelection(false);
    setSelectionNotice("");
    setSelectionError("");
    setForwardingMessages(null);
    return () => {
      bulkController.current?.abort();
      bulkController.current = null;
    };
  }, [scopeId, user?.id, readOnly, historyUnavailable, mayModerate]);

  useEffect(() => {
    if (selectingMessages) selectionToolbar.current?.focus();
    else if (wasSelectingMessages.current && visible) composer.current?.focus();
    wasSelectingMessages.current = selectingMessages;
  }, [selectingMessages, visible]);

  // Refetches may remove a selected message or revoke permission to delete it.
  useEffect(() => {
    const allowed = new Set(
      mergeChatPages(query.data?.pages || [])
        .filter(
          (message) =>
            !message.deletedAt &&
            !message.call &&
            (mayForward ||
              (!readOnly && (message.senderId === user?.id || mayModerate))),
        )
        .map((message) => message.id),
    );
    setSelectedMessages((current) => {
      const next = new Set([...current].filter((id) => allowed.has(id)));
      return next.size === current.size ? current : next;
    });
  }, [query.data, mayModerate, mayForward, readOnly, user?.id]);

  const toggleMessage = (message: ChatMessage) => {
    if (!canSelectMessage(message) || bulkController.current) return;
    setSelectedMessages((current) => {
      const next = new Set(current);
      if (next.has(message.id)) next.delete(message.id);
      else next.add(message.id);
      return next;
    });
    setSelectionError("");
  };
  const cancelSelection = () => {
    if (bulkController.current) return;
    setSelectingMessages(false);
    setSelectedMessages(new Set());
    setConfirmSelection(false);
    setSelectionError("");
    composer.current?.focus();
  };
  useEffect(() => {
    if (!historyUnavailable) return;
    downloadController.current?.abort();
    setDownload(null);
    setReply(null);
    setEditing(null);
    setDeleting(null);
    setEditText("");
  }, [historyUnavailable]);
  const confirmedPending =
    !!pending &&
    messages.some(
      (message) =>
        message.senderId === user?.id &&
        message.clientRequestId === pending.clientRequestId,
    );
  const latest = messages.at(-1)?.id;

  /** Обновляет историю и счётчик непрочитанных сообщений после изменения на сервере. */
  const invalidate = () => {
    void client.invalidateQueries({ queryKey: [historyKey, scopeId] });
    void client.invalidateQueries({ queryKey: [readKey, scopeId] });
    if (personal) {
      void client.resetQueries({
        queryKey: ["personal-chat-search", scopeId, user?.id],
      });
      void client.invalidateQueries({ queryKey: ["personal-list", user?.id] });
      void client.invalidateQueries({
        queryKey: ["personal-summary", user?.id],
      });
    }
  };
  const invalidateChangedMessage = () => {
    invalidate();
    if (personal)
      void client.resetQueries({
        queryKey: ["personal-chat-pins", scopeId, user?.id],
      });
  };
  const send = useMutation({
    mutationFn: (body: {
      text: string;
      replyTo?: string;
      attachmentIds: string[];
      clientRequestId: string;
    }) => transport.sendMessage(scopeId, body),
    onSuccess: ({ item }) => {
      if (!mounted.current) return;
      client.setQueryData<InfiniteData<ChatPage>>(
        [historyKey, scopeId, user?.id],
        (current) =>
          current
            ? {
                ...current,
                pages: current.pages.map((page, index) =>
                  index ? page : { ...page, items: [...page.items, item] },
                ),
              }
            : current,
      );
      setPending(null);
      newestVisible.current = true;
      setText("");
      setReply(null);
      setAttachments([]);
      setComposerGeneration((value) => value + 1);
      selection.current = { start: 0, end: 0 };
      retryRequest.current = null;
      invalidate();
      if (focused) onLatest?.();
    },
    onError: () => {
      if (mounted.current)
        setPending((current) =>
          current ? { ...current, state: "failed" } : current,
        );
    },
    onSettled: () => {
      sendActive.current = false;
    },
  });
  const edit = useMutation({
    mutationFn: () =>
      transport.editMessage(scopeId, editing!.id, editText.trim()),
    onSuccess: () => {
      setEditing(null);
      invalidateChangedMessage();
    },
    onError: invalidateChangedMessage,
  });
  const remove = useMutation({
    mutationFn: () => transport.deleteMessage(scopeId, deleting!.id),
    onSuccess: () => {
      setDeleting(null);
      invalidateChangedMessage();
    },
    onError: invalidateChangedMessage,
  });
  const removeSelected = useMutation({
    mutationFn: async (ids: string[]) => {
      const controller = new AbortController();
      bulkController.current = controller;
      setDeleteProgress({ done: 0, total: ids.length });
      const result = await deleteChatMessages(
        ids,
        (id, signal) => transport.deleteMessage(scopeId, id, signal),
        controller.signal,
        (id, { item }) => {
          client.setQueriesData<InfiniteData<ChatPage>>(
            { queryKey: [historyKey, scopeId, user?.id] },
            (current) =>
              current
                ? {
                    ...current,
                    pages: current.pages.map((page) => ({
                      ...page,
                      items: page.items.map((message) =>
                        message.id === id ? item : message,
                      ),
                    })),
                  }
                : current,
          );
          if (!controller.signal.aborted) {
            setReply((current) => (current?.id === id ? null : current));
            setDownload(null);
            setDeleteProgress((progress) => ({
              ...progress,
              done: progress.done + 1,
            }));
            setSelectedMessages((current) => {
              const next = new Set(current);
              next.delete(id);
              return next;
            });
          }
        },
      );
      invalidateChangedMessage();
      if (!personal)
        void client.invalidateQueries({
          queryKey: ["conference-chat-pins", scopeId],
        });
      if (mounted.current && !controller.signal.aborted) {
        bulkController.current = null;
        setConfirmSelection(false);
        setSelectionNotice(`Удалено сообщений: ${result.deleted.length}.`);
        if (result.remaining.length) {
          setSelectionError(
            `Не удалено сообщений: ${result.remaining.length}. ${errorMessage(result.error)} Оставшиеся сообщения выделены — можно повторить удаление.`,
          );
          selectionToolbar.current?.focus();
        } else {
          setSelectedMessages(new Set());
          setSelectingMessages(false);
          setSelectionError("");
        }
      }
    },
    retry: false,
  });
  const bookmark = useMutation({
    mutationFn: (value: { messageId: string; important: boolean }) =>
      api.setConferenceChatImportant(scopeId, value.messageId, value.important),
    onSuccess: () => {
      invalidate();
      void client.invalidateQueries({
        queryKey: ["conference-chat-pins", scopeId],
      });
    },
  });

  useEffect(() => {
    if (readOnly) {
      setReply(null);
      setEditing(null);
      setDeleting(null);
      pendingCaret.current = null;
    }
  }, [readOnly]);

  // Изменение размера поля не двигает список сообщений; курсор возвращается после выбора смайлика.
  useEffect(() => {
    const element = composer.current;
    if (!element || !visible) return;
    const resize = () => {
      element.style.height = "auto";
      element.style.height = `${Math.max(24, Math.min(120, element.scrollHeight))}px`;
    };
    resize();
    if (pendingCaret.current !== null) {
      element.focus();
      element.setSelectionRange(pendingCaret.current, pendingCaret.current);
      pendingCaret.current = null;
    }
    let width = element.clientWidth;
    const observer = new ResizeObserver(() => {
      if (element.clientWidth === width) return;
      width = element.clientWidth;
      resize();
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, [text, open, readOnly, visible, textSize]);

  useEffect(() => {
    initialScroll.current = false;
    previousLatest.current = undefined;
    olderAnchor.current = null;
    readingAnchor.current = null;
    markerVisible.current = false;
    newestVisible.current = false;
    setReadCandidate(null);
    setHasNewMessages(false);
  }, [focused, focusRequest]);

  useEffect(() => {
    if (!open || !visible || !latest) return;
    const element = viewport.current;
    if (!element) return;
    /** Проверяет край с новыми сообщениями и сохраняет место чтения старой истории. */
    const observe = () => {
      newestVisible.current =
        element.scrollHeight - element.scrollTop - element.clientHeight < 40;
      if (personal) {
        const bounds = element.getBoundingClientRect();
        const anchor = Array.from(
          element.querySelectorAll<HTMLElement>(
            'article[data-testid^="chat-message-"]',
          ),
        ).find((node) => {
          const rect = node.getBoundingClientRect();
          return rect.bottom > bounds.top && rect.top < bounds.bottom;
        });
        readingAnchor.current = anchor
          ? {
              id: anchor.dataset.testid!,
              offset: anchor.getBoundingClientRect().top - bounds.top,
            }
          : null;
      }
      setReadCandidate(
        document.visibilityState === "visible" &&
          newestVisible.current &&
          markerVisible.current
          ? latest
          : null,
      );
    };
    if (olderAnchor.current && !query.isFetchingNextPage) {
      element.scrollTop =
        olderAnchor.current.top +
        element.scrollHeight -
        olderAnchor.current.height;
      olderAnchor.current = null;
    } else if (!initialScroll.current && focused) {
      const target = Array.from(
        element.querySelectorAll<HTMLElement>("[data-testid]"),
      ).find((node) => node.dataset.testid === `chat-message-${focused}`);
      target?.scrollIntoView?.({ block: "center" });
      target?.focus({ preventScroll: true });
    } else if (!initialScroll.current || newestVisible.current) {
      element.scrollTop = element.scrollHeight;
      setHasNewMessages(false);
    } else if (previousLatest.current && previousLatest.current !== latest) {
      if (personal && readingAnchor.current) {
        const anchor = Array.from(
          element.querySelectorAll<HTMLElement>(
            'article[data-testid^="chat-message-"]',
          ),
        ).find((node) => node.dataset.testid === readingAnchor.current!.id);
        if (anchor)
          element.scrollTop +=
            anchor.getBoundingClientRect().top -
            element.getBoundingClientRect().top -
            readingAnchor.current.offset;
      }
      setHasNewMessages(true);
    }
    previousLatest.current = latest;
    initialScroll.current = true;
    const observer =
      typeof IntersectionObserver === "undefined"
        ? undefined
        : new IntersectionObserver(
            (entries) => {
              markerVisible.current = entries.some(
                (entry) => entry.isIntersecting,
              );
              observe();
            },
            { threshold: 1 },
          );
    if (lastMarker.current) observer?.observe(lastMarker.current);
    observe();
    element.addEventListener("scroll", observe);
    document.addEventListener("visibilitychange", observe);
    return () => {
      element.removeEventListener("scroll", observe);
      document.removeEventListener("visibilitychange", observe);
      observer?.disconnect();
    };
  }, [
    open,
    visible,
    latest,
    messages.length,
    query.isFetchingNextPage,
    focused,
    focusRequest,
    personal,
    pending?.clientRequestId,
  ]);

  useEffect(() => {
    if (
      !open ||
      !visible ||
      !!focused ||
      !readCandidate ||
      readCandidate === lastRead.current ||
      readCandidate === read.data?.item.lastReadMessageId
    )
      return;
    const timer = setTimeout(() => {
      const marker = lastMarker.current;
      const list = viewport.current;
      if (
        document.visibilityState !== "visible" ||
        !newestVisible.current ||
        !markerVisible.current ||
        !marker ||
        !list ||
        !marker.getClientRects().length
      )
        return;
      // Учитываем скрытие боковой панели и прокрутку, даже если observer ещё не успел обновить refs.
      const markerRect = marker.getBoundingClientRect();
      const listRect = list.getBoundingClientRect();
      if (
        markerRect.top < Math.max(0, listRect.top) ||
        markerRect.bottom > Math.min(window.innerHeight, listRect.bottom) ||
        markerRect.left < Math.max(0, listRect.left) ||
        markerRect.right > Math.min(window.innerWidth, listRect.right) ||
        list.scrollHeight - list.scrollTop - list.clientHeight >= 40
      )
        return;
      void transport
        .markChatRead(scopeId, readCandidate)
        .then(() => {
          lastRead.current = readCandidate;
          if (personal) {
            void client.invalidateQueries({ queryKey: ["personal-list"] });
            void client.invalidateQueries({ queryKey: ["personal-summary"] });
          }
          void client.invalidateQueries({
            queryKey: [readKey, scopeId],
          });
        })
        .catch(() => {});
    }, 2500);
    return () => clearTimeout(timer);
  }, [
    open,
    visible,
    readCandidate,
    focused,
    read.data?.item.lastReadMessageId,
    scopeId,
    client,
    transport,
    readKey,
    personal,
  ]);

  /** Сохраняет выделение до перевода фокуса с редактора на кнопку палитры. */
  const rememberSelection = () => {
    const element = composer.current;
    if (element)
      selection.current = {
        start: element.selectionStart,
        end: element.selectionEnd,
      };
  };

  /**
   * Заменяет выделенный текст смайликом, соблюдая серверный лимит, и возвращает курсор в редактор.
   * @args emoji — выбранная последовательность Unicode, включая составные смайлики.
   */
  const addEmoji = (emoji: string) => {
    if (readOnly || send.isPending) return;
    const result = insertChatEmoji(
      text,
      emoji,
      selection.current.start,
      selection.current.end,
    );
    if (!result) {
      setValidation("Сообщение не должно превышать 4000 символов.");
      return;
    }
    setValidation("");
    selection.current = { start: result.caret, end: result.caret };
    if (result.text === text) {
      composer.current?.focus();
      composer.current?.setSelectionRange(result.caret, result.caret);
    } else {
      pendingCaret.current = result.caret;
      setText(result.text);
    }
  };

  /** Проверяет черновик перед отправкой; вложения разрешены и без текстовой части. */
  const sendDraft = () => {
    if (readOnly || sendActive.current || send.isPending || uploadBusy) return;
    setValidation("");
    if (
      (!text.trim() && !attachments.length) ||
      Array.from(text.trim()).length > 4000
    ) {
      setValidation("Добавьте текст (до 4000 символов) или файл.");
      return;
    }
    const body = {
      text: text.trim(),
      replyTo: reply?.id,
      attachmentIds: attachments.map((file) => file.id),
    };
    const signature = JSON.stringify(body);
    if (retryRequest.current?.signature !== signature)
      retryRequest.current = { signature, id: crypto.randomUUID() };
    sendActive.current = true;
    setPending({
      clientRequestId: retryRequest.current.id,
      text: body.text,
      attachments: [...attachments],
      replyName: reply?.senderName,
      createdAt: new Date().toISOString(),
      state: "sending",
    });
    newestVisible.current = true;
    send.mutate({ ...body, clientRequestId: retryRequest.current.id });
  };

  /**
   * Получает временную ссылку на приватное вложение и проверяет допустимую схему адреса.
   * @args file — вложение сообщения, доступ к которому проверит сервер.
   */
  const prepareDownload = async (file: ChatAttachment) => {
    setDownloadError("");
    downloadController.current?.abort();
    const controller = new AbortController();
    downloadController.current = controller;
    try {
      const result = await transport.attachmentDownload(scopeId, file.id);
      controller.signal.throwIfAborted();
      const url = new URL(result.url, window.location.origin);
      if (requireAuthenticatedDownloads && result.authenticated !== true)
        throw new Error("unauthenticated_group_file");
      if (result.authenticated === true) {
        const path = `/api/v1/${personal ? "conversations" : "conferences"}/${encodeURIComponent(scopeId)}/attachments/${encodeURIComponent(file.id)}/content`;
        if (
          url.origin !== window.location.origin ||
          url.pathname !== path ||
          url.search ||
          url.hash
        )
          throw new Error("invalid_private_file_url");
        const blob = await transport.attachmentContent(
          scopeId,
          file.id,
          controller.signal,
        );
        if (!mounted.current || controller.signal.aborted) return;
        setDownload({
          id: file.id,
          url: URL.createObjectURL(blob),
          expiresAt: new Date(Date.now() + 60_000).toISOString(),
        });
        return;
      }
      if (!["http:", "https:"].includes(url.protocol))
        throw new Error("invalid_download_url");
      if (!mounted.current || controller.signal.aborted) return;
      setDownload({
        id: file.id,
        url: url.toString(),
        expiresAt: result.expiresAt,
      });
    } catch (error) {
      if (
        mounted.current &&
        !controller.signal.aborted &&
        error instanceof ApiError &&
        error.status === 403
      )
        onAccessDenied?.();
      if (mounted.current && !controller.signal.aborted)
        setDownloadError(errorMessage(error));
    }
  };

  const attachmentIcon = (file: ChatAttachment, tile = false) => (
    <ChatFileIcon
      key={`${user?.id}:${scopeId}:${file.id}`}
      file={file}
      scopeId={scopeId}
      transport={transport}
      authenticated={requireAuthenticatedDownloads}
      visible={visible && !!user}
      tile={tile}
    />
  );

  const attachmentDownload = (file: ChatAttachment, tile = false) =>
    download?.id === file.id ? (
      <a
        className="icon-button chat-file-download"
        href={download.url}
        target="_blank"
        rel="noreferrer"
        download={file.filename}
        aria-label={tile ? `Скачать ${file.filename}` : "Скачать файл"}
        title={file.filename}
      >
        <Download size={18} aria-hidden="true" />
      </a>
    ) : (
      <button
        type="button"
        className="icon-button chat-file-download"
        aria-label={
          tile ? `Получить ссылку на ${file.filename}` : "Получить ссылку"
        }
        title="Получить ссылку на скачивание"
        onClick={() => void prepareDownload(file)}
      >
        <Download size={18} aria-hidden="true" />
      </button>
    );

  const unread =
    read.data?.item.unreadCount ?? query.data?.pages[0]?.unreadCount ?? 0;
  const olderMessagesButton = query.hasNextPage && (
    <Button
      variant="outline"
      busy={query.isFetchingNextPage}
      onClick={() => {
        const list = viewport.current;
        if (list) {
          olderAnchor.current = {
            height: list.scrollHeight,
            top: list.scrollTop,
          };
          newestVisible.current = false;
        }
        void query.fetchNextPage().catch(() => {
          olderAnchor.current = null;
        });
      }}
    >
      Предыдущие сообщения
    </Button>
  );
  const pendingMessage = pending && !confirmedPending && (
    <article
      className="chat-message chat-message-own chat-message-pending"
      data-testid={`chat-pending-${pending.clientRequestId}`}
    >
      {personal ? (
        <UserAvatar
          id={user?.id}
          name={user?.displayName || "Вы"}
          version={user?.avatarVersion}
          className="chat-message-avatar"
        />
      ) : (
        <span className="chat-message-avatar" aria-hidden="true">
          {initials(user?.displayName || "Вы")}
        </span>
      )}
      <div className="chat-message-content">
        <header>
          <strong>{user?.displayName || "Вы"}</strong>
          <span className="chat-message-self">вы</span>
          <time dateTime={pending.createdAt}>
            {chatTime(pending.createdAt)}
          </time>
        </header>
        <div className="chat-message-bubble">
          {pending.replyName && (
            <blockquote>Ответ: {pending.replyName}</blockquote>
          )}
          {pending.text && <p className="chat-text">{pending.text}</p>}
          {pending.attachments.map((file) => (
            <div className="chat-attachment" key={file.id}>
              <span>
                <Paperclip size={15} />
                {file.filename}
                <small>{formatBytes(file.size)}</small>
              </span>
            </div>
          ))}
          <footer className="chat-message-meta">
            {!personal && (
              <time dateTime={pending.createdAt}>
                {chatTime(pending.createdAt)}
              </time>
            )}
            <span role="status">
              {pending.state === "sending" ? "Отправляется…" : "Не отправлено"}
            </span>
          </footer>
        </div>
      </div>
    </article>
  );
  if (accessDenied) return <ErrorNotice error={query.error || read.error} />;
  return (
    <section
      className={`content-card chat-panel ${personal ? "personal-thread" : "conference-thread"}${selectingMessages ? " chat-selecting" : ""}`}
      aria-label={personal ? "Личная переписка" : "Чат конференции"}
      onKeyDown={(event) => {
        if (
          event.key === "Escape" &&
          selectingMessages &&
          !confirmSelection &&
          !forwardingMessages &&
          !removeSelected.isPending
        ) {
          event.preventDefault();
          event.stopPropagation();
          cancelSelection();
        }
      }}
    >
      <div className="section-heading chat-panel-heading" hidden={personal}>
        <h2>
          <MessageCircle size={20} />
          Чат{" "}
          {unread > 0 && (
            <span className="count-badge" data-testid="chat-unread">
              {unread}
            </span>
          )}
        </h2>
        <Button
          variant="outline"
          onClick={() => setOpen((value) => !value)}
          aria-expanded={open}
        >
          {open ? "Свернуть чат" : "Открыть чат"}
        </Button>
      </div>
      <div className="chat-panel-body" hidden={!open}>
        {personal && onFocusMessage && (
          <PersonalChatPins
            items={pins.items}
            onSelect={onFocusMessage}
            onUnpin={
              readOnly
                ? undefined
                : (messageId) =>
                    pins.mutation.mutate({ messageId, pinned: false })
            }
            busy={pins.mutation.isPending}
            error={pins.mutation.error || pins.query.error}
            loading={pins.query.isPending}
          />
        )}
        {personal && (
          <>
            <span className="sr-only" role="status">
              {pins.announcement}
            </span>
            <ErrorNotice error={pins.mutation.error || pins.query.error} />
          </>
        )}
        {focused && onLatest && (
          <Button variant="outline" onClick={onLatest}>
            К последним сообщениям
          </Button>
        )}
        <ErrorNotice error={bookmark.error} />
        {readOnly && (
          <p className="field-hint chat-readonly-note">
            {readOnlyReason ||
              "Встреча завершена. Чат сохранён и доступен только для чтения."}
          </p>
        )}
        <ErrorNotice
          error={
            focused &&
            query.error instanceof ApiError &&
            query.error.status === 404
              ? null
              : query.error
          }
        />
        {focused &&
          query.error instanceof ApiError &&
          query.error.status === 404 && (
            <p className="field-hint" role="status">
              Сообщение больше недоступно. Вернитесь к последним сообщениям.
            </p>
          )}
        {query.isError && (
          <Button variant="outline" onClick={() => void query.refetch()}>
            Повторить загрузку чата
          </Button>
        )}
        {query.isPending ? (
          <MessagingSkeleton rows={4} />
        ) : (
          <div
            ref={viewport}
            className="chat-messages"
            role="log"
            aria-label={personal ? "Личные сообщения" : "Сообщения встречи"}
            aria-live="polite"
          >
            {olderMessagesButton}
            {!messages.length && !query.isError && (
              <div className="messaging-empty">
                <MessageCircle aria-hidden="true" />
                <h3>
                  {readOnly ? "В чате пока нет сообщений" : "Начните разговор"}
                </h3>
                <p>
                  {readOnly
                    ? "Чат доступен только для чтения. Новые сообщения здесь отправить нельзя."
                    : "Отправьте первое сообщение или прикрепите файл."}
                </p>
              </div>
            )}
            {messages.map((message, index) => {
              const own = message.senderId === user?.id;
              const selectable = canSelectMessage(message);
              const selected = selectedMessages.has(message.id) && selectable;
              const pinned = personal && pinnedIds.has(message.id);
              const photos = message.attachments.filter(
                canPreviewChatAttachment,
              );
              const photoGrid = photos.length > 1;
              const files = photoGrid
                ? message.attachments.filter(
                    (file) => !canPreviewChatAttachment(file),
                  )
                : message.attachments;
              const inlineMeta =
                personal &&
                !message.call &&
                !message.forwardedFrom &&
                !message.replyPreview &&
                message.attachments.length === 0 &&
                !/[\r\n]/.test(message.text);
              const day = chatDayKey(message.createdAt);
              return (
                <Fragment key={message.id}>
                  {day &&
                    (!index ||
                      day !== chatDayKey(messages[index - 1].createdAt)) && (
                      <div className="chat-day-label">
                        <span>{chatDayLabel(message.createdAt)}</span>
                      </div>
                    )}
                  <article
                    className={`chat-message ${own ? "chat-message-own" : ""} ${pinned ? "chat-message-pinned" : ""} ${message.id === focused ? "chat-message-focused" : ""}${selectingMessages && selectable ? " chat-message-selectable" : ""}${selectingMessages && selected ? " chat-message-selected" : ""}`}
                    data-testid={`chat-message-${message.id}`}
                    tabIndex={message.id === focused ? -1 : undefined}
                    aria-current={message.id === focused ? "true" : undefined}
                    onClickCapture={(event) => {
                      if (
                        !selectingMessages ||
                        (event.target as Element).closest(
                          ".chat-message-picker",
                        )
                      )
                        return;
                      event.preventDefault();
                      event.stopPropagation();
                      toggleMessage(message);
                    }}
                  >
                    {personal ? (
                      <UserAvatar
                        id={message.senderId}
                        name={message.senderName}
                        className="chat-message-avatar"
                      />
                    ) : (
                      <span className="chat-message-avatar" aria-hidden="true">
                        {initials(message.senderName)}
                      </span>
                    )}
                    <div className="chat-message-content">
                      <header>
                        <strong>{message.senderName}</strong>
                        {own && <span className="chat-message-self">вы</span>}
                        <time
                          dateTime={message.createdAt}
                          title={formatDate(message.createdAt)}
                        >
                          {chatTime(message.createdAt)}
                        </time>
                        {message.version > 1 &&
                          !message.deletedAt &&
                          !message.call && (
                            <span
                              className="chat-message-edited"
                              title="Сообщение изменено"
                            >
                              <Pencil size={11} aria-hidden="true" />
                              <span className="sr-only">изменено</span>
                            </span>
                          )}
                      </header>
                      <div
                        className={`chat-message-bubble${inlineMeta ? " chat-message-bubble-text" : ""}`}
                      >
                        {!message.deletedAt && message.forwardedFrom && (
                          <div className="chat-forwarded-from">
                            <Forward size={14} aria-hidden="true" />
                            <span>
                              Переслано от {message.forwardedFrom.senderName}
                            </span>
                          </div>
                        )}
                        {message.replyPreview && (
                          <blockquote>
                            {personal && onFocusMessage ? (
                              <button
                                type="button"
                                className="chat-reply-jump"
                                onClick={() =>
                                  onFocusMessage(message.replyPreview!.id)
                                }
                                aria-label={`Перейти к сообщению от ${message.replyPreview.senderName}`}
                              >
                                <strong>
                                  {message.replyPreview.senderName}
                                </strong>
                                <span>
                                  {message.replyPreview.deleted
                                    ? "Сообщение удалено"
                                    : message.replyPreview.text || "Вложение"}
                                </span>
                              </button>
                            ) : (
                              <>
                                <strong>
                                  {message.replyPreview.senderName}
                                </strong>
                                <span>
                                  {message.replyPreview.deleted
                                    ? "Сообщение удалено"
                                    : message.replyPreview.text || "Вложение"}
                                </span>
                              </>
                            )}
                          </blockquote>
                        )}
                        {message.call && !message.deletedAt && (
                          <CallHistoryMessage call={message.call} />
                        )}
                        {(message.deletedAt ||
                          (!message.call && message.text)) && (
                          <p
                            className={
                              message.deletedAt
                                ? "chat-text muted"
                                : "chat-text"
                            }
                          >
                            {message.deletedAt
                              ? "Сообщение удалено"
                              : message.text}
                          </p>
                        )}
                        {!message.deletedAt && photoGrid && (
                          <div
                            className="chat-photo-grid"
                            role="group"
                            aria-label="Фотографии во вложениях"
                          >
                            {photos.map((file) => (
                              <div
                                className="chat-photo-tile"
                                key={file.id}
                                title={file.filename}
                              >
                                {attachmentIcon(file, true)}
                                {attachmentDownload(file, true)}
                              </div>
                            ))}
                          </div>
                        )}
                        {!message.deletedAt &&
                          files.map((file) => (
                            <div className="chat-attachment" key={file.id}>
                              {attachmentIcon(file)}
                              <span className="chat-file-copy">
                                <strong>{file.filename}</strong>
                                <small>
                                  {file.filename
                                    .split(".")
                                    .at(-1)
                                    ?.toUpperCase()}{" "}
                                  · {formatBytes(file.size)}
                                </small>
                              </span>
                              {attachmentDownload(file)}
                            </div>
                          ))}
                        <footer className="chat-message-footer">
                          {pinned && (
                            <span title="Закреплено">
                              <Pin size={12} aria-hidden="true" />
                              <span className="sr-only">Закреплено</span>
                            </span>
                          )}
                          {message.version > 1 &&
                            !message.deletedAt &&
                            !message.call && <span>изменено</span>}
                          <time
                            dateTime={message.createdAt}
                            title={formatDate(message.createdAt)}
                          >
                            {chatTime(message.createdAt)}
                          </time>
                          {own && !message.deletedAt && (
                            <span title="Отправлено">
                              <Check size={14} aria-hidden="true" />
                              <span className="sr-only">Отправлено</span>
                            </span>
                          )}
                        </footer>
                      </div>
                    </div>
                    {selectingMessages && selectable && (
                      <label className="chat-message-picker">
                        <input
                          className="chat-message-checkbox"
                          type="checkbox"
                          checked={selected}
                          disabled={removeSelected.isPending}
                          aria-label={`Выбрать сообщение: ${message.senderName}, ${chatTime(message.createdAt)}`}
                          onChange={() => toggleMessage(message)}
                        />
                        <span
                          className="chat-message-checkmark"
                          aria-hidden="true"
                        >
                          <Check size={15} />
                        </span>
                      </label>
                    )}
                    {!selectingMessages &&
                      !message.deletedAt &&
                      !message.call &&
                      (!readOnly || mayBookmark || mayForward) && (
                        <div className="chat-message-tools">
                          <ItemActions
                            label={`Действия с сообщением: ${message.senderName}, ${chatTime(message.createdAt)}`}
                            actions={
                              [
                                ...(selectable
                                  ? [
                                      {
                                        label: "Выбрать",
                                        icon: <CheckSquare size={16} />,
                                        run: () => {
                                          setSelectedMessages(
                                            new Set([message.id]),
                                          );
                                          setSelectingMessages(true);
                                          setSelectionNotice("");
                                          setSelectionError("");
                                        },
                                      },
                                    ]
                                  : []),
                                ...(mayForward
                                  ? [
                                      {
                                        label: "Переслать",
                                        icon: <Forward size={16} />,
                                        run: () =>
                                          setForwardingMessages([message]),
                                      },
                                    ]
                                  : []),
                                ...(personal && !readOnly
                                  ? [
                                      {
                                        label: pinned
                                          ? "Открепить"
                                          : "Закрепить",
                                        icon: pinned ? (
                                          <PinOff size={16} />
                                        ) : (
                                          <Pin size={16} />
                                        ),
                                        disabled:
                                          pins.mutation.isPending ||
                                          pins.query.isPending ||
                                          pins.query.isError,
                                        run: () =>
                                          pins.mutation.mutate({
                                            messageId: message.id,
                                            pinned: !pinned,
                                          }),
                                      },
                                    ]
                                  : []),
                                ...(mayBookmark
                                  ? [
                                      {
                                        label: message.important
                                          ? "Убрать из важных"
                                          : "В важные",
                                        icon: (
                                          <Star
                                            size={16}
                                            fill={
                                              message.important
                                                ? "currentColor"
                                                : "none"
                                            }
                                          />
                                        ),
                                        pressed: !!message.important,
                                        disabled: bookmark.isPending,
                                        run: () =>
                                          bookmark.mutate({
                                            messageId: message.id,
                                            important: !message.important,
                                          }),
                                      },
                                    ]
                                  : []),
                                ...(!readOnly
                                  ? [
                                      {
                                        label: "Ответить",
                                        icon: <Reply size={16} />,
                                        disabled: send.isPending,
                                        run: () => {
                                          setReply(message);
                                          composer.current?.focus();
                                        },
                                      },
                                    ]
                                  : []),
                                ...(!readOnly && own && !message.forwardedFrom
                                  ? [
                                      {
                                        label: "Изменить",
                                        icon: <Pencil size={16} />,
                                        run: () => {
                                          setEditing(message);
                                          setEditText(message.text);
                                          edit.reset();
                                        },
                                      },
                                    ]
                                  : []),
                                ...(!readOnly && (own || mayModerate)
                                  ? [
                                      {
                                        label: "Удалить",
                                        icon: <Trash2 size={16} />,
                                        danger: true,
                                        run: () => {
                                          setDeleting(message);
                                          remove.reset();
                                        },
                                      },
                                    ]
                                  : []),
                              ] satisfies ItemAction[]
                            }
                          />
                        </div>
                      )}
                  </article>
                </Fragment>
              );
            })}
            {pendingMessage}
            <span
              ref={lastMarker}
              className="chat-read-marker"
              aria-hidden="true"
            />
          </div>
        )}
        {hasNewMessages && (
          <Button
            variant="outline"
            onClick={() => {
              const list = viewport.current;
              if (list) list.scrollTop = list.scrollHeight;
              setHasNewMessages(false);
            }}
          >
            Новые сообщения ↓
          </Button>
        )}
        <ErrorNotice error={downloadError || null} />
        {selectionNotice && (
          <p className="chat-selection-notice" role="status">
            {selectionNotice}
          </p>
        )}
        <ErrorNotice>{selectionError || null}</ErrorNotice>
        {selectingMessages && !historyUnavailable && (
          <div
            className="chat-selection-toolbar"
            ref={selectionToolbar}
            tabIndex={-1}
            role="group"
            aria-label="Выбранные сообщения"
          >
            <button
              className="icon-button"
              type="button"
              aria-label="Отменить выбор"
              title="Отменить выбор"
              disabled={removeSelected.isPending}
              onClick={cancelSelection}
            >
              <X size={20} />
            </button>
            <span role="status">Выбрано: {selectedIds.length}</span>
            <div className="chat-selection-actions">
              {mayForward && (
                <Button
                  variant="outline"
                  disabled={
                    !selectedIds.length ||
                    selectedIds.length > 30 ||
                    removeSelected.isPending
                  }
                  onClick={() => setForwardingMessages(selectedItems)}
                  title={
                    selectedIds.length > 30
                      ? "За один раз можно переслать до 30 сообщений"
                      : undefined
                  }
                >
                  <Forward size={18} />
                  Переслать
                </Button>
              )}
              <Button
                variant="danger"
                className="chat-selection-delete"
                disabled={!canDeleteSelection || removeSelected.isPending}
                title={
                  !canDeleteSelection && selectedItems.length
                    ? "Среди выбранных есть сообщения, которые вы не можете удалить"
                    : undefined
                }
                onClick={() => setConfirmSelection(true)}
              >
                <Trash2 size={18} />
                Удалить выбранные
              </Button>
            </div>
            {selectedIds.length > 30 && mayForward && (
              <small className="chat-selection-limit">
                За один раз можно переслать до 30 сообщений.
              </small>
            )}
          </div>
        )}
        {!readOnly && (
          <form
            className="chat-composer"
            hidden={selectingMessages}
            onSubmit={(event) => {
              event.preventDefault();
              sendDraft();
            }}
          >
            <div className="chat-composer-details">
              {reply && (
                <div className="chat-reply">
                  <div>
                    <strong>Ответ: {reply.senderName}</strong>
                    <p>{reply.text || "Вложение"}</p>
                  </div>
                  <button
                    type="button"
                    className="icon-button"
                    aria-label="Отменить ответ"
                    disabled={send.isPending}
                    onClick={() => setReply(null)}
                  >
                    <X size={16} />
                  </button>
                </div>
              )}
              <div
                className="chat-upload-details"
                ref={setUploadDetailsTarget}
              />
              <ErrorNotice error={send.error}>{validation || null}</ErrorNotice>
            </div>
            <div className="chat-compose-row">
              <AttachmentUploader
                key={composerGeneration}
                conferenceId={scopeId}
                transport={transport}
                value={attachments}
                onChange={setAttachments}
                onBusy={setUploadBusy}
                disabled={send.isPending}
                compact
                detailsTarget={uploadDetailsTarget}
              />
              <label
                className="chat-compose-field"
                htmlFor={`chat-text-${scopeId}`}
              >
                <span className="sr-only">Сообщение</span>
                <textarea
                  ref={composer}
                  aria-label="Сообщение"
                  id={`chat-text-${scopeId}`}
                  value={text}
                  rows={1}
                  placeholder={composerPlaceholder}
                  disabled={send.isPending}
                  onSelect={rememberSelection}
                  onBlur={rememberSelection}
                  onChange={(event) => {
                    setText(event.target.value);
                    selection.current = {
                      start: event.target.selectionStart,
                      end: event.target.selectionEnd,
                    };
                  }}
                  onKeyDown={(event) => {
                    if (
                      event.key === "Enter" &&
                      !event.shiftKey &&
                      !event.nativeEvent.isComposing &&
                      event.keyCode !== 229
                    ) {
                      event.preventDefault();
                      sendDraft();
                    }
                  }}
                />
              </label>
              <EmojiPicker onSelect={addEmoji} disabled={send.isPending} />
              <span className="chat-compose-keyhint" aria-hidden="true">
                Enter — отправить
              </span>
              <Button
                type="submit"
                className="chat-send-button"
                busy={send.isPending}
                disabled={uploadBusy || (!text.trim() && !attachments.length)}
                aria-label={send.isError ? "Повторить отправку" : "Отправить"}
                title={send.isError ? "Повторить отправку" : "Отправить"}
              >
                {!send.isPending && <SendHorizontal size={19} />}
              </Button>
            </div>
            <p className="chat-compose-caption" aria-hidden="true">
              {personal && "Enter — отправить · "}Shift + Enter — новая строка
            </p>
            <p className="sr-only">
              Enter — отправить, Shift + Enter — новая строка. Если связь
              прервётся, повторная отправка не создаст копию сообщения.
            </p>
          </form>
        )}
      </div>
      {forwardingMessages && (
        <ForwardMessagesDialog
          key={`${scopeId}:${user?.id}`}
          sourceType={personal ? "conversation" : "conference"}
          sourceId={scopeId}
          messages={forwardingMessages}
          onClose={() => setForwardingMessages(null)}
          onForwarded={(target, count) => {
            setForwardingMessages(null);
            setSelectedMessages(new Set());
            setSelectingMessages(false);
            setSelectionError("");
            setSelectionNotice(
              `Переслано сообщений: ${count}. Получатель: ${target.type === "group" ? target.name : target.peer.displayName}.`,
            );
            void client.invalidateQueries({
              queryKey: ["personal-chat", target.id, user?.id],
            });
            void client.invalidateQueries({
              queryKey: ["personal-chat-read", target.id, user?.id],
            });
            void client.invalidateQueries({
              queryKey: ["personal-list", user?.id],
            });
            void client.invalidateQueries({
              queryKey: ["personal-summary", user?.id],
            });
          }}
        />
      )}
      {confirmSelection && selectingMessages && (
        <Modal
          title="Удалить выбранные сообщения?"
          onClose={() => {
            if (!removeSelected.isPending) setConfirmSelection(false);
          }}
          returnFocus={() =>
            selectionToolbar.current?.querySelector<HTMLButtonElement>(
              ".chat-selection-delete",
            ) || composer.current
          }
        >
          <p className="modal-description">
            Выбрано сообщений:{" "}
            {removeSelected.isPending
              ? deleteProgress.total
              : selectedIds.length}
            . Текст и вложения станут недоступны всем участникам. В чате
            останутся отметки об удалении. Отменить удаление нельзя.
          </p>
          {removeSelected.isPending && (
            <p role="status">
              Удалено: {deleteProgress.done} из {deleteProgress.total}
            </p>
          )}
          <div className="chat-selection-confirm-actions">
            <Button
              variant="outline"
              disabled={removeSelected.isPending}
              onClick={() => setConfirmSelection(false)}
            >
              Отмена
            </Button>
            <Button
              variant="danger"
              busy={removeSelected.isPending}
              disabled={!canDeleteSelection}
              onClick={() => {
                if (
                  !readOnly &&
                  canDeleteSelection &&
                  !removeSelected.isPending &&
                  !bulkController.current
                ) {
                  setSelectionError("");
                  removeSelected.mutate(selectedIds);
                }
              }}
            >
              Удалить для всех
            </Button>
          </div>
        </Modal>
      )}
      {editing && (
        <Modal
          title="Изменить сообщение"
          onClose={() => {
            if (!edit.isPending) setEditing(null);
          }}
        >
          <form
            onSubmit={(event) => {
              event.preventDefault();
              if (
                !edit.isPending &&
                !readOnly &&
                (editText.trim() || editing.attachments.length) &&
                Array.from(editText.trim()).length <= 4000
              )
                edit.mutate();
            }}
          >
            <label className="field" htmlFor="edit-message">
              Текст
              <textarea
                id="edit-message"
                data-autofocus
                value={editText}
                onChange={(event) => setEditText(event.target.value)}
                rows={4}
                disabled={edit.isPending}
              />
            </label>
            <ErrorNotice error={edit.error} />
            <Button
              type="submit"
              busy={edit.isPending}
              disabled={
                (!editText.trim() && !editing.attachments.length) ||
                Array.from(editText.trim()).length > 4000
              }
            >
              Сохранить сообщение
            </Button>
          </form>
        </Modal>
      )}
      {deleting && (
        <Modal
          title="Удалить сообщение?"
          onClose={() => {
            if (!remove.isPending) setDeleting(null);
          }}
        >
          <p className="modal-description">
            Текст и вложения станут недоступны другим участникам. В чате
            останется отметка об удалении.
          </p>
          <ErrorNotice error={remove.error} />
          <Button
            variant="danger"
            busy={remove.isPending}
            onClick={() => {
              if (!readOnly) remove.mutate();
            }}
          >
            Да, удалить сообщение
          </Button>
        </Modal>
      )}
    </section>
  );
}
