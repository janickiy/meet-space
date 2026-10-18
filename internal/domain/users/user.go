package users

import (
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// User persists account identity and a password hash; public views are built separately.
type User struct {
	ID                string    `gorm:"type:uuid;primaryKey" json:"-"`
	Email             string    `json:"-"`
	PasswordHash      string    `gorm:"column:password_hash" json:"-"`
	DisplayName       *string   `gorm:"column:display_name" json:"-"`
	IsAdmin           bool      `gorm:"column:is_admin" json:"-"`
	GuestConferenceID *string   `gorm:"column:guest_conference_id" json:"-"`
	CreatedAt         time.Time `json:"-"`
	UpdatedAt         time.Time `json:"-"`
}

const (
	MinPasswordCharacters = 8
	MaxPasswordCharacters = 128
)

// TableName returns the explicit database table name so GORM does not infer a different plural
// form.
//
// Returns:
//   - result 1: Exact database table name used by GORM.
func (User) TableName() string { return "users" }

// View is the safe public API projection of an account.
type View struct {
	ID                string    `json:"id"`
	Email             string    `json:"email"`
	DisplayName       *string   `json:"displayName"`
	IsAdmin           bool      `json:"isAdmin"`
	GuestConferenceID *string   `json:"guestConferenceId,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// View builds the public API representation without exposing private persistence fields.
//
// Returns:
//   - result 1: The resulting view.
func (u User) View() View {
	email := u.Email
	if u.GuestConferenceID != nil {
		email = ""
	}
	return View{ID: u.ID, Email: email, DisplayName: u.DisplayName, IsAdmin: u.IsAdmin, GuestConferenceID: u.GuestConferenceID, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt}
}

// ParticipantName chooses the public participant name used in conference membership views.
//
// Returns:
//   - result 1: Safe public participant display name.
func (u User) ParticipantName() string {
	if u.DisplayName != nil && *u.DisplayName != "" {
		return *u.DisplayName
	}
	return "Participant"
}

// RegisterRequest contains an email address, password and optional public registration name.
type RegisterRequest struct {
	Email       string  `json:"email"`
	Password    string  `json:"password"`
	DisplayName *string `json:"displayName"`
}

// UpdateProfileRequest permits changes only to the current account's display name.
type UpdateProfileRequest struct {
	DisplayName string `json:"displayName"`
}

// NormalizeDisplayName trims and validates a display name before it is saved.
//
// Parameters:
//   - value: Value to validate, normalize or convert.
//
// Returns:
//   - result 1: Validated public display name.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NormalizeDisplayName(value string) (string, error) {
	name := strings.TrimSpace(value)
	length := utf8.RuneCountInString(name)
	if !utf8.ValidString(name) || length < 1 || length > 100 {
		return "", apperrors.New(apperrors.ErrInvalidInput, "displayName must contain 1 to 100 characters")
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return "", apperrors.New(apperrors.ErrInvalidInput, "displayName must not contain control characters")
		}
	}
	return name, nil
}

// LoginRequest contains the supplied sign-in credentials.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse contains the sign-in result, access token and safe public account information.
type LoginResponse struct {
	Status      string `json:"status"`
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
	ExpiresIn   int    `json:"expiresIn"`
	User        View   `json:"user"`
	// SessionToken is delivered only as an HttpOnly cookie, never in JSON.
	SessionToken string `json:"-"`
}

// NormalizeEmail normalizes an email address for consistent account lookup.
//
// Parameters:
//   - value: Value to validate, normalize or convert.
//
// Returns:
//   - result 1: Normalized email address used for account lookup.
func NormalizeEmail(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

// ValidateEmail checks whether an email address meets the account registration rules.
//
// Parameters:
//   - value: Value to validate, normalize or convert.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func ValidateEmail(value string) error {
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value || len(value) > 254 || !strings.Contains(value, "@") {
		return apperrors.New(apperrors.ErrInvalidInput, "email must be a valid address, up to 254 bytes")
	}
	return nil
}

// NormalizeRegister normalizes registration fields and rejects invalid credentials before
// account creation.
//
// Parameters:
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting register request.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NormalizeRegister(request RegisterRequest) (RegisterRequest, error) {
	request.Email = NormalizeEmail(request.Email)
	if err := ValidateEmail(request.Email); err != nil {
		return request, err
	}
	length := utf8.RuneCountInString(request.Password)
	if !utf8.ValidString(request.Password) || length < MinPasswordCharacters || length > MaxPasswordCharacters {
		return request, apperrors.New(apperrors.ErrInvalidInput, "password must contain 8 to 128 characters")
	}
	if request.DisplayName != nil {
		name := strings.TrimSpace(*request.DisplayName)
		if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 100 {
			return request, apperrors.New(apperrors.ErrInvalidInput, "displayName must be at most 100 characters")
		}
		request.DisplayName = nil
		if name != "" {
			request.DisplayName = &name
		}
	}
	return request, nil
}
