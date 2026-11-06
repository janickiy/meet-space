package httptransport

import (
	"bytes"
	"context"
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
	"github.com/janickiy/meet-space/internal/infrastructure/security"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	adminusecase "github.com/janickiy/meet-space/internal/usecase/admin"
)

type adminRoutePasswords struct{}

func (adminRoutePasswords) Hash(v string) (string, error)    { return "hash:" + v, nil }
func (adminRoutePasswords) Verify(v, h string) (bool, error) { return h == "hash:"+v, nil }

type adminRouteRepo struct {
	domain.Repository
	account domain.Administrator
	session domain.Session
	revoked bool
}

type adminDeadlineRepo struct {
	domain.Repository
	observed context.Context
}

func (r *adminDeadlineRepo) Authenticate(ctx context.Context, _ string) (domain.Administrator, error) {
	r.observed = ctx
	return domain.Administrator{}, apperrors.ErrUnauthorized
}

func TestAdminRequestContextHasBoundedLifetime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &adminDeadlineRepo{}
	service, err := adminusecase.NewService(repo, adminRoutePasswords{})
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewAdminRouter(service, config.AdminConfig{Enabled: true, PublicURL: "https://meet.example:8081"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, parentLimit := range []time.Duration{time.Hour, time.Second} {
		ctx, cancel := context.WithTimeout(context.Background(), parentLimit)
		before := time.Now()
		req := httptest.NewRequest("GET", domain.CookiePath+"/auth/me", nil).WithContext(ctx)
		req.AddCookie(&http.Cookie{Name: domain.CookieName, Value: strings.Repeat("A", 43)})
		router.ServeHTTP(httptest.NewRecorder(), req)
		deadline, bounded := repo.observed.Deadline()
		if !bounded || deadline.After(before.Add(26*time.Second)) || deadline.After(before.Add(parentLimit)) {
			cancel()
			t.Fatal("repository request context has no bounded deadline or extends its parent")
		}
		select {
		case <-repo.observed.Done():
		default:
			cancel()
			t.Fatal("request context retained after handler return")
		}
		cancel()
	}
}

func (r *adminRouteRepo) LoginAllowed(context.Context, string, string) (bool, error) {
	return true, nil
}
func (r *adminRouteRepo) ByUsername(_ context.Context, v string) (domain.Administrator, error) {
	if v == r.account.Username {
		return r.account, nil
	}
	return domain.Administrator{}, apperrors.ErrNotFound
}
func (r *adminRouteRepo) Audit(context.Context, domain.AuditInput) error { return nil }
func (r *adminRouteRepo) CreateSession(_ context.Context, s domain.Session, _ string, _ domain.AuditInput) error {
	r.session = s
	r.revoked = false
	return nil
}
func (r *adminRouteRepo) Authenticate(_ context.Context, h string) (domain.Administrator, error) {
	if r.session.TokenHash == h && !r.revoked && r.account.Status == "active" {
		return r.account, nil
	}
	return domain.Administrator{}, apperrors.ErrUnauthorized
}
func (r *adminRouteRepo) RevokeSession(context.Context, string, domain.AuditInput) error {
	r.revoked = true
	return nil
}
func TestAdminHTTPBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &adminRouteRepo{account: domain.Administrator{ID: "00000000-0000-4000-8000-000000000001", Username: "admin", PasswordHash: "hash:password12345", Status: "active"}}
	service, err := adminusecase.NewService(repo, adminRoutePasswords{})
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewAdminRouter(service, config.AdminConfig{Enabled: true, PublicURL: "https://meet.example:8081"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body, origin, csrf string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	login := `{"username":"admin","password":"password12345"}`
	for _, origin := range []string{"", "https://evil.example", "https://meet.example"} {
		if w := request("POST", domain.CookiePath+"/auth/login", login, origin, "", nil); w.Code != 403 {
			t.Fatalf("untrusted origin allowed: %d", w.Code)
		}
	}
	if w := request("POST", domain.CookiePath+"/auth/login", `{"username":"admin@example.com","password":"password12345"}`, "https://meet.example:8081", "", nil); w.Code != 401 {
		t.Fatal("email login accepted", w.Code)
	}
	w := request("POST", domain.CookiePath+"/auth/login", login, "https://meet.example:8081", "", nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing isolated cookie")
	}
	cookie := cookies[0]
	if cookie.Name != domain.CookieName || cookie.Path != domain.CookiePath || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Domain != "" {
		t.Fatalf("unsafe cookie %+v", cookie)
	}
	if bytes.Contains(w.Body.Bytes(), []byte("password12345")) || bytes.Contains(w.Body.Bytes(), []byte(cookie.Value)) {
		t.Fatal("credentials exposed")
	}
	var result adminusecase.LoginResult
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if w = request("GET", domain.CookiePath+"/auth/me", "", "", "", &http.Cookie{Name: "meetspace_session", Value: cookie.Value}); w.Code != 401 {
		t.Fatal("user cookie accepted")
	}
	if w = request("GET", domain.CookiePath+"/auth/me", "", "", "", cookie); w.Code != 200 {
		t.Fatal("admin cookie rejected")
	}
	// Neither the isolated cookie nor its raw value grants a public user identity.
	publicTokens, e := security.NewTokenService("synthetic-public-jwt-signing-secret-long-enough")
	if e != nil {
		t.Fatal(e)
	}
	userJWT, e := publicTokens.Issue("00000000-0000-4000-8000-000000000002")
	if e != nil {
		t.Fatal(e)
	}
	public := gin.New()
	public.GET("/api/v1/auth/me", httpmiddleware.Authenticate(publicTokens), func(c *gin.Context) { c.Status(200) })
	for _, bearer := range []string{"", cookie.Value, userJWT} {
		req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
		req.AddCookie(cookie)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		public.ServeHTTP(rec, req)
		want := 401
		if bearer == userJWT {
			want = 200
		}
		if rec.Code != want {
			t.Fatalf("public identity boundary status=%d want=%d", rec.Code, want)
		}
	}
	req := httptest.NewRequest("GET", domain.CookiePath+"/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+userJWT)
	rejected := httptest.NewRecorder()
	router.ServeHTTP(rejected, req)
	if rejected.Code != 401 {
		t.Fatal("user JWT granted admin identity")
	}
	if w = request("GET", domain.CookiePath+"/auth/me", "", "", "", &http.Cookie{Name: domain.CookieName, Value: userJWT}); w.Code != 401 {
		t.Fatal("user JWT accepted as admin cookie")
	}
	if w = request("POST", domain.CookiePath+"/auth/logout", "{}", "https://meet.example:8081", "forged", cookie); w.Code != 403 {
		t.Fatal("forged csrf accepted")
	}
	if w = request("POST", domain.CookiePath+"/auth/logout", "{}", "https://meet.example:8081", result.CSRFToken, cookie); w.Code != 204 {
		t.Fatal("logout failed", w.Code)
	}
	if w = request("GET", domain.CookiePath+"/auth/me", "", "", "", cookie); w.Code != 401 {
		t.Fatal("revoked cookie accepted")
	}
	disabled, err := NewAdminRouter(service, config.AdminConfig{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	disabled.ServeHTTP(rec, httptest.NewRequest("GET", domain.CookiePath+"/auth/me", nil))
	if rec.Code != 404 {
		t.Fatal("disabled admin accessible")
	}
}

type adminFailingAuditRepo struct {
	adminRouteRepo
	attempted   bool
	independent bool
}

func (r *adminFailingAuditRepo) Audit(ctx context.Context, _ domain.AuditInput) error {
	r.attempted = true
	deadline, bounded := ctx.Deadline()
	r.independent = ctx.Err() == nil && bounded && time.Until(deadline) <= time.Second
	<-ctx.Done()
	return ctx.Err()
}
func TestAdminFailureAuditIsBoundedAndPreservesResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	token := strings.Repeat("A", 43)
	repo := &adminFailingAuditRepo{adminRouteRepo: adminRouteRepo{
		account: domain.Administrator{ID: "00000000-0000-4000-8000-000000000001", Username: "operator", Status: "active"},
		session: domain.Session{TokenHash: adminusecase.HashToken(token)},
	}}
	svc, err := adminusecase.NewService(repo, adminRoutePasswords{})
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewAdminRouter(svc, config.AdminConfig{Enabled: true, PublicURL: "https://meet.example:8081"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("PATCH", domain.CookiePath+"/administrators/invalid-id", strings.NewReader(`{"name":"Operator"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://meet.example:8081")
	req.Header.Set("X-CSRF-Token", adminusecase.CSRFToken(token))
	req.AddCookie(&http.Cookie{Name: domain.CookieName, Value: token})
	rec := httptest.NewRecorder()
	start := time.Now()
	router.ServeHTTP(rec, req)
	if rec.Code != 400 || !repo.attempted || !repo.independent || time.Since(start) > 2*time.Second {
		t.Fatalf("failure audit changed response, inherited cancellation or exceeded deadline: status=%d attempted=%v independent=%v", rec.Code, repo.attempted, repo.independent)
	}
}
