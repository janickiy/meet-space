package auth

import (
	"context"
	"errors"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// AuthorizeLegacyAccount also revokes pre-session JWTs and WebSocket tickets.
// Once blocked, an account must sign in again using a durable session; unblocking
// must never resurrect a bearer token that predates the block.
func (s *Service) AuthorizeLegacyAccount(ctx context.Context, userID string) error {
	user, err := s.repository.GetByID(ctx, userID)
	if err == nil && user.AccessBlocked() {
		return apperrors.ErrAccountBlocked
	}
	if errors.Is(err, apperrors.ErrNotFound) || err == nil && user.AuthInvalidBefore != nil {
		return apperrors.ErrUnauthorized
	}
	// Корпоративный вход всегда выдаёт SID; устаревший JWT не должен обходить отключение LDAP.
	if err == nil && user.IsLDAP() {
		return apperrors.ErrUnauthorized
	}
	if err == nil {
		if err := s.requireAccountMode(ctx, user); err != nil {
			return err
		}
		return s.enforceEmailVerification(ctx, user)
	}
	return err
}
