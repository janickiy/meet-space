// Содержимое пакета включает расшифровки, подтверждённые результаты AI и поиск по конференции.
// Оно не зависит от провайдера SDKs и не участвует в передаче медиаданных.
package content

import (
	"context"
	"encoding/json"
	"io"
	"time"
)

const (
	Queued     = "queued"
	Processing = "processing"
	Ready      = "ready"
	Failed     = "failed"
)

// Transcript — расшифровка содержит одно поколение транскрипции для завершённой записи.  В общедоступных
// ответах не используются ключи MinIO, данные об аренде рабочих мест и секреты провайдера.
type Transcript struct {
	ID           string     `json:"id"`
	ConferenceID string     `json:"conferenceId"`
	RecordingID  string     `json:"recordingId"`
	Status       string     `json:"status"`
	Language     string     `json:"language"`
	Provider     string     `json:"provider"`
	Generation   int64      `json:"generation"`
	ErrorCode    *string    `json:"errorCode,omitempty"`
	ErrorMessage *string    `json:"errorMessage,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	ProcessedAt  *time.Time `json:"processedAt,omitempty"`
}

// Segment — сегмент привязывает текст к временному интервалу завершённой записи.  speakerLabel — это
// метка для диаризации, а не подтверждённая личность участника конференции.
type Segment struct {
	ID           string   `json:"id"`
	TranscriptID string   `json:"transcriptId"`
	Ordinal      int      `json:"-"`
	StartMS      int64    `json:"startMs" gorm:"column:start_ms"`
	EndMS        int64    `json:"endMs" gorm:"column:end_ms"`
	SpeakerID    *string  `json:"speakerId"`
	SpeakerLabel *string  `json:"speakerLabel"`
	Text         string   `json:"text"`
	Confidence   *float64 `json:"confidence"`
}

// ActionItem содержит только действия, поддерживаемые исходным текстом. Целеуказатель и срок
// выполнения остаются пустыми, если исходные сегменты не поддерживают их явно.
type ActionItem struct {
	Text             string   `json:"text"`
	Assignee         *string  `json:"assignee"`
	DueDate          *string  `json:"dueDate"`
	SourceSegmentIDs []string `json:"sourceSegmentIds"`
}

// SummaryOutput определяет единственную допустимую схему JSON для сводного ответа AI.
type SummaryOutput struct {
	Summary     string       `json:"summary"`
	KeyPoints   []string     `json:"keyPoints"`
	ActionItems []ActionItem `json:"actionItems"`
	Topics      []string     `json:"topics"`
}

// Summary — в сводном ответе хранится валидированный результат с исходным стенограммой и версиями
// инструкций.
type Summary struct {
	ID                   string `json:"id"`
	ConferenceID         string `json:"conferenceId"`
	TranscriptID         string `json:"transcriptId"`
	TranscriptGeneration int64  `json:"transcriptGeneration"`
	Status               string `json:"status"`
	SummaryOutput        `gorm:"-"`
	StructuredOutput     json.RawMessage `json:"-" gorm:"column:structured_output"`
	Provider             string          `json:"provider"`
	Model                string          `json:"model"`
	PromptVersion        string          `json:"promptVersion"`
	SchemaVersion        string          `json:"schemaVersion"`
	Generation           int64           `json:"generation"`
	ErrorCode            *string         `json:"errorCode,omitempty"`
	ErrorMessage         *string         `json:"errorMessage,omitempty"`
	CreatedAt            time.Time       `json:"createdAt"`
	UpdatedAt            time.Time       `json:"updatedAt"`
	ProcessedAt          *time.Time      `json:"processedAt,omitempty"`
}

// TranscriptState позволяет отличить отсутствующую стенограмму от отключенной функции и
// предоставляет подтвержденное сервером разрешение на повторную попытку.
type TranscriptState struct {
	Item         *Transcript `json:"item"`
	Enabled      bool        `json:"enabled"`
	CanRetry     bool        `json:"canRetry"`
	ProviderMode string      `json:"providerMode"`
}

// SummaryState возвращает необязательный сохраненный результат и разрешения без создания
// промежуточных состояний обработки.
type SummaryState struct {
	Item          *Summary `json:"item"`
	Enabled       bool     `json:"enabled"`
	CanRegenerate bool     `json:"canRegenerate"`
	ProviderMode  string   `json:"providerMode"`
}

// SegmentPage содержит ограниченную страницу с хронологически упорядоченным текстом
// стенограммы.
type SegmentPage struct {
	Items  []Segment `json:"items"`
	Total  int64     `json:"total"`
	Limit  int       `json:"limit"`
	Offset int       `json:"offset"`
}

// SearchQuery определяет фильтры на стороне сервера; время однозначно определяется временными
// метками UTC.
type SearchQuery struct {
	Mode, ParticipantID, Membership string
	Query, Source, ConferenceID     string
	From, To                        *time.Time
	Limit, Offset                   int
}

// SearchResult содержит фрагменты в виде обычного текста и позиции навигации по записи, но не
// объектное хранилище URLs.
type SearchResult struct {
	SpeakerID       *string `json:"speakerId,omitempty"`
	Speaker         string  `json:"speaker,omitempty"`
	Type            string  `json:"type"`
	ConferenceID    string  `json:"conferenceId"`
	ConferenceTitle string  `json:"conferenceTitle"`
	RecordingID     *string `json:"recordingId"`
	TranscriptID    *string `json:"transcriptId"`
	SegmentID       *string `json:"segmentId"`
	StartMS         *int64  `json:"startMs" gorm:"column:start_ms"`
	Snippet         string  `json:"snippet"`
	Rank            float64 `json:"rank"`
}

// SearchPage содержит только ограниченную страницу с авторизованными результатами.
type SearchPage struct {
	EffectiveMode  string         `json:"effectiveMode"`
	FallbackReason string         `json:"fallbackReason,omitempty"`
	Items          []SearchResult `json:"items"`
	Total          int64          `json:"total"`
	Limit          int            `json:"limit"`
	Offset         int            `json:"offset"`
}

// RecordingSource идентифицирует приватный объект, загруженный из доверенного состояния базы
// данных, а не предоставленный пользователем URL.
type RecordingSource struct {
	RecordingID, ConferenceID, ObjectKey string
	DurationSec                          int
	SizeBytes                            int64
}

// Audio содержит ограниченный моно 16-битный источник kHz WAV и обязательную операцию очистки.
type Audio struct {
	Reader      io.ReadCloser
	Size        int64
	ContentType string
	Cleanup     func()
}

// TranscriptionRequest предоставляет аудио, автоматический выбор языка и стабильный ключ
// идемпотентности для адаптера провайдера.
type TranscriptionRequest struct {
	Audio                                 io.Reader
	Size                                  int64
	ContentType, Language, IdempotencyKey string
	DurationSec                           int
}

// TranscriptionResult содержит обнаруженный язык и сегменты, которые провайдер ещё не проверил.
type TranscriptionResult struct {
	Language string    `json:"language"`
	Segments []Segment `json:"segments"`
}

// TranscriptionProvider изолирует внешнее распознавание речи от кода домена и приложения.
type TranscriptionProvider interface {
	// Name возвращает настроенный идентификатор провайдера, используемый для маркировки и
	// проверки сгенерированного контента.
	//
	// @returns:
	//   - result1: Настроенный идентификатор провайдера.
	Name() string
	// Transcribe отправляет ограниченный объём аудиозаписи настроенный провайдер транскрипции.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Валидированный запрос приложения или входящие параметры операции.
	//
	// @returns:
	//   - result1: Результат транскрипции.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Transcribe(context.Context, TranscriptionRequest) (TranscriptionResult, error)
}

// AIRequest отделяет неизменяемые инструкции от недоверенного контента конференции JSON.
// Использование инструментов и сетевые действия не входят в рамки данного контракта.
type AIRequest struct {
	PromptVersion, SchemaVersion, Instructions, InputJSON, IdempotencyKey string
	Merge                                                                 bool
}

// AIProvider возвращает JSON, который приложение должно проверить перед сохранением.
type AIProvider interface {
	// Name возвращает настроенный идентификатор провайдера, используемый для маркировки и
	// проверки сгенерированного контента.
	//
	// @returns:
	//   - result1: Настроенный идентификатор провайдера.
	Name() string
	// Model возвращает настроенный идентификатор модели, используемый для маркировки и проверки
	// сгенерированного контента.
	//
	// @returns:
	//   - result1: Настроенный идентификатор модели провайдера.
	Model() string
	// Summarize создает сводную информацию с ограничениями по схеме на основе переданного
	// транскрипта.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Валидированный запрос приложения или входящие параметры операции.
	//
	// @returns:
	//   - result1: Результирующее необработанное сообщение.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Summarize(context.Context, AIRequest) (json.RawMessage, error)
}
