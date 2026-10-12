import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useLocation } from "react-router";
import { Mic, MicOff, Phone, PhoneOff, Video, VideoOff, X } from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import type { DirectCall } from "../types";
import {
  readDevicePreferences,
  type DevicePreferences,
} from "../prejoinDevices";
import { useCallPreview } from "../useCallPreview";
import { initials } from "../utils";
import { DirectCallMedia } from "./DirectCallMedia";
import { MediaTile } from "./RealtimePanel";
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
    setMediaStopped(false);
  }, [call?.id]);
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
    if (kind !== "accept") setMediaStopped(true);
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
          <CallSession
            key={`${userId}:${call.id}`}
            call={call}
            userId={userId}
            incoming={incoming}
            owned={owned}
            pending={pending}
            mediaStopped={mediaStopped}
            mediaAllowed={intent.current.has(call.id)}
            previewBlocked={conferencePage}
            onAccept={() => void action("accept")}
            onEnd={close}
          />
        </Modal>
      )}
    </DirectCallsContext.Provider>
  );
}

type CallDevices = Pick<
  DevicePreferences,
  "microphoneEnabled" | "cameraEnabled"
>;

// Keep the pre-answer selection across the transition from local preview to the room.
function CallSession(props: {
  call: DirectCall;
  userId: string;
  incoming: boolean;
  owned: boolean;
  pending: boolean;
  mediaStopped: boolean;
  mediaAllowed: boolean;
  previewBlocked: boolean;
  onAccept: () => void;
  onEnd: () => void;
}) {
  const [devices, setDevices] = useState<CallDevices>(() => ({
    microphoneEnabled: readDevicePreferences(props.userId).microphoneEnabled,
    cameraEnabled: false,
  }));
  const setMicrophone = useCallback((enabled: boolean) => {
    setDevices((old) => ({ ...old, microphoneEnabled: enabled }));
  }, []);
  const setCamera = useCallback((enabled: boolean) => {
    setDevices((old) => ({ ...old, cameraEnabled: enabled }));
  }, []);
  if (
    props.call.status === "accepted" &&
    props.owned &&
    !props.mediaStopped &&
    props.call.conferenceId &&
    props.mediaAllowed
  ) {
    return (
      <DirectCallMedia
        call={props.call}
        userId={props.userId}
        clientId={clientId}
        initialDevices={devices}
        onEnd={props.onEnd}
        pending={props.pending}
      />
    );
  }
  return (
    <CallRinging
      {...props}
      devices={devices}
      setMicrophone={setMicrophone}
      setCamera={setCamera}
    />
  );
}

function CallRinging({
  call,
  userId,
  incoming,
  owned,
  pending,
  mediaStopped,
  previewBlocked,
  devices,
  setMicrophone,
  setCamera,
  onAccept,
  onEnd,
}: {
  call: DirectCall;
  userId: string;
  incoming: boolean;
  owned: boolean;
  pending: boolean;
  mediaStopped: boolean;
  previewBlocked: boolean;
  devices: CallDevices;
  setMicrophone: (enabled: boolean) => void;
  setCamera: (enabled: boolean) => void;
  onAccept: () => void;
  onEnd: () => void;
}) {
  const peer = incoming ? call.caller : call.callee;
  const self = incoming ? call.callee : call.caller;
  const [preferences] = useState(() => readDevicePreferences(userId));
  // Incoming rings never capture devices until the recipient interacts or answers.
  const [microphoneArmed, setMicrophoneArmed] = useState(!incoming && owned);
  const previewEnabled =
    call.status === "ringing" && !mediaStopped && !previewBlocked;
  const microphoneUnavailable = useCallback(
    () => setMicrophone(false),
    [setMicrophone],
  );
  const cameraUnavailable = useCallback(() => setCamera(false), [setCamera]);
  const microphone = useCallPreview(
    "audio",
    previewEnabled && microphoneArmed && devices.microphoneEnabled,
    preferences.audioInputId,
    preferences.noiseSuppression,
    microphoneUnavailable,
  );
  const camera = useCallPreview(
    "video",
    previewEnabled && devices.cameraEnabled,
    preferences.videoInputId,
    false,
    cameraUnavailable,
  );
  const preview = useMemo(() => {
    const tracks = [
      ...(microphone.stream?.getTracks() || []),
      ...(camera.stream?.getTracks() || []),
    ];
    return tracks.length ? new MediaStream(tracks) : undefined;
  }, [microphone.stream, camera.stream]);
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
      <div className="direct-call-body">
        <div className="direct-call-stage">
          {call.status === "ringing" && (
            <div className="direct-call-self">
              <MediaTile
                name={self.displayName}
                local
                stream={preview}
                video={!!camera.stream}
                microphoneEnabled={!!microphone.stream}
              />
            </div>
          )}
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
        </div>
      </div>
      {call.status === "ringing" && (
        <>
          <p className="direct-call-preview-note">
            {incoming
              ? "Выберите устройства перед ответом."
              : "Собеседник услышит и увидит вас после ответа."}
          </p>
          {(microphone.error || camera.error) && (
            <ErrorNotice>{microphone.error || camera.error}</ErrorNotice>
          )}
        </>
      )}
      <div className="direct-call-controls">
        {call.status === "ringing" && (
          <>
            <button
              type="button"
              className="direct-call-control"
              aria-label={
                devices.microphoneEnabled
                  ? "Выключить микрофон"
                  : "Включить микрофон"
              }
              title={
                devices.microphoneEnabled
                  ? "Выключить микрофон"
                  : "Включить микрофон"
              }
              aria-pressed={devices.microphoneEnabled}
              aria-busy={microphone.loading}
              disabled={pending || mediaStopped || previewBlocked}
              onClick={() => {
                setMicrophoneArmed(true);
                setMicrophone(!devices.microphoneEnabled);
              }}
            >
              {devices.microphoneEnabled ? <Mic /> : <MicOff />}
            </button>
            <button
              type="button"
              className="direct-call-control"
              aria-label={
                devices.cameraEnabled ? "Выключить камеру" : "Включить камеру"
              }
              title={
                devices.cameraEnabled ? "Выключить камеру" : "Включить камеру"
              }
              aria-pressed={devices.cameraEnabled}
              aria-busy={camera.loading}
              disabled={pending || mediaStopped || previewBlocked}
              onClick={() => setCamera(!devices.cameraEnabled)}
            >
              {devices.cameraEnabled ? <Video /> : <VideoOff />}
            </button>
          </>
        )}
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
