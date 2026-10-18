package auth

import (
	"context"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
)

// SessionAccount returns the public account view for a valid persistent session.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - raw: Serialized access token or refresh credential to validate; never log or expose it.
//
// Returns:
//   - result 1: The resulting view.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) SessionAccount(ctx context.Context, raw string) (users.View, error) {
	session, err := s.sessionForCookie(ctx, raw)
	if err != nil {
		return users.View{}, err
	}
	user, err := s.account(ctx, session.UserID)
	return user.View(), err
}

// DeleteAccount verifies account ownership and removes the account together with its scoped
// session state.
//
// Deletion always reauthenticates the current identity; a submitted user ID is never accepted.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//   - password: Plaintext password accepted only for validation or hashing.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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
		// DeleteAccount verifies account ownership and removes the account together with its scoped
		// session state.
		//
		// Parameters:
		//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
		//   - argument 2: Identifier of the resource addressed by this operation.
		//   - argument 3: Plaintext password accepted only for validation or hashing.
		//
		// Returns:
		//   - result 1: nil on success; otherwise the validation, access, transport or persistence
		//     failure.
		DeleteAccount(context.Context, string, string) error
	})
	if !ok {
		return apperrors.ErrUnavailable
	}
	return repo.DeleteAccount(ctx, id, user.PasswordHash)
}
