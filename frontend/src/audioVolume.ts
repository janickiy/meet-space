/** Начальные уровни микрофона, динамика и сигналов; значения от 0 до 1. */
export const DEFAULT_AUDIO_VOLUMES = {
  microphoneVolume: 0.85,
  speakerVolume: 0.85,
  signalVolume: 1,
} as const;

/** Проверяет сохранённый уровень и ограничивает его диапазоном 0–100%. */
export function normalizeAudioVolume(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value)
    ? Math.max(0, Math.min(1, value))
    : fallback;
}
