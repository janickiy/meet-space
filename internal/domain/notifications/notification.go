package notifications

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// Payload contains resource references and state only. A notification neither reveals chat or
// file content nor grants resource access.
type Payload struct {
	ConferenceID     string     `json:"conferenceId"`
	ConversationID   string     `json:"conversationId,omitempty"`
	ConversationType string     `json:"conversationType,omitempty"`
	IsReply          bool       `json:"isReply,omitempty"`
	InvitationID     string     `json:"invitationId,omitempty"`
	RecordingID      string     `json:"recordingId,omitempty"`
	MessageID        string     `json:"messageId,omitempty"`
	ScheduledAt      *time.Time `json:"scheduledAt,omitempty"`
	AdmissionState   string     `json:"admissionState,omitempty"`
	TranscriptID     string     `json:"transcriptId,omitempty"`
	SummaryID        string     `json:"summaryId,omitempty"`
	Generation       int64      `json:"generation,omitempty"`
}

// Notification persists a personal event, referenced resources, deduplication, read state and
// delivery state.
type Notification struct {
	ID          string     `json:"id" gorm:"type:uuid;primaryKey"`
	UserID      string     `json:"userId" gorm:"type:uuid"`
	Type        string     `json:"type"`
	Version     int        `json:"version"`
	Payload     Payload    `json:"payload" gorm:"serializer:json;type:jsonb"`
	DedupKey    string     `json:"-"`
	CreatedAt   time.Time  `json:"createdAt"`
	ReadAt      *time.Time `json:"readAt"`
	PublishedAt *time.Time `json:"-"`
}

// TableName returns the explicit database table name so GORM does not infer a different plural
// form.
//
// Returns:
//   - result 1: Exact database table name used by GORM.
func (Notification) TableName() string { return "notifications" }

// Page contains one bounded notification page and continuation metadata.
type Page struct {
	Items       []Notification `json:"items"`
	NextCursor  *string        `json:"nextCursor"`
	UnreadCount int64          `json:"unreadCount"`
}

// Cursor contains the CreatedAt and ID boundary of the next account notification page.
type Cursor struct {
	UserID string    `json:"userId"`
	At     time.Time `json:"at"`
	ID     string    `json:"id"`
}

// DecodeCursor decodes and validates an opaque pagination cursor.
//
// Parameters:
//   - value: Value to validate, normalize or convert.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: The constructed or loaded cursor.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func DecodeCursor(value, userID string) (*Cursor, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > 512 {
		return nil, apperrors.ErrInvalidInput
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(value)
	var c Cursor
	if err != nil || json.Unmarshal(b, &c) != nil || c.UserID != userID || c.At.IsZero() {
		return nil, apperrors.ErrInvalidInput
	}
	id, err := uuid.Parse(c.ID)
	if err != nil || id == uuid.Nil {
		return nil, apperrors.ErrInvalidInput
	}
	c.ID = id.String()
	return &c, nil
}

// EncodeCursor encodes a stable boundary for the next page of results.
//
// Parameters:
//   - n: Number of items or operations requested.
//
// Returns:
//   - result 1: Opaque notification continuation cursor.
func EncodeCursor(n Notification) string {
	b, _ := json.Marshal(Cursor{UserID: n.UserID, At: n.CreatedAt, ID: n.ID})
	return base64.RawURLEncoding.EncodeToString(b)
}
