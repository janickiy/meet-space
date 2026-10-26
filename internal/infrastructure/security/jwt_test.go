package security

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

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
