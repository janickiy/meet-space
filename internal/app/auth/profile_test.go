package authapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/domain/users"
)

type profilePatchProbe struct {
	Service
	patch  users.UpdateProfileRequest
	called bool
}

func (p *profilePatchProbe) UpdateProfile(_ context.Context, _ string, patch users.UpdateProfileRequest) (users.View, error) {
	p.called = true
	normalized, err := patch.Normalized()
	p.patch = normalized
	if err != nil {
		return users.View{}, err
	}
	user := users.User{DisplayName: normalized.DisplayName}
	if normalized.Bio != nil {
		user.Bio = *normalized.Bio
	}
	return user.View(), nil
}
func TestProfileHTTPPatchPresenceAndUnicodeBody(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		bioOnly    bool
	}{
		{"bio-only", `{"bio":"О себе"}`, 200, true},
		{"clear", `{"bio":""}`, 200, true},
		{"legacy-name", `{"displayName":"Имя"}`, 200, false},
		{"escaped-unicode-limit", `{"bio":"` + strings.Repeat(`\ud83d\ude42`, 1000) + `"}`, 200, true},
		{"over-limit", `{"bio":"` + strings.Repeat(`\ud83d\ude42`, 1001) + `"}`, 422, false},
		{"empty-name", `{"displayName":"","bio":"valid"}`, 422, false},
		{"empty-patch", `{}`, 422, false},
		{"null-bio", `{"bio":null}`, 400, false},
		{"other-account", `{"id":"other","bio":"text"}`, 400, false},
		{"trailing", `{"bio":"text"} {}`, 400, false},
		{"body-limit", `{"bio":"` + strings.Repeat("x", 33*1024) + `"}`, 400, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probe := &profilePatchProbe{}
			writer := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(writer)
			ctx.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/auth/me", strings.NewReader(tc.body))
			NewHandler(probe).UpdateProfile(ctx)
			if writer.Code != tc.status {
				t.Fatalf("status=%d body=%s", writer.Code, writer.Body.String())
			}
			if tc.status == 400 && probe.called {
				t.Fatal("malformed body reached service")
			}
			if tc.bioOnly && (probe.patch.Bio == nil || probe.patch.DisplayName != nil) {
				t.Fatal("bio-only patch overwrote omitted display name")
			}
		})
	}
}
