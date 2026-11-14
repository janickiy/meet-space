package postgres

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// forwardReplay uses existing per-message idempotency keys. A batch commits
// atomically, so either every position exists or none does.
func (r *ChatRepository) forwardReplay(tx *gorm.DB, user, target string, request chat.ForwardRequest, fingerprint string) ([]chat.Message, error) {
	ids := make([]string, len(request.MessageIDs))
	for i := range ids {
		ids[i] = chat.ForwardMessageRequestID(request.ClientRequestID, i)
	}
	var found []chat.Message
	if err := r.messages(tx).Where("conversation_id=? AND sender_user_id=? AND client_request_id IN ?", target, user, ids).Find(&found).Error; err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, nil
	}
	if len(found) != len(ids) {
		return nil, apperrors.New(apperrors.ErrConflict, "forwarding request key is already in use")
	}
	byRequest := map[string]string{}
	for _, m := range found {
		if m.RequestFingerprint != fingerprint {
			return nil, apperrors.New(apperrors.ErrConflict, "clientRequestId was used for another forwarding request")
		}
		byRequest[m.ClientRequestID] = m.ID
	}
	result := make([]chat.Message, 0, len(ids))
	for _, id := range ids {
		m, err := r.loadMessage(tx, target, byRequest[id], user)
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, nil
}

// Lock source and target in a stable order, including when two users forward in
// opposite directions. Membership mutations take the same scope locks.
func (r *ChatRepository) authorizeForward(tx *gorm.DB, user, target string, request chat.ForwardRequest) (*ChatRepository, error) {
	source := NewChatRepository(r.db)
	if request.SourceType == "conversation" {
		source.direct = true
	}
	type scope struct {
		key, id string
		repo    *ChatRepository
		write   bool
	}
	scopes := []scope{{"conversation:" + target, target, r, true}, {request.SourceType + ":" + request.SourceID, request.SourceID, source, false}}
	sort.Slice(scopes, func(i, j int) bool { return scopes[i].key < scopes[j].key })
	for _, s := range scopes {
		if _, err := s.repo.authorize(tx, user, s.id, true, s.write); err != nil {
			return nil, err
		}
	}
	return source, nil
}

func (r *ChatRepository) forwardSources(tx *gorm.DB, user string, request chat.ForwardRequest) ([]chat.Message, error) {
	cutoff, err := r.historyCutoff(tx, user, request.SourceID)
	if err != nil {
		return nil, err
	}
	var messages []chat.Message
	if err := r.messageQuery(tx).Where(r.sql("chat_messages.conference_id=? AND chat_messages.id IN ? AND chat_messages.sequence>?"), request.SourceID, request.MessageIDs, cutoff).Order(r.sql("chat_messages.sequence ASC")).Find(&messages).Error; err != nil {
		return nil, err
	}
	if len(messages) != len(request.MessageIDs) {
		return nil, apperrors.New(apperrors.ErrNotFound, "one or more messages are unavailable")
	}
	if err := r.decorateMessages(tx, messages, cutoff); err != nil {
		return nil, err
	}
	var bytes int64
	count := 0
	for _, m := range messages {
		if m.DeletedAt != nil || m.CallID != nil || m.Call != nil {
			return nil, apperrors.New(apperrors.ErrConflict, "deleted messages and call events cannot be forwarded")
		}
		for _, a := range m.Attachments {
			count++
			bytes += a.Size
			if a.Status != "attached" || a.ObjectKey == "" || len(a.Checksum) != 64 || a.Size < 1 || a.Size > chat.AbsoluteMaxAttachmentBytes {
				return nil, apperrors.New(apperrors.ErrConflict, "source attachment is unavailable")
			}
		}
	}
	if count > chat.MaxForwardAttachments || bytes > chat.MaxForwardBytes {
		return nil, apperrors.New(apperrors.ErrInvalidInput, "forward at most 20 attachments totalling 100 MiB at a time")
	}
	return messages, nil
}

func (r *ChatRepository) existingForward(ctx context.Context, user, target string, request chat.ForwardRequest, fingerprint string) ([]chat.Message, error) {
	var result []chat.Message
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := r.authorize(tx, user, target, false, true); err != nil {
			return err
		}
		var err error
		result, err = r.forwardReplay(tx, user, target, request, fingerprint)
		return err
	})
	return result, err
}

func (r *ChatRepository) PrepareForward(ctx context.Context, user, target string, request chat.ForwardRequest, fingerprint string) (chat.ForwardPlan, error) {
	plan := chat.ForwardPlan{}
	if !r.direct {
		return plan, apperrors.ErrForbidden
	}
	// A completed replay does not need the original message to still exist.
	existing, err := r.existingForward(ctx, user, target, request, fingerprint)
	if err != nil {
		return plan, err
	}
	if existing != nil {
		plan.Existing = existing
		return plan, nil
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		source, err := r.authorizeForward(tx, user, target, request)
		if err != nil {
			return err
		}
		plan.Messages, err = source.forwardSources(tx, user, request)
		return err
	})
	return plan, err
}

// CommitForward rechecks source snapshots after private objects have been copied.
// Pending target attachments are durable before this transaction and retain the
// existing TTL cleanup lifecycle if copying, permission checks or commit fail.
func (r *ChatRepository) CommitForward(ctx context.Context, user, target string, request chat.ForwardRequest, fingerprint string, expected []chat.Message, copied map[string]string) ([]chat.Message, bool, error) {
	var result []chat.Message
	created := false
	if !r.direct {
		return nil, false, apperrors.ErrForbidden
	}
	existing, err := r.existingForward(ctx, user, target, request, fingerprint)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		return existing, false, nil
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		source, err := r.authorizeForward(tx, user, target, request)
		if err != nil {
			return err
		}
		// Another batch may have completed while waiting for the scope locks.
		existing, err = r.forwardReplay(tx, user, target, request, fingerprint)
		if err != nil {
			return err
		}
		if existing != nil {
			result = existing
			return nil
		}
		messages, err := source.forwardSources(tx, user, request)
		if err != nil {
			return err
		}
		if len(messages) != len(expected) {
			return apperrors.ErrConflict
		}
		for i, m := range messages {
			if m.ID != expected[i].ID || m.Version != expected[i].Version || len(m.Attachments) != len(expected[i].Attachments) {
				return apperrors.New(apperrors.ErrConflict, "source messages changed; select them again")
			}
			for j, a := range m.Attachments {
				old := expected[i].Attachments[j]
				if a.ID != old.ID || a.ObjectKey != old.ObjectKey || a.Checksum != old.Checksum {
					return apperrors.ErrConflict
				}
			}
		}
		for i, m := range messages {
			author := strings.TrimSpace(m.SenderName)
			if m.ForwardedSenderName != nil {
				author = *m.ForwardedSenderName
			}
			if author == "" {
				author = "Пользователь"
			}
			if len([]rune(author)) > 100 {
				author = string([]rune(author)[:100])
			}
			out := chat.Message{ID: uuid.NewString(), ConversationID: target, SenderID: user, ClientRequestID: chat.ForwardMessageRequestID(request.ClientRequestID, i), RequestFingerprint: fingerprint, Text: m.Text, Version: 1, ForwardedSenderName: &author}
			if err := r.messages(tx).Create(&out).Error; err != nil {
				return err
			}
			for _, a := range m.Attachments {
				destinationID, ok := copied[a.ID]
				if !ok {
					return apperrors.ErrConflict
				}
				var dest chat.Attachment
				if err := r.attachments(tx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND conversation_id=? AND owner_user_id=?", destinationID, target, user).Take(&dest).Error; err != nil {
					return mapNotFound(err)
				}
				if dest.Status != "ready" || !dest.ExpiresAt.After(time.Now()) || dest.MessageID != nil || dest.Size != a.Size || dest.Checksum != a.Checksum || dest.Filename != a.Filename || dest.MimeType != a.MimeType || dest.ObjectKey == a.ObjectKey || dest.ClientRequestID != chat.ForwardAttachmentRequestID(request.ClientRequestID, a.ID) {
					return apperrors.New(apperrors.ErrConflict, "copied attachment is unavailable")
				}
				if err := r.attachments(tx).Where("id=?", dest.ID).Updates(map[string]any{"status": "attached", "message_id": out.ID, "updated_at": gorm.Expr("clock_timestamp()")}).Error; err != nil {
					return err
				}
			}
			if err := createConversationNotifications(tx, out.ID); err != nil {
				return err
			}
			loaded, err := r.loadMessage(tx, target, out.ID, user)
			if err != nil {
				return err
			}
			result = append(result, loaded)
		}
		last := result[len(result)-1]
		if err := tx.Table("conversations").Where("id=?", target).Updates(map[string]any{"last_message_at": last.CreatedAt, "last_message_id": last.ID, "updated_at": gorm.Expr("clock_timestamp()")}).Error; err != nil {
			return err
		}
		if err := tx.Table("conversation_members").Where("conversation_id=? AND hidden_at IS NOT NULL", target).Updates(map[string]any{"hidden_at": nil, "updated_at": gorm.Expr("clock_timestamp()")}).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return result, created, nil
}
