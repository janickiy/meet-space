package auth

import (
	"context"
	"errors"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
)

// PasswordRecovery checks account eligibility without issuing reset credentials.
// Password delivery is not configured yet; blocked accounts still get an explicit
// status instead of being offered a reset that cannot restore their access.
func (s *Service) PasswordRecovery(ctx context.Context, email string) error {
	email = users.NormalizeEmail(email)
	if err := users.ValidateEmail(email); err != nil {
		return err
	}
	user, err := s.repository.GetByEmail(ctx, email)
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		return err
	}
	if err == nil && user.IsBlocked {
		return apperrors.ErrAccountBlocked
	}
	return apperrors.New(apperrors.ErrUnavailable, "password recovery unavailable")
}
