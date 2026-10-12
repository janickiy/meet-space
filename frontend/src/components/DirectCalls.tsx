import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useLocation } from "react-router";
import {
  MessageCircle,
  Mic,
  MicOff,
  MonitorUp,
  Phone,
  PhoneOff,
  RefreshCw,
  Video,
  VideoOff,
  X,
} from "lucide-react";
import { api, personalChatAPI } from "../api";
import { useAuth } from "../auth";
import type { DirectCall } from "../types";
import { useRealtime } from "../realtime";
import { useMedia } from "../useMedia";
import {
  readDevicePreferences,
  subscribeDevicePreferences,
} from "../prejoinDevices";
import { initials } from "../utils";
import { MediaTile } from "./RealtimePanel";
import { MessageThread } from "./ChatPanel";
import { Button, ErrorNotice, Modal } from "./ui";
import toneURL from "../assets/device-tone.wav";
import "./direct-calls.css";

// A fresh document owns a fresh media endpoint. Duplicating a tab must never copy ownership.
const clientId = crypto.randomUUID();
const changedEvent = "meet:direct-call";
const ongoing = (call: DirectCall) =>
  ["ringing", "accepted"].includes(call.status);

/** Accept only complete account events; REST remains the authority after reconnect. */
export function receiveDirectCallEvent(raw: string, userId: string): boolean {
  try {
    const event = JSON.parse(raw);
    if (event?.type !== "call.updated") return false;
    const call = event.data?.call;
    if (
      !call ||
      typeof call.id !== "string" ||
      typeof call.conversationId !== "string" ||
      typeof call.caller?.id !== "string" ||
      typeof call.callee?.id !== "string" ||
      typeof call.caller.displayName !== "string" ||
      typeof call.callee.displayName !== "string" ||
      ![
        "ringing",
        "accepted",
        "declined",
        "cancelled",
        "ended",
        "missed",
      ].includes(call.status) ||
      !Number.isSafeInteger(call.version) ||
      ![call.caller.id, call.callee.id].includes(userId)
    )
      return true;
    window.dispatchEvent(
      new CustomEvent(changedEvent, { detail: { userId, call } }),
    );
    return true;
  } catch {
    return false;
  }
}

const DirectCallsContext = createContext<{
  startCall: (targetUserId: string) => Promise<void>;
  busy: boolean;
  error: Error | null;
} | null>(null);

export function useDirectCalls() {
  const context = useContext(DirectCallsContext);
  if (!context) throw new Error("DirectCallsProvider is required");
  return context;
}

/** One account call manager persists across workspace routes; media mounts only in the answering document. */
export function DirectCallsProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const userId = user?.guestConferenceId ? undefined : user?.id;
  const { pathname } = useLocation();
  const client = useQueryClient();
  const [call, setCall] = useState<DirectCall | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [mediaStopped, setMediaStopped] = useState(false);
  const operation = useRef<AbortController | null>(null);
  const actor = useRef(userId);
  actor.current = userId;
  const current = useRef(call);
  current.current = call;
  const intent = useRef(new Set<string>());
  const trigger = useRef<HTMLElement | null>(null);
  const apply = useCallback((next: DirectCall | null) => {
    if (
      !next ||
      !actor.current ||
      ![next.caller.id, next.callee.id].includes(actor.current)
    )
      return;
    // Recover a committed start/accept whose HTTP response was lost in this document.
    if (
      next.callerClientId === clientId ||
      next.acceptedByClientId === clientId
    )
      intent.current.add(next.id);
    setCall((old) => {
      if (!ongoing(next) && old?.id !== next.id) return old;
      if (old?.id === next.id && old.version > next.version) return old;
      if (old && ongoing(old) && old.id !== next.id) return old;
      return next;
    });
  }, []);
  const active = useQuery({
    queryKey: ["direct-call-active", userId],
    queryFn: ({ signal }) => api.activeDirectCall(signal),
    enabled: !!userId,
    refetchInterval: 10000,
    refetchIntervalInBackground: true,
    retry: false,
  });
  const detail = useQuery({
    queryKey: ["direct-call", userId, call?.id],
    queryFn: ({ signal }) => api.directCall(call!.id, signal),
    enabled: !!userId && !!call && ongoing(call),
    refetchInterval: 3000,
    refetchIntervalInBackground: true,
    retry: false,
  });
  useEffect(() => {
    apply(active.data?.item || null);
  }, [active.data, apply]);
  useEffect(() => {
    apply(detail.data?.item || null);
  }, [detail.data, apply]);
  useEffect(() => {
    setCall(null);
    setPending(false);
    setError(null);
    setMediaStopped(false);
    intent.current.clear();
    return () => {
      operation.current?.abort();
      operation.current = null;
    };
  }, [userId]);
  useEffect(() => {
    if (!userId) return;
    const receive = (event: Event) => {
      const value = (event as CustomEvent<{ userId: string; call: DirectCall }>)
        .detail;
      if (value.userId !== userId) return;
      apply(value.call);
      void client.invalidateQueries({
        queryKey: ["direct-call-active", userId],
      });
      void client.invalidateQueries({ queryKey: ["personal-list", userId] });
      void client.invalidateQueries({ queryKey: ["personal-summary", userId] });
      void client.invalidateQueries({
        queryKey: ["personal-chat", value.call.conversationId, userId],
      });
    };
    window.addEventListener(changedEvent, receive);
    return () => window.removeEventListener(changedEvent, receive);
  }, [apply, client, userId]);

  const owned =
    !!call &&
    (call.caller.id === userId
      ? call.callerClientId === clientId
      : call.acceptedByClientId === clientId);
  const incoming = !!call && call.callee.id === userId;
  const conferencePage =
    /^\/(?:meetings|conferences)\/[^/]+$/.test(pathname) &&
    !pathname.endsWith("/new");
  const lastHeartbeat = useRef(0);
  useEffect(() => {
    if (!call || !owned || call.status !== "accepted" || !userId) return;
    const abort = new AbortController();
    let sending = false;
    lastHeartbeat.current = Date.now();
    const beat = async () => {
      if (sending || abort.signal.aborted) return;
      sending = true;
      try {
        const result = await api.directCallAction(
          call.id,
          "heartbeat",
          clientId,
          AbortSignal.any([abort.signal, AbortSignal.timeout(8000)]),
        );
        if (!abort.signal.aborted) {
          lastHeartbeat.current = Date.now();
          apply(result.item);
        }
      } catch {
        if (
          !abort.signal.aborted &&
          Date.now() - lastHeartbeat.current > 35000
        ) {
          setMediaStopped(true);
          setError(
            new Error(
              "Связь с сервером потеряна. Микрофон и камера отключены. Завершите звонок и попробуйте снова.",
            ),
          );
        }
      } finally {
        sending = false;
      }
    };
    void beat();
    const timer = window.setInterval(() => void beat(), 10000);
    return () => {
      abort.abort();
      window.clearInterval(timer);
    };
  }, [call?.id, call?.status, owned, userId, apply]);
  useEffect(() => {
    const hide = () => setMediaStopped(true);
    window.addEventListener("pagehide", hide);
    return () => window.removeEventListener("pagehide", hide);
  }, []);

  const startCall = async (targetUserId: string) => {
    if (
      !userId ||
      operation.current ||
      (current.current && ongoing(current.current))
    )
      return;
    if (conferencePage) {
      setError(new Error("Перед личным звонком покиньте конференцию."));
      return;
    }
    if (targetUserId === userId) return;
    const abort = new AbortController();
    operation.current = abort;
    const expected = userId;
    trigger.current = document.activeElement as HTMLElement | null;
    setPending(true);
    setError(null);
    setMediaStopped(false);
    try {
      const result = await api.startDirectCall(
        targetUserId,
        crypto.randomUUID(),
        clientId,
        AbortSignal.any([abort.signal, AbortSignal.timeout(15000)]),
      );
      if (!abort.signal.aborted && actor.current === expected) {
        intent.current.add(result.item.id);
        apply(result.item);
      }
    } catch (cause) {
      if (!abort.signal.aborted)
        setError(
          cause instanceof Error
            ? cause
            : new Error("Не удалось начать звонок."),
        );
    } finally {
      if (operation.current === abort) {
        operation.current = null;
        setPending(false);
      }
    }
  };
  const action = async (kind: "accept" | "decline" | "cancel" | "end") => {
    if (!call || operation.current || !userId) return;
    if (kind === "accept" && conferencePage) {
      setError(new Error("Покиньте конференцию, чтобы ответить на звонок."));
      return;
    }
    const abort = new AbortController();
    operation.current = abort;
    const expected = userId;
    setPending(true);
    setError(null);
    if (kind === "end" || kind === "cancel") setMediaStopped(true);
    try {
      const result = await api.directCallAction(
        call.id,
        kind,
        clientId,
        AbortSignal.any([abort.signal, AbortSignal.timeout(15000)]),
      );
      if (!abort.signal.aborted && actor.current === expected) {
        if (kind === "accept") {
          intent.current.add(call.id);
          setMediaStopped(false);
        }
        apply(result.item);
      }
    } catch (cause) {
      if (!abort.signal.aborted) {
        setError(
          cause instanceof Error
            ? cause
            : new Error("Не удалось изменить состояние звонка."),
        );
        void client.invalidateQueries({
          queryKey: ["direct-call", userId, call.id],
        });
      }
    } finally {
      if (operation.current === abort) {
        operation.current = null;
        setPending(false);
      }
    }
  };
  const close = () => {
    if (call && ongoing(call)) {
      if (call.status === "ringing")
        void action(incoming ? "decline" : "cancel");
      else if (owned) void action("end");
      else setCall(null);
      return;
    }
    setCall(null);
    setError(null);
  };
  const show =
    call &&
    (call.status === "ringing"
      ? incoming || owned
      : call.status !== "accepted" || owned);
  return (
    <DirectCallsContext.Provider
      value={{ startCall, busy: pending || (!!call && ongoing(call)), error }}
    >
      {children}
      {error && !show && (
        <div className="direct-call-notice" role="alert">
          <span>{error.message}</span>
          <button
            type="button"
            aria-label="Закрыть сообщение"
            onClick={() => setError(null)}
          >
            <X size={18} />
          </button>
        </div>
      )}
      {show && userId && (
        <Modal
          title="Личный звонок"
          className="direct-call-modal"
          wide
          onClose={close}
          returnFocus={() => trigger.current}
        >
          {error && <ErrorNotice>{error.message}</ErrorNotice>}
          {call.status === "accepted" &&
          owned &&
          !mediaStopped &&
          call.conferenceId &&
          intent.current.has(call.id) ? (
            <DirectCallMedia
              key={call.id}
              call={call}
              userId={userId}
              onEnd={() => void action("end")}
              pending={pending}
            />
          ) : (
            <CallRinging
              call={call}
              userId={userId}
              incoming={incoming}
              owned={owned}
              pending={pending}
              onAccept={() => void action("accept")}
              onEnd={close}
            />
          )}
        </Modal>
      )}
    </DirectCallsContext.Provider>
  );
}

function CallRinging({
  call,
  userId,
  incoming,
  owned,
  pending,
  onAccept,
  onEnd,
}: {
  call: DirectCall;
  userId: string;
  incoming: boolean;
  owned: boolean;
  pending: boolean;
  onAccept: () => void;
  onEnd: () => void;
}) {
  const peer = incoming ? call.caller : call.callee;
  const player = useRef<HTMLAudioElement | null>(null);
  const [soundBlocked, setSoundBlocked] = useState(false);
  useEffect(() => {
    setSoundBlocked(false);
    if (call.status !== "ringing" || (!incoming && !owned)) return;
    const audio = new Audio(toneURL);
    audio.loop = true;
    audio.volume = 0.25;
    let disposed = false;
    let release: (() => void) | undefined;
    const preferences = readDevicePreferences(userId);
    const play = async () => {
      if (!preferences.notificationSounds || disposed) return;
      if (preferences.notificationOutputId && "setSinkId" in audio)
        await audio.setSinkId(preferences.notificationOutputId).catch(() => {});
      if (!disposed) {
        player.current = audio;
        await audio.play().catch(() => {
          if (!disposed) setSoundBlocked(true);
        });
      }
    };
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    if (navigator.locks) {
      void navigator.locks
        .request(
          `meet.call-ring:${userId}:${call.id}`,
          { ifAvailable: true },
          async (lock) => {
            if (!lock || disposed) return;
            await play();
            await held;
          },
        )
        .catch(() => {});
    } else void play();
    return () => {
      disposed = true;
      release?.();
      audio.pause();
      audio.removeAttribute("src");
      audio.load();
      if (player.current === audio) player.current = null;
    };
  }, [call.id, call.status, incoming, owned, userId]);
  const label =
    call.status === "ringing"
      ? incoming
        ? "Входящий звонок"
        : owned
          ? "Вы звоните…"
          : "Звонок открыт на другом устройстве"
      : {
          accepted: "Звонок подключён",
          declined: "Звонок отклонён",
          cancelled: "Звонок отменён",
          missed: "Нет ответа",
          ended: "Звонок завершён",
        }[call.status];
  return (
    <>
      <div className="direct-call-person">
        <div className="direct-call-avatar">
          {peer.avatarUrl ? (
            <img src={peer.avatarUrl} alt="" />
          ) : (
            initials(peer.displayName)
          )}
        </div>
        <h3>{peer.displayName}</h3>
        <p role="status" aria-live="polite">
          {label}
        </p>
        {soundBlocked && call.status === "ringing" && (
          <Button
            variant="outline"
            onClick={() => {
              void player.current
                ?.play()
                .then(() => setSoundBlocked(false))
                .catch(() => {});
            }}
          >
            Включить звук звонка
          </Button>
        )}
      </div>
      <div className="direct-call-controls">
        {call.status === "ringing" && incoming && (
          <Button
            className="direct-call-accept"
            onClick={onAccept}
            disabled={pending}
          >
            <Phone size={22} />
            Ответить
          </Button>
        )}
        <Button className="direct-call-end" onClick={onEnd} disabled={pending}>
          <PhoneOff size={22} />
          {ongoing(call)
            ? incoming && call.status === "ringing"
              ? "Отклонить"
              : "Завершить"
            : "Закрыть"}
        </Button>
      </div>
    </>
  );
}

function DirectCallMedia({
  call,
  userId,
  onEnd,
  pending,
}: {
  call: DirectCall;
  userId: string;
  onEnd: () => void;
  pending: boolean;
}) {
  const live = useRealtime(call.conferenceId!, true, clientId);
  const media = useMedia(live, call.conferenceId!, {});
  const [preferences, setPreferences] = useState(() =>
    readDevicePreferences(userId),
  );
  const [chat, setChat] = useState(false);
  const [now, setNow] = useState(Date.now);
  const started = useRef("");
  const desired = useRef({
    microphoneEnabled: preferences.microphoneEnabled,
    cameraEnabled: false,
  });
  const peer = call.caller.id === userId ? call.callee : call.caller;
  const self = call.caller.id === userId ? call.caller : call.callee;
  useEffect(() => subscribeDevicePreferences(userId, setPreferences), [userId]);
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);
  useEffect(() => {
    const connectionId = live.state?.connectionId;
    if (!connectionId || started.current === connectionId) return;
    started.current = connectionId;
    media.start(true, { ...preferences, ...desired.current });
  }, [live.state?.connectionId]);
  useEffect(() => {
    if (media.running) media.configure(preferences);
  }, [preferences, media.configure, media.running, media.view.controlBusy]);
  const seconds = Math.max(
    0,
    Math.floor((now - Date.parse(call.acceptedAt || call.createdAt)) / 1000),
  );
  const elapsed = `${Math.floor(seconds / 60)
    .toString()
    .padStart(2, "0")}:${(seconds % 60).toString().padStart(2, "0")}`;
  const busy = pending || media.view.controlBusy || !media.view.mediaPeerId;
  const remote = media.view.remoteStreams.filter((stream) => !stream.screen);
  const screens = media.view.remoteStreams.filter((stream) => stream.screen);
  return (
    <>
      <div className="direct-call-meta">
        <span>{peer.displayName}</span>
        <time>{elapsed}</time>
      </div>
      <div
        className={`direct-call-body${chat ? " direct-call-with-chat" : ""}`}
      >
        <div className="direct-call-stage">
          {preferences.showSelf && (
            <div className="direct-call-self">
              <MediaTile
                stream={media.view.localStream || undefined}
                name={self.displayName}
                local
                video={media.view.cameraEnabled}
                microphoneEnabled={media.view.microphoneEnabled}
              />
            </div>
          )}
          <div className="direct-call-peer">
            {media.view.localScreen && (
              <MediaTile
                stream={media.view.localScreen}
                name="Ваш экран"
                local
                screen
              />
            )}
            {screens.map((stream) => (
              <MediaTile
                key={stream.id}
                stream={stream.stream}
                name={`${peer.displayName} · экран`}
                screen
                sinkId={preferences.audioOutputId}
              />
            ))}
            {remote.length
              ? remote.map((stream) => (
                  <MediaTile
                    key={stream.id}
                    stream={stream.stream}
                    name={peer.displayName}
                    video={
                      stream.kinds.includes("video") &&
                      !preferences.hideParticipantVideo
                    }
                    sinkId={preferences.audioOutputId}
                    microphoneEnabled={
                      live.state?.participants.find(
                        (participant) =>
                          participant.id === stream.participantId,
                      )?.microphoneEnabled ?? stream.kinds.includes("audio")
                    }
                    covered={!!media.view.localScreen || screens.length > 0}
                  />
                ))
              : !screens.length &&
                !media.view.localScreen && (
                  <div className="direct-call-person">
                    <div className="direct-call-avatar">
                      {peer.avatarUrl ? (
                        <img src={peer.avatarUrl} alt="" />
                      ) : (
                        initials(peer.displayName)
                      )}
                    </div>
                    <h3>{peer.displayName}</h3>
                    <p role="status">
                      {media.view.connectionState === "connected"
                        ? "Ожидаем собеседника…"
                        : "Подключаем звонок…"}
                    </p>
                  </div>
                )}
          </div>
        </div>
        {chat && (
          <aside className="direct-call-chat">
            <header>
              <strong>Чат</strong>
              <button
                type="button"
                className="icon-button"
                onClick={() => setChat(false)}
                aria-label="Закрыть чат"
              >
                <X size={20} />
              </button>
            </header>
            <MessageThread
              scopeId={call.conversationId}
              transport={personalChatAPI}
              personal
            />
          </aside>
        )}
      </div>
      <ErrorNotice error={live.error} />
      {media.view.error && <ErrorNotice>{media.view.error}</ErrorNotice>}
      <div className="direct-call-controls">
        {!media.running ? (
          <Button
            onClick={() => media.start(false)}
            disabled={!live.state?.connectionId}
          >
            Подключиться без микрофона
          </Button>
        ) : (
          <>
            <button
              type="button"
              className="direct-call-control"
              disabled={busy}
              aria-pressed={media.view.microphoneEnabled}
              onClick={() => {
                desired.current.microphoneEnabled =
                  !media.view.microphoneEnabled;
                void media.microphone(desired.current.microphoneEnabled);
              }}
            >
              {media.view.microphoneEnabled ? <Mic /> : <MicOff />}
              <span>Микрофон</span>
            </button>
            <button
              type="button"
              className="direct-call-control"
              disabled={busy}
              aria-pressed={media.view.cameraEnabled}
              onClick={() => {
                desired.current.cameraEnabled = !media.view.cameraEnabled;
                void media.camera(desired.current.cameraEnabled);
              }}
            >
              {media.view.cameraEnabled ? <Video /> : <VideoOff />}
              <span>Камера</span>
            </button>
            <button
              type="button"
              className="direct-call-control"
              disabled={busy}
              aria-pressed={media.view.screenSharing}
              onClick={() =>
                void (media.view.screenSharing
                  ? media.stopScreen()
                  : media.startScreen())
              }
            >
              <MonitorUp />
              <span>Экран</span>
            </button>
          </>
        )}
        <button
          type="button"
          className="direct-call-control"
          aria-pressed={chat}
          onClick={() => setChat(!chat)}
        >
          <MessageCircle />
          <span>Чат</span>
        </button>
        <button
          type="button"
          className="direct-call-control"
          onClick={() => {
            media.stop();
            live.reconnect();
          }}
        >
          <RefreshCw />
          <span>Переподключить</span>
        </button>
        <Button className="direct-call-end" onClick={onEnd} disabled={pending}>
          <PhoneOff size={24} />
          <span>Завершить</span>
        </Button>
      </div>
    </>
  );
}
