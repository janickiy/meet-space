package chat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

const (
	MaxTextRunes             = 4000
	MaxAttachments           = 5
	MaxAttachmentBytes int64 = 10 << 20
	AttachmentTTL            = 30 * time.Minute
	UploadLease              = 2 * time.Minute
	DownloadTTL              = 5 * time.Minute
)

// Message persists chat order, revision, author and retry identity. Its internal request
// fingerprint is excluded from JSON.
type Message struct {
	ID                 string        `json:"id" gorm:"type:uuid;primaryKey"`
	Sequence           int64         `json:"sequence,string" gorm:"autoIncrement"`
	ConferenceID       string        `json:"conferenceId,omitempty"`
	ConversationID     string        `json:"conversationId,omitempty"`
	SenderID           string        `json:"senderId" gorm:"column:sender_user_id"`
	SenderName         string        `json:"senderName" gorm:"->;-:migration"`
	ClientRequestID    string        `json:"clientRequestId"`
	RequestFingerprint string        `json:"-"`
	Text               string        `json:"text"`
	ReplyTo            *string       `json:"replyTo" gorm:"column:reply_to_id"`
	ReplyPreview       *ReplyPreview `json:"replyPreview,omitempty" gorm:"-"`
	Version            int64         `json:"version"`
	CreatedAt          time.Time     `json:"createdAt"`
	UpdatedAt          time.Time     `json:"updatedAt"`
	DeletedAt          *time.Time    `json:"deletedAt"`
	Attachments        []Attachment  `json:"attachments" gorm:"-"`
	Important          bool          `json:"important,omitempty" gorm:"-"`
	Pinned             bool          `json:"pinned,omitempty" gorm:"-"`
	CallID             *string       `json:"-" gorm:"->;-:migration"`
	Call               *CallHistory  `json:"call,omitempty" gorm:"-"`
}

// CallHistory projects the authoritative call state without exposing room or device credentials.
type CallHistory struct {
	ID           string     `json:"id"`
	CallerUserID string     `json:"callerUserId"`
	CalleeUserID string     `json:"calleeUserId"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"createdAt"`
	AcceptedAt   *time.Time `json:"acceptedAt"`
	EndedAt      *time.Time `json:"endedAt"`
	DurationSec  int64      `json:"durationSec"`
}

// TableName returns the explicit database table name so GORM does not infer a different plural
// form.
//
// Returns:
//   - result 1: Exact database table name used by GORM.
func (Message) TableName() string { return "chat_messages" }

// ReplyPreview contains a bounded view of the replied-to message and respects soft deletion.
type ReplyPreview struct {
	Sequence   int64  `json:"sequence,string,omitempty"`
	ID         string `json:"id"`
	Text       string `json:"text"`
	SenderName string `json:"senderName"`
	Deleted    bool   `json:"deleted"`
}

// Attachment persists its owner, conference, object metadata and upload state. Internal object
// paths and lease tokens are excluded from JSON.
type Attachment struct {
	ID               string     `json:"id" gorm:"type:uuid;primaryKey"`
	ConferenceID     string     `json:"conferenceId,omitempty"`
	ConversationID   string     `json:"conversationId,omitempty"`
	OwnerID          string     `json:"ownerId" gorm:"column:owner_user_id"`
	ClientRequestID  string     `json:"clientRequestId"`
	Filename         string     `json:"filename"`
	MimeType         string     `json:"mimeType"`
	Size             int64      `json:"size"`
	Checksum         string     `json:"checksum,omitempty"`
	Status           string     `json:"status"`
	MessageID        *string    `json:"messageId,omitempty"`
	ObjectKey        string     `json:"-"`
	UploadToken      *string    `json:"-"`
	UploadLeaseUntil *time.Time `json:"-"`
	UploadedAt       *time.Time `json:"-"`
	CleanedAt        *time.Time `json:"-"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	ExpiresAt        time.Time  `json:"expiresAt"`
}

// TableName returns the explicit database table name so GORM does not infer a different plural
// form.
//
// Returns:
//   - result 1: Exact database table name used by GORM.
func (Attachment) TableName() string { return "chat_attachments" }

// Prefix returns the server-controlled object prefix for the attachment.
//
// Returns:
//   - result 1: Server-controlled private attachment prefix.
func (a Attachment) Prefix() string {
	if a.ConversationID != "" {
		return "attachments/direct/" + a.ConversationID + "/" + a.ID + "/"
	}
	return "attachments/" + a.ConferenceID + "/" + a.ID + "/"
}

// SendRequest contains message text, a reply reference, attachment identifiers and a retry
// idempotency key.
type SendRequest struct {
	ClientRequestID string   `json:"clientRequestId"`
	Text            string   `json:"text"`
	ReplyTo         string   `json:"replyTo,omitempty"`
	AttachmentIDs   []string `json:"attachmentIds,omitempty"`
}

// EditRequest contains the replacement text of an existing message.
type EditRequest struct {
	Text string `json:"text"`
}

// InitRequest describes a future attachment before a server-owned upload key is allocated.
type InitRequest struct {
	ClientRequestID string `json:"clientRequestId"`
	Filename        string `json:"filename"`
	MimeType        string `json:"mimeType"`
	Size            int64  `json:"size"`
}

// ReadRequest identifies the message through which the account has read the conversation.
type ReadRequest struct {
	MessageID string `json:"messageId"`
}

// ReadState contains a monotonic read boundary and the visible unread-message count.
type ReadState struct {
	LastReadMessageID *string `json:"lastReadMessageId"`
	LastReadSequence  int64   `json:"-"`
	UnreadCount       int64   `json:"unreadCount"`
	ParticipantID     string  `json:"-"`
}

// Page contains one bounded result page and its continuation metadata.
type Page struct {
	Items             []Message `json:"items"`
	NextCursor        string    `json:"nextCursor,omitempty"`
	UnreadCount       int64     `json:"unreadCount"`
	LastReadMessageID *string   `json:"lastReadMessageId"`
}

// UUID checks and normalizes a UUID before it is used as a resource identifier.
//
// Parameters:
//   - value: Value to validate, normalize or convert.
//
// Returns:
//   - result 1: Canonical nonzero UUID.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func UUID(value string) (string, error) {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil {
		return "", apperrors.New(apperrors.ErrInvalidInput, "a non-zero UUID is required")
	}
	return id.String(), nil
}

// NormalizeText normalizes and bounds message text before persistence.
//
// Parameters:
//   - text: UTF-8 text to validate, normalize or process.
//   - allowEmpty: Whether empty normalized text is permitted.
//
// Returns:
//   - result 1: Validated normalized message text.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NormalizeText(text string, allowEmpty bool) (string, error) {
	text = strings.TrimSpace(text)
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > MaxTextRunes || (!allowEmpty && text == "") || strings.ContainsRune(text, 0) {
		return "", apperrors.New(apperrors.ErrInvalidInput, "message must contain 1 to 4000 characters or an attachment")
	}
	return text, nil
}

// NormalizeSend validates message text, reply and attachment references before a message is
// sent.
//
// Parameters:
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting send request.
//   - result 2: Stable SHA-256 fingerprint of the normalized send request.
//   - result 3: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NormalizeSend(request SendRequest) (SendRequest, string, error) {
	var err error
	request.ClientRequestID, err = UUID(request.ClientRequestID)
	if err != nil {
		return request, "", err
	}
	if len(request.AttachmentIDs) > MaxAttachments {
		return request, "", apperrors.New(apperrors.ErrInvalidInput, "at most 5 attachments are allowed")
	}
	request.AttachmentIDs = append([]string(nil), request.AttachmentIDs...)
	seen := map[string]bool{}
	for i, id := range request.AttachmentIDs {
		id, err = UUID(id)
		if err != nil {
			return request, "", err
		}
		if seen[id] {
			return request, "", apperrors.New(apperrors.ErrInvalidInput, "duplicate attachment")
		}
		seen[id] = true
		request.AttachmentIDs[i] = id
	}
	sort.Strings(request.AttachmentIDs)
	if request.ReplyTo != "" {
		request.ReplyTo, err = UUID(request.ReplyTo)
		if err != nil {
			return request, "", err
		}
	}
	request.Text, err = NormalizeText(request.Text, len(request.AttachmentIDs) > 0)
	if err != nil {
		return request, "", err
	}
	raw, _ := json.Marshal(request)
	sum := sha256.Sum256(raw)
	return request, hex.EncodeToString(sum[:]), nil
}

// NormalizeInit validates attachment metadata before allocating an upload attempt.
//
// Parameters:
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting init request.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NormalizeInit(request InitRequest) (InitRequest, error) {
	var err error
	request.ClientRequestID, err = UUID(request.ClientRequestID)
	if err != nil {
		return request, err
	}
	request.Filename = strings.TrimSpace(request.Filename)
	if !utf8.ValidString(request.Filename) || request.Filename == "" || utf8.RuneCountInString(request.Filename) > 180 || filepath.Base(request.Filename) != request.Filename || strings.ContainsAny(request.Filename, "/\\") || strings.Contains(request.Filename, "..") {
		return request, apperrors.New(apperrors.ErrInvalidInput, "invalid filename")
	}
	for _, r := range request.Filename {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return request, apperrors.New(apperrors.ErrInvalidInput, "invalid filename")
		}
	}
	types := map[string]string{".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".webp": "image/webp", ".pdf": "application/pdf", ".txt": "text/plain", ".csv": "text/csv"}
	expected, ok := types[strings.ToLower(filepath.Ext(request.Filename))]
	if !ok {
		return request, apperrors.New(apperrors.ErrInvalidInput, "only JPG, PNG, WebP, PDF, TXT and CSV attachments are supported")
	}
	if request.MimeType != "" && request.MimeType != expected && !(expected == "text/csv" && request.MimeType == "text/plain") {
		return request, apperrors.New(apperrors.ErrInvalidInput, "filename and MIME type disagree")
	}
	request.MimeType = expected
	if request.Size < 1 || request.Size > MaxAttachmentBytes {
		return request, apperrors.New(apperrors.ErrInvalidInput, fmt.Sprintf("attachment size must be between 1 and %d bytes", MaxAttachmentBytes))
	}
	return request, nil
}
