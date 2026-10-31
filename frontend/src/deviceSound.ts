import toneURL from "./assets/device-tone.wav";
import { normalizeAudioVolume } from "./audioVolume";

/** Только локальное воспроизведение.  Данные о записи и устройстве не отправляются на сервер. */
export function playDeviceTone(
  sinkId: string,
  done: (error?: unknown) => void,
  volume = 1,
) {
  const audio = new Audio(toneURL);
  audio.volume = normalizeAudioVolume(volume, 1);
  let stopped = false;
  const finish = (error?: unknown) => {
    if (stopped) return;
    stopped = true;
    audio.pause();
    audio.removeAttribute("src");
    audio.load();
    done(error);
  };
  audio.onended = () => finish();
  audio.onerror = () => finish(new Error("audio_playback_failed"));
  void (async () => {
    try {
      if (sinkId && "setSinkId" in audio) await audio.setSinkId(sinkId);
      if (!stopped) await audio.play();
    } catch (error) {
      finish(error);
    }
  })();
  return Object.assign(() => finish(), {
    setVolume(value: number) {
      if (!stopped) audio.volume = normalizeAudioVolume(value, 1);
    },
  });
}
