package httptransport

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math/rand/v2"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	profileapp "github.com/janickiy/meet-space/internal/app/profile"
	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/users"
	"github.com/janickiy/meet-space/internal/operations"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	"github.com/janickiy/meet-space/internal/usecase/profile"
)

const avatarActor = "00000000-0000-4000-8000-000000000001"

type avatarTokens struct{}

func (avatarTokens) Verify(string) (string, error) { return avatarActor, nil }
func (avatarTokens) VerifySession(raw string) (string, string, time.Time, error) {
	scope := ""
	if raw == "guest" {
		scope = "00000000-0000-4000-8000-000000000002"
	}
	return avatarActor, scope, time.Now().Add(time.Hour), nil
}

type httpAvatarRepo struct {
	data    []byte
	savedID string
}

func (r *httpAvatarRepo) GetByID(context.Context, string) (users.User, error) {
	return users.User{ID: avatarActor, PasswordHash: "synthetic"}, nil
}
func (r *httpAvatarRepo) SaveAvatar(_ context.Context, id string, data []byte) (users.User, error) {
	r.savedID = id
	r.data = data
	return users.User{ID: id}, nil
}
func (r *httpAvatarRepo) ReadAvatar(context.Context, string, string) ([]byte, error) {
	return r.data, nil
}

// TestProfileAvatarHTTP checks authentication, actor scope, private delivery and deletion.
func TestProfileAvatarHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &httpAvatarRepo{}
	router := gin.New()
	router.Use(operations.New("api", "avatar-test", config.OperationsConfig{HTTPBodyBytes: 1 << 20}, nil).Middleware())
	RegisterProfileAvatarRoutes(router, &profileapp.AvatarHandler{Service: profile.NewAvatarService(repo)}, middleware.Authenticate(avatarTokens{}), func(c *gin.Context) { c.Next() }, nil)
	var input bytes.Buffer
	if err := png.Encode(&input, image.NewRGBA(image.Rect(0, 0, 24, 24))); err != nil {
		t.Fatal(err)
	}
	request := func(method, path, token, mime string, body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if mime != "" {
			req.Header.Set("Content-Type", mime)
		}
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		return out
	}
	for _, token := range []string{"", "guest"} {
		want := 401
		if token == "guest" {
			want = 403
		}
		if out := request("PUT", "/api/v1/auth/me/avatar", token, "image/png", input.Bytes()); out.Code != want {
			t.Fatalf("token=%q code=%d", token, out.Code)
		}
	}
	if repo.savedID != "" {
		t.Fatal("unauthorized upload reached persistence")
	}
	out := request("PUT", "/api/v1/auth/me/avatar?userId=other", "account", "image/png", input.Bytes())
	if out.Code != 200 || repo.savedID != avatarActor {
		t.Fatalf("put=%d actor=%q", out.Code, repo.savedID)
	}
	out = request("GET", "/api/v1/users/"+avatarActor+"/avatar/content", "account", "", nil)
	if out.Code != 200 || out.Header().Get("Content-Type") != "image/jpeg" || out.Header().Get("Cache-Control") != "private, no-store" || out.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("read=%d headers=%v", out.Code, out.Header())
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(out.Body.Bytes())); err != nil {
		t.Fatal(err)
	}
	if out := request("PUT", "/api/v1/auth/me/avatar", "account", "image/svg+xml", []byte("<svg/>")); out.Code != 422 {
		t.Fatalf("svg=%d", out.Code)
	}
	largeImage := image.NewRGBA(image.Rect(0, 0, 900, 600))
	random := rand.New(rand.NewPCG(1, 2))
	for y := range 600 {
		for x := range 900 {
			largeImage.SetRGBA(x, y, color.RGBA{R: uint8(random.Uint32()), G: uint8(random.Uint32()), B: uint8(random.Uint32()), A: 255})
		}
	}
	var large bytes.Buffer
	if err := png.Encode(&large, largeImage); err != nil {
		t.Fatal(err)
	}
	if large.Len() <= 1<<20 || large.Len() > 2<<20 {
		t.Fatalf("large fixture bytes=%d", large.Len())
	}
	if out := request("PUT", "/api/v1/auth/me/avatar", "account", "image/png", large.Bytes()); out.Code != 200 {
		t.Fatalf("large profile image=%d", out.Code)
	}
	if out := request("PUT", "/api/v1/auth/me/avatar", "account", "image/png", make([]byte, (2<<20)+1)); out.Code != 422 {
		t.Fatalf("oversized image=%d", out.Code)
	}
	if out := request("DELETE", "/api/v1/auth/me/avatar", "account", "", nil); out.Code != 200 || repo.data != nil {
		t.Fatalf("delete=%d", out.Code)
	}
}
