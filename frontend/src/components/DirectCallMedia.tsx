import { useEffect, useRef, useState, type CSSProperties } from "react";
import {
  Mic,
  MicOff,
  MonitorUp,
  PhoneOff,
  Settings,
  Video,
  VideoOff,
} from "lucide-react";
import type { DirectCall } from "../types";
import { useRealtime } from "../realtime";
import { useMedia } from "../useMedia";
import {
  readDevicePreferences,
  subscribeDevicePreferences,
  type DevicePreferences,
} from "../prejoinDevices";
import { useSpeakingParticipants } from "../useSpeakingParticipants";
import { meetingShortcut } from "../conferenceShortcuts";
import { initials } from "../utils";
import { MediaTile } from "./RealtimePanel";
import { useAccountSettings } from "./AccountSettingsContext";
import { Button, ErrorNotice } from "./ui";

/**
 * В приватной комнате для двоих используется медиа-клиент для конференций и настройки аккаунта.
 * Документ, в котором находится вызов, поддерживает работу потоков, пока открыты настройки
 * аккаунта.
 */
export function DirectCallMedia({
  call,
  userId,
  clientId,
  initialDevices,
  onEnd,
  pending,
}: {
  call: DirectCall;
  userId: string;
  clientId: string;
  initialDevices: Pick<
    DevicePreferences,
    "microphoneEnabled" | "cameraEnabled"
  >;
  onEnd: () => void;
  pending: boolean;
}) {
  const live = useRealtime(call.conferenceId!, true, clientId);
  const media = useMedia(live, call.conferenceId!, {});
  const openSettings = useAccountSettings();
  const [preferences, setPreferences] = useState(() =>
    readDevicePreferences(userId),
  );
  const [online, setOnline] = useState(() => navigator.onLine);
  const [now, setNow] = useState(Date.now);
  const started = useRef("");
  const body = useRef<HTMLDivElement>(null);
  const desired = useRef({ ...initialDevices });
  const peer = call.caller.id === userId ? call.callee : call.caller;
  const self = call.caller.id === userId ? call.caller : call.callee;
  useEffect(() => subscribeDevicePreferences(userId, setPreferences), [userId]);
  useEffect(() => {
    const update = () => setOnline(navigator.onLine);
    window.addEventListener("online", update);
    window.addEventListener("offline", update);
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => {
      window.removeEventListener("online", update);
      window.removeEventListener("offline", update);
      window.clearInterval(timer);
    };
  }, []);
  useEffect(() => {
    const connectionId = live.state?.connectionId;
    if (!connectionId || started.current === connectionId) return;
    started.current = connectionId;
    media.start(true, { ...preferences, ...desired.current });
  }, [live.state?.connectionId]);
  useEffect(() => {
    if (media.running) media.configure(preferences);
  }, [
    preferences,
    media.configure,
    media.running,
    media.view.mediaPeerId,
    media.view.controlBusy,
  ]);
  // Перед заменой подключения запоминаем фактическое состояние устройства, в том числе в случае
  // сбоев.
  useEffect(() => {
    if (
      media.view.active &&
      media.view.mediaPeerId &&
      !media.view.controlBusy &&
      media.view.connectionState !== "closed"
    ) {
      desired.current = {
        microphoneEnabled: media.view.microphoneEnabled,
        cameraEnabled: media.view.cameraEnabled,
      };
    }
  }, [
    media.view.active,
    media.view.connectionState,
    media.view.mediaPeerId,
    media.view.controlBusy,
    media.view.microphoneEnabled,
    media.view.cameraEnabled,
  ]);

  const connectionProblem =
    !online ||
    !live.state ||
    ["failed", "disconnected", "closed"].includes(media.view.connectionState) ||
    ["failed", "disconnected", "closed"].includes(media.view.iceState);
  const busy =
    pending ||
    media.view.controlBusy ||
    !media.view.mediaPeerId ||
    connectionProblem;
  const peerPresence = live.state?.participants.find(
    (person) => person.userId === peer.id,
  );
  // Только собеседник может занимать удалённую плитку, в том числе во время повторного
  // подключения.
  const remote = media.view.remoteStreams.filter(
    (stream) =>
      peerPresence?.online &&
      stream.participantId === peerPresence.id &&
      !stream.screen,
  );
  const screens = media.view.remoteStreams.filter(
    (stream) =>
      peerPresence?.online &&
      stream.participantId === peerPresence.id &&
      stream.screen,
  );
  const showingScreen =
    !!media.view.localScreen ||
    (!preferences.hideParticipantVideo && screens.length > 0);
  const speaking = useSpeakingParticipants(
    [
      {
        id: "local",
        participantId: self.id,
        stream: media.view.localStream,
        enabled: media.view.microphoneEnabled,
      },
      ...remote.map((stream) => ({
        id: stream.id,
        participantId: peer.id,
        stream: stream.stream,
        enabled:
          peerPresence?.microphoneEnabled ?? stream.kinds.includes("audio"),
      })),
    ],
    media.running && !connectionProblem,
  );
  const localLevel = speaking.get(self.id) ?? 0;
  const seconds = Math.max(
    0,
    Math.floor((now - Date.parse(call.acceptedAt || call.createdAt)) / 1000),
  );
  const elapsed = `${Math.floor(seconds / 60)
    .toString()
    .padStart(2, "0")}:${(seconds % 60).toString().padStart(2, "0")}`;
  useEffect(() => {
    const dialog = body.current?.closest('[role="dialog"]');
    if (!dialog) return;
    const handle = (event: Event) => {
      const key = meetingShortcut(event as KeyboardEvent, ["m", "v"], dialog);
      if (!busy && key) {
        event.preventDefault();
        if (key === "m") void media.microphone(!media.view.microphoneEnabled);
        else void media.camera(!media.view.cameraEnabled);
      }
    };
    dialog.addEventListener("keydown", handle);
    return () => dialog.removeEventListener("keydown", handle);
  }, [busy, media.view.microphoneEnabled, media.view.cameraEnabled]);
  const status = !online
    ? "Нет подключения к сети"
    : connectionProblem
      ? "Восстанавливаем связь…"
      : media.view.connectionState === "connected"
        ? "Подключено"
        : "Подключаем звонок…";

  return (
    <>
      <div className="direct-call-meta">
        <span>{peer.displayName}</span>
        <time aria-label={`Длительность звонка: ${elapsed}`}>{elapsed}</time>
      </div>
      <div ref={body} className="direct-call-body">
        <div className="direct-call-stage">
          {preferences.showSelf && (
            <div className="direct-call-self">
              <MediaTile
                stream={media.view.localStream || undefined}
                name={self.displayName}
                local
                video={media.view.cameraEnabled}
                microphoneEnabled={media.view.microphoneEnabled}
                audioLevel={localLevel}
                reconnecting={connectionProblem}
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
                covered={preferences.hideParticipantVideo}
                reconnecting={connectionProblem}
              />
            ))}
            {remote.map((stream) => (
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
                  peerPresence?.microphoneEnabled ??
                  stream.kinds.includes("audio")
                }
                audioLevel={speaking.get(peer.id) ?? 0}
                covered={showingScreen}
                reconnecting={connectionProblem}
              />
            ))}
            {!remote.length && !showingScreen && (
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
                  {peerPresence?.online
                    ? "Собеседник в звонке"
                    : media.view.connectionState === "connected"
                      ? "Ожидаем собеседника…"
                      : status}
                </p>
              </div>
            )}
          </div>
        </div>
      </div>
      {connectionProblem && (
        <p className="direct-call-connection-notice" role="status">
          {status}
        </p>
      )}
      <ErrorNotice error={live.error} />
      {media.view.error && <ErrorNotice>{media.view.error}</ErrorNotice>}
      {!media.running && (
        <div className="direct-call-retry">
          <Button
            disabled={!live.state || !online || pending}
            onClick={() =>
              media.start(true, { ...preferences, ...desired.current })
            }
          >
            Подключить медиасвязь снова
          </Button>
          <Button
            variant="secondary"
            disabled={!live.state || !online || pending}
            onClick={() => {
              desired.current = {
                microphoneEnabled: false,
                cameraEnabled: false,
              };
              media.start(false, preferences);
            }}
          >
            Подключиться без камеры и микрофона
          </Button>
        </div>
      )}
      <div
        className="direct-call-controls"
        role="group"
        aria-label="Управление звонком"
      >
        <button
          type="button"
          className="direct-call-control"
          disabled={busy}
          aria-pressed={media.view.microphoneEnabled}
          aria-label={
            media.view.microphoneEnabled
              ? "Выключить микрофон"
              : "Включить микрофон"
          }
          title="Микрофон"
          onClick={() => void media.microphone(!media.view.microphoneEnabled)}
        >
          <span
            className="media-tile-microphone"
            style={{ "--audio-level": localLevel } as CSSProperties}
          >
            {media.view.microphoneEnabled ? <Mic /> : <MicOff />}
          </span>
        </button>
        <button
          type="button"
          className="direct-call-control"
          disabled={busy}
          aria-pressed={media.view.cameraEnabled}
          aria-label={
            media.view.cameraEnabled ? "Выключить камеру" : "Включить камеру"
          }
          title="Камера"
          onClick={() => void media.camera(!media.view.cameraEnabled)}
        >
          {media.view.cameraEnabled ? <Video /> : <VideoOff />}
        </button>
        <button
          type="button"
          className="direct-call-control direct-call-control-wide"
          disabled={busy}
          aria-pressed={media.view.screenSharing}
          aria-label={
            media.view.screenSharing
              ? "Остановить демонстрацию экрана"
              : "Начать демонстрацию экрана"
          }
          title="Демонстрация экрана"
          onClick={() =>
            void (media.view.screenSharing
              ? media.stopScreen()
              : media.startScreen())
          }
        >
          <MonitorUp />
          <span>Демонстрация</span>
        </button>
        <button
          type="button"
          className="direct-call-control"
          aria-label="Настройки"
          title="Настройки"
          disabled={!openSettings}
          onClick={(event) => openSettings?.(event.currentTarget)}
        >
          <Settings />
        </button>
        <button
          type="button"
          className="direct-call-control direct-call-end"
          aria-label="Завершить звонок"
          title="Завершить звонок"
          disabled={pending}
          onClick={onEnd}
        >
          <PhoneOff />
        </button>
      </div>
    </>
  );
}
