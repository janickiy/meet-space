package security

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
)

// TestGuestTokenLifetime verifies the shared guest lifetime without extending
// ordinary access tokens and rejects a guest token after its expiry.
func TestGuestTokenLifetime(t *testing.T) {
	s, err := NewTokenService("synthetic-audit-secret-32-bytes-long")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	userID := "00000000-0000-4000-8000-000000000001"
	conferenceID := "00000000-0000-4000-8000-000000000002"
	guest, err := s.IssueGuest(userID, conferenceID)
	if err != nil {
		t.Fatal(err)
	}
	_, scope, _, expires, err := s.VerifyAuthorization(guest)
	if err != nil || scope != conferenceID || expires.Sub(now) != users.GuestSessionTTL || users.GuestSessionTTL != 3*time.Hour {
		t.Fatalf("guest scope=%q lifetime=%s error=%v", scope, expires.Sub(now), err)
	}
	regular, err := s.Issue(userID)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, expires, err = s.VerifyAuthorization(regular)
	if err != nil || expires.Sub(now) != time.Hour {
		t.Fatalf("ordinary token lifetime=%s error=%v", expires.Sub(now), err)
	}
	now = now.Add(users.GuestSessionTTL + time.Second)
	if _, _, _, _, err := s.VerifyAuthorization(guest); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatalf("expired guest token: got %v, want unauthorized", err)
	}
}

// TestTokenClaimsAreRequired checks signature algorithm, expiry, issuer,
// audience and guest/session separation using synthetic signed tokens.
func TestTokenClaimsAreRequired(t *testing.T) {
	s, err := NewTokenService("synthetic-audit-secret-32-bytes-long")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	valid := func() sessionClaims {
		return sessionClaims{RegisteredClaims: jwt.RegisteredClaims{Subject: "00000000-0000-4000-8000-000000000001", Issuer: tokenIssuer, Audience: jwt.ClaimStrings{tokenAudience}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour))}}
	}
	cases := []struct {
		name    string
		mutate  func(*sessionClaims)
		method  jwt.SigningMethod
		allowed bool
	}{
		{"valid", func(*sessionClaims) {}, jwt.SigningMethodHS256, true},
		{"wrong algorithm", func(*sessionClaims) {}, jwt.SigningMethodHS384, false},
		{"missing expiry", func(c *sessionClaims) { c.ExpiresAt = nil }, jwt.SigningMethodHS256, false},
		{"expired", func(c *sessionClaims) { c.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Second)) }, jwt.SigningMethodHS256, false},
		{"missing issued at", func(c *sessionClaims) { c.IssuedAt = nil }, jwt.SigningMethodHS256, false},
		{"future issued at", func(c *sessionClaims) { c.IssuedAt = jwt.NewNumericDate(now.Add(time.Minute)) }, jwt.SigningMethodHS256, false},
		{"wrong issuer", func(c *sessionClaims) { c.Issuer = "other" }, jwt.SigningMethodHS256, false},
		{"wrong audience", func(c *sessionClaims) { c.Audience = jwt.ClaimStrings{"other"} }, jwt.SigningMethodHS256, false},
		{"invalid subject", func(c *sessionClaims) { c.Subject = "not-a-user-id" }, jwt.SigningMethodHS256, false},
		{"guest and durable session", func(c *sessionClaims) {
			c.GuestConferenceID = "00000000-0000-4000-8000-000000000002"
			c.SessionID = "00000000-0000-4000-8000-000000000003"
		}, jwt.SigningMethodHS256, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := valid()
			tc.mutate(&claims)
			raw, err := jwt.NewWithClaims(tc.method, claims).SignedString(s.secret)
			if err != nil {
				t.Fatal(err)
			}
			_, _, _, _, err = s.VerifyAuthorization(raw)
			if (err == nil) != tc.allowed {
				t.Fatalf("verification error=%v, allowed=%v", err, tc.allowed)
			}
		})
	}
}
