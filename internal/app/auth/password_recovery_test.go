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
		{"queued", `{ "email":"active@example.invalid"}`, "https://meet.example.invalid", nil, 200, "Если аккаунт существует, ссылка для восстановления пароля будет отправлена.", 1},
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
			wantStatus := "failed"
			if test.status == 200 {
				wantStatus = "success"
			}
			if writer.Code != test.status || body.Status != wantStatus || body.Message != test.message || probe.calls != test.calls {
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

type resetProbe struct {
	Service
	err             error
	calls           int
	token, password string
}

func (p *resetProbe) ResetPassword(_ context.Context, token, password string) error {
	p.calls++
	p.token = token
	p.password = password
	return p.err
}
func TestPasswordResetHTTPContractAndOrigin(t *testing.T) {
	for _, test := range []struct {
		name, body, origin string
		err                error
		status, calls      int
	}{
		{"reset", `{"token":"synthetic-reset-token","password":"SyntheticPassword123"}`, "https://meet.example.invalid", nil, 200, 1},
		{"expired", `{"token":"synthetic-reset-token","password":"SyntheticPassword123"}`, "https://meet.example.invalid", apperrors.New(apperrors.ErrInvalidInput, "invalid or expired password reset link"), 422, 1},
		{"blocked", `{"token":"synthetic-reset-token","password":"SyntheticPassword123"}`, "https://meet.example.invalid", apperrors.ErrAccountBlocked, 403, 1},
		{"strict fields", `{"token":"synthetic-reset-token","password":"SyntheticPassword123","email":"private@example.invalid"}`, "https://meet.example.invalid", nil, 400, 0},
		{"cross origin", `{"token":"synthetic-reset-token","password":"SyntheticPassword123"}`, "https://other.example.invalid", nil, 403, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			probe := &resetProbe{err: test.err}
			handler := NewHandler(probe).WithSessionCookies("https://meet.example.invalid", true)
			writer := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(writer)
			ctx.Request = httptest.NewRequest(http.MethodPost, "https://meet.example.invalid/api/v1/auth/reset-password", strings.NewReader(test.body))
			ctx.Request.Header.Set("Origin", test.origin)
			ctx.Request.Header.Set("Content-Type", "application/json")
			handler.ResetPassword(ctx)
			if writer.Code != test.status || probe.calls != test.calls {
				t.Fatalf("status=%d calls=%d", writer.Code, probe.calls)
			}
			if test.calls > 0 && (probe.token != "synthetic-reset-token" || probe.password != "SyntheticPassword123" || writer.Header().Get("Cache-Control") != "no-store") {
				t.Fatal("reset fields or no-store contract changed")
			}
			if strings.Contains(writer.Body.String(), "SyntheticPassword123") || strings.Contains(writer.Body.String(), "synthetic-reset-token") || len(writer.Result().Cookies()) != 0 {
				t.Fatal("reset leaked credential or issued cookie")
			}
		})
	}
}
