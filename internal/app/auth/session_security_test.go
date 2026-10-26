package authapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestSessionOriginProtection checks browser CSRF rejection while preserving
// non-browser JSON clients that do not send Origin or Fetch Metadata headers.
func TestSessionOriginProtection(t *testing.T) {
	cases := []struct {
		name, origin, fetchSite, contentType string
		allowed                              bool
	}{
		{"same origin", "https://meet.example.invalid", "same-origin", "application/json", true},
		{"cli", "", "", "application/json", true},
		{"cross origin", "https://attacker.example.invalid", "same-site", "application/json", false},
		{"null origin", "null", "", "application/json", false},
		{"cross site metadata", "https://meet.example.invalid", "cross-site", "application/json", false},
		{"missing browser origin", "", "same-origin", "application/json", false},
		{"form content", "https://meet.example.invalid", "same-origin", "application/x-www-form-urlencoded", false},
		{"text content", "https://meet.example.invalid", "same-origin", "text/plain", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writer := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(writer)
			c.Request = httptest.NewRequest(http.MethodPost, "https://meet.example.invalid/api/v1/auth/refresh", strings.NewReader("{}"))
			c.Request.Header.Set("Origin", tc.origin)
			c.Request.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			c.Request.Header.Set("Content-Type", tc.contentType)
			h := (&Handler{}).WithSessionCookies("https://meet.example.invalid", true)
			if got := h.sameOrigin(c); got != tc.allowed {
				t.Fatalf("allowed=%v, want %v", got, tc.allowed)
			}
			if !tc.allowed && !c.IsAborted() {
				t.Fatal("rejected request was not aborted")
			}
		})
	}
}

// TestSessionCookieSecurity ensures refresh credentials are inaccessible to
// JavaScript, HTTPS-only, host-only and limited to the auth route prefix.
func TestSessionCookieSecurity(t *testing.T) {
	h := (&Handler{}).WithSessionCookies("https://meet.example.invalid", true)
	for _, raw := range []string{"synthetic-cookie", ""} {
		writer := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(writer)
		h.setSessionCookie(c, raw)
		cookies := writer.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatalf("cookies=%d, want 1", len(cookies))
		}
		cookie := cookies[0]
		if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Domain != "" || cookie.Path != "/api/v1/auth" {
			t.Fatalf("unexpected cookie protections: %+v", cookie)
		}
		if raw == "" && cookie.MaxAge != -1 {
			t.Fatal("logout cookie must expire immediately")
		}
	}
}
