package auth

import (
	"context"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
)

func (s *Service) SessionAccount(ctx context.Context, raw string) (users.View, error) {
	session, err := s.sessionForCookie(ctx, raw)
	if err != nil {
		return users.View{}, err
	}
	user, err := s.account(ctx, session.UserID)
	return user.View(), err
}

// Deletion always reauthenticates the current identity; a submitted user ID is never accepted.
func (s *Service) DeleteAccount(ctx context.Context, id, password string) error {
	user, err := s.account(ctx, id)
	if err != nil {
		return err
	}
	if password == "" || !utf8.ValidString(password) || utf8.RuneCountInString(password) > users.MaxPasswordCharacters {
		return apperrors.New(apperrors.ErrInvalidInput, "incorrect account password")
	}
	valid, err := s.passwords.Verify(password, user.PasswordHash)
	if err != nil {
		return err
	}
	if !valid {
		return apperrors.New(apperrors.ErrInvalidInput, "incorrect account password")
	}
	repo, ok := s.repository.(interface {
		DeleteAccount(context.Context, string, string) error
	})
	if !ok {
		return apperrors.ErrUnavailable
	}
	return repo.DeleteAccount(ctx, id, user.PasswordHash)
}
