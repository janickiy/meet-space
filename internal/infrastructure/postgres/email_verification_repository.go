package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"github.com/janickiy/meet-space/internal/domain/users"
	authusecase "github.com/janickiy/meet-space/internal/usecase/auth"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type EmailVerificationRepository struct{ db *gorm.DB }

func NewEmailVerificationRepository(db *gorm.DB) *EmailVerificationRepository {
	return &EmailVerificationRepository{db: db.Session(&gorm.Session{Logger: logger.Discard})}
}
func queueVerification(tx *gorm.DB, v authusecase.Verification) error {
	if err := tx.Exec(`INSERT INTO email_verifications(id,user_id,token_hash,token_ciphertext,expires_at) VALUES(?::uuid,?::uuid,?,?,?)
 ON CONFLICT(user_id) DO UPDATE SET id=excluded.id,token_hash=excluded.token_hash,token_ciphertext=excluded.token_ciphertext,created_at=clock_timestamp(),expires_at=excluded.expires_at,consumed_at=NULL`, v.ID, v.UserID, v.TokenHash, v.TokenCiphertext, v.ExpiresAt).Error; err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,user_id,dedup_key) VALUES(?,?::uuid,'00000000-0000-0000-0000-000000000000',?::uuid,?)`, authusecase.VerificationJobKind, v.ID, v.UserID, "verify-email:"+v.ID).Error
}
func (r *EmailVerificationRepository) CreatePending(ctx context.Context, user users.User, v authusecase.Verification) (users.User, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		return queueVerification(tx, v)
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key" {
		return users.User{}, apperrors.New(apperrors.ErrConflict, "email is already registered")
	}
	return user, err
}
func (r *EmailVerificationRepository) Resend(ctx context.Context, user users.User, v authusecase.Verification) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var eligible bool
		result := tx.Raw(`SELECT email_verification_required AND email_verified_at IS NULL AND NOT is_blocked AS eligible FROM users WHERE id=?::uuid AND deleted_at IS NULL AND auth_source='local' AND guest_conference_id IS NULL FOR UPDATE`, user.ID).Scan(&eligible)
		if result.Error != nil {
			return result.Error
		}
		if !eligible {
			return nil
		}
		var recent bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM email_verifications WHERE user_id=?::uuid AND created_at>clock_timestamp()-INTERVAL '1 minute')`, user.ID).Scan(&recent).Error; err != nil {
			return err
		}
		if recent {
			return nil
		}
		return queueVerification(tx, v)
	})
}
func (r *EmailVerificationRepository) Consume(ctx context.Context, hash string) error {
	invalid := apperrors.New(apperrors.ErrInvalidInput, "invalid or expired verification link")
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Match resend's users -> verification lock order.
		var userID string
		found := tx.Raw(`SELECT user_id FROM email_verifications WHERE token_hash=?`, hash).Scan(&userID)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected == 0 {
			return invalid
		}
		var eligible bool
		found = tx.Raw(`SELECT NOT is_blocked AND deleted_at IS NULL AND auth_source='local' AND guest_conference_id IS NULL FROM users WHERE id=?::uuid FOR UPDATE`, userID).Scan(&eligible)
		if found.Error != nil {
			return found.Error
		}
		if !eligible {
			return invalid
		}
		var row struct {
			UserID     string
			ExpiresAt  time.Time
			ConsumedAt *time.Time
		}
		result := tx.Raw(`SELECT user_id,expires_at,consumed_at FROM email_verifications WHERE token_hash=? FOR UPDATE`, hash).Scan(&row)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 || row.ConsumedAt != nil {
			return invalid
		}
		result = tx.Exec(`UPDATE email_verifications SET consumed_at=clock_timestamp(),token_ciphertext='' WHERE token_hash=? AND consumed_at IS NULL AND expires_at>clock_timestamp()`, hash)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return invalid
		}
		result = tx.Exec(`UPDATE users SET email_verified_at=clock_timestamp() WHERE id=?::uuid AND deleted_at IS NULL AND auth_source='local' AND guest_conference_id IS NULL AND NOT is_blocked`, row.UserID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return invalid
		}
		return nil
	})
}
func (r *EmailVerificationRepository) Delivery(ctx context.Context, job jobs.Job) (authusecase.VerificationDelivery, error) {
	var value authusecase.VerificationDelivery
	result := r.db.WithContext(ctx).Raw(`SELECT v.id,v.user_id,v.token_hash,v.token_ciphertext,v.expires_at,u.email FROM email_verifications v JOIN users u ON u.id=v.user_id
 JOIN background_jobs j ON j.entity_id=v.id AND j.user_id=u.id
 WHERE v.id=?::uuid AND j.id=?::uuid AND j.state='processing' AND j.lease_token=?::uuid AND j.lease_until>clock_timestamp()
 AND v.consumed_at IS NULL AND v.expires_at>clock_timestamp() AND u.email_verified_at IS NULL AND u.email_verification_required AND NOT u.is_blocked`, job.EntityID, job.ID, job.LeaseToken).Scan(&value)
	if result.Error != nil {
		return value, result.Error
	}
	if result.RowsAffected != 1 {
		return value, apperrors.ErrNotFound
	}
	return value, nil
}
