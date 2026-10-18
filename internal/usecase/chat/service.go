package chat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/realtime"
)

// Repository defines the replaceable dependency contract for persistent chat messages and
// private attachments.
type Repository interface {
	// List returns a bounded page of chat message records using the supplied pagination and filters.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Opaque continuation boundary from the previous page.
	//   - argument 5: Maximum number of results or operations in this batch.
	//
	// Returns:
	//   - result 1: The resulting page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	List(context.Context, string, string, string, int) (domain.Page, error)
	// Send validates access, reply and attachment references and persists the chat message with
	// idempotency protection against duplicate retries.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Validated application request or incoming operation parameters.
	//   - argument 5: Stable request fingerprint used to detect conflicting idempotent retries.
	//
	// Returns:
	//   - result 1: The resulting message.
	//   - result 2: Whether this request created new state rather than reusing an idempotent result.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Send(context.Context, string, string, domain.SendRequest, string) (domain.Message, bool, error)
	// Edit changes the author's existing message text after checking visibility and edit
	// eligibility.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Identifier of the resource addressed by this operation.
	//   - argument 5: UTF-8 text to validate, normalize or process.
	//
	// Returns:
	//   - result 1: The resulting message.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Edit(context.Context, string, string, string, string) (domain.Message, error)
	// Delete removes the requested chat message using the caller's scope and the resource lifecycle
	// rules.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting message.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Delete(context.Context, string, string, string) (domain.Message, error)
	// ReadState returns the member's saved read boundary and visible unread-message count.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//
	// Returns:
	//   - result 1: The resulting read state.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ReadState(context.Context, string, string) (domain.ReadState, error)
	// MarkRead advances the saved read boundary without allowing late or repeated requests to move
	// it backward.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Message identifier within the authorized conversation.
	//
	// Returns:
	//   - result 1: The resulting read state.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	MarkRead(context.Context, string, string, string) (domain.ReadState, error)
	// InitAttachment creates or reuses pending attachment metadata so an upload can be retried
	// safely.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The resulting attachment.
	//   - result 2: Whether this request created new state rather than reusing an idempotent result.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	InitAttachment(context.Context, string, string, domain.InitRequest) (domain.Attachment, bool, error)
	// ClaimUpload atomically claims an attachment upload attempt using a time-limited ownership
	// token.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Identifier of the resource addressed by this operation.
	//   - argument 5: Signed credential or operation-ownership token to verify.
	//
	// Returns:
	//   - result 1: The resulting attachment.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ClaimUpload(context.Context, string, string, string, string) (domain.Attachment, error)
	// CompleteUpload stores the uploaded object result only while the attempt token still owns the
	// attachment.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Identifier of the resource addressed by this operation.
	//   - argument 5: Signed credential or operation-ownership token to verify.
	//   - argument 6: Expected content digest used to verify transfer integrity.
	//
	// Returns:
	//   - result 1: The resulting attachment.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	CompleteUpload(context.Context, string, string, string, string, string) (domain.Attachment, error)
	// AbortUpload releases only the upload attempt identified by its current ownership token.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Signed credential or operation-ownership token to verify.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	AbortUpload(context.Context, string, string) error
	// AttachmentForFinalize loads the owner-visible attachment metadata needed to confirm its
	// upload.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting attachment.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	AttachmentForFinalize(context.Context, string, string, string) (domain.Attachment, error)
	// FinalizeAttachment marks a successfully uploaded attachment ready to be referenced by a
	// message.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Identifier of the resource addressed by this operation.
	//   - argument 5: Server-generated private object-storage key.
	//
	// Returns:
	//   - result 1: The resulting attachment.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	FinalizeAttachment(context.Context, string, string, string, string) (domain.Attachment, error)
	// DownloadAttachment loads a finalized attachment only when it belongs to a message visible to
	// the caller.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting attachment.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	DownloadAttachment(context.Context, string, string, string) (domain.Attachment, error)
	// CleanupCandidates returns a bounded batch of expired attachment attempts eligible for object
	// cleanup.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Maximum number of results or operations in this batch.
	//
	// Returns:
	//   - result 1: The collected attachment values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	CleanupCandidates(context.Context, int) ([]domain.Attachment, error)
	// CompleteCleanup records completion of the claimed attachment cleanup.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Storage, cache or cryptographic key used by the operation.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	CompleteCleanup(context.Context, string, string) error
}

// Storage defines the replaceable dependency contract for persistent chat messages and private
// attachments.
type Storage interface {
	// CheckAttachmentPrivacy rejects object-storage settings that expose private chat attachments
	// through a public bucket policy.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	CheckAttachmentPrivacy(context.Context) error
	// PutAttachment stores attachment bytes under a server-generated private object key.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Storage, cache or cryptographic key used by the operation.
	//   - argument 3: Source content stream or recording-card reader, as defined by its type.
	//   - argument 4: Content size in bytes or the requested bounded capacity.
	//   - argument 5: Declared or detected MIME type of the content.
	//   - argument 6: Expected content digest used to verify transfer integrity.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	PutAttachment(context.Context, string, io.Reader, int64, string, string) error
	// StatAttachment reads the actual object size and metadata before upload finalization.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Storage, cache or cryptographic key used by the operation.
	//
	// Returns:
	//   - result 1: The resulting count, size or numeric value.
	//   - result 2: Actual stored object content type.
	//   - result 3: Stored SHA-256 content digest used for finalization.
	//   - result 4: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	StatAttachment(context.Context, string) (int64, string, string, error)
	// AttachmentDownloadURL creates a short-lived signed URL that requests attachment download.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Storage, cache or cryptographic key used by the operation.
	//   - argument 3: Safe download filename included in the content disposition.
	//   - argument 4: Expiration deadline or validity interval, as defined by its type.
	//
	// Returns:
	//   - result 1: Short-lived signed attachment-download URL.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	AttachmentDownloadURL(context.Context, string, string, time.Duration) (string, error)
	// CleanAttachmentObjects removes only the exact attachment prefix objects that are not the
	// retained finalized object.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Server-controlled object or key prefix.
	//   - argument 3: Exact finalized object key that cleanup must retain.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	CleanAttachmentObjects(context.Context, string, string) error
}

// Events defines the replaceable dependency contract for persistent chat messages and private
// attachments.
type Events interface {
	// Broadcast publishes a trusted event only to the currently authorized conference or
	// conversation recipients.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Trusted event to publish or process.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Broadcast(context.Context, realtime.Envelope) error
	// SendToParticipant publishes a trusted event to the physical sessions of the selected
	// authorized participant.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier restricting event visibility.
	//   - argument 3: Account identifier of the intended participant.
	//   - argument 4: Trusted event to publish or process.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SendToParticipant(context.Context, string, string, realtime.Envelope) error
}

// Service coordinates persistent chat messages and private attachments through its configured
// persistence and transport dependencies.
type Service struct {
	repo    Repository
	storage Storage
	events  Events
	uploads chan struct{}
}

// NewService constructs the service with the dependencies and limits required for persistent
// chat messages and private attachments.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - repo: Persistence dependency used by the application operation.
//   - storage: Private object or file storage dependency.
//   - events: Destination for committed lifecycle or chat events.
//
// Returns:
//   - result 1: The constructed or loaded service.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NewService(ctx context.Context, repo Repository, storage Storage, events Events) (*Service, error) {
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := storage.CheckAttachmentPrivacy(check); err != nil {
		return nil, err
	}
	return &Service{repo: repo, storage: storage, events: events, uploads: make(chan struct{}, 4)}, nil
}

// List returns a bounded page of chat message records using the supplied pagination and filters.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//   - cursor: Opaque continuation boundary from the previous page.
//   - limit: Maximum number of results or operations in this batch.
//
// Returns:
//   - result 1: The resulting page.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) List(ctx context.Context, user, conference, cursor string, limit int) (domain.Page, error) {
	return s.repo.List(ctx, user, conference, cursor, limit)
}

// Send validates access, reply and attachment references and persists the chat message with
// idempotency protection against duplicate retries.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting message.
//   - result 2: Whether this request created new state rather than reusing an idempotent result.
//   - result 3: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Send(ctx context.Context, user, conference string, request domain.SendRequest) (domain.Message, bool, error) {
	request, fingerprint, err := domain.NormalizeSend(request)
	if err != nil {
		return domain.Message{}, false, err
	}
	message, created, err := s.repo.Send(ctx, user, conference, request, fingerprint)
	if err == nil && created {
		s.publish(ctx, "chat.message.created", message)
	}
	return message, created, err
}

// Edit changes the author's existing message text after checking visibility and edit
// eligibility.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//   - id: Identifier of the resource addressed by this operation.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting message.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Edit(ctx context.Context, user, conference, id string, request domain.EditRequest) (domain.Message, error) {
	text, err := domain.NormalizeText(request.Text, true)
	if err != nil {
		return domain.Message{}, err
	}
	message, err := s.repo.Edit(ctx, user, conference, id, text)
	if err == nil {
		s.publish(ctx, "chat.message.updated", message)
	}
	return message, err
}

// Delete removes the requested chat message using the caller's scope and the resource lifecycle
// rules.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting message.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Delete(ctx context.Context, user, conference, id string) (domain.Message, error) {
	message, err := s.repo.Delete(ctx, user, conference, id)
	if err == nil {
		s.publish(ctx, "chat.message.deleted", message)
	}
	return message, err
}

// publish publishes the saved chat message change through its configured event or command
// transport.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - kind: Validated event, job, media or resource category.
//   - message: Persisted or outgoing chat message.
func (s *Service) publish(ctx context.Context, kind string, message domain.Message) {
	if s.events != nil {
		if err := s.events.Broadcast(ctx, realtime.Event(kind, message.ConferenceID, message)); err != nil {
			slog.Warn("chat realtime delivery deferred to client refresh", "conference_id", message.ConferenceID, "event_type", kind)
		}
	}
}

// ReadState returns the member's saved read boundary and visible unread-message count.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//
// Returns:
//   - result 1: The resulting read state.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) ReadState(ctx context.Context, user, conference string) (domain.ReadState, error) {
	return s.repo.ReadState(ctx, user, conference)
}

// MarkRead advances the saved read boundary without allowing late or repeated requests to move
// it backward.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting read state.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) MarkRead(ctx context.Context, user, conference string, request domain.ReadRequest) (domain.ReadState, error) {
	id, err := domain.UUID(request.MessageID)
	if err != nil {
		return domain.ReadState{}, err
	}
	state, err := s.repo.MarkRead(ctx, user, conference, id)
	if err == nil && s.events != nil {
		_ = s.events.SendToParticipant(ctx, conference, state.ParticipantID, realtime.Event("chat.read.updated", conference, state))
	}
	return state, err
}

// InitAttachment creates or reuses pending attachment metadata so an upload can be retried
// safely.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting attachment.
//   - result 2: Whether this request created new state rather than reusing an idempotent result.
//   - result 3: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) InitAttachment(ctx context.Context, user, conference string, request domain.InitRequest) (domain.Attachment, bool, error) {
	request, err := domain.NormalizeInit(request)
	if err != nil {
		return domain.Attachment{}, false, err
	}
	return s.repo.InitAttachment(ctx, user, conference, request)
}

// ValidateContent checks attachment bytes, size, digest and actual media format rather than
// trusting declared metadata.
//
// Parameters:
//   - data: Input bytes or structured event payload.
//   - attachment: Private attachment identity or metadata, as defined by its type.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func ValidateContent(data []byte, attachment domain.Attachment) error {
	if int64(len(data)) != attachment.Size || len(data) == 0 || int64(len(data)) > domain.MaxAttachmentBytes {
		return apperrors.New(apperrors.ErrInvalidInput, "uploaded size differs from the declared attachment size")
	}
	detected := strings.Split(http.DetectContentType(data), ";")[0]
	expected := attachment.MimeType
	if expected == "text/csv" {
		expected = "text/plain"
	}
	if detected != expected {
		return apperrors.New(apperrors.ErrInvalidInput, "file content does not match its allowed type")
	}
	if expected == "text/plain" && (!utf8.Valid(data) || bytes.ContainsRune(data, 0)) {
		return apperrors.New(apperrors.ErrInvalidInput, "text attachments must be valid UTF-8")
	}
	if expected == "image/jpeg" || expected == "image/png" {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
			return apperrors.New(apperrors.ErrInvalidInput, "invalid image or image dimensions exceed the limit")
		}
	}
	return nil
}

// Upload validates a bounded content stream and stores it for the currently owned upload
// attempt.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//   - id: Identifier of the resource addressed by this operation.
//   - reader: Source content stream or recording-card reader, as defined by its type.
//
// Returns:
//   - result: The resulting attachment.
//   - err: nil on success; otherwise the validation, access, transport or persistence failure.
func (s *Service) Upload(ctx context.Context, user, conference, id string, reader io.Reader) (result domain.Attachment, err error) {
	select {
	case s.uploads <- struct{}{}:
		defer func() { <-s.uploads }()
	default:
		return result, apperrors.New(apperrors.ErrUnavailable, "attachment upload capacity is busy")
	}
	op, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	token := uuid.NewString()
	a, err := s.repo.ClaimUpload(op, user, conference, id, token)
	if err != nil {
		return result, err
	}
	defer func() {
		if err != nil && a.UploadedAt == nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = s.repo.AbortUpload(cleanup, id, token)
		}
	}()
	data, err := io.ReadAll(io.LimitReader(reader, domain.MaxAttachmentBytes+1))
	if err != nil {
		return result, apperrors.New(apperrors.ErrInvalidInput, "attachment upload was interrupted")
	}
	if err = ValidateContent(data, a); err != nil {
		return result, err
	}
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	if a.UploadedAt != nil {
		if a.Checksum != checksum {
			return result, apperrors.New(apperrors.ErrConflict, "attachment already contains different bytes")
		}
		return a, nil
	}
	if err = s.storage.PutAttachment(op, a.ObjectKey, bytes.NewReader(data), int64(len(data)), a.MimeType, checksum); err != nil {
		return result, apperrors.ErrUnavailable
	}
	return s.repo.CompleteUpload(op, user, conference, id, token, checksum)
}

// FinalizeAttachment marks a successfully uploaded attachment ready to be referenced by a
// message.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting attachment.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) FinalizeAttachment(ctx context.Context, user, conference, id string) (domain.Attachment, error) {
	a, err := s.repo.AttachmentForFinalize(ctx, user, conference, id)
	if err != nil {
		return a, err
	}
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	size, mime, checksum, err := s.storage.StatAttachment(op, a.ObjectKey)
	if err != nil {
		return domain.Attachment{}, apperrors.ErrUnavailable
	}
	if size != a.Size || mime != a.MimeType || checksum != a.Checksum {
		return domain.Attachment{}, apperrors.New(apperrors.ErrConflict, "stored attachment validation failed")
	}
	return s.repo.FinalizeAttachment(ctx, user, conference, id, a.ObjectKey)
}

// Download checks attachment visibility and returns a short-lived private download URL.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: Temporary signed download URL for the authorized private attachment.
//   - result 2: The timestamp or validity boundary associated with the result.
//   - result 3: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Download(ctx context.Context, user, conference, id string) (string, time.Time, error) {
	a, err := s.repo.DownloadAttachment(ctx, user, conference, id)
	if err != nil {
		return "", time.Time{}, err
	}
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	url, err := s.storage.AttachmentDownloadURL(op, a.ObjectKey, a.Filename, domain.DownloadTTL)
	if err != nil {
		return "", time.Time{}, apperrors.ErrUnavailable
	}
	return url, time.Now().UTC().Add(domain.DownloadTTL), nil
}

// Cleanup claims expired unfinished attachment uploads and removes only their owned private
// objects.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Cleanup(ctx context.Context) error {
	rows, err := s.repo.CleanupCandidates(ctx, 100)
	if err != nil {
		return err
	}
	for _, a := range rows {
		keep := ""
		if a.Status == "attached" {
			keep = a.ObjectKey
		}
		if err := s.storage.CleanAttachmentObjects(ctx, a.Prefix(), keep); err != nil {
			return err
		}
		if err := s.repo.CompleteCleanup(ctx, a.ID, a.ObjectKey); err != nil {
			return err
		}
	}
	return nil
}

// Run runs persistent chat messages and private attachments until the context is canceled or
// processing fails.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		op, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := s.Cleanup(op)
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.Warn("attachment orphan cleanup will retry")
		}
	}
}
