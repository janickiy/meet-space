import toneURL from "./assets/device-tone.wav";
import {
  readDevicePreferences,
  subscribeDevicePreferences,
} from "./prejoinDevices";

/** Один многократно используемый проигрыватель для каждого потока учётной записи; микрофон и push-уведомления браузера не запрашиваются. */
export function createNotificationSound(userId: string) {
  const audio = new Audio(toneURL);
  audio.preload = "auto";
  audio.volume = 0.35;
  let disposed = false;
  let unlocked = false;
  let priming: Promise<void> | undefined;
  let lastPlayedAt = 0;
  const seen = new Set<string>();
  const claimKey = `meet.notification-sound.v1:${userId}`;

  // При воспроизведении по инициативе пользователя без звука этот же элемент разблокируется,
  // когда браузеру требуется взаимодействие.  Старые уведомления никогда не помещаются в
  // очередь для последующего нажатия.
  const prime = () => {
    if (
      disposed ||
      unlocked ||
      priming ||
      !readDevicePreferences(userId).notificationSounds
    )
      return;
    audio.muted = true;
    priming = audio
      .play()
      .then(() => {
        if (disposed) return;
        audio.pause();
        audio.currentTime = 0;
        unlocked = true;
      })
      .catch(() => {})
      .finally(() => {
        audio.muted = false;
        priming = undefined;
      });
  };
  window.addEventListener("pointerdown", prime);
  window.addEventListener("keydown", prime);
  const unsubscribe = subscribeDevicePreferences(userId, (preferences) => {
    if (!preferences.notificationSounds) audio.pause();
  });

  const claim = (id: string) => {
    if (seen.has(id)) return false;
    seen.add(id);
    if (seen.size > 128) seen.delete(seen.values().next().value!);
    try {
      const raw: unknown = JSON.parse(localStorage.getItem(claimKey) || "[]");
      const recent = (Array.isArray(raw) ? raw : []).filter(
        (item): item is { id: string; at: number } =>
          !!item &&
          typeof item.id === "string" &&
          typeof item.at === "number" &&
          item.at > Date.now() - 120000,
      );
      if (recent.some((item) => item.id === id)) return false;
      localStorage.setItem(
        claimKey,
        JSON.stringify([...recent.slice(-127), { id, at: Date.now() }]),
      );
    } catch {
      // Окно идентификации для каждого потока по-прежнему работает при отключённом приватном
      // хранилище.
    }
    return true;
  };
  const play = async (id: string) => {
    const preferences = readDevicePreferences(userId);
    if (disposed || !preferences.notificationSounds || !claim(id)) return;
    // Объединять пакеты в один мягкий сигнал; все уведомления остаются в ленте.
    if (Date.now() - lastPlayedAt < 1000) return;
    lastPlayedAt = Date.now();
    if (priming) await priming;
    if (disposed || !readDevicePreferences(userId).notificationSounds) return;
    if (preferences.notificationOutputId && "setSinkId" in audio) {
      try {
        await audio.setSinkId(preferences.notificationOutputId);
      } catch {
        // Недоступное сохранённое устройство не должно прерывать отправку уведомлений.
        try {
          await audio.setSinkId("");
        } catch {}
      }
    }
    if (disposed || !readDevicePreferences(userId).notificationSounds) return;
    audio.currentTime = 0;
    await audio.play().catch(() => {});
  };
  return {
    notify(id: string) {
      if (!id || id.length > 128 || disposed) return;
      // Сериализация утверждений предотвращает одновременное срабатывание одного и того же
      // события SSE на нескольких вкладках.
      const pending = navigator.locks
        ? navigator.locks.request(claimKey, () => play(id))
        : play(id);
      void pending.catch(() => {});
    },
    dispose() {
      disposed = true;
      window.removeEventListener("pointerdown", prime);
      window.removeEventListener("keydown", prime);
      unsubscribe();
      audio.pause();
      audio.removeAttribute("src");
      audio.load();
    },
  };
}
