# Управление медиа и запись конференций

Актуализировано 2026-10-21 по коду `6e577b6cced4e34d1c031353fbb9055909055185`.
Текущий UI использует conference recording API и SFU egress. Go record-worker
(`cmd/record-worker`, Compose service `record-worker`) захватывает и обрабатывает запись отдельно
от API и SFU.

## Возможности

Участник может независимо включать микрофон и камеру, менять устройства и показывать
экран вместе с камерой. Повторная публикация сохраняет PeerConnection. Владелец
управляет правами, назначает соорганизаторов и исключает участников. Запись может
запустить любой допущенный участник с аккаунтом. В интерфейсе доступны управление
медиа, модерация, индикатор и история записей с подписанными ссылками.
Можно подключиться без камеры и микрофона: слушать встречу и отдельно
включить только нужное устройство или экран, не запрашивая оба устройства сразу.

## Архитектурные решения

```text
Browser ⇄ WebRTC ⇄ media-worker SFU ⇄ другие Browser
                         │ HTTP NDJSON: metadata + base64 RTP
                         ▼
                 защищённый HTTP egress
                         ▼
                 record-worker spool
                         ▼
           FFmpeg segment composition и audio mix
                         ▼
           concat MP4 → optional preview → ffprobe → S3
```

SFU не запускает FFmpeg и не декодирует медиа. API не проксирует RTP. Передача
в recorder выделена в независимую ограниченную очередь: медленный или сломанный
recorder не тормозит участников.

## Демонстрация экрана

`media.offer.publications` связывает SDP MID с источником `microphone`, `camera`,
`video/screen` или `audio/screen`. Дополнительный `trackId` обозначает поколение
browser capture, но не является серверным ID публикации. Offer без `publications`
поддерживается для совместимости. Камера и экран имеют отдельные streamId:
`mediaPeerId` и `mediaPeerId-screen`.

Браузер вызывает `getDisplayMedia()` по нажатию кнопки. Аудио экрана необязательно;
его завершение не останавливает видео. Завершение video/screen, отключение вкладки
или модерация освобождают владение. Лимит `MEDIA_MAX_SCREEN_SHARERS=1` задаётся
конфигурацией; второй владелец получает `screen_sharing_conflict`. Клиент откатывает
отклонённый offer, останавливает захват экрана и сохраняет звонок.

## Матрица прав

| Действие | Owner | Co host | Participant |
| --- | --- | --- | --- |
| Собственные устройства и экран | Да, если разрешены | Да, если разрешены | Да, если разрешены |
| Завершение конференции | Да | Нет | Нет |
| Запуск записи с аккаунтом | Да | Да | Да |
| Остановка записи | Любая запись встречи | Своя запись | Своя запись |
| Удаление завершённой записи с аккаунтом | Любая запись встречи | Своя запись | Своя запись |
| Mute и остановка экрана другого | Любой не owner | Только participant | Нет |
| Запрет видео другого | Любой не owner | Нет | Нет |
| Исключение другого | Любой не owner | Только participant | Нет |
| Назначение и снятие co_host | Да | Нет | Нет |
| Просмотр записей своей конференции | Да | Да | Да |

Для запуска нужны постоянный аккаунт, допуск `admitted`, членство `joined` и
активная обычная конференция. Stop/delete требуют прав инициатора либо owner и
доступа к истории (`joined` или `left`); UI может скрывать stop после выхода.
Удаление разрешено только для терминального состояния без живой аренды recorder.
Роль co_host сама по себе не даёт прав остановить/удалить чужую запись. Owner неизменяем;
исключённый участник не получает историю записей. Вышедший, но не исключённый
участник сохраняет доступ к истории. Гости с `guest_conference_id` не управляют записью, даже имея JWT и joined membership.

## Семантика модерации

Принудительный mute блокирует всё аудио участника, включая audio/screen.
Модераторский запрет видео блокирует камеру и экран; в UI он называется
«Отключить видео». Это предотвращает обход переименованием источника. Собственный
переключатель микрофона или камеры влияет только на соответствующее устройство.

Ограничения записываются в PostgreSQL и действуют на все вкладки. Разрешение
`blocked:false` снимает запрет, но никогда не включает оборудование автоматически.
Браузер освобождает захват; сервер независимо прекращает forwarding и запрещает
повторную публикацию. Физическое происхождение картинки сервер подтвердить не может:
typed sources описывают заявленное назначение, а не аппаратную аттестацию.

Каждое изменение имеет монотонную policy version и audit row. Свежая policy
проверяется на join и offer, возвращается в media.joined и доставляется как
media.policy. Reconciler повторяет неуспешную доставку каждые 2 секунды. При kick
membership/history сохраняются, WS и медиа закрываются; повторный вход запрещён
без отдельной будущей процедуры восстановления. После последней вкладки control
flags очищаются; наличие control flag не доказывает наличие RTP.
Флаги хранятся для каждой WebSocket-сессии и объединяются через OR: открытие пустой
вкладки не выключает индикатор работающей камеры в другой. Обновления содержат
connectionId и возрастающий sequence; закрытая/чужая сессия отвергается, запоздавшее
обновление не перезаписывает новое. Изменение статуса сессии и агрегация используют
тот же порядок блокировок, что и модерация. Снятие запрета не восстанавливает старые
флаги устройств.

## Архитектура общей записи

Recorder получает VP8 и Opus без транскодирования на SFU, депакетизирует их в
короткие IVF/Ogg-фрагменты и сохраняет атомарные manifests. Отдельный layout module
выбирает full frame для одного видео, две плитки для двух, 2×2 для трёх или четырёх
и сетку для большего количества. При screen share экран занимает основную область,
а камеры — дополнительную. Порядок определяется стабильными participant/track IDs.

FFmpeg строит одинаковые H.264/AAC сегменты, затем соединяет их без повторного
перекодирования. Audio mix использует нормализацию, limiter и единые 48 kHz.
Общий clock egress и offsets выравнивают потоки; пустые области и тишина заполняют
пропуски. Смена layout происходит на границах сегментов. Последний неполный сегмент
сохраняется. Это фиксированная композиция по источникам, без active-speaker layout или рендера DOM браузера.
Для нерегулярных кадров браузера последнее видеоизображение сохраняется до нового
кадра либо явного track.end; тихие сегменты используют ограниченный GOP pre-roll.
Завершённый источник не продолжается бесконечно. Сбой FFmpeg немедленно прерывает
capture записи, не ожидая ручного stop; число удерживаемых буферов ограничено также
при частой смене track ID.

### Режимы и опубликованные файлы

`mode` задаётся при запуске; по умолчанию `composite`. Все четыре режима используют
один conference recording lifecycle и единственную активную запись на конференцию.

| Режим | Результат | Файлы в `files[]` |
| --- | --- | --- |
| `composite` | Камеры в сетке; при экране основная область 3/4, камеры в оставшейся части | `final_mp4` (`final.mp4`, `video/mp4`), `preview_jpg` |
| `screen_focus` | Экран на весь кадр; без экрана — сетка камер | `final_mp4`, `preview_jpg` |
| `audio_only` | Смешанное AAC аудио; video RTP не записывается | `final_audio` (`final.mp4`, `audio/mp4`), без preview |
| `individual_tracks` | Смешанное AAC аудио и архив исходных фрагментов дорожек | `final_audio` плюс `tracks_archive` (`tracks.zip`), без preview |

`individual_tracks` не создаёт готовые MP4 по одному на участника. ZIP содержит
IVF/Ogg-фрагменты и manifest с participant/source/track instance и временными
границами. Видео общей композиции кодируется `libx264`, CRF 23, `veryfast`, `yuv420p`;
аудио — AAC 128 kbit/s, 48 kHz stereo. Итоговые сегменты объединяются concat/copy
с `faststart`; ffprobe проверяет кодеки, ненулевую длительность и A/V-разницу ≤350 ms.
Для аудиорежимов видео и preview не требуются.

### Spool и объектное хранилище

`STORAGE_PATH=/storage`; в production монтируется
`${RECORDER_STORAGE_DIR}:/storage`, record-worker работает UID/GID 1000.
Каталог `/storage/records/{recordId}` содержит:

- `sources/{chunk}_{trackId}.ivf` / `.ogg` — VP8 / Opus;
- `chunk_000000.json` — закрытые manifests;
- `segment_000000.mp4` — композиции; `.partial` до атомарного переименования;
- `concat.txt`, `final.mp4`, при необходимости `preview.jpg` и `tracks.zip`.

Локальные сегменты не публикуются в S3 по отдельности. В bucket `recordings`
по умолчанию публикуется префикс
`recordings/{conferenceId}/{recordId}/artifacts/{recorderToken}/`.
Token изолирует поколения: незавершённый upload прежнего владельца не перезаписывает
результат нового. Подтверждённое поколение хранится в БД; API выдаёт ссылки только
на него, bucket приватный. URL нельзя строить самостоятельно по UUID.
Compose service `minio` — классический MinIO либо SeaweedFS из hardened-установщика;
приватный S3 API и имена `MINIO_*` общие, локальный spool — отдельное хранилище.

## Контракт между SFU и recorder

Внутренний `POST /internal/media/egress` защищён отдельным Bearer secret и Redis
ownership lease. Запрос содержит UUID `requestId`, `conferenceId`, `recordingId`, `route`,
`segmentDurationSec` (1–30 на внутренней границе).
`X-Request-ID` должен совпадать с телом. Ответ `application/x-ndjson` содержит
`hello`, `track`, `rtp`, `track.end`, `ping` либо `error`.
Метаданные содержат codec и typed source; RTP передаётся как base64, capturedAt —
Unix timestamp в наносекундах на SFU. Sequence позволяет обнаружить пропуски контракта.

На SFU очередь ограничена `MEDIA_EGRESS_QUEUE_SIZE`; overflow завершает только
recording output. Пустая комната сохраняется, пока существует recording subscriber.
Внутренние адреса и secrets не доступны браузеру. HTTP write deadline обновляется
на каждой отправке; shutdown закрывает egress до ожидания завершения HTTP.

`hello` вызывает `OnStarted`: именно после него record-worker устанавливает `recording`
и `started_at`, затем публикует `recording.started`. Это ещё не доказательство
получения первого RTP. Media timeline хранится отдельно (`media_started_at`,
`timelineOriginNs`). Без владельца SFU открытие egress завершается ошибкой; при
живом владельце и отсутствии медиапакетов возможна композиция фона и тишины.

`MEDIA_EGRESS_QUEUE_SIZE=2048` по умолчанию, ping — раз в секунду, PLI — по
длительности сегмента. Capture прерывается при 15 секундах отсутствия входных
кадров контракта. RabbitMQ не переносит RTP.

## Жизненный цикл и восстановление

Публичные состояния: `starting → recording → stopping → processing → ready/failed`.
`finalizing` и `uploading` отображаются как `processing`. `ready` выставляется
после проверки итогового медиа, загрузки всех необходимых выбранному режиму объектов
и транзакции метаданных. Ошибка записи не завершает конференцию.

Start создаёт DB row и outbox в одной транзакции под блокировкой конференции.
Повторный start любого режима возвращает `409 recording is already active`, пока
предыдущая запись находится в starting/recording/degraded/stopping/finalizing/uploading.
Защита распространяется на параллельные запросы; уникальный индекс дополнительно
запрещает вторую незавершённую запись.
Dispatcher получает Redis lock и подтверждение RabbitMQ; stop
не ждёт FFmpeg в HTTP. Finish конференции атомарно добавляет stopping и stop command.
Worker наблюдает DB state, поэтому stop, доставленный другой реплике, не теряется.

События API и recorder содержат `recordingId`, `conferenceId`, публичный `status`,
`mode` и `requestedBy` из сохранённой записи. `recording.starting` означает подготовку;
по `recording.started` все допущенные участники, включая инициатора, слышат
«Recording has started». Текстовая плашка показывается остальным участникам.
Плашка использует существующий WebSocket и дедупликацию по UUID записи. После
входа или переподключения список записей сверяется с API; активная запись также
показывает предупреждение, если событие было пропущено. Уведомления о начале
не добавляются в личный каталог уведомлений и не отправляются по email/push.

Статусы хранения и их отображение:

| Состояние БД | Публичный статус | Значение в текущем conference pipeline |
| --- | --- | --- |
| `starting` | `starting` | Заявка/outbox создана, идёт захват аренды и открытие egress |
| `recording` | `recording` | Получен `hello`, идёт capture и композиция сегментов |
| `stopping` | `stopping` | Принята остановка; record-worker завершает захват и последний сегмент |
| `finalizing` | `processing` | Concat, ffprobe и подготовка артефактов |
| `uploading` | `processing` | Публикация файлов и fenced commit |
| `ready` | `ready` | Подтверждён полный набор файлов выбранного режима |
| `failed` | `failed` | Неуспех; доступный spool сохраняется |
| `degraded` | `degraded` | Зарезервировано моделью; блокирует новый start, текущий composite record-worker не выставляет |
| `partial_ready` | `partial_ready` | Терминальное состояние модели; текущий composite record-worker не выставляет |
| `cancelled` | `cancelled` | Терминальное состояние модели; текущий composite record-worker не выставляет |

Outbox dispatcher опрашивает БД каждые 250 ms, захватывает до 16 команд на 15 s,
сохраняет порядок одной записи и повторяет неудачную доставку. Redis lock
`record:conference:{conferenceId}:lock` по умолчанию живёт 6 h. RabbitMQ использует
persistent `record.start`/`record.stop`, publisher confirms и QoS 1. Первая ошибка
обработчика requeue, повторная ошибка доставки отправляет команду в `.failed`.
ACK после запуска фоновой capture-задачи не означает успешную запись: последующий
сбой фиксируется как `failed`, а не повторяется бесконечно через RabbitMQ.

Recorder lease по умолчанию 20 s, продление каждые TTL/4, reconciliation раз в
секунду. SQL-переходы и публикация сверяют живую аренду и `recorder_token`.
Рестарт `starting`/`recording` с прежним token означает оборванный capture и `failed`.
`stopping`/`finalizing`/`uploading` можно восстановить по локальным закрытым manifests
и сегментам, если они доступны новому владельцу. Это не перенос активной записи
между произвольными хостами. При предельной длительности record-worker запрашивает stop
с `duration_limit`; finish конференции — с `conference_finished`.

Загрузка имеет timeout 10 min, S3 SDK — до 3 попыток; после upload `StatObject`
проверяет размер. SHA-256 сохраняется в метаданных, но это не удалённое сравнение
checksum. Ошибка публикации запускает удаление объектов текущего token; `ready`
ставится транзакционно вместе с file/segment rows. Ошибка очистки локального spool
после успеха не отменяет `ready`.

## Схема базы данных

`record` хранит mode, platform_conference_id, состояние, recorder token/lease
и временные отметки. `recording_outbox` обеспечивает транзакционную постановку
команд; `record_file` и segment rows описывают подтверждённые артефакты.
Частичный unique index запрещает вторую активную запись любого conference mode.

`conference_participants` содержит политики медиа и их версию,
`conference_moderation_audit` — действия модерации, `participant_sessions` —
флаги устройств и sequence отдельных вкладок. Определения находятся в
[`database/migrations`](../database/migrations), в частности миграциях 10, 11 и 18.

## API и события WebSocket

HTTP endpoints требуют JWT:

| Метод и путь относительно `/api/v1/conferences/{id}` | Назначение |
| --- | --- |
| `PUT /participants/me/media` | connectionId, sequence, microphoneEnabled, cameraEnabled, screenSharing |
| `POST /participants/{participantId}/moderation` | action mute/camera/screen/kick/role; blocked либо role |
| `POST /recordings` | Запуск; `mode`, optional `segmentDurationSec` 2–30 (по умолчанию 5); ответ 202 item |
| `POST /recordings/{recordingId}/stop` | Асинхронная остановка, ответ 202 item |
| `GET /recordings` | История с limit/offset |
| `GET /recordings/{recordingId}` | Метаданные и подписанные URLs |
| `DELETE /recordings/{recordingId}` | Удаление терминальной записи владельцем/инициатором; 200 |

Префикс всех маршрутов — `/api/v1/conferences/{id}`. Отдельных `/status` и
`/download` нет: статус и `files[].url` возвращаются в карточке. Ссылки действуют
до 24 h. Стандартный ответ — `{ "status":"success", "item":{...} }`, список —
`items`; `limit` по умолчанию 20, допустимо 1–100, `offset >= 0`.
JSON запуска/остановки ограничен 32 KiB; неизвестные поля и лишний JSON отклоняются.

События: participant.media.updated, participant.role.updated, participant.kicked,
media.policy, recording.starting, recording.started, recording.stopping,
recording.processing, recording.ready, recording.failed, recording.deleted.
Legacy record-worker endpoints не читают и не изменяют conference recording rows.
Публичного legacy create endpoint в текущем router нет.

## Основные файлы

- [`internal/usecase/recordings`](../internal/usecase/recordings) — API lifecycle и outbox dispatcher;
- [`internal/usecase/recorder/composite.go`](../internal/usecase/recorder/composite.go) — capture, lease и восстановление;
- [`internal/infrastructure/composite`](../internal/infrastructure/composite) — депакетизация, manifests, layout и FFmpeg composition;
- [`internal/transport/mediaworker/control_egress.go`](../internal/transport/mediaworker/control_egress.go) — внутренний egress;
- [`internal/app/recordings`](../internal/app/recordings), [`frontend/src/components/RecordingPanel.tsx`](../frontend/src/components/RecordingPanel.tsx) — HTTP и UI;
- [`internal/config/composite.go`](../internal/config/composite.go) — лимиты записи.

## Известные ограничения

На конференцию разрешена одна активная запись любого из четырёх режимов.
Видеокомпозиция имеет фиксированное разрешение/FPS; layout меняется
на границах сегментов. Source attribution заявляется клиентом.
После остановки экрана его основная область может оставаться пустой до следующей
границы сегмента (по умолчанию до 5 секунд); живые камеры продолжают отображаться.
Screen audio зависит от браузера и выбранного источника.
Синхронизация использует RTP timestamps и время получения; точного выравнивания
часов независимых отправителей по RTCP Sender Reports пока нет.

История в UI показывает первую страницу recording API; это не общая личная библиотека.
Live capture не восстанавливается бесшовно после смерти recorder. Для восстановления
обработки нужны сохранённые закрытые сегменты на доступном новой реплике диске.
Файлы S3 завершённых записей удаляются автоматически через 7 суток после
`ended_at` (для старых задач без этой отметки — `stopped_at`, затем `updated_at`).
API выполняет очистку при старте и раз в минуту; удаляются все поколения артефактов
в пространстве записи, включая неподтверждённые поколения. Ошибки повторяются,
активные задачи и записи с действующей арендой не удаляются. Доступ к истёкшей
записи закрывается при чтении до завершения физической очистки.
Retention для локальных failed spools пока не автоматизирован.
После успешной fenced-публикации локальные файлы удаляются, если
`RECORDING_KEEP_LOCAL=false` (по умолчанию). `RECORDING_MAX_MIB` ограничивает объём
входящего RTP, а не весь диск: pre-roll, промежуточные и итоговые файлы увеличивают
потребление. Диск требует отдельной квоты и мониторинга.

После kick API больше не выдаёт ссылки участнику, но уже выданный presigned URL
действует до истечения своего срока (до 24 часов). Немедленный отзыв таких ссылок
потребует отдельного авторизованного download proxy либо иной схемы доставки.

### Временные метки и частота входящего видео

SFU сохраняет исходный RTP timestamp и отдельно добавляет серверный `capturedAt`.
VP8 собирается по UUID дорожки с clock rate 90000; IVF хранит разность RTP-меток
от первого кадра. Заголовок IVF задаёт временную базу, поэтому `r_frame_rate=90000/1`
из ffprobe нельзя принимать за фактический FPS. Для диагностики частоты и
длительности источника используйте интервалы меток отдельных кадров.
Разные подключения одного аккаунта имеют независимые состояния дорожек.

Браузер запрашивает для экрана `frameRate: { ideal: 15, max: 30 }`, но это target
захвата, а не серверный ограничитель входящего RTP. Поток может превышать ожидаемую
частоту; частоту отправки нужно контролировать в клиенте. Выходной
`RECORDING_FPS` задаёт профиль композиции и не ограничивает входной pre-roll.

Видео pre-roll хранит кадры от последнего ключевого кадра и ограничен
**4096 кадрами и 4 MiB на источник**. Превышение любого предела завершает запись ошибкой
`recording keyframe pre-roll limit exceeded`. Новый ключевой кадр сбрасывает
этот буфер. Для Opus сохраняются последние 6 samples; окно переупорядочивания
RTP — 200 ms. Эти пределы защищают память и не заменяют контроль bitrate/FPS
отправителя. Исходники: [`capture.go`](../internal/infrastructure/composite/capture.go),
[`frontend/src/media.ts`](../frontend/src/media.ts).

## Эксплуатация

Контролируйте FFmpeg backlog, свободное место spool, записи в `failed`, record-worker
leases, очередь outbox и выполнение retention. Восстановление финализации требует
доступа к spool; при передаче egress за пределы доверенной сети защищайте транспорт
TLS/mTLS. Настройки по умолчанию:
до 2 active recordings на recorder, 1 composition FFmpeg, 1280×720/30 FPS,
лимит входных данных 10 GiB и длительность до 4 часов. Эти лимиты не заменяют sizing.

## Legacy recorder и текущая граница совместимости

Legacy — отдельный внутренний ingest в `internal/infrastructure/webrtc` и
`internal/usecase/recorder/service.go`. Защищённые `WORKER_INTERNAL_SECRET`
маршруты record-worker `/records/:id/start`, `/records/:id/stop`,
`/records/:id/webrtc/offer` принимают SDP для отдельной Pion-сессии и не заменяют
conference API. Guard запрещает использовать их для conference recording rows.

Legacy передаёт RTP через локальные UDP sockets в FFmpeg по `input.sdp`, пишет
MKV в `/storage/tmp/{id}` и затем перекодирует итоговый H.264/AAC MP4. Его объектный
префикс — `records/{id}/`, без conference token hierarchy. Поддерживаемые legacy
кодеки шире (VP8/VP9/H.264 и Opus/PCMU/PCMA), чем текущий SFU (VP8/Opus).
Основной UI не отправляет медиа непосредственно этому record-worker; production Compose
отключает его legacy ICE mux-порты. Ни публичного `/record-worker` proxy, ни
анонимного `/records` create текущий backend router не обещает.
