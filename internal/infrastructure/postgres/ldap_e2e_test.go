// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	authapp "github.com/janickiy/meet-space/internal/app/auth"
	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/directory"
	"github.com/janickiy/meet-space/internal/domain/users"
	ldapinfra "github.com/janickiy/meet-space/internal/infrastructure/ldap"
	"github.com/janickiy/meet-space/internal/infrastructure/security"
	httptransport "github.com/janickiy/meet-space/internal/transport/http"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	adminusecase "github.com/janickiy/meet-space/internal/usecase/admin"
	authusecase "github.com/janickiy/meet-space/internal/usecase/auth"
	directoryusecase "github.com/janickiy/meet-space/internal/usecase/directory"
)

type ldapHTTPResources struct {
	settingsHTTPResources
	directory *directoryusecase.Service
}

func (r ldapHTTPResources) DirectoryStatus(ctx context.Context) (directory.SyncStatus, error) {
	return r.directory.Status(ctx)
}
func (r ldapHTTPResources) DirectoryTest(ctx context.Context, revision int64, _ admin.AuditInput) (directory.TestResult, error) {
	return r.directory.Test(ctx, revision)
}
func (r ldapHTTPResources) DirectorySync(ctx context.Context, revision int64, audit admin.AuditInput) error {
	return r.directory.Enqueue(ctx, true, audit, revision)
}

// Проверка реальных HTTP-обработчиков, PostgreSQL и локального LLDAP. Членство Bob
// временно меняется и восстанавливается; рабочая БД приложения не используется.
func TestLocalLDAPEndToEnd(t *testing.T) {
	if os.Getenv("LDAP_DEV_E2E") != "1" {
		t.Skip("set LDAP_DEV_E2E=1 and disposable MEETSPACE_AUDIT_POSTGRES_DSN")
	}
	secrets := filepath.Join("..", "..", "..", "tools", "ldap-dev", "secrets")
	raw, err := os.ReadFile(filepath.Join(secrets, "credentials.json"))
	if err != nil {
		t.Fatal("read local LDAP test credentials:", err)
	}
	var credentials map[string]string
	if err := json.Unmarshal(raw, &credentials); err != nil {
		t.Fatal(err)
	}
	ca, err := os.ReadFile(filepath.Join(secrets, "tls", "ca.crt"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ldapClient := ldapinfra.NewClient()
	// Этот тест меняет членство в каталоге. Допускаем только исходную тестовую группу:
	// расширенный стенд с другими пользователями остаётся полностью нетронутым.
	pristine, err := ldapClient.Scan(ctx, directory.Config{
		Mode: directory.ModeGeneric, ServerURL: "ldaps://127.0.0.1:6360",
		BaseDN: "dc=meetspace,dc=test", UsersDN: "ou=people,dc=meetspace,dc=test",
		BindDN: "uid=svc-meetspace,ou=people,dc=meetspace,dc=test", BindPassword: credentials["svc-meetspace"],
		UserFilter: "(objectClass=person)", LoginAttribute: "uid", IDAttribute: "entryUUID", EmailAttribute: "mail", NameAttribute: "cn",
		GroupDN: "cn=meetspace-users,ou=groups,dc=meetspace,dc=test", GroupMembershipAttribute: "memberOf",
		CA: string(ca), SyncIntervalMinutes: 60, TimeoutSeconds: 10,
	})
	if err != nil {
		t.Fatal("read local LDAP precondition:", err)
	}
	allowed := map[string]bool{}
	for _, entry := range pristine {
		allowed[entry.Login] = true
	}
	if len(pristine) != 2 || !allowed["alice"] || !allowed["bob"] {
		t.Skip("LDAP mutation test requires pristine meetspace-users group containing only alice and bob")
	}
	db, settings, _, actor, _ := settingsFixture(t)
	local, err := NewUserRepository(db).Create(ctx, users.User{ID: uuid.NewString(), Email: "local-ldap-e2e@example.invalid", PasswordHash: "synthetic-not-a-login-hash"})
	if err != nil {
		t.Fatal(err)
	}
	service := directoryusecase.New(settings, ldapClient, NewDirectoryRepository(db), NewDirectorySyncRepository(db))
	adminRepo := NewAdminRepository(db)
	token := strings.Repeat("A", 43)
	if err := adminRepo.CreateSession(ctx, admin.Session{ID: uuid.NewString(), AdministratorID: actor.AdministratorID, TokenHash: adminusecase.HashToken(token), CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}, "synthetic", actor); err != nil {
		t.Fatal(err)
	}
	adminAuth, err := adminusecase.NewService(adminRepo, adminTestPasswords{})
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router, err := httptransport.NewAdminRouter(adminAuth, config.AdminConfig{Enabled: true, PublicURL: "https://ldap-test.invalid:8081"}, ldapHTTPResources{settingsHTTPResources{service: settings}, service}, nil)
	if err != nil {
		t.Fatal(err)
	}
	adminRequest := func(method, path string, body any, csrf bool) *httptest.ResponseRecorder {
		encoded, _ := json.Marshal(body)
		req := httptest.NewRequest(method, admin.CookiePath+path, bytes.NewReader(encoded))
		req.Header.Set("Origin", "https://ldap-test.invalid:8081")
		req.Header.Set("Content-Type", "application/json")
		if csrf {
			req.Header.Set("X-CSRF-Token", adminusecase.CSRFToken(token))
		}
		req.AddCookie(&http.Cookie{Name: admin.CookieName, Value: token})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	changes := map[string]any{
		"ldapEnabled": true, "ldapMode": "generic", "ldapServerURL": "ldaps://127.0.0.1:6360",
		"ldapBaseDN": "dc=meetspace,dc=test", "ldapUsersDN": "ou=people,dc=meetspace,dc=test",
		"ldapBindDN": "uid=svc-meetspace,ou=people,dc=meetspace,dc=test", "ldapBindPassword": credentials["svc-meetspace"],
		"ldapUserFilter": "(objectClass=person)", "ldapLoginAttribute": "uid", "ldapIDAttribute": "entryUUID",
		"ldapEmailAttribute": "mail", "ldapNameAttribute": "cn", "ldapGroupDN": "cn=meetspace-users,ou=groups,dc=meetspace,dc=test",
		"ldapGroupMembershipAttribute": "memberOf", "ldapNestedGroups": false, "ldapCA": string(ca), "ldapSyncIntervalMinutes": 60,
	}
	saved := adminRequest("PATCH", "/settings", map[string]any{"revision": 0, "changes": changes}, true)
	if saved.Code != 200 {
		t.Fatalf("save LDAP settings: HTTP %d", saved.Code)
	}
	if strings.Contains(saved.Body.String(), credentials["svc-meetspace"]) {
		t.Fatal("bind password exposed in admin response")
	}
	if response := adminRequest("POST", "/directory/test", map[string]int{"revision": 1}, false); response.Code != 403 {
		t.Fatal("directory test accepted without CSRF")
	}
	if response := adminRequest("POST", "/directory/test", map[string]int{"revision": 99}, true); response.Code != 409 {
		t.Fatal("directory test accepted stale revision")
	}
	checked := adminRequest("POST", "/directory/test", map[string]int{"revision": 1}, true)
	var probe directory.TestResult
	if checked.Code != 200 || json.Unmarshal(checked.Body.Bytes(), &probe) != nil || !probe.Reachable || probe.MatchedUsers != 2 {
		t.Fatalf("LDAP test did not see two allowed users: HTTP %d", checked.Code)
	}
	jobsRepo := NewJobRepository(db)
	syncNow := func(revision int64, wantTotal int, wantFailure bool) {
		t.Helper()
		res := adminRequest("POST", "/directory/sync", map[string]int64{"revision": revision}, true)
		if res.Code != 200 {
			t.Fatalf("enqueue sync: HTTP %d", res.Code)
		}
		job, ok, err := jobsRepo.Claim(ctx, directory.SyncJobKind, time.Minute)
		if err != nil || !ok {
			t.Fatal("claim sync:", err)
		}
		err = service.Handle(ctx, job)
		if (err != nil) != wantFailure {
			t.Fatalf("sync failure=%v, expected=%v", err, wantFailure)
		}
		state := "succeeded"
		if wantFailure {
			state = "failed"
		}
		jobState := "done"
		if wantFailure {
			jobState = "failed"
		}
		if err := jobsRepo.Finish(ctx, job, jobState, "", nil); err != nil {
			t.Fatal(err)
		}
		statusResponse := adminRequest("GET", "/directory/status", nil, false)
		var status directory.SyncStatus
		if statusResponse.Code != 200 || json.Unmarshal(statusResponse.Body.Bytes(), &status) != nil || status.Status != state || status.Total != wantTotal || status.Queued {
			t.Fatalf("unexpected sync status: %+v HTTP %d", status, statusResponse.Code)
		}
	}
	syncNow(1, 2, false)
	var imported int64
	if err := db.Model(&users.User{}).Where("auth_source='ldap'").Count(&imported).Error; err != nil || imported != 2 {
		t.Fatal("sync did not import both users before login")
	}
	tokens, err := security.NewTokenService("local-ldap-e2e-signing-secret-32-bytes")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := authusecase.NewService(NewUserRepository(db), auditPasswords{}, tokens)
	if err != nil {
		t.Fatal(err)
	}
	auth.WithSessions(NewAuthSessionRepository(db)).WithLDAP(service).WithAuthenticationPolicy(settings)
	handler := authapp.NewHandler(auth).WithSessionCookies("https://meet-ldap.invalid", true).WithLDAPProviders(service)
	public := gin.New()
	public.GET("/api/v1/auth/providers", handler.Providers)
	public.POST("/api/v1/auth/ldap/login", handler.LDAPLogin)
	public.POST("/api/v1/auth/login", handler.Login)
	public.POST("/api/v1/auth/refresh", handler.Refresh)
	public.GET("/api/v1/auth/me", httpmiddleware.Authenticate(auth), handler.Me)
	request := func(method, path string, body any, access string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://meet-ldap.invalid")
		if access != "" {
			req.Header.Set("Authorization", "Bearer "+access)
		}
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		res := httptest.NewRecorder()
		public.ServeHTTP(res, req)
		return res
	}
	login := func(email, password string, expected int) (users.LoginResponse, []*http.Cookie) {
		t.Helper()
		res := request("POST", "/api/v1/auth/login", users.LoginRequest{Email: email, Password: password}, "", nil)
		if res.Code != expected {
			t.Fatalf("email login %s: HTTP %d, expected %d", email, res.Code, expected)
		}
		var response users.LoginResponse
		if expected == 200 {
			if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil || response.User.AuthSource != "ldap" {
				t.Fatal("incorrect login response")
			}
			found := false
			for _, cookie := range res.Result().Cookies() {
				if cookie.Name == authapp.SessionCookieName {
					found = cookie.HttpOnly && cookie.Secure
				}
			}
			if !found {
				t.Fatal("missing secure HTTP-only session cookie")
			}
		}
		return response, res.Result().Cookies()
	}
	login("alice@meetspace.test", "wrong-password", 401)
	login("outsider@meetspace.test", credentials["outsider"], 401)
	alice, aliceCookies := login("alice@meetspace.test", credentials["alice"], 200)
	bob, bobCookies := login("bob@meetspace.test", credentials["bob"], 200)
	if request("GET", "/api/v1/auth/me", nil, bob.AccessToken, nil).Code != 200 || request("POST", "/api/v1/auth/refresh", map[string]any{}, "", bobCookies).Code != 200 {
		t.Fatal("LDAP SID cannot use /me or refresh")
	}
	if request("POST", "/api/v1/auth/login", users.LoginRequest{Email: "alice", Password: credentials["alice"]}, "", nil).Code != 401 {
		t.Fatal("unified email form accepted a bare LDAP username")
	}
	if request("POST", "/api/v1/auth/ldap/login", users.LDAPLoginRequest{Login: "alice", Password: credentials["alice"]}, "", nil).Code != 200 {
		t.Fatal("legacy LDAP endpoint lost username compatibility in LDAP mode")
	}
	if request("POST", "/api/v1/auth/login", users.LoginRequest{Email: local.Email, Password: "synthetic-password"}, "", nil).Code != 401 {
		t.Fatal("LDAP-only mode accepted an existing local account")
	}

	setMembership := localLLDAPGroupMembership(t, credentials["admin"])
	t.Cleanup(func() { setMembership("alice", true) })
	t.Cleanup(func() { setMembership("bob", true) })
	setMembership("bob", false)
	syncNow(1, 1, false)
	meCode := request("GET", "/api/v1/auth/me", nil, bob.AccessToken, nil).Code
	refreshCode := request("POST", "/api/v1/auth/refresh", map[string]any{}, "", bobCookies).Code
	denied := func(code int) bool { return code == 401 || code == 403 }
	if !denied(meCode) || !denied(refreshCode) {
		t.Fatalf("removed user retained SID access: me=%d refresh=%d", meCode, refreshCode)
	}
	if request("GET", "/api/v1/auth/me", nil, alice.AccessToken, nil).Code != 200 {
		t.Fatal("sync revoked unaffected user")
	}
	login("bob@meetspace.test", credentials["bob"], 401)
	setMembership("alice", false)
	syncNow(1, 0, false)
	if !denied(request("GET", "/api/v1/auth/me", nil, alice.AccessToken, nil).Code) {
		t.Fatal("empty complete group did not revoke last user")
	}
	setMembership("alice", true)
	setMembership("bob", true)
	syncNow(1, 2, false)
	restored, _ := login("bob@meetspace.test", credentials["bob"], 200)
	if restored.User.ID != bob.User.ID {
		t.Fatal("rejoined user lost original MeetSpace ID")
	}
	if request("POST", "/api/v1/auth/refresh", map[string]any{}, "", aliceCookies).Code != 401 {
		t.Fatal("rejoining resurrected old SID")
	}
	res := adminRequest("PATCH", "/settings", map[string]any{"revision": 1, "changes": map[string]any{"ldapServerURL": "ldaps://127.0.0.1:6361"}}, true)
	if res.Code != 200 {
		t.Fatal("save unavailable endpoint failed")
	}
	syncNow(2, 0, true)
	if request("POST", "/api/v1/auth/login", users.LoginRequest{Email: local.Email, Password: "synthetic-password"}, "", nil).Code != 503 {
		t.Fatal("LDAP outage fell back to a local account password")
	}
	var disabled int64
	if err := db.Model(&users.User{}).Where("auth_source='ldap' AND ldap_disabled").Count(&disabled).Error; err != nil || disabled != 0 {
		t.Fatal("directory outage disabled valid profiles")
	}
	res = adminRequest("PATCH", "/settings", map[string]any{"revision": 2, "changes": map[string]any{"ldapEnabled": false}}, true)
	if res.Code != 200 {
		t.Fatal("disable LDAP failed")
	}
	login("alice@meetspace.test", credentials["alice"], 401)
	if request("POST", "/api/v1/auth/ldap/login", users.LDAPLoginRequest{Login: "alice", Password: credentials["alice"]}, "", nil).Code != 403 {
		t.Fatal("legacy LDAP endpoint bypassed local-only mode")
	}
	if request("GET", "/api/v1/auth/me", nil, restored.AccessToken, nil).Code != 401 {
		t.Fatal("disabled LDAP retained authorization")
	}
	localLogin, err := auth.Login(ctx, users.LoginRequest{Email: local.Email, Password: "synthetic-password"})
	if err != nil || localLogin.User.ID != local.ID {
		t.Fatal("preserved local account cannot sign in after selecting local mode", err)
	}
	if request("GET", "/api/v1/auth/me", nil, localLogin.AccessToken, nil).Code != 200 {
		t.Fatal("fresh local session cannot use /me in local mode")
	}
	t.Log("LDAPS settings, CSRF/revision, import, unified email login/SID, group removal, stable identity, exclusive modes and no local fallback passed")
}

func localLLDAPGroupMembership(t *testing.T, password string) func(string, bool) {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	post := func(path, token string, body any, result any) {
		t.Helper()
		encoded, _ := json.Marshal(body)
		req, err := http.NewRequest("POST", "http://127.0.0.1:17170"+path, bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal("local LLDAP test management unavailable")
		}
		defer res.Body.Close()
		if res.StatusCode != 200 || json.NewDecoder(res.Body).Decode(result) != nil {
			t.Fatal("local LLDAP management request failed")
		}
	}
	var session struct {
		Token string `json:"token"`
	}
	post("/auth/simple/login", "", map[string]string{"username": "admin", "password": password}, &session)
	var groups struct {
		Data struct {
			Groups []struct {
				ID          int
				DisplayName string
			}
		}
		Errors []json.RawMessage
	}
	post("/api/graphql", session.Token, map[string]string{"query": "{groups{id displayName}}"}, &groups)
	groupID := 0
	for _, group := range groups.Data.Groups {
		if group.DisplayName == "meetspace-users" {
			groupID = group.ID
		}
	}
	if groupID == 0 || len(groups.Errors) != 0 {
		t.Fatal("local test group not found")
	}
	return func(user string, member bool) {
		t.Helper()
		var state struct {
			Data struct {
				Users []struct {
					ID     string
					Groups []struct{ ID int }
				}
			}
			Errors []json.RawMessage
		}
		post("/api/graphql", session.Token, map[string]string{"query": "{users{id groups{id}}}"}, &state)
		if len(state.Errors) != 0 {
			t.Fatal("read membership before restoration failed")
		}
		for _, candidate := range state.Data.Users {
			if candidate.ID != user {
				continue
			}
			current := false
			for _, group := range candidate.Groups {
				if group.ID == groupID {
					current = true
				}
			}
			if current == member {
				return
			}
		}
		operation := "removeUserFromGroup"
		if member {
			operation = "addUserToGroup"
		}
		var result struct{ Errors []json.RawMessage }
		post("/api/graphql", session.Token, map[string]any{"query": fmt.Sprintf("mutation($user:String!,$group:Int!){%s(userId:$user,groupId:$group){ok}}", operation), "variables": map[string]any{"user": user, "group": groupID}}, &result)
		if len(result.Errors) != 0 {
			t.Fatalf("cannot restore/change local membership for %s", user)
		}
	}
}
