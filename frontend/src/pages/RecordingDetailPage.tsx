import { useEffect, useRef, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import {
  ArrowLeft,
  CalendarDays,
  Clock3,
  Download,
  FileAudio,
  FileVideo,
  RefreshCw,
} from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import { Button, ErrorNotice, Loading } from "../components/ui";
import {
  formatRecordingDuration,
  formatRecordingSize,
  recordingDate,
  recordingMediaFile,
  recordingPreviewFile,
  safeRecordingUrl,
} from "../recordingPresentation";
import "./recordings.css";

/** RecordingDetailPage открывает приватную запись только в контексте указанной встречи.
 * Идентификатор записи сам по себе не даёт права доступа: все материалы подтверждает API.
 * @return страница просмотра либо безопасная подсказка, если ссылка неполная или пользователь не вошёл.
 */
export function RecordingDetailPage() {
  const { id = "" } = useParams();
  const [params] = useSearchParams();
  const { user } = useAuth();
  const conferenceId = params.get("conference") || "";

  if (!id || !conferenceId)
    return (
      <section className="recordings-page recording-detail-page recordings-empty">
        <FileVideo size={36} aria-hidden="true" />
        <h1>Не удалось открыть запись</h1>
        <p>
          Откройте запись из списка записей встречи, чтобы перейти к её
          материалам.
        </p>
        <Link className="button button-primary" to="/recordings">
          К записям
        </Link>
      </section>
    );
  if (!user)
    return (
      <section className="recordings-page recording-detail-page recordings-empty">
        <h1>Войдите, чтобы открыть запись</h1>
        <p>Материалы встречи доступны только авторизованным участникам.</p>
        <Link className="button button-primary" to="/login">
          Войти
        </Link>
      </section>
    );
  return (
    <RecordingDetailContent
      key={`${user.id}:${conferenceId}:${id}`}
      userId={user.id}
      conferenceId={conferenceId}
      recordingId={id}
    />
  );
}

/** RecordingDetailContent загружает запись и показывает только реальные файлы, разрешённые сервером.
 * Кеш разделён по пользователю, встрече и записи; ссылки обновляются исключительно по команде пользователя.
 * @args userId — текущая учётная запись; conferenceId — контекст встречи; recordingId — UUID записи.
 * @return видеоплеер либо аудиоплеер, доступные материалы и необязательная сводка завершённой встречи.
 */
function RecordingDetailContent({
  userId,
  conferenceId,
  recordingId,
}: {
  userId: string;
  conferenceId: string;
  recordingId: string;
}) {
  const [mediaFailed, setMediaFailed] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [mediaRevision, setMediaRevision] = useState(0);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const conference = useQuery({
    queryKey: ["recording-detail-conference", userId, conferenceId],
    queryFn: ({ signal }) => api.conference(conferenceId, signal),
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const conferenceMatches = conference.data?.item.id === conferenceId;
  const recording = useQuery({
    queryKey: ["recording-detail", userId, conferenceId, recordingId],
    queryFn: ({ signal }) => api.recording(conferenceId, recordingId, signal),
    enabled: conference.isSuccess && conferenceMatches,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const recordingMatches =
    recording.data?.item.uuid === recordingId &&
    recording.data?.item.conferenceId === conferenceId;
  const history = useQuery({
    queryKey: ["recording-detail-history", userId, conferenceId],
    queryFn: ({ signal }) => api.history(conferenceId, signal),
    enabled:
      conference.isSuccess &&
      conferenceMatches &&
      ["finished", "cancelled"].includes(conference.data?.item.status ?? "") &&
      recording.isSuccess &&
      recordingMatches,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const backPath = `/recordings?${new URLSearchParams({ conference: conferenceId })}`;

  /** refresh проверяет доступ заново и получает свежие серверные ссылки после ошибки воспроизведения.
   * Автоматических повторов и циклического продления ссылок нет; повторное нажатие блокируется до ответа.
   */
  async function refresh() {
    if (refreshing) return;
    setRefreshing(true);
    try {
      const nextConference = await conference.refetch();
      if (
        !mounted.current ||
        !nextConference.isSuccess ||
        nextConference.data?.item.id !== conferenceId
      )
        return;
      const nextRecording = await recording.refetch();
      if (!mounted.current) return;
      if (
        nextRecording.isSuccess &&
        nextRecording.data?.item.uuid === recordingId &&
        nextRecording.data.item.conferenceId === conferenceId
      ) {
        setMediaFailed(false);
        setMediaRevision((value) => value + 1);
      }
    } finally {
      if (mounted.current) setRefreshing(false);
    }
  }

  const backLink = (
    <Link className="recordings-back" to={backPath}>
      <ArrowLeft size={18} aria-hidden="true" />
      Назад к записям
    </Link>
  );
  if (
    conference.isError ||
    (conference.isSuccess && !conferenceMatches) ||
    recording.isError ||
    (recording.isSuccess && !recordingMatches)
  )
    return (
      <div className="recordings-page recording-detail-page">
        {backLink}
        <section className="recordings-empty recording-detail-unavailable">
          <FileVideo size={36} aria-hidden="true" />
          <h1>Не удалось открыть запись</h1>
          <ErrorNotice>
            Запись недоступна. Проверьте доступ к встрече или попробуйте позже.
          </ErrorNotice>
          <Button
            variant="secondary"
            busy={refreshing}
            onClick={() => void refresh()}
          >
            <RefreshCw size={16} aria-hidden="true" />
            Попробовать снова
          </Button>
        </section>
      </div>
    );
  if (conference.isPending || recording.isPending)
    return (
      <div className="recordings-page recording-detail-page">
        {backLink}
        <Loading />
      </div>
    );

  const meeting = conference.data.item;
  const item = recording.data.item;
  const media =
    item.mode === "audio_only"
      ? item.files.find(
          (file) =>
            file.fileType === "final_audio" && safeRecordingUrl(file.url),
        )
      : recordingMediaFile(item);
  const mediaUrl = safeRecordingUrl(media?.url);
  const previewUrl = safeRecordingUrl(recordingPreviewFile(item)?.url);
  const playable = item.status === "ready" && !!mediaUrl;
  const audioOnly = media?.fileType === "final_audio";
  const meetingHistory =
    history.isSuccess && history.data.item.conference.id === conferenceId
      ? history.data.item
      : undefined;

  return (
    <div className="recordings-page recording-detail-page">
      {backLink}
      <header className="recordings-heading recording-detail-heading">
        <div>
          <h1>{meeting.title || "Запись встречи"}</h1>
          <p className="recordings-meta">
            <span>
              <CalendarDays size={15} aria-hidden="true" />
              {recordingDate(item)}
            </span>
            <span>
              <Clock3 size={15} aria-hidden="true" />
              {formatRecordingDuration(item.durationSec)}
            </span>
          </p>
        </div>
        {playable && (
          <a
            className="button button-primary"
            href={mediaUrl}
            download
            target="_blank"
            rel="noreferrer"
          >
            <Download size={17} aria-hidden="true" />
            Скачать запись
          </a>
        )}
      </header>
      <div className="recording-detail-layout">
        <div className="recording-detail-main">
          <div
            className={`recording-detail-player${audioOnly ? " recording-detail-player-audio" : ""}${!playable || mediaFailed ? " recording-detail-player-unavailable" : ""}`}
          >
            {playable && !mediaFailed ? (
              audioOnly ? (
                <div className="recording-detail-audio">
                  <FileAudio size={48} aria-hidden="true" />
                  <p>Аудиозапись встречи</p>
                  <audio
                    key={mediaRevision}
                    aria-label="Аудиозапись встречи"
                    controls
                    preload="metadata"
                    src={mediaUrl}
                    onError={() => setMediaFailed(true)}
                  />
                </div>
              ) : (
                <video
                  key={mediaRevision}
                  aria-label="Видеозапись встречи"
                  controls
                  playsInline
                  preload="metadata"
                  src={mediaUrl}
                  poster={previewUrl}
                  onError={() => setMediaFailed(true)}
                />
              )
            ) : (
              <div className="recordings-empty recording-detail-unavailable">
                <FileVideo size={40} aria-hidden="true" />
                <h2>Запись пока недоступна</h2>
                <p>
                  {mediaFailed
                    ? "Не удалось воспроизвести запись. Обновите ссылку и попробуйте снова."
                    : "Не удалось открыть материалы записи. Попробуйте обновить страницу позже."}
                </p>
                <Button
                  variant="secondary"
                  busy={refreshing}
                  onClick={() => void refresh()}
                >
                  <RefreshCw size={16} aria-hidden="true" />
                  {mediaFailed ? "Обновить ссылку" : "Обновить запись"}
                </Button>
              </div>
            )}
          </div>
        </div>
        <aside className="recording-detail-sidebar">
          <section
            className="recording-detail-section"
            aria-labelledby="recording-info-title"
          >
            <h2 id="recording-info-title">О записи</h2>
            <dl className="recording-detail-info">
              <div>
                <dt>Дата записи</dt>
                <dd>{recordingDate(item)}</dd>
              </div>
              <div>
                <dt>Длительность записи</dt>
                <dd>{formatRecordingDuration(item.durationSec)}</dd>
              </div>
              {media && (
                <div>
                  <dt>Размер файла</dt>
                  <dd>{formatRecordingSize(media.sizeBytes)}</dd>
                </div>
              )}
              {meetingHistory?.owner.displayName && (
                <div>
                  <dt>Организатор встречи</dt>
                  <dd>{meetingHistory.owner.displayName}</dd>
                </div>
              )}
              {meetingHistory && (
                <div>
                  <dt>Участники встречи</dt>
                  <dd>{meetingHistory.participantCount}</dd>
                </div>
              )}
            </dl>
          </section>
        </aside>
      </div>
    </div>
  );
}
