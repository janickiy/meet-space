package auth

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
)

// userRepository defines the replaceable dependency contract for account authentication and
// persistent sessions.
type userRepository interface {
	// Create persists the validated account identity and protected password hash.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//
	// Returns:
	//   - result 1: The resulting user.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Create(context.Context, users.User) (users.User, error)
	// GetByID loads the account by ID.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting user.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	GetByID(context.Context, string) (users.User, error)
	// GetByEmail loads the account by email.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Account or delivery email address.
	//
	// Returns:
	//   - result 1: The resulting user.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	GetByEmail(context.Context, string) (users.User, error)
	// UpdateDisplayName updates only the public name of the account resolved from the authenticated
	// subject.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Name of the setting, cookie, resource or provider being addressed.
	//
	// Returns:
	//   - result 1: The resulting user.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	UpdateDisplayName(context.Context, string, string) (users.User, error)
}

// passwordHasher defines the replaceable dependency contract for account authentication and
// persistent sessions.
type passwordHasher interface {
	// Hash hashes the content to obtain a stable integrity or cache key.
	//
	// Parameters:
	//   - argument 1: UTF-8 text to validate, normalize or process.
	//
	// Returns:
	//   - result 1: Protected password hash or content digest, according to this component.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Hash(string) (string, error)
	// Verify compares a plaintext password with the stored encoded hash using the bounded supported
	// hash parameters.
	//
	// Parameters:
	//   - argument 1: Plaintext password accepted only for validation or hashing.
	//   - argument 2: Stored encoded password hash or token value.
	//
	// Returns:
	//   - result 1: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Verify(string, string) (bool, error)
}

// tokenIssuer defines the replaceable dependency contract for account authentication and
// persistent sessions.
type tokenIssuer interface {
	// Issue creates a signed token bound to the supplied identity and expiry.
	//
	// Parameters:
	//   - argument 1: Acting account identifier used to scope access.
	//
	// Returns:
	//   - result 1: Signed short-lived credential for the verified identity.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Issue(string) (string, error)
}

// Service coordinates account authentication and persistent sessions through its configured
// persistence and transport dependencies.
type Service struct {
	repository userRepository
	passwords  passwordHasher
	tokens     tokenIssuer
	sessions   SessionRepository
	dummyHash  string
}

// NewService constructs the service with the dependencies and limits required for account
// authentication and persistent sessions.
//
// Parameters:
//   - repository: Persistence dependency used by the application operation.
//   - passwords: Password hashing and verification dependency.
//   - tokens: Token issuer or verifier used for trusted identity checks.
//
// Returns:
//   - result 1: The constructed or loaded service.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NewService(repository userRepository, passwords passwordHasher, tokens tokenIssuer) (*Service, error) {
	dummy, err := passwords.Hash("dummy-password-for-login-timing")
	if err != nil {
		return nil, fmt.Errorf("initialize authentication: %w", err)
	}
	return &Service{repository: repository, passwords: passwords, tokens: tokens, dummyHash: dummy}, nil
}

// Register validates registration fields, hashes the password and creates a new account without
// returning private credential data.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting view.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Register(ctx context.Context, request users.RegisterRequest) (users.View, error) {
	request, err := users.NormalizeRegister(request)
	if err != nil {
		return users.View{}, err
	}
	hash, err := s.passwords.Hash(request.Password)
	if err != nil {
		return users.View{}, err
	}
	if err := ctx.Err(); err != nil {
		return users.View{}, err
	}
	user, err := s.repository.Create(ctx, users.User{ID: uuid.NewString(), Email: request.Email, PasswordHash: hash, DisplayName: request.DisplayName})
	if err != nil {
		return users.View{}, err
	}
	return user.View(), nil
}

// Login checks the supplied credentials and returns the safe account view and access token.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting login response.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Login(ctx context.Context, request users.LoginRequest) (users.LoginResponse, error) {
	request.Email = users.NormalizeEmail(request.Email)
	// Legacy accounts may have passwords shorter than eight characters because the old rule counted
	// bytes. Apply the new minimum only during registration; do not reject an otherwise valid legacy
	// password.
	if users.ValidateEmail(request.Email) != nil || request.Password == "" || !utf8.ValidString(request.Password) || utf8.RuneCountInString(request.Password) > users.MaxPasswordCharacters {
		return users.LoginResponse{}, invalidCredentials()
	}
	user, err := s.repository.GetByEmail(ctx, request.Email)
	missing := errors.Is(err, apperrors.ErrNotFound)
	if err != nil && !missing {
		return users.LoginResponse{}, err
	}
	hash := user.PasswordHash
	if missing || user.GuestConferenceID != nil {
		hash = s.dummyHash
	}
	valid, err := s.passwords.Verify(request.Password, hash)
	if err != nil {
		return users.LoginResponse{}, err
	}
	if missing || user.GuestConferenceID != nil || !valid {
		return users.LoginResponse{}, invalidCredentials()
	}
	if err := ctx.Err(); err != nil {
		return users.LoginResponse{}, err
	}
	return s.signIn(ctx, user)
}

// Me returns the public view of the currently authenticated account.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: The resulting view.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Me(ctx context.Context, userID string) (users.View, error) {
	user, err := s.repository.GetByID(ctx, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return users.View{}, apperrors.ErrUnauthorized
	}
	if err != nil {
		return users.View{}, err
	}
	return user.View(), nil
}

// UpdateProfile updates only the authenticated account's validated public profile fields.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting view.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) UpdateProfile(ctx context.Context, userID string, request users.UpdateProfileRequest) (users.View, error) {
	name, err := users.NormalizeDisplayName(request.DisplayName)
	if err != nil {
		return users.View{}, err
	}
	user, err := s.repository.UpdateDisplayName(ctx, userID, name)
	if errors.Is(err, apperrors.ErrNotFound) {
		return users.View{}, apperrors.ErrUnauthorized
	}
	if err != nil {
		return users.View{}, err
	}
	return user.View(), nil
}

// invalidCredentials returns the same safe authentication error for invalid account credentials.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func invalidCredentials() error {
	return apperrors.New(apperrors.ErrUnauthorized, "invalid email or password")
}
