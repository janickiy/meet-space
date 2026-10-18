package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
)

type SessionRepository interface {
	// Create persists the new server session and refresh-secret digest without storing the raw
	// refresh secret.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Persistent or physical session state, as defined by its type.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Create(context.Context, users.AuthSession) error
	// GetByHash loads the persistent server session by its refresh-secret digest.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Previously computed digest used for identity or integrity lookup.
	//
	// Returns:
	//   - result 1: The resulting auth session.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	GetByHash(context.Context, string) (users.AuthSession, error)
	// GetByID loads the persistent server session by its session identifier.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting auth session.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	GetByID(context.Context, string) (users.AuthSession, error)
	// RevokeByHash revokes the persistent session matching the refresh-secret digest.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Previously computed digest used for identity or integrity lookup.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	RevokeByHash(context.Context, string) error
	// RevokeByID revokes only the persistent session belonging to the specified account and session
	// identifier.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Acting account identifier used to scope access.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	RevokeByID(context.Context, string, string) error
	// Bootstrap atomically creates or reuses the candidate persistent session while checking
	// legacy-token revocation and the preferred browser session.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Candidate session or ICE record to validate before use.
	//   - argument 3: Digest of the legacy access token used for revocation checks.
	//   - argument 4: Existing account-session identifier preferred when reusing a browser sign-in.
	//
	// Returns:
	//   - result 1: The resulting auth session.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Bootstrap(context.Context, users.AuthSession, string, string) (users.AuthSession, error)
	// RevokeLegacy records revocation of a legacy access token so it cannot bootstrap a new session.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Previously computed digest used for identity or integrity lookup.
	//   - argument 3: Acting account identifier used to scope access.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	RevokeLegacy(context.Context, string, string) error
	// AuthorizeLegacy rejects legacy access credentials that have been explicitly revoked.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Previously computed digest used for identity or integrity lookup.
	//   - argument 3: Acting account identifier used to scope access.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	AuthorizeLegacy(context.Context, string, string) error
}

type sessionTokens interface {
	// IssueSession issues an access JWT bound to the specified persistent server session.
	//
	// Parameters:
	//   - argument 1: Acting account identifier used to scope access.
	//   - argument 2: Persistent or physical session identifier, as defined by the operation.
	//
	// Returns:
	//   - result 1: Signed access JWT bound to the persistent session.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	IssueSession(string, string) (string, error)
	// VerifyAuthorization verifies the authorization token and resolves its user, session, guest
	// scope and expiry.
	//
	// Parameters:
	//   - argument 1: Untrusted serialized value to decode or validate.
	//
	// Returns:
	//   - result 1: Verified account identifier.
	//   - result 2: Guest conference scope, or an empty string for a registered account.
	//   - result 3: Persistent server-session identifier, or an empty string for a guest or legacy
	//     token.
	//   - result 4: The timestamp or validity boundary associated with the result.
	//   - result 5: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	VerifyAuthorization(string) (string, string, string, time.Time, error)
}

// WithSessions enables durable sign-in and revocation checks for the API.
//
// Parameters:
//   - repository: Persistence dependency used by the application operation.
//
// Returns:
//   - result 1: The constructed or loaded service.
func (s *Service) WithSessions(repository SessionRepository) *Service {
	s.sessions = repository
	return s
}

// signIn creates a persistent account session when available and issues its short-lived access
// token.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Validated account record whose public identity or server session is being processed.
//
// Returns:
//   - result 1: The resulting login response.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) signIn(ctx context.Context, user users.User) (users.LoginResponse, error) {
	if s.sessions == nil {
		token, err := s.tokens.Issue(user.ID)
		return loginResponse(user, token, ""), err
	}
	if user.GuestConferenceID != nil {
		return users.LoginResponse{}, apperrors.ErrForbidden
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return users.LoginResponse{}, err
	}
	raw := base64.RawURLEncoding.EncodeToString(secret[:])
	session := users.AuthSession{ID: uuid.NewString(), UserID: user.ID, TokenHash: sessionHash(raw), CreatedAt: time.Now().UTC()}
	response, err := s.sessionResponse(user, session, raw)
	if err != nil {
		return users.LoginResponse{}, err
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return users.LoginResponse{}, err
	}
	return response, nil
}

// loginResponse builds the public login response and the internal refresh-secret value used to
// set the cookie.
//
// Parameters:
//   - user: Validated account record whose public identity or server session is being processed.
//   - token: Signed credential or operation-ownership token to verify.
//   - raw: Serialized access token or refresh credential to validate; never log or expose it.
//
// Returns:
//   - result 1: The resulting login response.
func loginResponse(user users.User, token, raw string) users.LoginResponse {
	return users.LoginResponse{Status: "success", AccessToken: token, TokenType: "Bearer", ExpiresIn: 3600, User: user.View(), SessionToken: raw}
}

// sessionResponse issues a short-lived access token bound to the supplied persistent session.
//
// Parameters:
//   - user: Validated account record whose public identity or server session is being processed.
//   - session: Persistent or physical session state, as defined by its type.
//   - raw: Serialized access token or refresh credential to validate; never log or expose it.
//
// Returns:
//   - result 1: The resulting login response.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) sessionResponse(user users.User, session users.AuthSession, raw string) (users.LoginResponse, error) {
	tokens, ok := s.tokens.(sessionTokens)
	if !ok {
		return users.LoginResponse{}, apperrors.ErrUnavailable
	}
	token, err := tokens.IssueSession(user.ID, session.ID)
	if err != nil {
		return users.LoginResponse{}, err
	}
	return loginResponse(user, token, raw), nil
}

// sessionHash hashes the raw refresh secret so only its SHA-256 digest is stored.
//
// Parameters:
//   - raw: Raw refresh secret read from the HttpOnly cookie.
//
// Returns:
//   - result 1: Hex-encoded SHA-256 of the raw refresh secret.
func sessionHash(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

// validSessionSecret checks that a refresh secret has the canonical 32-byte URL-safe encoding.
//
// Parameters:
//   - raw: Raw refresh secret read from the HttpOnly cookie.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func validSessionSecret(raw string) bool {
	if len(raw) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	return err == nil && len(decoded) == 32 && base64.RawURLEncoding.EncodeToString(decoded) == raw
}

// sessionForCookie resolves an unrevoked persistent session from a validated refresh cookie.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - raw: Raw refresh secret read from the HttpOnly cookie.
//
// Returns:
//   - result 1: The resulting auth session.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) sessionForCookie(ctx context.Context, raw string) (users.AuthSession, error) {
	if s.sessions == nil || !validSessionSecret(raw) {
		return users.AuthSession{}, apperrors.ErrUnauthorized
	}
	session, err := s.sessions.GetByHash(ctx, sessionHash(raw))
	if errors.Is(err, apperrors.ErrNotFound) || err == nil && session.RevokedAt != nil {
		return users.AuthSession{}, apperrors.ErrUnauthorized
	}
	return session, err
}

// account loads a registered account and rejects missing or guest identities for
// persistent-session operations.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting user.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) account(ctx context.Context, id string) (users.User, error) {
	user, err := s.repository.GetByID(ctx, id)
	if errors.Is(err, apperrors.ErrNotFound) {
		return users.User{}, apperrors.ErrUnauthorized
	}
	if err != nil {
		return users.User{}, err
	}
	if user.GuestConferenceID != nil {
		return users.User{}, apperrors.ErrForbidden
	}
	return user, nil
}

// Refresh renews the short access token without imposing an idle/session deadline.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - raw: Serialized access token or refresh credential to validate; never log or expose it.
//
// Returns:
//   - result 1: The resulting login response.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Refresh(ctx context.Context, raw string) (users.LoginResponse, error) {
	session, err := s.sessionForCookie(ctx, raw)
	if err != nil {
		return users.LoginResponse{}, err
	}
	user, err := s.account(ctx, session.UserID)
	if err != nil {
		return users.LoginResponse{}, err
	}
	return s.sessionResponse(user, session, raw)
}

// Bootstrap upgrades a valid existing account token. Repeated tabs reuse its cookie.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - raw: Serialized access token or refresh credential to validate; never log or expose it.
//   - access: Current signed access token, used only for session verification.
//
// Returns:
//   - result 1: The resulting login response.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Bootstrap(ctx context.Context, userID, raw, access string) (users.LoginResponse, error) {
	user, err := s.account(ctx, userID)
	if err != nil {
		return users.LoginResponse{}, err
	}
	if s.sessions == nil {
		return users.LoginResponse{}, apperrors.ErrUnavailable
	}
	tokens, ok := s.tokens.(sessionTokens)
	if !ok {
		return users.LoginResponse{}, apperrors.ErrUnavailable
	}
	identity, scope, sessionID, _, err := tokens.VerifyAuthorization(access)
	if err != nil {
		return users.LoginResponse{}, err
	}
	if identity != userID || scope != "" {
		return users.LoginResponse{}, apperrors.ErrForbidden
	}
	legacyHash := ""
	if sessionID == "" {
		legacyHash = sessionHash(access)
	}
	var preferredID string
	var existingCookieID, existingCookie string
	if raw != "" {
		session, err := s.sessionForCookie(ctx, raw)
		if err == nil && session.UserID == userID {
			existingCookieID, existingCookie = session.ID, raw
			if sessionID != "" && sessionID == session.ID {
				return s.sessionResponse(user, session, raw)
			}
			if sessionID == "" {
				preferredID = session.ID
			}
		}
		if err != nil && !errors.Is(err, apperrors.ErrUnauthorized) {
			return users.LoginResponse{}, err
		}
	}
	if sessionID != "" {
		// A short-lived JS-readable access token must never mint a durable secret.
		// Matching cookies are reused above; cookie loss requires a fresh login.
		return users.LoginResponse{}, apperrors.ErrUnauthorized
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return users.LoginResponse{}, err
	}
	raw = base64.RawURLEncoding.EncodeToString(secret[:])
	candidate := users.AuthSession{ID: uuid.NewString(), UserID: userID, TokenHash: sessionHash(raw), CreatedAt: time.Now().UTC()}
	session, err := s.sessions.Bootstrap(ctx, candidate, legacyHash, preferredID)
	if errors.Is(err, apperrors.ErrNotFound) {
		err = apperrors.ErrUnauthorized
	}
	if err != nil {
		return users.LoginResponse{}, err
	}
	if session.ID == existingCookieID {
		raw = existingCookie
	}
	return s.sessionResponse(user, session, raw)
}

// Logout revokes both the current browser cookie and a supplied valid SID token.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - raw: Serialized access token or refresh credential to validate; never log or expose it.
//   - access: Current signed access token, used only for session verification.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Logout(ctx context.Context, raw, access string) error {
	if s.sessions == nil {
		return nil
	}
	if validSessionSecret(raw) {
		if err := s.sessions.RevokeByHash(ctx, sessionHash(raw)); err != nil {
			return err
		}
	}
	if tokens, ok := s.tokens.(sessionTokens); ok && access != "" {
		userID, scope, sessionID, _, err := tokens.VerifyAuthorization(access)
		if err == nil && sessionID != "" {
			return s.sessions.RevokeByID(ctx, sessionID, userID)
		}
		if err == nil && scope == "" {
			return s.sessions.RevokeLegacy(ctx, sessionHash(access), userID)
		}
	}
	return nil
}

// AuthorizeSession also serves long-lived WebSocket/SSE connections after JWT expiry.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - sessionID: Persistent or physical session identifier, as defined by the operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) AuthorizeSession(ctx context.Context, userID, sessionID string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if s.sessions == nil || sessionID == "" {
		return apperrors.ErrUnauthorized
	}
	session, err := s.sessions.GetByID(ctx, sessionID)
	if errors.Is(err, apperrors.ErrNotFound) || err == nil && (session.UserID != userID || session.RevokedAt != nil) {
		return apperrors.ErrUnauthorized
	}
	return err
}

// VerifyAuthorization verifies the authorization token and resolves its user, session, guest
// scope and expiry.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - raw: Serialized access token or refresh credential to validate; never log or expose it.
//
// Returns:
//   - result 1: Verified account identifier.
//   - result 2: Guest conference scope, or an empty string for a registered account.
//   - result 3: Persistent server-session identifier, or an empty string for a guest or legacy
//     token.
//   - result 4: The timestamp or validity boundary associated with the result.
//   - result 5: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) VerifyAuthorization(ctx context.Context, raw string) (string, string, string, time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tokens, ok := s.tokens.(sessionTokens)
	if !ok {
		return "", "", "", time.Time{}, apperrors.ErrUnauthorized
	}
	userID, scope, sessionID, expiry, err := tokens.VerifyAuthorization(raw)
	if err == nil && sessionID != "" {
		err = s.AuthorizeSession(ctx, userID, sessionID)
	}
	if err == nil && sessionID == "" && scope == "" && s.sessions != nil {
		err = s.sessions.AuthorizeLegacy(ctx, sessionHash(raw), userID)
	}
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	return userID, scope, sessionID, expiry, nil
}

// VerifySession verifies the JWT and returns its account, guest-conference scope and expiration
// claims.
//
// Parameters:
//   - raw: Serialized access token or refresh credential to validate; never log or expose it.
//
// Returns:
//   - result 1: Verified account identifier.
//   - result 2: Guest conference scope, or an empty string for an unrestricted registered
//     account.
//   - result 3: The timestamp or validity boundary associated with the result.
//   - result 4: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) VerifySession(raw string) (string, string, time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	userID, scope, _, expiry, err := s.VerifyAuthorization(ctx, raw)
	return userID, scope, expiry, err
}

// VerifyWithExpiry verifies an access token and returns the account identity and token deadline.
//
// Parameters:
//   - raw: Serialized access token or refresh credential to validate; never log or expose it.
//
// Returns:
//   - result 1: Authenticated account identifier.
//   - result 2: The timestamp or validity boundary associated with the result.
//   - result 3: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) VerifyWithExpiry(raw string) (string, time.Time, error) {
	userID, _, expiry, err := s.VerifySession(raw)
	return userID, expiry, err
}

// Verify validates the supplied credentials or signed binding before accepting the caller's
// identity.
//
// Parameters:
//   - raw: Serialized access token or refresh credential to validate; never log or expose it.
//
// Returns:
//   - result 1: Verified account identifier.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Verify(raw string) (string, error) {
	userID, _, err := s.VerifyWithExpiry(raw)
	return userID, err
}
