import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import { Clapperboard } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../api";
import { isAdmitted } from "../collaboration";
import type { ConferenceRecording, Participant } from "../types";
import { Button, ErrorNotice, Loading } from "./ui";
import "../pages/history-notifications.css";

/** Shows ready recordings without requesting speech-processing materials.
 * @param conferenceId Conference whose recordings are displayed.
 * @param membership Current participant membership, used to gate playback.
 * @param recordings Recordings returned by the authorized conference API.
 */
export function RecordingPlayback({
  conferenceId,
  membership,
  recordings,
}: {
  conferenceId: string;
  membership: Participant;
  recordings: ConferenceRecording[];
}) {
  const [params, setParams] = useSearchParams();
  const ready = recordings.filter((recording) => recording.status === "ready");
  const selected = params.get("recording") || ready[0]?.uuid;
  if (!isAdmitted(membership) || !selected) return null;
  return (
    <div className="recording-playback">
      <div className="section-heading">
        <h3>Просмотр записи</h3>
        <label>
          Запись{" "}
          <select
            aria-label="Выбрать запись"
            value={selected}
            onChange={(event) => {
              const next = new URLSearchParams(params);
              next.set("recording", event.target.value);
              next.delete("t");
              setParams(next, { replace: true });
            }}
          >
            {!ready.some((recording) => recording.uuid === selected) && (
              <option value={selected}>Запись по ссылке</option>
            )}
            {ready.map((recording, index) => (
              <option key={recording.uuid} value={recording.uuid}>
                Запись {index + 1} ·{" "}
                {new Date(recording.createdAt).toLocaleString("ru-RU")}
              </option>
            ))}
          </select>
        </label>
      </div>
      <RecordingPlayer
        key={`${membership.userId}:${conferenceId}:${selected}`}
        conferenceId={conferenceId}
        recordingId={selected}
        userId={membership.userId}
      />
    </div>
  );
}

/** Plays an authorized recording and refreshes expired signed file links.
 * @param conferenceId Conference identifier used for authorization.
 * @param recordingId Selected recording identifier.
 * @param userId Current account identifier, included in the private query key.
 */
function RecordingPlayer({
  conferenceId,
  recordingId,
  userId,
}: {
  conferenceId: string;
  recordingId: string;
  userId: string | null;
}) {
  const [params] = useSearchParams();
  const media = useRef<HTMLMediaElement | null>(null);
  const pendingSeek = useRef<number | null>(null);
  const [source, setSource] = useState<string>();
  const [playbackError, setPlaybackError] = useState(false);
  const record = useQuery({
    queryKey: ["recording-detail", userId, conferenceId, recordingId],
    queryFn: ({ signal }) => api.recording(conferenceId, recordingId, signal),
    staleTime: 60_000,
    retry: false,
  });
  const available = record.data?.item.status === "ready" && !record.isError;
  const seekOffset = params.get("t");

  /** Restores a validated playback offset once media metadata is available. */
  function applySeek() {
    if (
      !media.current ||
      pendingSeek.current === null ||
      media.current.readyState < 1
    )
      return;
    const duration = media.current.duration;
    const seconds = pendingSeek.current / 1000;
    media.current.currentTime = Number.isFinite(duration)
      ? Math.min(seconds, Math.max(0, duration))
      : seconds;
    pendingSeek.current = null;
  }

  useEffect(() => {
    const url = record.data?.item.files?.find(
      (file) =>
        file.fileType === "final_mp4" || file.fileType === "final_audio",
    )?.url;
    if (available && url) setSource((current) => current || url);
  }, [record.data, available]);
  useEffect(() => {
    const milliseconds = seekOffset === null ? NaN : Number(seekOffset);
    if (Number.isFinite(milliseconds) && milliseconds >= 0) {
      pendingSeek.current = milliseconds;
      applySeek();
    }
  }, [seekOffset]);

  if (record.isPending) return <Loading />;
  if (record.isError)
    return (
      <>
        <ErrorNotice error={record.error} />
        <Button
          variant="outline"
          busy={record.isFetching}
          onClick={() => void record.refetch()}
        >
          Проверить доступность записи
        </Button>
      </>
    );
  if (!available)
    return (
      <p className="field-hint">Просмотр доступен после готовности записи.</p>
    );

  return (
    <div className="recording-player-column">
      {source ? (
        record.data?.item.mode === "audio_only" ||
        record.data?.item.mode === "individual_tracks" ? (
          <audio
            ref={(element) => {
              media.current = element;
            }}
            aria-label="Аудиозапись встречи"
            controls
            preload="metadata"
            src={source}
            onLoadedMetadata={applySeek}
            onError={() => setPlaybackError(true)}
          />
        ) : (
          <video
            ref={(element) => {
              media.current = element;
            }}
            className="recording-video"
            aria-label="Запись встречи"
            controls
            preload="metadata"
            src={source}
            onLoadedMetadata={applySeek}
            onError={() => setPlaybackError(true)}
          />
        )
      ) : (
        <div className="recording-placeholder" role="status">
          <Clapperboard size={40} aria-hidden="true" />
          <p>Файл записи пока недоступен.</p>
        </div>
      )}
      {playbackError && (
        <div role="alert">
          <p>Не удалось воспроизвести запись. Возможно, ссылка устарела.</p>
          <Button
            variant="outline"
            busy={record.isFetching}
            onClick={async () => {
              pendingSeek.current =
                Math.max(0, media.current?.currentTime || 0) * 1000;
              const result = await record.refetch();
              if (result.data && !result.error) {
                setSource(
                  result.data.item.files.find(
                    (file) =>
                      file.fileType === "final_mp4" ||
                      file.fileType === "final_audio",
                  )?.url,
                );
                setPlaybackError(false);
              }
            }}
          >
            Обновить ссылку
          </Button>
        </div>
      )}
      <p className="field-hint">
        Приватная запись. Доступ к файлу проверяется сервером.
      </p>
    </div>
  );
}
