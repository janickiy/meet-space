package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/calls"
	"github.com/janickiy/meet-space/internal/domain/users"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CallRepository struct{ db *gorm.DB }

func NewCallRepository(db *gorm.DB) *CallRepository { return &CallRepository{db: db} }

func callView(tx *gorm.DB, call calls.Call) (calls.Call, error) {
	var peers []users.User
	if err := tx.Select("id", "display_name").Where("id IN ?", []string{call.CallerUserID, call.CalleeUserID}).Find(&peers).Error; err != nil {
		return call, err
	}
	for _, u := range peers {
		peer := calls.Peer{ID: u.ID, DisplayName: u.ParticipantName()}
		if u.ID == call.CallerUserID {
			call.Caller = peer
		} else {
			call.Callee = peer
		}
	}
	call.SetDuration(time.Now().UTC())
	return call, nil
}
func (r *CallRepository) Get(ctx context.Context, actor, id string) (calls.Call, error) {
	var c calls.Call
	err := r.db.WithContext(ctx).Where("id=? AND (caller_user_id=? OR callee_user_id=?)", id, actor, actor).Take(&c).Error
	if err != nil {
		return c, mapNotFound(err)
	}
	return callView(r.db.WithContext(ctx), c)
}
func (r *CallRepository) Active(ctx context.Context, actor string) (*calls.Call, error) {
	var c calls.Call
	err := r.db.WithContext(ctx).Table("direct_calls c").Select("c.*").Joins("JOIN direct_call_slots s ON s.call_id=c.id").Where("s.user_id=? AND c.status IN ('ringing','accepted')", actor).Take(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c, err = callView(r.db.WithContext(ctx), c)
	return &c, err
}

// Create locks both registered accounts in a stable order before checking slots.
// This serializes crossed calls as well as callers racing for the same callee.
func (r *CallRepository) Create(ctx context.Context, actor, target, requestID, clientID string) (calls.Call, bool, error) {
	var c calls.Call
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var accounts []users.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ? AND guest_conference_id IS NULL", []string{actor, target}).Order("id").Find(&accounts).Error; err != nil {
			return err
		}
		if len(accounts) != 2 || actor == target {
			return apperrors.ErrNotFound
		}
		err := tx.Where("caller_user_id=? AND client_request_id=?", actor, requestID).Take(&c).Error
		if err == nil {
			if c.CalleeUserID != target || c.CallerClientID != clientID {
				return apperrors.New(apperrors.ErrConflict, "call request already used")
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var busy int64
		if err := tx.Table("direct_call_slots").Where("user_id IN ?", []string{actor, target}).Count(&busy).Error; err != nil {
			return err
		}
		if busy > 0 {
			return apperrors.New(apperrors.ErrConflict, "user is already in a call")
		}
		lo, hi := actor, target
		if lo > hi {
			lo, hi = hi, lo
		}
		conversation := uuid.NewString()
		inserted := tx.Exec("INSERT INTO conversations(id,user_low_id,user_high_id) VALUES(?,?,?) ON CONFLICT(user_low_id,user_high_id) DO NOTHING", conversation, lo, hi)
		if inserted.Error != nil {
			return inserted.Error
		}
		if inserted.RowsAffected == 1 {
			if err := tx.Exec("INSERT INTO conversation_members(conversation_id,user_id) VALUES(?,?),(?,?)", conversation, lo, conversation, hi).Error; err != nil {
				return err
			}
		} else {
			var row struct{ ID string }
			if err := tx.Table("conversations").Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("user_low_id=? AND user_high_id=?", lo, hi).Take(&row).Error; err != nil {
				return err
			}
			conversation = row.ID
		}
		now := time.Now().UTC()
		c = calls.Call{ID: uuid.NewString(), ConversationID: conversation, CallerUserID: actor, CalleeUserID: target, CallerClientID: clientID, ClientRequestID: requestID, Status: "ringing", Version: 1, CreatedAt: now, ExpiresAt: now.Add(calls.RingTimeout)}
		if err := tx.Create(&c).Error; err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO direct_call_slots(user_id,call_id) VALUES(?,?),(?,?)", actor, c.ID, target, c.ID).Error; err != nil {
			return err
		}
		messageID := uuid.NewString()
		fingerprint := sha256.Sum256([]byte("call:" + c.ID))
		if err := tx.Exec(`INSERT INTO conversation_messages(id,conversation_id,sender_user_id,client_request_id,request_fingerprint,text,kind,call_id,created_at,updated_at)
   VALUES(?,?,?,?,?,'Звонок','call',?,?,?)`, messageID, conversation, actor, uuid.NewString(), hex.EncodeToString(fingerprint[:]), c.ID, now, now).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE conversations SET last_message_at=?,last_message_id=?,updated_at=? WHERE id=?", now, messageID, now, conversation).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE conversation_members SET hidden_at=NULL,updated_at=? WHERE conversation_id=?", now, conversation).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return c, false, err
	}
	c, err = callView(r.db.WithContext(ctx), c)
	return c, created, err
}

// Change serializes every state transition on the call. The first accept wins;
// retries by that same client are idempotent and other devices cannot take over.
func (r *CallRepository) Change(ctx context.Context, actor, id, action, clientID string) (calls.Call, bool, error) {
	var c calls.Call
	changed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", id).Take(&c).Error; err != nil {
			return mapNotFound(err)
		}
		if action != "expire" && !c.IsParticipant(actor) {
			return apperrors.ErrNotFound
		}
		now := time.Now().UTC()
		if c.Expired(now) {
			if c.Status == "ringing" {
				c.Status = "missed"
			} else {
				c.Status = "ended"
			}
			changed = true
			return finishCall(tx, &c, now)
		}
		if action == "expire" {
			return nil
		}
		switch action {
		case "accept":
			if actor != c.CalleeUserID {
				return apperrors.ErrForbidden
			}
			if c.Status == "accepted" && c.AcceptedByClientID != nil && *c.AcceptedByClientID == clientID {
				return nil
			}
			if c.Status != "ringing" {
				return apperrors.New(apperrors.ErrConflict, "call is no longer ringing")
			}
			conference := uuid.NewString()
			until := now.Add(calls.LeaseTimeout)
			c.Status = "accepted"
			c.AcceptedAt = &now
			c.AcceptedByClientID = &clientID
			c.ConferenceID = &conference
			c.CallerLeaseUntil = &until
			c.CalleeLeaseUntil = &until
			c.Version++
			// A random unused invite satisfies the conference schema; public access is
			// explicitly rejected for kind=direct_call throughout conference APIs.
			invite := strings.ReplaceAll(uuid.NewString(), "-", "")
			if err := tx.Exec(`INSERT INTO conferences(id,owner_id,title,invite_code,status,kind,started_at,waiting_room_enabled)
    VALUES(?,?,'Личный звонок',?,'active','direct_call',?,false)`, conference, c.CallerUserID, invite, now).Error; err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO conference_participants(id,conference_id,user_id,display_name,role,status,joined_at,admission_state)
    SELECT gen_random_uuid(),?,u.id,COALESCE(NULLIF(u.display_name,''),'Пользователь'),CASE WHEN u.id=? THEN 'owner' ELSE 'participant' END,'joined',?,'admitted'
    FROM users u WHERE u.id IN (?,?) AND u.guest_conference_id IS NULL`, conference, c.CallerUserID, now, c.CallerUserID, c.CalleeUserID).Error; err != nil {
				return err
			}
			if err := tx.Save(&c).Error; err != nil {
				return err
			}
			changed = true
			return updateCallHistory(tx, c, now)
		case "heartbeat":
			if c.Status != "accepted" || !c.OwnsMedia(actor, clientID) {
				return apperrors.ErrConflict
			}
			until := now.Add(calls.LeaseTimeout)
			column := "callee_lease_until"
			if actor == c.CallerUserID {
				column = "caller_lease_until"
				c.CallerLeaseUntil = &until
			} else {
				c.CalleeLeaseUntil = &until
			}
			return tx.Model(&calls.Call{}).Where("id=?", id).Update(column, until).Error
		case "decline":
			if actor != c.CalleeUserID {
				return apperrors.ErrForbidden
			}
			if c.Status == "declined" {
				return nil
			}
			if c.Status != "ringing" {
				return apperrors.ErrConflict
			}
			c.Status = "declined"
		case "cancel":
			if actor != c.CallerUserID || !c.OwnsMedia(actor, clientID) {
				return apperrors.ErrForbidden
			}
			if c.Status == "cancelled" {
				return nil
			}
			if c.Status != "ringing" {
				return apperrors.ErrConflict
			}
			c.Status = "cancelled"
		case "end":
			if !c.OwnsMedia(actor, clientID) {
				return apperrors.ErrForbidden
			}
			if c.Status == "ended" {
				return nil
			}
			if c.Status != "accepted" {
				return apperrors.ErrConflict
			}
			c.Status = "ended"
		default:
			return apperrors.ErrInvalidInput
		}
		changed = true
		return finishCall(tx, &c, now)
	})
	if err != nil {
		return c, false, err
	}
	c, err = callView(r.db.WithContext(ctx), c)
	return c, changed, err
}
func updateCallHistory(tx *gorm.DB, c calls.Call, now time.Time) error {
	return tx.Exec("UPDATE conversation_messages SET version=version+1,updated_at=? WHERE call_id=?", now, c.ID).Error
}
func finishCall(tx *gorm.DB, c *calls.Call, now time.Time) error {
	c.EndedAt = &now
	c.Version++
	if c.ConferenceID != nil {
		if err := tx.Exec("UPDATE conferences SET status='finished',finished_at=?,updated_at=? WHERE id=? AND status='active'", now, now, *c.ConferenceID).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE conference_participants SET status='left',left_at=?,microphone_enabled=false,camera_enabled=false,screen_sharing=false,media_policy_version=media_policy_version+1 WHERE conference_id=? AND status='joined'", now, *c.ConferenceID).Error; err != nil {
			return err
		}
	}
	if err := tx.Save(c).Error; err != nil {
		return err
	}
	if err := tx.Exec("DELETE FROM direct_call_slots WHERE call_id=?", c.ID).Error; err != nil {
		return err
	}
	return updateCallHistory(tx, *c, now)
}
func (r *CallRepository) Expiring(ctx context.Context) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Table("direct_calls").Where("(status='ringing' AND expires_at<=clock_timestamp()) OR (status='accepted' AND (caller_lease_until<=clock_timestamp() OR callee_lease_until<=clock_timestamp()))").Order("created_at,id").Limit(100).Pluck("id", &ids).Error
	return ids, err
}
