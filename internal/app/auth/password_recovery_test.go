package authapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

type recoveryProbe struct {
	Service
	err   error
	email string
	calls int
}

func (p *recoveryProbe) PasswordRecovery(_ context.Context, email string) error {
	p.calls++
	p.email = email
	return p.err
}

func TestPasswordRecoveryHTTPContractAndOrigin(t *testing.T) {
	for _, test := range []struct {
		name, body, origin string
		err                error
		status             int
		message            string
		calls              int
	}{
		{"blocked", `{"email":"blocked@example.invalid"}`, "https://meet.example.invalid", apperrors.ErrAccountBlocked, 403, "account blocked", 1},
		{"unavailable", `{"email":"active@example.invalid"}`, "https://meet.example.invalid", apperrors.New(apperrors.ErrUnavailable, "password recovery unavailable"), 503, "password recovery unavailable", 1},
		{"unknown fields", `{"email":"blocked@example.invalid","token":"discard"}`, "https://meet.example.invalid", apperrors.ErrAccountBlocked, 400, "invalid json body or unknown fields", 0},
		{"cross origin", `{"email":"blocked@example.invalid"}`, "https://other.example.invalid", apperrors.ErrAccountBlocked, 403, "access denied", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			probe := &recoveryProbe{err: test.err}
			handler := NewHandler(probe).WithSessionCookies("https://meet.example.invalid", true)
			writer := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(writer)
			ctx.Request = httptest.NewRequest(http.MethodPost, "https://meet.example.invalid/api/v1/auth/password-recovery", strings.NewReader(test.body))
			ctx.Request.Header.Set("Origin", test.origin)
			ctx.Request.Header.Set("Content-Type", "application/json")
			handler.PasswordRecovery(ctx)
			var body struct{ Status, Message string }
			if err := json.Unmarshal(writer.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if writer.Code != test.status || body.Status != "failed" || body.Message != test.message || probe.calls != test.calls {
				t.Fatalf("status=%d message=%q calls=%d", writer.Code, body.Message, probe.calls)
			}
			if test.calls > 0 && writer.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("recovery response may be cached")
			}
			if len(writer.Result().Cookies()) != 0 {
				t.Fatal("recovery issued a session cookie")
			}
		})
	}
}
