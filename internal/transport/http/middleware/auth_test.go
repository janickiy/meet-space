package httpmiddleware

import "testing"

// TestGuestScopeCannotCrossResourceBoundaries protects canonical route-prefix
// checks against another meeting, similarly prefixed IDs and account routes.
func TestGuestScopeCannotCrossResourceBoundaries(t *testing.T) {
	const meeting = "00000000-0000-4000-8000-000000000001"
	for _, tc := range []struct {
		method, path string
		allowed      bool
	}{
		{"GET", "/api/v1/conferences/" + meeting, true},
		{"GET", "/api/v1/conferences/" + meeting + "/chat/messages", true},
		{"POST", "/api/v1/conferences/00000000-0000-4000-8000-000000000002/join", false},
		{"GET", "/api/v1/conferences/" + meeting + "extra", false},
		{"GET", "/api/v1/conversations/" + meeting, false},
		{"PATCH", "/api/v1/auth/me", false},
		{"GET", "/api/v1/auth/me", true},
		{"POST", "/api/v1/auth/session", false},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			if got := GuestRequestAllowed(tc.method, tc.path, meeting); got != tc.allowed {
				t.Fatalf("allowed=%v, want %v", got, tc.allowed)
			}
		})
	}
}
