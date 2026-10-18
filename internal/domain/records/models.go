package records

import (
	"time"

	"gorm.io/datatypes"
)

const (
	ModeLegacy           = "legacy"
	ModeComposite        = "composite"
	ModeAudioOnly        = "audio_only"
	ModeIndividualTracks = "individual_tracks"
	ModeScreenFocus      = "screen_focus"
	StatusStarting       = "starting"
	StatusRecording      = "recording"
	StatusStopping       = "stopping"
	StatusFinalizing     = "finalizing"
	StatusUploading      = "uploading"
	StatusReady          = "ready"
	StatusPartialReady   = "partial_ready"
	StatusFailed         = "failed"
	StatusCancelled      = "cancelled"
	StatusDegraded       = "degraded"

	FileTypeFinalMP4      = "final_mp4"
	FileTypeFinalAudio    = "final_audio"
	FileTypeTracksArchive = "tracks_archive"
	FileTypePreviewJPG    = "preview_jpg"
	FileTypeDebugLog      = "debug_log"
)

// IsTerminalStatus сообщает, достигла ли запись финального этапа жизненного цикла.
//
// @Parameters:
//   - status: Состояние жизненного цикла или статус ответа на запись.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func IsTerminalStatus(status string) bool {
	switch status {
	case StatusReady, StatusPartialReady, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}

// Record сохраняет настройки записи, состояние жизненного цикла, данные о правах владения и
// метаданные о возможных сбоях.
type Record struct {
	ID                   int64          `gorm:"primaryKey" json:"id"`
	UUID                 string         `gorm:"column:uuid;type:uuid;default:gen_random_uuid()" json:"uuid"`
	Mode                 string         `gorm:"column:mode;default:legacy" json:"mode"`
	PlatformConferenceID *string        `gorm:"column:platform_conference_id;type:uuid" json:"-"`
	RecorderToken        *string        `gorm:"column:recorder_token;type:uuid" json:"-"`
	RecorderLeaseUntil   *time.Time     `gorm:"column:recorder_lease_until" json:"-"`
	ConferenceID         string         `gorm:"column:conference_id;type:uuid" json:"conferenceId"`
	RequestedBy          *string        `gorm:"column:requested_by;type:uuid" json:"requestedBy,omitempty"`
	SourceType           string         `gorm:"column:source_type" json:"sourceType"`
	TransportType        string         `gorm:"column:transport_type" json:"transportType"`
	Status               string         `gorm:"column:status" json:"status"`
	QualityMode          string         `gorm:"column:quality_mode" json:"qualityMode"`
	SegmentDurationSec   int            `gorm:"column:segment_duration_sec" json:"segmentDurationSec"`
	NeedPreview          bool           `gorm:"column:need_preview" json:"needPreview"`
	StorageBucket        *string        `gorm:"column:storage_bucket" json:"storageBucket,omitempty"`
	StorageObjectKey     *string        `gorm:"column:storage_object_key" json:"storageObjectKey,omitempty"`
	PreviewObjectKey     *string        `gorm:"column:preview_object_key" json:"previewObjectKey,omitempty"`
	DurationSec          *int           `gorm:"column:duration_sec" json:"durationSec,omitempty"`
	SizeBytes            *int64         `gorm:"column:size_bytes" json:"sizeBytes,omitempty"`
	WorkerID             *string        `gorm:"column:worker_id" json:"workerId,omitempty"`
	StartedAt            *time.Time     `gorm:"column:started_at" json:"startedAt,omitempty"`
	StoppedAt            *time.Time     `gorm:"column:stopped_at" json:"stoppedAt,omitempty"`
	EndedAt              *time.Time     `gorm:"column:ended_at" json:"endedAt,omitempty"`
	EndedReason          *string        `gorm:"column:ended_reason" json:"endedReason,omitempty"`
	ErrorMessage         *string        `gorm:"column:error_message" json:"errorMessage,omitempty"`
	MetadataJSON         datatypes.JSON `gorm:"column:metadata_json" json:"metadataJson,omitempty"`
	CreatedAt            time.Time      `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt            time.Time      `gorm:"column:updated_at" json:"updatedAt"`
}

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (Record) TableName() string { return "record" }

// RecordSegment сохраняет метаданные одного сегмента записи.
type RecordSegment struct {
	ID             int64      `gorm:"primaryKey" json:"id"`
	RecordID       int64      `gorm:"column:record_id" json:"recordId"`
	SeqNo          int        `gorm:"column:seq_no" json:"seqNo"`
	Status         string     `gorm:"column:status" json:"status"`
	LocalPath      *string    `gorm:"column:local_path" json:"localPath,omitempty"`
	ObjectKey      *string    `gorm:"column:object_key" json:"objectKey,omitempty"`
	FileName       *string    `gorm:"column:file_name" json:"fileName,omitempty"`
	MimeType       *string    `gorm:"column:mime_type" json:"mimeType,omitempty"`
	StartedAt      *time.Time `gorm:"column:started_at" json:"startedAt,omitempty"`
	EndedAt        *time.Time `gorm:"column:ended_at" json:"endedAt,omitempty"`
	DurationMS     *int       `gorm:"column:duration_ms" json:"durationMs,omitempty"`
	SizeBytes      *int64     `gorm:"column:size_bytes" json:"sizeBytes,omitempty"`
	ChecksumSHA256 *string    `gorm:"column:checksum_sha256" json:"checksumSha256,omitempty"`
	CreatedAt      time.Time  `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt      time.Time  `gorm:"column:updated_at" json:"updatedAt"`
}

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (RecordSegment) TableName() string { return "record_segment" }

// RecordFile сохраняет метаданные финального артефакта в приватном объектном хранилище.
type RecordFile struct {
	// Related содержит дополнительные приватные артефакты, атомарно зафиксированные вместе с
	// подтверждением готовности записи.
	Related        []RecordFile   `gorm:"-" json:"-"`
	ID             int64          `gorm:"primaryKey" json:"id"`
	UUID           string         `gorm:"column:uuid;type:uuid;default:gen_random_uuid()" json:"uuid"`
	RecordID       int64          `gorm:"column:record_id" json:"recordId"`
	FileType       string         `gorm:"column:file_type" json:"fileType"`
	Bucket         string         `gorm:"column:bucket" json:"bucket"`
	ObjectKey      string         `gorm:"column:object_key" json:"objectKey"`
	FileName       string         `gorm:"column:file_name" json:"fileName"`
	MimeType       string         `gorm:"column:mime_type" json:"mimeType"`
	SizeBytes      *int64         `gorm:"column:size_bytes" json:"sizeBytes,omitempty"`
	DurationSec    *int           `gorm:"column:duration_sec" json:"durationSec,omitempty"`
	ChecksumSHA256 *string        `gorm:"column:checksum_sha256" json:"checksumSha256,omitempty"`
	IsPrimary      bool           `gorm:"column:is_primary" json:"isPrimary"`
	IsPublic       bool           `gorm:"column:is_public" json:"isPublic"`
	MetadataJSON   datatypes.JSON `gorm:"column:metadata_json" json:"metadataJson,omitempty"`
	CreatedAt      time.Time      `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt      time.Time      `gorm:"column:updated_at" json:"updatedAt"`
}

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (RecordFile) TableName() string { return "record_file" }

// RecordEvent сохраняет ограниченную диагностику жизненного цикла записи.
type RecordEvent struct {
	ID          int64          `gorm:"primaryKey" json:"id"`
	RecordID    int64          `gorm:"column:record_id" json:"recordId"`
	EventType   string         `gorm:"column:event_type" json:"eventType"`
	EventSource string         `gorm:"column:event_source" json:"eventSource"`
	Severity    string         `gorm:"column:severity" json:"severity"`
	Message     string         `gorm:"column:message" json:"message"`
	PayloadJSON datatypes.JSON `gorm:"column:payload_json" json:"payloadJson,omitempty"`
	WorkerID    string         `gorm:"column:worker_id" json:"workerId"`
	CreatedAt   time.Time      `gorm:"column:created_at" json:"createdAt"`
}

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (RecordEvent) TableName() string { return "record_event" }
