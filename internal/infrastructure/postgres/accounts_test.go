package postgres_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	authapp "github.com/janickiy/meet-space/internal/app/auth"
	"github.com/janickiy/meet-space/internal/domain/personal"
	"github.com/janickiy/meet-space/internal/domain/users"
	pg "github.com/janickiy/meet-space/internal/infrastructure/postgres"
	"github.com/janickiy/meet-space/internal/infrastructure/security"
	transport "github.com/janickiy/meet-space/internal/transport/http"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	authusecase "github.com/janickiy/meet-space/internal/usecase/auth"
)

func TestBrowserAccountsAndErasure(t *testing.T) {
	db := searchDB(t)
	ctx := context.Background()
	repo := pg.NewUserRepository(db)
	tokens, err := security.NewTokenService(strings.Repeat("isolated-test-key", 3))
	searchMust(t, err)
	service, err := authusecase.NewService(repo, security.PasswordHasher{}, tokens)
	searchMust(t, err)
	service.WithSessions(pg.NewAuthSessionRepository(db))
	router := gin.New()
	server := httptest.NewServer(router)
	defer server.Close()
	transport.RegisterPlatformRoutes(router, authapp.NewHandler(service).WithSessionCookies(server.URL, false), nil, middleware.Authenticate(service))
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	request := func(method, path, token string, body any, status int) []byte {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, server.URL+"/api/v1/auth"+path, bytes.NewReader(raw))
		req.Header.Set("Origin", server.URL)
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := client.Do(req)
		searchMust(t, err)
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		if response.StatusCode != status {
			t.Fatalf("%s %s: expected %d got %d: %s", method, path, status, response.StatusCode, data)
		}
		for _, cookie := range response.Cookies() {
			if !cookie.HttpOnly || cookie.Path != "/api/v1/auth" || cookie.SameSite != http.SameSiteLaxMode {
				t.Fatal("insecure account cookie")
			}
		}
		if bytes.Contains(data, []byte("SessionToken")) || bytes.Contains(data, []byte("password_hash")) {
			t.Fatal("secret leaked in JSON")
		}
		return data
	}
	password := "test-password-123"
	var a, b users.LoginResponse
	for index, out := range []*users.LoginResponse{&a, &b} {
		email := uuid.NewString() + "@example.invalid"
		request("POST", "/register", "", map[string]string{"email": email, "password": password, "displayName": []string{"Первый", "Второй"}[index]}, 201)
		searchMust(t, json.Unmarshal(request("POST", "/login", "", map[string]string{"email": email, "password": password}, 200), out))
	}
	request("GET", "/me", a.AccessToken, nil, 200) // Adding B keeps A's session.
	// Upgrade an already signed-in browser which only has the original cookie.
	reqURL, _ := http.NewRequest("GET", server.URL+"/api/v1/auth/me", nil)
	jar.SetCookies(reqURL.URL, []*http.Cookie{{Name: "meetrix_account_" + b.User.ID, Value: "", Path: "/api/v1/auth", MaxAge: -1}})
	request("POST", "/refresh", "", map[string]string{"userId": b.User.ID}, 200)
	request("POST", "/refresh", "", map[string]string{"userId": uuid.NewString()}, 403)
	var list struct{ Items []users.View }
	searchMust(t, json.Unmarshal(request("POST", "/accounts/list", "", map[string]any{}, 200), &list))
	if len(list.Items) != 2 {
		t.Fatal("previous account was lost")
	}
	var switched users.LoginResponse
	searchMust(t, json.Unmarshal(request("POST", "/accounts/switch", "", map[string]string{"userId": a.User.ID}, 200), &switched))
	if switched.User.ID != a.User.ID {
		t.Fatal("wrong account selected")
	}
	var renewed users.LoginResponse
	searchMust(t, json.Unmarshal(request("POST", "/refresh", "", map[string]string{"userId": b.User.ID}, 200), &renewed))
	if renewed.User.ID != b.User.ID {
		t.Fatal("refresh silently switched identity")
	}
	var active users.LoginResponse
	searchMust(t, json.Unmarshal(request("POST", "/refresh", "", map[string]any{}, 200), &active))
	if active.User.ID != a.User.ID {
		t.Fatal("background refresh changed active account")
	}
	request("POST", "/accounts/switch", "", map[string]string{"userId": uuid.NewString()}, 401)
	// A cookie secret cannot be relabelled as another identity.
	forged := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/auth/accounts/switch", strings.NewReader(`{"userId":"`+a.User.ID+`"}`))
	req.Header.Set("Origin", server.URL)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "meetrix_account_" + a.User.ID, Value: cookieValue(jar, server.URL, b.User.ID)})
	router.ServeHTTP(forged, req)
	if forged.Code != 403 {
		t.Fatal("relabeled cookie switched account")
	}
	cross := httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/v1/auth/accounts/switch", strings.NewReader(`{"userId":"`+a.User.ID+`"}`))
	req.Header.Set("Origin", "https://evil.invalid")
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(cross, req)
	if cross.Code != 403 {
		t.Fatal("cross-origin selection permitted")
	}

	oldA := a.AccessToken
	searchMust(t, json.Unmarshal(request("POST", "/login", "", map[string]string{"email": a.User.Email, "password": password}, 200), &a))
	request("GET", "/me", oldA, nil, 401)
	request("GET", "/me", b.AccessToken, nil, 200)
	request("DELETE", "/me", a.AccessToken, map[string]string{"password": "wrong"}, 422)
	request("GET", "/me", a.AccessToken, nil, 200)
	personalRepo := pg.NewPersonalRepository(db)
	group, _, err := personalRepo.CreateGroup(ctx, a.User.ID, personal.CreateGroupRequest{Name: "test ownership", ClientRequestID: uuid.NewString()})
	searchMust(t, err)
	request("DELETE", "/me", a.AccessToken, map[string]string{"password": password}, 409)
	searchMust(t, db.Exec("UPDATE conversations SET deleted_at=clock_timestamp() WHERE id=?", group.ID).Error)
	searchMust(t, db.Exec("UPDATE conversation_members SET left_at=clock_timestamp() WHERE conversation_id=?", group.ID).Error)
	direct, _, err := personalRepo.GetOrCreate(ctx, a.User.ID, b.User.ID)
	searchMust(t, err)
	message := searchSend(t, pg.NewDirectChatRepository(db), a.User.ID, direct.ID, "shared history", "")
	legacy, err := tokens.Issue(a.User.ID)
	searchMust(t, err)
	request("DELETE", "/me", a.AccessToken, map[string]string{"password": password}, 200)
	request("GET", "/me", a.AccessToken, nil, 401)
	request("GET", "/me", legacy, nil, 401)
	request("POST", "/refresh", "", map[string]string{"userId": a.User.ID}, 401)
	request("POST", "/login", "", map[string]string{"email": a.User.Email, "password": password}, 401)
	request("GET", "/me", b.AccessToken, nil, 200)
	var erased users.User
	searchMust(t, db.Where("id=?", a.User.ID).Take(&erased).Error)
	if erased.PasswordHash != "" || erased.Email == a.User.Email || erased.DisplayName == nil || *erased.DisplayName != "Удалённый аккаунт" {
		t.Fatal("personal identity was retained")
	}
	var count int64
	searchMust(t, db.Table("conversation_messages").Where("id=? AND text='shared history'", message.ID).Count(&count).Error)
	if count != 1 {
		t.Fatal("peer history removed")
	}
	peers, err := personalRepo.Search(ctx, b.User.ID, "Удалённый")
	searchMust(t, err)
	if len(peers) != 0 {
		t.Fatal("deleted identity discoverable")
	}
	// A brand-new account is physically removed, including all of its sessions.
	empty, err := service.Register(ctx, users.RegisterRequest{Email: uuid.NewString() + "@example.invalid", Password: password})
	searchMust(t, err)
	signed, err := service.Login(ctx, users.LoginRequest{Email: empty.Email, Password: password})
	searchMust(t, err)
	searchMust(t, service.DeleteAccount(ctx, empty.ID, password))
	searchMust(t, db.Table("users").Where("id=?", empty.ID).Count(&count).Error)
	if count != 0 {
		t.Fatal("empty account not removed")
	}
	if _, _, _, _, err := service.VerifyAuthorization(ctx, signed.AccessToken); err == nil {
		t.Fatal("deleted empty identity authorised")
	}
	// Re-registering a deleted email gets an unrelated identity and no old history.
	replacement, err := service.Register(ctx, users.RegisterRequest{Email: a.User.Email, Password: password})
	searchMust(t, err)
	if replacement.ID == a.User.ID {
		t.Fatal("old identity resurrected")
	}
	for n := 0; n < 5; n++ {
		email := uuid.NewString() + "@example.invalid"
		request("POST", "/register", "", map[string]string{"email": email, "password": password}, 201)
		status := 200
		if n == 4 {
			status = 409
		}
		request("POST", "/login", "", map[string]string{"email": email, "password": password}, status)
	}
	searchMust(t, json.Unmarshal(request("POST", "/accounts/list", "", map[string]any{}, 200), &list))
	if len(list.Items) != 5 {
		t.Fatal("account limit changed browser identities")
	}
	request("POST", "/accounts/switch", "", map[string]string{"userId": b.User.ID}, 200)
	request("POST", "/logout", b.AccessToken, map[string]string{"userId": b.User.ID}, 200)
	request("POST", "/accounts/switch", "", map[string]string{"userId": b.User.ID}, 401)
	request("GET", "/me", b.AccessToken, nil, 401)
	searchMust(t, json.Unmarshal(request("POST", "/accounts/list", "", map[string]any{}, 200), &list))
	if len(list.Items) != 4 {
		t.Fatal("logout removed other saved identities")
	}
}

func cookieValue(jar http.CookieJar, origin, id string) string {
	req, _ := http.NewRequest("GET", origin+"/api/v1/auth/me", nil)
	for _, cookie := range jar.Cookies(req.URL) {
		if cookie.Name == "meetrix_account_"+id {
			return cookie.Value
		}
	}
	return ""
}
