import { DEFAULT_AUDIO_VOLUMES, normalizeAudioVolume } from "./audioVolume";

export interface DevicePreferences {
  audioInputId: string;
  videoInputId: string;
  audioOutputId: string;
  microphoneEnabled: boolean;
  cameraEnabled: boolean;
  notificationOutputId: string;
  notificationSounds: boolean;
  microphoneVolume: number;
  speakerVolume: number;
  signalVolume: number;
  noiseSuppression: boolean;
  showSelf: boolean;
  hideParticipantVideo: boolean;
}

// Один явный клик по кнопке «Подключиться» может запустить воспроизведение медиа после
// авторизации.  Это намерение существует только на этой вкладке и используется однократно;
// повторное подключение никогда не активирует устройства.
const mediaEntries = new Map<string, number>();
export function queueMediaEntry(conferenceId: string) {
  mediaEntries.set(conferenceId, Date.now());
}
export function consumeMediaEntry(conferenceId: string): boolean {
  const requestedAt = mediaEntries.get(conferenceId);
  mediaEntries.delete(conferenceId);
  return requestedAt !== undefined && Date.now() - requestedAt < 60 * 60 * 1000;
}

const emptyPreferences: DevicePreferences = {
  ...DEFAULT_AUDIO_VOLUMES,
  audioInputId: "",
  videoInputId: "",
  audioOutputId: "",
  microphoneEnabled: true,
  cameraEnabled: true,
  notificationOutputId: "",
  notificationSounds: true,
  noiseSuppression: true,
  showSelf: true,
  hideParticipantVideo: false,
};

const key = (userId: string) => `meet.devices.v1:${userId}`;
const safeId = (value: unknown) =>
  typeof value === "string" && value.length <= 512 ? value : "";

/** ID устройств используются только как предпочтения и не определяют личность участника. */
export function readDevicePreferences(userId: string): DevicePreferences {
  if (!userId) return { ...emptyPreferences };
  try {
    const data: unknown = JSON.parse(
      localStorage.getItem(key(userId)) || "null",
    );
    if (!data || typeof data !== "object") return { ...emptyPreferences };
    const value = data as Record<string, unknown>;
    return {
      audioInputId: safeId(value.audioInputId),
      videoInputId: safeId(value.videoInputId),
      audioOutputId: safeId(value.audioOutputId),
      microphoneEnabled: value.microphoneEnabled === true,
      cameraEnabled: value.cameraEnabled === true,
      notificationOutputId: safeId(value.notificationOutputId),
      notificationSounds: value.notificationSounds !== false,
      ...readAudioVolumes(value),
      noiseSuppression: value.noiseSuppression !== false,
      showSelf: value.showSelf !== false,
      hideParticipantVideo: value.hideParticipantVideo === true,
    };
  } catch {
    return { ...emptyPreferences };
  }
}

type StoredAudioVolumes = Partial<
  Record<
    | "microphoneVolume"
    | "speakerVolume"
    | "signalVolume"
    | "notificationVolume"
    | "ringtoneVolume",
    unknown
  >
>;

/** Переносит прежние раздельные уровни, сохраняя пользовательское уменьшение громкости. */
function readSignalVolume(value: StoredAudioVolumes) {
  if (Object.hasOwn(value, "signalVolume"))
    return normalizeAudioVolume(
      value.signalVolume,
      DEFAULT_AUDIO_VOLUMES.signalVolume,
    );
  const customized = [
    [value.notificationVolume, 0.35],
    [value.ringtoneVolume, 0.75],
  ].flatMap(([volume, previousDefault]) =>
    typeof volume === "number" &&
    Number.isFinite(volume) &&
    volume !== previousDefault
      ? [normalizeAudioVolume(volume, DEFAULT_AUDIO_VOLUMES.signalVolume)]
      : [],
  );
  return customized.length
    ? Math.min(...customized)
    : DEFAULT_AUDIO_VOLUMES.signalVolume;
}

function readAudioVolumes(value: StoredAudioVolumes) {
  return {
    microphoneVolume: normalizeAudioVolume(
      value.microphoneVolume,
      DEFAULT_AUDIO_VOLUMES.microphoneVolume,
    ),
    speakerVolume: normalizeAudioVolume(
      value.speakerVolume,
      DEFAULT_AUDIO_VOLUMES.speakerVolume,
    ),
    signalVolume: readSignalVolume(value),
  };
}

export function hasDevicePreferences(userId: string) {
  if (!userId) return false;
  try {
    return localStorage.getItem(key(userId)) !== null;
  } catch {
    return false;
  }
}

export function saveDevicePreferences(
  userId: string,
  preferences: DevicePreferences,
) {
  if (!userId) return;
  const normalized = { ...preferences, ...readAudioVolumes(preferences) };
  try {
    localStorage.setItem(key(userId), JSON.stringify(normalized));
  } catch {
    // Хранилище может быть отключено; встреча продолжает работать с устройствами по умолчанию.
  }
  window.dispatchEvent(
    new CustomEvent("meet:devices", {
      detail: { userId, preferences: normalized },
    }),
  );
}

/** Обновляет открытые настройки и встречу, в том числе в соседней вкладке. */
export function subscribeDevicePreferences(
  userId: string,
  listener: (preferences: DevicePreferences) => void,
) {
  const local = (event: Event) => {
    const detail = (
      event as CustomEvent<{ userId: string; preferences: DevicePreferences }>
    ).detail;
    if (detail.userId === userId) listener(detail.preferences);
  };
  const storage = (event: StorageEvent) => {
    if (event.key === key(userId) || event.key === null)
      listener(readDevicePreferences(userId));
  };
  window.addEventListener("meet:devices", local);
  window.addEventListener("storage", storage);
  return () => {
    window.removeEventListener("meet:devices", local);
    window.removeEventListener("storage", storage);
  };
}

/** Сохраняет непрозрачное предпочтение, пока браузер не предоставит реальные ID устройств. */
export function availableDeviceId(
  devices: MediaDeviceInfo[],
  kind: MediaDeviceKind,
  preferredId: string,
) {
  if (!preferredId) return "";
  const known = devices.filter(
    (device) =>
      device.kind === kind &&
      device.deviceId &&
      device.deviceId !== "default" &&
      device.deviceId !== "communications",
  );
  return known.length &&
    !known.some((device) => device.deviceId === preferredId)
    ? ""
    : preferredId;
}

export function captureErrorMessage(error: unknown, kind: "audio" | "video") {
  const name =
    error &&
    typeof error === "object" &&
    "name" in error &&
    typeof error.name === "string"
      ? error.name
      : "";
  const label = kind === "audio" ? "микрофон" : "камеру";
  if (name === "NotAllowedError" || name === "PermissionDeniedError")
    return `Доступ к устройству запрещён. Разрешите ${label} для этого сайта в настройках браузера и повторите.`;
  if (name === "NotFoundError" || name === "DevicesNotFoundError")
    return `Устройство не найдено. Подключите ${label} или войдите без него.`;
  if (name === "NotReadableError" || name === "TrackStartError")
    return `Не удалось открыть ${label}. Возможно, устройство занято другим приложением.`;
  if (name === "OverconstrainedError")
    return `Выбранное устройство недоступно. Выберите другое или системное устройство.`;
  if (name === "SecurityError")
    return "Браузер запретил доступ к устройствам. Откройте страницу по HTTPS или через localhost.";
  return `Не удалось включить ${label}. Проверьте разрешения и попробуйте снова.`;
}
