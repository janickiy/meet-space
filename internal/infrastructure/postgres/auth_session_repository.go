package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// AuthorizeLegacy preserves still-valid old credentials unless this browser logged out.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - hash: Previously computed digest used for identity or integrity lookup.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *AuthSessionRepository) AuthorizeLegacy(ctx context.Context, hash, userID string) error {
	var enabled bool
	if err := r.db.WithContext(ctx).Raw("SELECT EXISTS(SELECT 1 FROM users WHERE id=? AND password_hash<>'')", userID).Scan(&enabled).Error; err != nil {
		return err
	}
	if !enabled {
		return apperrors.ErrUnauthorized
	}
	var legacy authLegacyToken
	err := r.db.WithContext(ctx).Where("token_hash = ?", hash).Take(&legacy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if legacy.UserID != userID || legacy.RevokedAt != nil {
		return apperrors.ErrUnauthorized
	}
	if legacy.SessionID != nil {
		session, err := r.GetByID(ctx, *legacy.SessionID)
		if errors.Is(err, apperrors.ErrNotFound) || err == nil && session.RevokedAt != nil {
			return apperrors.ErrUnauthorized
		}
		return err
	}
	return nil
}

// AuthSessionRepository deliberately has no idle timeout or expiration cleanup.
type AuthSessionRepository struct{ db *gorm.DB }

type authSessionToken struct {
	TokenHash string `gorm:"primaryKey"`
	SessionID string
}

type authLegacyToken struct {
	TokenHash string `gorm:"primaryKey"`
	UserID    string
	SessionID *string
	RevokedAt *time.Time
}

// NewAuthSessionRepository constructs the auth session repository with the dependencies and
// limits required for auth session.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//
// Returns:
//   - result 1: The constructed or loaded auth session repository.
func NewAuthSessionRepository(db *gorm.DB) *AuthSessionRepository {
	return &AuthSessionRepository{db: db.Session(&gorm.Session{Logger: logger.Discard})}
}

// Create persists the new server session and refresh-secret digest without storing the raw
// refresh secret.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - session: Persistent or physical session state, as defined by its type.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *AuthSessionRepository) Create(ctx context.Context, session users.AuthSession) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&session).Error; err != nil {
			return err
		}
		return tx.Create(&authSessionToken{TokenHash: session.TokenHash, SessionID: session.ID}).Error
	})
}

// GetByHash loads the persistent server session by its refresh-secret digest.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - hash: Previously computed digest used for identity or integrity lookup.
//
// Returns:
//   - result 1: The resulting auth session.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *AuthSessionRepository) GetByHash(ctx context.Context, hash string) (users.AuthSession, error) {
	var session users.AuthSession
	err := r.db.WithContext(ctx).Model(&users.AuthSession{}).Select("auth_sessions.*").
		Joins("JOIN users u ON u.id=auth_sessions.user_id AND u.password_hash<>''").
		Joins("JOIN auth_session_tokens ON auth_session_tokens.session_id = auth_sessions.id").
		Where("auth_session_tokens.token_hash = ?", hash).Take(&session).Error
	return session, mapNotFound(err)
}

// GetByID loads the persistent server session by its session identifier.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting auth session.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *AuthSessionRepository) GetByID(ctx context.Context, id string) (users.AuthSession, error) {
	var session users.AuthSession
	err := r.db.WithContext(ctx).Model(&users.AuthSession{}).Select("auth_sessions.*").Joins("JOIN users u ON u.id=auth_sessions.user_id AND u.password_hash<>''").Where("auth_sessions.id = ?", id).Take(&session).Error
	return session, mapNotFound(err)
}

// RevokeByHash revokes the persistent session matching the refresh-secret digest.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - hash: Previously computed digest used for identity or integrity lookup.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *AuthSessionRepository) RevokeByHash(ctx context.Context, hash string) error {
	return r.db.WithContext(ctx).Exec(`UPDATE auth_sessions SET revoked_at = COALESCE(revoked_at, now())
  WHERE id IN (SELECT session_id FROM auth_session_tokens WHERE token_hash = ?)`, hash).Error
}

// RevokeByID revokes only the persistent session belonging to the specified account and session
// identifier.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *AuthSessionRepository) RevokeByID(ctx context.Context, id, userID string) error {
	return r.db.WithContext(ctx).Exec(`UPDATE auth_sessions SET revoked_at = COALESCE(revoked_at, now())
  WHERE id = ? AND user_id = ?`, id, userID).Error
}

// Bootstrap serializes one legacy JWT's concurrent upgrades and logout marker.
// Opaque secrets remain unrecoverable; concurrent tabs receive separate hashes for one SID.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - candidate: Candidate session or ICE record to validate before use.
//   - legacyHash: Digest of the legacy access token used for revocation checks.
//   - preferredID: Existing account-session identifier preferred when reusing a browser sign-in.
//
// Returns:
//   - result 1: The resulting auth session.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *AuthSessionRepository) Bootstrap(ctx context.Context, candidate users.AuthSession, legacyHash, preferredID string) (users.AuthSession, error) {
	var session users.AuthSession
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var legacy authLegacyToken
		if legacyHash != "" {
			legacy = authLegacyToken{TokenHash: legacyHash, UserID: candidate.UserID}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&legacy).Error; err != nil {
				return err
			}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", legacyHash).Take(&legacy).Error; err != nil {
				return err
			}
			if legacy.UserID != candidate.UserID || legacy.RevokedAt != nil {
				return apperrors.ErrUnauthorized
			}
			if legacy.SessionID != nil {
				preferredID = *legacy.SessionID
			}
		}
		if preferredID != "" {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", preferredID).Take(&session).Error; err != nil {
				return mapNotFound(err)
			}
			if session.UserID != candidate.UserID || session.RevokedAt != nil {
				return apperrors.ErrUnauthorized
			}
		} else {
			session = candidate
			if err := tx.Create(&session).Error; err != nil {
				return err
			}
		}
		if legacyHash != "" && legacy.SessionID == nil {
			if err := tx.Model(&authLegacyToken{}).Where("token_hash = ?", legacyHash).Update("session_id", session.ID).Error; err != nil {
				return err
			}
		}
		return tx.Create(&authSessionToken{TokenHash: candidate.TokenHash, SessionID: session.ID}).Error
	})
	return session, err
}

// RevokeLegacy creates a durable marker even if a concurrent upgrade has not run yet.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - hash: Previously computed digest used for identity or integrity lookup.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *AuthSessionRepository) RevokeLegacy(ctx context.Context, hash, userID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		legacy := authLegacyToken{TokenHash: hash, UserID: userID, RevokedAt: &now}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&legacy).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", hash).Take(&legacy).Error; err != nil {
			return err
		}
		if legacy.UserID != userID {
			return apperrors.ErrUnauthorized
		}
		if err := tx.Model(&authLegacyToken{}).Where("token_hash = ?", hash).Update("revoked_at", now).Error; err != nil {
			return err
		}
		if legacy.SessionID != nil {
			return tx.Exec("UPDATE auth_sessions SET revoked_at = COALESCE(revoked_at, ?) WHERE id = ? AND user_id = ?", now, *legacy.SessionID, userID).Error
		}
		return nil
	})
}
