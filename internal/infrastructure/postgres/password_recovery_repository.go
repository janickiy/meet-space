package postgres

import (
	"context"
	"errors"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"github.com/janickiy/meet-space/internal/domain/users"
	authusecase "github.com/janickiy/meet-space/internal/usecase/auth"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type PasswordRecoveryRepository struct{ db *gorm.DB }

func NewPasswordRecoveryRepository(db *gorm.DB) *PasswordRecoveryRepository {
	return &PasswordRecoveryRepository{db: db.Session(&gorm.Session{Logger: logger.Discard})}
}
func (r *PasswordRecoveryRepository) Request(ctx context.Context, user users.User, value authusecase.PasswordRecovery) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireLocalAuthentication(tx); err != nil {
			return err
		}
		var current users.User
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?::uuid", user.ID).Take(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if current.AccessBlocked() {
			return apperrors.ErrAccountBlocked
		}
		if current.IsLDAP() || current.DeletedAt != nil || current.PasswordHash == "" || current.GuestConferenceID != nil || current.Email != user.Email {
			return nil
		}
		var recent bool
		if err = tx.Raw(`SELECT EXISTS(SELECT 1 FROM password_recoveries WHERE user_id=?::uuid AND created_at>clock_timestamp()-INTERVAL '1 minute')`, user.ID).Scan(&recent).Error; err != nil {
			return err
		}
		if recent {
			return nil
		}
		if err = tx.Exec(`INSERT INTO password_recoveries(id,user_id,token_hash,token_ciphertext,expires_at) VALUES(?::uuid,?::uuid,?,?,?) ON CONFLICT(user_id) DO UPDATE SET id=excluded.id,token_hash=excluded.token_hash,token_ciphertext=excluded.token_ciphertext,created_at=clock_timestamp(),expires_at=excluded.expires_at,consumed_at=NULL`, value.ID, user.ID, value.TokenHash, value.TokenCiphertext, value.ExpiresAt).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,user_id,dedup_key) VALUES(?,?::uuid,'00000000-0000-0000-0000-000000000000',?::uuid,?)`, authusecase.PasswordRecoveryJobKind, value.ID, user.ID, "password-recovery:"+value.ID).Error
	})
}
func (r *PasswordRecoveryRepository) Consume(ctx context.Context, hash, passwordHash string) (string, error) {
	invalid := apperrors.New(apperrors.ErrInvalidInput, "invalid or expired password reset link")
	var userID string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireLocalAuthentication(tx); err != nil {
			return err
		}
		// Match request/session revocation lock order: user, then recovery/session rows.
		found := tx.Raw(`SELECT user_id FROM password_recoveries WHERE token_hash=?`, hash).Scan(&userID)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected != 1 {
			return invalid
		}
		var user users.User
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?::uuid", userID).Take(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return invalid
		}
		if err != nil {
			return err
		}
		var resetID string
		found = tx.Raw(`SELECT id FROM password_recoveries WHERE token_hash=? AND consumed_at IS NULL AND expires_at>clock_timestamp() FOR UPDATE`, hash).Scan(&resetID)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected != 1 {
			return invalid
		}
		if user.AccessBlocked() {
			return apperrors.ErrAccountBlocked
		}
		if user.IsLDAP() || user.DeletedAt != nil || user.PasswordHash == "" || user.GuestConferenceID != nil {
			return invalid
		}
		if err = tx.Exec(`UPDATE password_recoveries SET consumed_at=clock_timestamp(),token_ciphertext='' WHERE id=?::uuid`, resetID).Error; err != nil {
			return err
		}
		if err = tx.Exec(`UPDATE users SET password_hash=?,auth_invalid_before=clock_timestamp(),updated_at=clock_timestamp() WHERE id=?::uuid`, passwordHash, userID).Error; err != nil {
			return err
		}
		if err = tx.Exec(`UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE user_id=?::uuid`, userID).Error; err != nil {
			return err
		}
		if err = tx.Exec(`UPDATE auth_legacy_tokens SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE user_id=?::uuid`, userID).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,user_id,dedup_key) VALUES(?,?::uuid,'00000000-0000-0000-0000-000000000000',?::uuid,?)`, authusecase.PasswordChangedJobKind, resetID, userID, "password-changed:"+resetID).Error
	})
	if err != nil {
		return "", err
	}
	return userID, nil
}
func (r *PasswordRecoveryRepository) Delivery(ctx context.Context, job jobs.Job) (authusecase.PasswordRecoveryDelivery, error) {
	var value authusecase.PasswordRecoveryDelivery
	result := r.db.WithContext(ctx).Raw(`SELECT v.id,v.user_id,v.token_hash,v.token_ciphertext,v.expires_at,u.email FROM password_recoveries v JOIN users u ON u.id=v.user_id JOIN background_jobs j ON j.entity_id=v.id AND j.user_id=u.id WHERE v.id=?::uuid AND j.id=?::uuid AND j.kind=? AND j.state='processing' AND j.lease_token=?::uuid AND j.lease_until>clock_timestamp() AND v.consumed_at IS NULL AND v.expires_at>clock_timestamp() AND u.deleted_at IS NULL AND u.auth_source='local' AND u.guest_conference_id IS NULL AND NOT u.is_blocked AND `+localAuthenticationModeSQL, job.EntityID, job.ID, authusecase.PasswordRecoveryJobKind, job.LeaseToken).Scan(&value)
	if result.Error != nil {
		return value, result.Error
	}
	if result.RowsAffected != 1 {
		return value, apperrors.ErrNotFound
	}
	return value, nil
}
func (r *PasswordRecoveryRepository) ChangedDelivery(ctx context.Context, job jobs.Job) (string, error) {
	var email string
	result := r.db.WithContext(ctx).Raw(`SELECT u.email FROM users u JOIN background_jobs j ON j.user_id=u.id WHERE j.id=?::uuid AND j.entity_id=?::uuid AND j.kind=? AND j.state='processing' AND j.lease_token=?::uuid AND j.lease_until>clock_timestamp() AND u.deleted_at IS NULL AND u.auth_source='local' AND u.guest_conference_id IS NULL`, job.ID, job.EntityID, authusecase.PasswordChangedJobKind, job.LeaseToken).Scan(&email)
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected != 1 {
		return "", apperrors.ErrNotFound
	}
	return email, nil
}
