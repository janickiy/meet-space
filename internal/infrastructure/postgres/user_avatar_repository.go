package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/avatar"
	"github.com/janickiy/meet-space/internal/domain/users"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SaveAvatar atomically replaces actor's thumbnail and public version.
// Nil data deletes the photo; the account row lock serializes deletion/replacement.
func (r *UserRepository) SaveAvatar(ctx context.Context, actor string, data []byte) (users.User, error) {
	if len(data) > avatar.MaxStoredBytes {
		return users.User{}, apperrors.ErrInvalidInput
	}
	var user users.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND password_hash<>'' AND guest_conference_id IS NULL", actor).Take(&user).Error; err != nil {
			return mapNotFound(err)
		}
		var version *string
		if len(data) > 0 {
			id := uuid.NewString()
			version = &id
			if err := tx.Exec(`INSERT INTO user_avatars(user_id,version,data) VALUES(?,?,?)
			 ON CONFLICT(user_id) DO UPDATE SET version=EXCLUDED.version,data=EXCLUDED.data,updated_at=clock_timestamp()`, actor, id, data).Error; err != nil {
				return err
			}
		} else if err := tx.Exec("DELETE FROM user_avatars WHERE user_id=?", actor).Error; err != nil {
			return err
		}
		if err := tx.Model(&users.User{}).Where("id=?", actor).Updates(map[string]any{"avatar_version": version}).Error; err != nil {
			return err
		}
		user.AvatarVersion = version
		return tx.Where("id=?", actor).Take(&user).Error
	})
	return user, err
}

// ReadAvatar returns only the target's bounded thumbnail, never credentials.
// Both actor and target must still be registered, non-deleted accounts.
func (r *UserRepository) ReadAvatar(ctx context.Context, actor, target string) ([]byte, error) {
	var row struct{ Data []byte }
	result := r.db.WithContext(ctx).Raw(`SELECT a.data FROM user_avatars a JOIN users u ON u.id=a.user_id
	 WHERE u.id=? AND u.password_hash<>'' AND u.guest_conference_id IS NULL
	 AND EXISTS(SELECT 1 FROM users actor WHERE actor.id=? AND actor.password_hash<>'' AND actor.guest_conference_id IS NULL)`, target, actor).Scan(&row)
	if result.Error != nil {
		return nil, result.Error
	}
	if len(row.Data) == 0 {
		return nil, apperrors.ErrNotFound
	}
	return row.Data, nil
}
