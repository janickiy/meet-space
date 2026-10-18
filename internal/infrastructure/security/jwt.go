package security

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

const AccessTokenTTL = time.Hour
const tokenIssuer = "go-recorder"
const tokenAudience = "go-recorder-api"

type sessionClaims struct {
	jwt.RegisteredClaims
	GuestConferenceID string `json:"guestConferenceId,omitempty"`
	SessionID         string `json:"sid,omitempty"`
}

// TokenService issues and verifies account JWTs with a configured bounded lifetime.
type TokenService struct {
	secret []byte
	now    func() time.Time
}

// NewTokenService constructs the token service with the dependencies and limits required for
// authentication and cryptographic bindings.
//
// Parameters:
//   - secret: Private key material or raw secret; it must not be logged or exposed.
//
// Returns:
//   - result 1: The constructed or loaded token service.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NewTokenService(secret string) (*TokenService, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must contain at least 32 bytes")
	}
	return &TokenService{secret: []byte(secret), now: time.Now}, nil
}

// Issue creates a signed token bound to the supplied identity and expiry.
//
// Parameters:
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: Signed short-lived credential for the verified identity.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *TokenService) Issue(userID string) (string, error) {
	return s.issue(userID, "", "")
}

// IssueSession binds a short-lived access token to a revocable account session.
//
// Parameters:
//   - userID: Acting account identifier used to scope access.
//   - sessionID: Persistent or physical session identifier, as defined by the operation.
//
// Returns:
//   - result 1: Signed access JWT bound to the persistent session.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *TokenService) IssueSession(userID, sessionID string) (string, error) {
	id, err := uuid.Parse(sessionID)
	if err != nil || id == uuid.Nil {
		return "", fmt.Errorf("invalid account session")
	}
	return s.issue(userID, "", id.String())
}

// IssueGuest grants only the invited meeting; it never grants account access.
//
// Parameters:
//   - userID: Acting account identifier used to scope access.
//   - conferenceID: Conference identifier limiting the operation scope.
//
// Returns:
//   - result 1: Signed guest JWT restricted to the invited conference.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *TokenService) IssueGuest(userID, conferenceID string) (string, error) {
	id, err := uuid.Parse(conferenceID)
	if err != nil || id == uuid.Nil {
		return "", fmt.Errorf("invalid guest conference")
	}
	return s.issue(userID, id.String(), "")
}

// issue creates a signed token bound to the supplied identity and expiry.
//
// Parameters:
//   - userID: Acting account identifier used to scope access.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - sessionID: Persistent or physical session identifier, as defined by the operation.
//
// Returns:
//   - result 1: Signed access JWT containing the supplied identity binding.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *TokenService) issue(userID, conferenceID, sessionID string) (string, error) {
	id, err := uuid.Parse(userID)
	if err != nil || id == uuid.Nil {
		return "", fmt.Errorf("invalid token subject")
	}
	now := s.now().UTC()
	claims := sessionClaims{GuestConferenceID: conferenceID, SessionID: sessionID, RegisteredClaims: jwt.RegisteredClaims{Subject: id.String(), Issuer: tokenIssuer,
		Audience: jwt.ClaimStrings{tokenAudience}, IssuedAt: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL))}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
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
func (s *TokenService) Verify(raw string) (string, error) {
	id, _, err := s.VerifyWithExpiry(raw)
	return id, err
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
func (s *TokenService) VerifyWithExpiry(raw string) (string, time.Time, error) {
	id, _, expiry, err := s.VerifySession(raw)
	return id, expiry, err
}

// VerifySession validates the signature and returns the optional meeting scope.
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
func (s *TokenService) VerifySession(raw string) (string, string, time.Time, error) {
	id, conferenceID, _, expiry, err := s.VerifyAuthorization(raw)
	return id, conferenceID, expiry, err
}

// VerifyAuthorization validates JWT claims; the application also checks session revocation.
//
// Parameters:
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
func (s *TokenService) VerifyAuthorization(raw string) (string, string, string, time.Time, error) {
	if len(raw) == 0 || len(raw) > 4096 {
		return "", "", "", time.Time{}, apperrors.ErrUnauthorized
	}
	claims := &sessionClaims{}
	token, err := jwt.ParseWithClaims(raw, claims,

		func(_ *jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(), jwt.WithIssuer(tokenIssuer), jwt.WithAudience(tokenAudience), jwt.WithTimeFunc(s.now))
	if err != nil || !token.Valid || claims.IssuedAt == nil {
		return "", "", "", time.Time{}, apperrors.ErrUnauthorized
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil || id == uuid.Nil {
		return "", "", "", time.Time{}, apperrors.ErrUnauthorized
	}
	if claims.GuestConferenceID != "" {
		conferenceID, err := uuid.Parse(claims.GuestConferenceID)
		if err != nil || conferenceID == uuid.Nil || conferenceID.String() != claims.GuestConferenceID {
			return "", "", "", time.Time{}, apperrors.ErrUnauthorized
		}
	}
	if claims.SessionID != "" {
		sessionID, err := uuid.Parse(claims.SessionID)
		if err != nil || sessionID == uuid.Nil || sessionID.String() != claims.SessionID || claims.GuestConferenceID != "" {
			return "", "", "", time.Time{}, apperrors.ErrUnauthorized
		}
	}
	return id.String(), claims.GuestConferenceID, claims.SessionID, claims.ExpiresAt.Time, nil
}
