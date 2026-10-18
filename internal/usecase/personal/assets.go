package personal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	domain "github.com/janickiy/meet-space/internal/domain/personal"
)

const MaxAvatarBytes int64 = 2 << 20
const AssetStreamTimeout = 30 * time.Second

// Asset keys are server-only data and never part of a public DTO.
type Asset struct {
	Version, ConversationID, Key, ContentType, Filename, Checksum string
	Size                                                          int64
	CleanupToken                                                  string
}

type AssetRepository interface {
	// BeginAvatar creates a pending avatar publication after locking the authorized group.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//   - argument 4: Private avatar or attachment metadata.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	BeginAvatar(context.Context, string, string, Asset) error
	// ActivateAvatar atomically makes the current avatar upload visible in the group conversation.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//   - argument 4: Version or fencing value used to reject stale ownership.
	//
	// Returns:
	//   - result 1: The resulting conversation.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ActivateAvatar(context.Context, string, string, string) (domain.Conversation, error)
	// ClearAvatar clears the group's active avatar metadata and identifies obsolete objects for
	// cleanup.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting conversation.
	//   - result 2: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ClearAvatar(context.Context, string, string) (domain.Conversation, bool, error)
	// GroupAttachment loads a finalized attachment only when a visible group message references it.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conversation identifier limiting the operation scope.
	//   - argument 4: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 2: The resulting asset.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	GroupAttachment(context.Context, string, string, string) (bool, Asset, error)
	// VisitAsset runs a scoped asset operation while its authorization and resource identity remain
	// valid.
	//
	// The callback runs under a conversation share lock. Membership mutations
	// lock that conversation exclusively, so removal cannot commit mid-stream.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Account identifier requesting the private asset.
	//   - argument 3: Group conversation identifier containing the asset.
	//   - argument 4: Attachment identifier, or the avatar selector expected by the repository.
	//   - argument 5: Callback invoked while the operation's resource ownership and authorization
	//     remain valid.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	VisitAsset(context.Context, string, string, string, func(Asset) error) error
	// AvatarCleanupCandidates returns a bounded batch of obsolete group-avatar objects.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Maximum number of results or operations in this batch.
	//
	// Returns:
	//   - result 1: The collected string values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	AvatarCleanupCandidates(context.Context, int) ([]string, error)
	// ClaimAvatarCleanup claims one obsolete avatar object so concurrent workers cannot delete a
	// current avatar.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Version or fencing value used to reject stale ownership.
	//
	// Returns:
	//   - result 1: The constructed or loaded asset.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ClaimAvatarCleanup(context.Context, string) (*Asset, error)
	// FinishAvatarCleanup records whether the claimed avatar object was successfully removed.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: The asset value required by the contract.
	//   - argument 3: Whether the claimed object cleanup completed successfully.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	FinishAvatarCleanup(context.Context, Asset, bool) error
}
type AssetStorage interface {
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
	// PutPersonalAvatar stores normalized avatar bytes under the server-owned group-avatar key.
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
	PutPersonalAvatar(context.Context, string, io.Reader, int64, string, string) error
	// OpenPersonalAsset opens a private group avatar or attachment with its actual object metadata.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Storage, cache or cryptographic key used by the operation.
	//
	// Returns:
	//   - result 1: The resulting read closer.
	//   - result 2: The resulting count, size or numeric value.
	//   - result 3: Detected asset content type.
	//   - result 4: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	OpenPersonalAsset(context.Context, string) (io.ReadCloser, int64, string, error)
	// DeletePersonalAvatar removes only the private object identified by the obsolete avatar key.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Storage, cache or cryptographic key used by the operation.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	DeletePersonalAvatar(context.Context, string) error
}
type AssetEvents interface {
	// PublishConversation publishes a committed conversation change to its authorized current
	// members.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Validated event, job, media or resource category.
	//   - argument 4: Input bytes or structured event payload.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	PublishConversation(context.Context, string, string, any) error
}
type AssetService struct {
	repo             AssetRepository
	storage          AssetStorage
	events           AssetEvents
	uploads, streams chan struct{}
}

// NewAssetService constructs the asset service with the dependencies and limits required for
// personal and group conversations.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - repo: Persistence dependency used by the application operation.
//   - storage: Private object or file storage dependency.
//   - events: Destination for committed lifecycle or chat events.
//
// Returns:
//   - result 1: The constructed or loaded asset service.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NewAssetService(ctx context.Context, repo AssetRepository, storage AssetStorage, events AssetEvents) (*AssetService, error) {
	if repo == nil || storage == nil {
		return nil, fmt.Errorf("personal assets dependencies are required")
	}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := storage.CheckAttachmentPrivacy(check); err != nil {
		return nil, err
	}
	return &AssetService{repo: repo, storage: storage, events: events, uploads: make(chan struct{}, 2), streams: make(chan struct{}, 8)}, nil
}

// assetIDs validates the account and conversation identifiers before private asset operations.
//
// Parameters:
//   - user: Acting account identifier used to scope access.
//   - conversation: Conversation identifier limiting the operation scope.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func assetIDs(user, conversation string) error {
	if _, err := chat.UUID(user); err != nil {
		return err
	}
	_, err := chat.UUID(conversation)
	return err
}

// acquireAssetSlot waits for a bounded asset-processing slot or stops when the context is
// canceled.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - slots: Bounded semaphore controlling concurrent asset processing.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func acquireAssetSlot(ctx context.Context, slots chan struct{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case slots <- struct{}{}:
		return nil
	default:
		return apperrors.ErrRateLimited
	}
}

// PutAvatar validates and stores a normalized group avatar, then publishes the committed
// conversation change.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conversation: Conversation identifier limiting the operation scope.
//   - contentType: Declared or detected MIME type of the content.
//   - body: Input content stream or serialized request body.
//
// Returns:
//   - result 1: The resulting conversation.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *AssetService) PutAvatar(ctx context.Context, user, conversation, contentType string, body io.Reader) (domain.Conversation, error) {
	if err := assetIDs(user, conversation); err != nil {
		return domain.Conversation{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, AssetStreamTimeout)
	defer cancel()
	if err := acquireAssetSlot(ctx, s.uploads); err != nil {
		return domain.Conversation{}, err
	}
	defer func() { <-s.uploads }()
	data, typ, err := normalizeAvatar(body, contentType)
	if err != nil {
		return domain.Conversation{}, err
	}
	version := uuid.NewString()
	sum := sha256.Sum256(data)
	asset := Asset{Version: version, ConversationID: conversation, Key: "avatars/conversations/" + conversation + "/" + version, ContentType: typ, Size: int64(len(data)), Checksum: hex.EncodeToString(sum[:])}
	// Persist before uploading: crashes or an ambiguous object-store response
	// leave a durable pending row that the cleanup worker can recover.
	if err = s.repo.BeginAvatar(ctx, user, conversation, asset); err != nil {
		return domain.Conversation{}, err
	}
	if err = s.storage.PutPersonalAvatar(ctx, asset.Key, bytes.NewReader(data), asset.Size, typ, asset.Checksum); err != nil {
		return domain.Conversation{}, apperrors.Wrap(apperrors.ErrUnavailable, err, "avatar upload failed")
	}
	item, err := s.repo.ActivateAvatar(ctx, user, conversation, version)
	if err != nil {
		return domain.Conversation{}, err
	}
	s.changed(ctx, conversation)
	return item, nil
}

// DeleteAvatar clears the authorized group avatar and schedules its obsolete private objects for
// cleanup.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conversation: Conversation identifier limiting the operation scope.
//
// Returns:
//   - result 1: The resulting conversation.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *AssetService) DeleteAvatar(ctx context.Context, user, conversation string) (domain.Conversation, error) {
	if err := assetIDs(user, conversation); err != nil {
		return domain.Conversation{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, AssetStreamTimeout)
	defer cancel()
	item, changed, err := s.repo.ClearAvatar(ctx, user, conversation)
	if err != nil {
		return domain.Conversation{}, err
	}
	if changed {
		s.changed(ctx, conversation)
	}
	return item, nil
}

// changed notifies the component observer after local or persisted state changes.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conversation: Conversation identifier limiting the operation scope.
func (s *AssetService) changed(ctx context.Context, conversation string) {
	if s.events != nil {
		_ = s.events.PublishConversation(ctx, conversation, "conversation.updated", map[string]any{"conversationId": conversation, "type": "group"})
	}
}

// normalizeAvatar decodes a bounded image and re-encodes a normalized avatar without trusting
// upload metadata.
//
// Parameters:
//   - body: Input content stream or serialized request body.
//   - contentType: Declared or detected MIME type of the content.
//
// Returns:
//   - result 1: The collected byte values.
//   - result 2: Canonical image content type after re-encoding.
//   - result 3: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func normalizeAvatar(body io.Reader, contentType string) ([]byte, string, error) {
	typ := strings.TrimSpace(strings.Split(contentType, ";")[0])
	if typ != "image/jpeg" && typ != "image/png" {
		return nil, "", apperrors.New(apperrors.ErrInvalidInput, "avatar must be JPEG or PNG")
	}
	raw, err := io.ReadAll(io.LimitReader(body, MaxAvatarBytes+1))
	if err != nil {
		return nil, "", apperrors.ErrInvalidInput
	}
	if len(raw) == 0 || int64(len(raw)) > MaxAvatarBytes {
		return nil, "", apperrors.New(apperrors.ErrInvalidInput, "avatar exceeds 2 MiB")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "jpeg" && format != "png") || "image/"+format != typ || config.Width < 1 || config.Height < 1 || config.Width > 2048 || config.Height > 2048 || int64(config.Width)*int64(config.Height) > 4<<20 {
		return nil, "", apperrors.New(apperrors.ErrInvalidInput, "invalid avatar image or dimensions")
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, "", apperrors.ErrInvalidInput
	}
	// Canonical re-encoding strips embedded metadata and trailing/polyglot data.
	var out avatarBuffer
	if format == "jpeg" {
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 85})
	} else {
		err = png.Encode(&out, img)
	}
	if err != nil || int64(out.Len()) > MaxAvatarBytes {
		return nil, "", apperrors.New(apperrors.ErrInvalidInput, "encoded avatar exceeds 2 MiB")
	}
	return out.Bytes(), typ, nil
}

// Encoding may expand a small compressed input. Bound its output allocation as
// well as the input and decoded dimensions.
type avatarBuffer struct{ bytes.Buffer }

// Write accepts output bytes through the io.Writer contract while enforcing the component's
// buffering rules.
//
// Parameters:
//   - data: Input bytes or structured event payload.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (b *avatarBuffer) Write(data []byte) (int, error) {
	if int64(b.Len()+len(data)) > MaxAvatarBytes {
		return 0, apperrors.ErrInvalidInput
	}
	return b.Buffer.Write(data)
}

// GroupDownload returns a non-capability URL; each byte request reauthorizes.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conversation: Conversation identifier limiting the operation scope.
//   - attachment: Private attachment identity or metadata, as defined by its type.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
//   - result 2: Temporary authorized attachment download URL.
//   - result 3: The timestamp or validity boundary associated with the result.
//   - result 4: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *AssetService) GroupDownload(ctx context.Context, user, conversation, attachment string) (bool, string, time.Time, error) {
	if err := assetIDs(user, conversation); err != nil {
		return true, "", time.Time{}, err
	}
	if _, err := chat.UUID(attachment); err != nil {
		return true, "", time.Time{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	handled, _, err := s.repo.GroupAttachment(ctx, user, conversation, attachment)
	if !handled || err != nil {
		return handled, "", time.Time{}, err
	}
	return true, "/api/v1/conversations/" + conversation + "/attachments/" + attachment + "/content", time.Now().UTC().Add(5 * time.Minute), nil
}

// Stream holds fresh membership authorization and the repository's share lock
// until the object reader closes. An empty attachment selects the current avatar.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conversation: Conversation identifier limiting the operation scope.
//   - attachment: Private attachment identity or metadata, as defined by its type.
//   - deliver: Delivery callback invoked only while the recipient or resource authorization
//     remains valid.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *AssetService) Stream(ctx context.Context, user, conversation, attachment string, deliver func(Asset, io.Reader) error) error {
	if err := assetIDs(user, conversation); err != nil {
		return err
	}
	if attachment != "" {
		if _, err := chat.UUID(attachment); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, AssetStreamTimeout)
	defer cancel()
	if err := acquireAssetSlot(ctx, s.streams); err != nil {
		return err
	}
	defer func() { <-s.streams }()
	return s.repo.VisitAsset(ctx, user, conversation, attachment, func(asset Asset) error {
		body, size, typ, err := s.storage.OpenPersonalAsset(ctx, asset.Key)
		if err != nil {
			return apperrors.Wrap(apperrors.ErrUnavailable, err, "asset is unavailable")
		}
		defer body.Close()
		if size != asset.Size || size < 1 || (attachment == "" && typ != asset.ContentType) {
			return apperrors.ErrUnavailable
		}
		return deliver(asset, io.LimitReader(body, asset.Size))
	})
}

// CleanAvatars claims obsolete avatar objects and deletes only assets no longer used by the
// group.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *AssetService) CleanAvatars(ctx context.Context) error {
	ids, err := s.repo.AvatarCleanupCandidates(ctx, 25)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		asset, err := s.repo.ClaimAvatarCleanup(ctx, id)
		if err != nil {
			return err
		}
		if asset == nil {
			continue
		}
		operation, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = s.storage.DeletePersonalAvatar(operation, asset.Key)
		cancel()
		if finishErr := s.repo.FinishAvatarCleanup(ctx, *asset, err == nil); finishErr != nil {
			return finishErr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Run runs personal and group conversations until the context is canceled or processing fails.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
func (s *AssetService) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		operation, cancel := context.WithTimeout(ctx, 20*time.Second)
		_ = s.CleanAvatars(operation)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
