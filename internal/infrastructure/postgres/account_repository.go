package postgres

import (
	"context"
	"errors"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DeleteAccount erases credentials and personal configuration atomically. Shared history
// retains an anonymous reference so another participant's messages are not destroyed.
func (r *UserRepository) DeleteAccount(ctx context.Context, id, verifiedHash string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user users.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND password_hash<>'' AND guest_conference_id IS NULL", id).Take(&user).Error; err != nil {
			return mapNotFound(err)
		}
		if user.PasswordHash != verifiedHash {
			return apperrors.ErrConflict
		}
		var blocked bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM direct_calls WHERE (caller_user_id=? OR callee_user_id=?) AND status IN ('ringing','accepted'))
 OR EXISTS(SELECT 1 FROM participant_sessions s JOIN conference_participants p ON p.id=s.participant_id WHERE p.user_id=? AND s.status='connected')`, id, id, id).Scan(&blocked).Error; err != nil {
			return err
		}
		if blocked {
			return apperrors.New(apperrors.ErrConflict, "leave active calls before deleting account")
		}
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM conversations c JOIN conversation_members m ON m.conversation_id=c.id WHERE m.user_id=? AND m.role='owner' AND m.left_at IS NULL AND c.deleted_at IS NULL)
 OR EXISTS(SELECT 1 FROM conferences WHERE owner_id=? AND status IN ('created','scheduled','active'))`, id, id).Scan(&blocked).Error; err != nil {
			return err
		}
		if blocked {
			return apperrors.New(apperrors.ErrConflict, "transfer group ownership and close meetings before deleting account")
		}
		for _, table := range []string{"auth_legacy_tokens", "auth_sessions"} {
			if err := tx.Table(table).Where("user_id=?", id).Update("revoked_at", gorm.Expr("COALESCE(revoked_at,clock_timestamp())")).Error; err != nil {
				return err
			}
		}
		// Removing calendar mappings first preserves their FK and deletes encrypted credentials.
		if err := tx.Exec(`DELETE FROM calendar_event_mappings WHERE connection_id IN (SELECT id FROM calendar_connections WHERE user_id=?)`, id).Error; err != nil {
			return err
		}
		for _, table := range []string{"calendar_oauth_states", "calendar_connections", "push_devices", "notification_preferences", "integration_deliveries", "notifications", "notification_jobs", "folders"} {
			if err := tx.Exec("DELETE FROM "+table+" WHERE user_id=?", id).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec("DELETE FROM background_jobs WHERE user_id=?", id).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE conference_invitations SET email='deleted.'||id::text||'@invalid',status='skipped',error_code='account_deleted' WHERE user_id=? OR email=?`, id, user.Email).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE conference_participants SET display_name='Удалённый аккаунт',status='left' WHERE user_id=?`, id).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE conversation_members SET left_at=clock_timestamp(),notifications_enabled=false WHERE user_id=? AND role<>'owner' AND conversation_id IN (SELECT id FROM conversations WHERE type='group')`, id).Error; err != nil {
			return err
		}
		// Empty accounts are physically removed. Accounts referenced by shared history
		// become anonymous tombstones; no email, password, name or login remains.
		if err := tx.SavePoint("account_erasure").Error; err != nil {
			return err
		}
		err := tx.Exec("DELETE FROM users WHERE id=?", id).Error
		if err == nil {
			return nil
		}
		var state interface{ SQLState() string }
		if !errors.As(err, &state) || state.SQLState() != "23503" {
			return err
		}
		if err := tx.RollbackTo("account_erasure").Error; err != nil {
			return err
		}
		return tx.Model(&users.User{}).Where("id=?", id).Updates(map[string]any{"email": "deleted." + id + "@invalid", "password_hash": "", "display_name": "Удалённый аккаунт", "is_admin": false}).Error
	})
}
