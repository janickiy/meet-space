import { useEffect, useState } from "react";
import { captureErrorMessage } from "./prejoinDevices";

/** Предварительный просмотр только для локального использования. Каждому эффекту принадлежат свои треки, включая результаты позднего разрешения. */
export function useCallPreview(
  kind: "audio" | "video",
  enabled: boolean,
  deviceId: string,
  noiseSuppression: boolean,
  onUnavailable: () => void,
) {
  const [stream, setStream] = useState<MediaStream>();
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  useEffect(() => {
    setStream(undefined);
    setLoading(enabled);
    if (!enabled) return;
    let disposed = false;
    let owned: MediaStream | undefined;
    const ended = () => {
      if (!disposed) onUnavailable();
    };
    const capture = async () => {
      setError("");
      try {
        const constraints = (preferred: string): MediaStreamConstraints => ({
          audio: kind === "audio" && {
            ...(preferred ? { deviceId: { exact: preferred } } : {}),
            echoCancellation: true,
            noiseSuppression,
            autoGainControl: true,
          },
          video: kind === "video" && {
            ...(preferred ? { deviceId: { exact: preferred } } : {}),
            width: { ideal: 1280 },
            height: { ideal: 720 },
            frameRate: { ideal: 30, max: 30 },
          },
        });
        try {
          owned = await navigator.mediaDevices.getUserMedia(
            constraints(deviceId),
          );
        } catch (cause) {
          if (disposed) return;
          if (
            !deviceId ||
            !(cause instanceof DOMException) ||
            !["NotFoundError", "OverconstrainedError"].includes(cause.name)
          )
            throw cause;
          owned = await navigator.mediaDevices.getUserMedia(constraints(""));
        }
        if (disposed) {
          owned.getTracks().forEach((track) => track.stop());
          return;
        }
        owned
          .getTracks()
          .forEach((track) => track.addEventListener("ended", ended));
        setStream(owned);
      } catch (cause) {
        if (!disposed) {
          setError(captureErrorMessage(cause, kind));
          onUnavailable();
        }
      } finally {
        if (!disposed) setLoading(false);
      }
    };
    const release = () => {
      disposed = true;
      owned?.getTracks().forEach((track) => {
        track.removeEventListener("ended", ended);
        track.stop();
      });
    };
    window.addEventListener("pagehide", release);
    void capture();
    return () => {
      release();
      window.removeEventListener("pagehide", release);
    };
  }, [kind, enabled, deviceId, noiseSuppression, onUnavailable]);
  return {
    stream: enabled ? stream : undefined,
    error,
    loading: enabled && loading,
  };
}
