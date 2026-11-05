package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	platformapp "github.com/janickiy/meet-space/internal/app/platform"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/platform"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

type diagnosticsTokens struct{}

func (diagnosticsTokens) Verify(v string) (string, error) {
	if v == "user-token" {
		return "user-id", nil
	}
	return "", apperrors.ErrUnauthorized
}

type diagnosticsAccess struct{ allowed bool }

func (a *diagnosticsAccess) IsAdmin(context.Context, string) (bool, error) { return a.allowed, nil }

type diagnosticsService struct{}

func (diagnosticsService) Capabilities(context.Context) platform.Capabilities {
	return platform.Capabilities{RecordingModes: []string{"audio_only"}}
}
func (diagnosticsService) Summary(context.Context) (platform.Summary, error) {
	return platform.Summary{APIReady: true, Dependencies: map[string]bool{}}, nil
}

func TestPlatformDiagnosticsPreservesUserCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()
	access := &diagnosticsAccess{}
	RegisterPlatformStatusRoutes(router, &platformapp.Handler{Service: diagnosticsService{}}, httpmiddleware.Authenticate(diagnosticsTokens{}), access)
	request := func(path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.AddCookie(&http.Cookie{Name: "__Secure-meetspace_admin_session", Value: "admin-cookie-cannot-grant-user-access"})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	if got := request("/api/v1/platform/summary", "").Code; got != 401 {
		t.Fatal("admin cookie granted diagnostics access", got)
	}
	if got := request("/api/v1/platform/summary", "user-token").Code; got != 403 {
		t.Fatal("ordinary user read diagnostics", got)
	}
	access.allowed = true
	summary := request("/api/v1/platform/summary", "user-token")
	if summary.Code != 200 || summary.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("existing diagnostics capability failed", summary.Code)
	}
	if got := request("/api/v1/admin/summary", "user-token").Code; got != 404 {
		t.Fatal("legacy admin namespace accessible", got)
	}
	access.allowed = false
	if got := request("/api/v1/platform/summary", "user-token").Code; got != 403 {
		t.Fatal("revoked capability still valid", got)
	}
	if got := request("/api/v1/capabilities", "user-token").Code; got != 200 {
		t.Fatal("public capabilities regressed", got)
	}
}
func TestPublicRouterAlwaysRejectsAdminNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()
	// Even accidental future route registration cannot reopen this namespace.
	router.Any("/api/v1/admin", func(c *gin.Context) { c.Status(200) })
	router.Any("/api/v1/admin/*rest", func(c *gin.Context) { c.Status(200) })
	for _, method := range []string{"GET", "POST", "PATCH", "DELETE"} {
		for _, path := range []string{
			"/api/v1/admin", "/api/v1/admin/summary", "/api/v1/admin/auth/me", "/api/v1/Admin/auth/me",
			"/api/v1/%61dmin/auth/me", "/api%2fv1%2fadmin%2fauth%2fme", "/api/v1/admin%2fauth%2fme",
			"//api/v1//admin//auth/me", "/api/v1/x/../admin/auth/me", "/api/v1/%2e/admin/auth/me",
		} {
			req := httptest.NewRequest(method, path, nil)
			req.Header.Set("X-Forwarded-Host", "meet.example:8081")
			req.Header.Set("X-Forwarded-Port", "8081")
			req.Header.Set("X-Forwarded-Proto", "https")
			req.Header.Set("Authorization", "Bearer user-token")
			req.AddCookie(&http.Cookie{Name: "__Secure-meetspace_admin_session", Value: "admin-session"})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != 404 {
				t.Errorf("%s %s leaked: %d", method, path, rec.Code)
			}
		}
	}
}
