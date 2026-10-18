package security

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/media"
)

// MediaTickets issues and verifies internal tickets bound to physical media connections.
type MediaTickets struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// mediaClaims contains the signed media ticket identity and expiration claims.
type mediaClaims struct {
	jwt.RegisteredClaims
	Purpose string        `json:"purpose"`
	Binding media.Binding `json:"binding"`
	Route   media.Route   `json:"route"`
}

// NewMediaTickets constructs the media tickets with the dependencies and limits required for
// authentication and cryptographic bindings.
//
// Parameters:
//   - secret: Private key material or raw secret; it must not be logged or exposed.
//   - ttl: Lifetime assigned to the temporary value or ownership record.
//
// Returns:
//   - result 1: The constructed or loaded media tickets.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NewMediaTickets(secret string, ttl time.Duration) (*MediaTickets, error) {
	if len(secret) < 32 || ttl < 30*time.Second || ttl > 60*time.Second {
		return nil, fmt.Errorf("media ticket secret must contain >=32 bytes and TTL must be 30–60 seconds")
	}
	return &MediaTickets{secret: []byte(secret), ttl: ttl, now: time.Now}, nil
}

// validMediaBinding validates the complete identity binding used by internal media tickets.
//
// Parameters:
//   - binding: Verified account, session and media-connection identity.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func validMediaBinding(binding media.Binding) bool {
	for _, id := range []string{binding.ConferenceID, binding.ParticipantID, binding.SessionID, binding.ConnectionID, binding.UserID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil || parsed.String() != id {
			return false
		}
	}
	return !binding.AuthorizationExpiresAt.IsZero()
}

// Issue creates a signed token bound to the supplied identity and expiry.
//
// Parameters:
//   - binding: Verified account, session and media-connection identity.
//   - route: Current media-room owner and fencing version.
//
// Returns:
//   - result 1: Signed short-lived credential for the verified identity.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *MediaTickets) Issue(binding media.Binding, route media.Route) (string, error) {
	now := s.now().UTC()
	lease, err := uuid.Parse(route.LeaseID)
	if !validMediaBinding(binding) || !binding.AuthorizationExpiresAt.After(now) || route.WorkerID == "" || route.Endpoint == "" || err != nil || lease == uuid.Nil {
		return "", media.ErrUnauthorized
	}
	expires := now.Add(s.ttl)
	if binding.AuthorizationExpiresAt.Before(expires) {
		expires = binding.AuthorizationExpiresAt
	}
	claims := mediaClaims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "go-recorder-api", Audience: jwt.ClaimStrings{"go-recorder-media"}, Subject: binding.ConnectionID,
		ID: uuid.NewString(), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expires),
	}, Purpose: "media", Binding: binding, Route: route}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

// Verify validates the supplied credentials or signed binding before accepting the caller's
// identity.
//
// Parameters:
//   - raw: Serialized access token or refresh credential to validate; never log or expose it.
//
// Returns:
//   - result 1: The resulting binding.
//   - result 2: The resulting route.
//   - result 3: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *MediaTickets) Verify(raw string) (media.Binding, media.Route, error) {
	if len(raw) == 0 || len(raw) > 8192 {
		return media.Binding{}, media.Route{}, media.ErrUnauthorized
	}
	claims := &mediaClaims{}
	token, err := jwt.ParseWithClaims(raw, claims,

		func(*jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(), jwt.WithIssuer("go-recorder-api"), jwt.WithAudience("go-recorder-media"), jwt.WithTimeFunc(s.now))
	if err != nil || !token.Valid || claims.Purpose != "media" || claims.IssuedAt == nil ||
		!validMediaBinding(claims.Binding) || claims.Subject != claims.Binding.ConnectionID ||
		!claims.Binding.AuthorizationExpiresAt.After(s.now()) ||
		claims.ExpiresAt.Time.After(claims.Binding.AuthorizationExpiresAt) ||
		claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time) > s.ttl+time.Second ||
		claims.Route.WorkerID == "" || claims.Route.Endpoint == "" {
		return media.Binding{}, media.Route{}, media.ErrUnauthorized
	}
	for _, id := range []string{claims.ID, claims.Route.LeaseID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil {
			return media.Binding{}, media.Route{}, media.ErrUnauthorized
		}
	}
	return claims.Binding, claims.Route, nil
}
