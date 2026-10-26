package auth

import (
	"context"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// Argon2 uses 64 MiB per derivation. The API owns one auth Service, so this
// shared budget bounds active password work to 128 MiB without weakening the
// password parameters. Per-IP request limits alone do not bound concurrency.
const maxPasswordWork = 2

// acquirePasswordWork reserves one password-work slot for ctx. Saturated
// requests fail immediately instead of accumulating goroutines and passwords.
func (s *Service) acquirePasswordWork(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case s.passwordWork <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-s.passwordWork
			return err
		}
		return nil
	default:
		return apperrors.ErrRateLimited
	}
}

// hashPassword derives a hash for password only while ctx is active and a
// shared work slot is available. The slot is released on every return path.
func (s *Service) hashPassword(ctx context.Context, password string) (string, error) {
	if err := s.acquirePasswordWork(ctx); err != nil {
		return "", err
	}
	defer func() { <-s.passwordWork }()
	return s.passwords.Hash(password)
}

// verifyPassword checks password against encoded without starting work for an
// already-cancelled ctx. A running Argon2 derivation cannot be interrupted.
func (s *Service) verifyPassword(ctx context.Context, password, encoded string) (bool, error) {
	if err := s.acquirePasswordWork(ctx); err != nil {
		return false, err
	}
	defer func() { <-s.passwordWork }()
	return s.passwords.Verify(password, encoded)
}
