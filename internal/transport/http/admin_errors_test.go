package httptransport

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/config"
	domain "github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/errorlogs"
	"github.com/janickiy/meet-space/internal/infrastructure/security"
	adminusecase "github.com/janickiy/meet-space/internal/usecase/admin"
)

type errorResourceProbe struct {
	AdminResources
	calls      int
	query      errorlogs.Query
	id         string
	authorized bool
	err        error
}

func (p *errorResourceProbe) ListErrors(ctx context.Context, q errorlogs.Query) (errorlogs.Page, error) {
	p.calls++
	p.query = q
	_, p.authorized = domain.AuthorizationFromContext(ctx)
	return errorlogs.Page{Items: []errorlogs.Event{{ID: "00000000-0000-4000-8000-000000000002", Service: "api", Level: "ERROR", Code: "request.failed", Message: "Ошибка обработки запроса", Count: 2}}, RetentionDays: 14, MaxRecords: 20000}, p.err
}
func (p *errorResourceProbe) ErrorDetail(ctx context.Context, id string) (errorlogs.Event, error) {
	p.calls++
	p.id = id
	_, p.authorized = domain.AuthorizationFromContext(ctx)
	return errorlogs.Event{ID: id, Service: "api", Level: "ERROR", Code: "request.failed", Message: "Ошибка обработки запроса"}, p.err
}

type errorRouteAuthRepo struct{ adminRouteRepo }

func (r *errorRouteAuthRepo) Authenticate(ctx context.Context, h string) (domain.Administrator, error) {
	if !r.session.ExpiresAt.After(time.Now()) {
		return domain.Administrator{}, apperrors.ErrUnauthorized
	}
	return r.adminRouteRepo.Authenticate(ctx, h)
}

func errorTestRouter(t *testing.T, resources AdminResources) (*gin.Engine, *errorRouteAuthRepo, *http.Cookie) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	token := strings.Repeat("A", 43)
	repo := &errorRouteAuthRepo{adminRouteRepo: adminRouteRepo{account: domain.Administrator{ID: "00000000-0000-4000-8000-000000000001", Username: "operator", Status: "active"}, session: domain.Session{TokenHash: adminusecase.HashToken(token), ExpiresAt: time.Now().Add(time.Hour)}}}
	service, err := adminusecase.NewService(repo, adminRoutePasswords{})
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewAdminRouter(service, config.AdminConfig{Enabled: true, PublicURL: "https://meet.example:8081"}, resources, nil)
	if err != nil {
		t.Fatal(err)
	}
	return router, repo, &http.Cookie{Name: domain.CookieName, Value: token}
}
func errorRequest(router *gin.Engine, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	out := httptest.NewRecorder()
	router.ServeHTTP(out, req)
	return out
}
func TestAdminErrorListAndDetailsContract(t *testing.T) {
	probe := &errorResourceProbe{}
	router, _, cookie := errorTestRouter(t, probe)
	w := errorRequest(router, domain.CookiePath+"/errors?service=api&level=error&code=request.failed&requestId=00000000-0000-4000-8000-000000000003&from=2026-11-01T00:00:00Z&to=2026-11-06T00:00:00Z&pageSize=50", cookie)
	if w.Code != 200 || probe.calls != 1 || !probe.authorized {
		t.Fatalf("list status=%d calls=%d authorized=%v", w.Code, probe.calls, probe.authorized)
	}
	if probe.query.Service != "api" || probe.query.Level != "ERROR" || probe.query.Code != "request.failed" || probe.query.PageSize != 50 || probe.query.RequestID != "00000000-0000-4000-8000-000000000003" {
		t.Fatalf("filters not passed: %+v", probe.query)
	}
	var page struct {
		Items         []errorlogs.Event `json:"items"`
		RetentionDays int               `json:"retentionDays"`
		MaxRecords    int               `json:"maxRecords"`
	}
	if json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.RetentionDays != 14 || page.MaxRecords != 20000 {
		t.Fatal("unexpected list contract", w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("private error response may be cached")
	}
	if strings.Contains(w.Body.String(), cookie.Value) {
		t.Fatal("session secret entered response")
	}
	id := "00000000-0000-4000-8000-000000000002"
	w = errorRequest(router, domain.CookiePath+"/errors/"+id, cookie)
	if w.Code != 200 || probe.id != id || probe.calls != 2 {
		t.Fatal("detail was not delegated", w.Code)
	}
	var detail errorlogs.Event
	if json.Unmarshal(w.Body.Bytes(), &detail) != nil || detail.ID != id {
		t.Fatal("detail envelope mismatch")
	}
	probe.err = apperrors.ErrNotFound
	if w = errorRequest(router, domain.CookiePath+"/errors/"+id, cookie); w.Code != 404 {
		t.Fatal("missing detail status", w.Code)
	}
}
func TestAdminErrorsRejectAmbiguousOrUnboundedFilters(t *testing.T) {
	probe := &errorResourceProbe{}
	router, _, cookie := errorTestRouter(t, probe)
	for _, query := range []string{
		"pageSize=0", "pageSize=101", "pageSize=-1", "pageSize=one", "pageSize=9999999999999999999999999", "service=postgres", "level=debug", "code=secret%20payload", "requestId=not-a-uuid", "from=yesterday", "to=2026-99-01T00:00:00Z", "from=2026-11-06T00:00:00Z&to=2026-11-05T00:00:00Z", "service=api&service=frontend", "unexpected=true", "cursor=not-base64!", "cursor=" + base64.RawURLEncoding.EncodeToString([]byte(`{"lastSeen":"not-a-date","id":"not-a-uuid"}`)), "cursor=" + strings.Repeat("A", 513), "code=" + strings.Repeat("A", 2049),
	} {
		w := errorRequest(router, domain.CookiePath+"/errors?"+query, cookie)
		if w.Code != 400 {
			t.Fatalf("bad query accepted (%s): %d", query, w.Code)
		}
	}
	for _, suffix := range []string{"not-a-uuid", "00000000-0000-4000-8000-000000000002?code=ignored"} {
		if w := errorRequest(router, domain.CookiePath+"/errors/"+suffix, cookie); w.Code != 400 {
			t.Fatal("bad detail accepted", w.Code)
		}
	}
	if probe.calls != 0 {
		t.Fatal("invalid filters reached storage")
	}
	w := errorRequest(router, domain.CookiePath+"/errors?level=warning", cookie)
	if w.Code != 200 || probe.query.Level != "WARN" || probe.query.PageSize != 25 {
		t.Fatal("warning alias or default page size mismatch", w.Code)
	}
}
func TestAdminErrorsRequireLiveAdministrativeSession(t *testing.T) {
	probe := &errorResourceProbe{}
	router, repo, cookie := errorTestRouter(t, probe)
	for _, path := range []string{domain.CookiePath + "/errors", domain.CookiePath + "/errors/00000000-0000-4000-8000-000000000002"} {
		for _, invalid := range []*http.Cookie{nil, {Name: "meetrix_session", Value: cookie.Value}, {Name: domain.CookieName, Value: strings.Repeat("B", 43)}} {
			if w := errorRequest(router, path, invalid); w.Code != 401 {
				t.Fatal("non-admin cookie accepted", w.Code)
			}
		}
	}
	tokenSvc, err := security.NewTokenService("synthetic-public-signing-secret-long-enough")
	if err != nil {
		t.Fatal(err)
	}
	jwt, err := tokenSvc.Issue("00000000-0000-4000-8000-000000000009")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, domain.CookiePath+"/errors", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	out := httptest.NewRecorder()
	router.ServeHTTP(out, req)
	if out.Code != 401 {
		t.Fatal("public JWT reached technical log")
	}
	repo.session.ExpiresAt = time.Now().Add(-time.Second)
	if w := errorRequest(router, domain.CookiePath+"/errors", cookie); w.Code != 401 {
		t.Fatal("expired admin session accepted", w.Code)
	}
	repo.session.ExpiresAt = time.Now().Add(time.Hour)
	repo.revoked = true
	if w := errorRequest(router, domain.CookiePath+"/errors", cookie); w.Code != 401 {
		t.Fatal("revoked admin session accepted", w.Code)
	}
	if probe.calls != 0 {
		t.Fatal("unauthenticated error request reached storage")
	}
}

func TestAdminErrorClientReportsRequireSessionOriginAndCSRF(t *testing.T) {
	router, _, cookie := errorTestRouter(t, &errorResourceProbe{})
	payload := `{"version":"synthetic-admin-release","route":"/errors","code":"uncaught_error","browser":"chromium","stack":[]}`
	for _, tc := range []struct {
		name, origin, csrf string
		cookie             *http.Cookie
		want               int
	}{
		{name: "no session", origin: "https://meet.example:8081", want: 401},
		{name: "wrong origin", origin: "https://evil.example", csrf: adminusecase.CSRFToken(cookie.Value), cookie: cookie, want: 403},
		{name: "no csrf", origin: "https://meet.example:8081", cookie: cookie, want: 403},
		{name: "forged csrf", origin: "https://meet.example:8081", csrf: "forged", cookie: cookie, want: 403},
		{name: "authenticated", origin: "https://meet.example:8081", csrf: adminusecase.CSRFToken(cookie.Value), cookie: cookie, want: 202},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, domain.CookiePath+"/errors/client", strings.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", tc.origin)
			if tc.csrf != "" {
				req.Header.Set("X-CSRF-Token", tc.csrf)
			}
			if tc.cookie != nil {
				req.AddCookie(tc.cookie)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status %d want %d", w.Code, tc.want)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("client report response may be cached")
			}
		})
	}
}
