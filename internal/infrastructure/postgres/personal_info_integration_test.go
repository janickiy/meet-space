package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	chatapp "github.com/janickiy/meet-space/internal/app/chat"
	personalapp "github.com/janickiy/meet-space/internal/app/personal"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/personal"
	"github.com/janickiy/meet-space/internal/domain/users"
	httptransport "github.com/janickiy/meet-space/internal/transport/http"
)

func TestConversationProfileRequiresActiveSharedMembership(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	repo := NewPersonalRepository(db)
	owner, peer, outsider := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, id := range []string{owner, peer, outsider} {
		name := "Profile " + id
		if err := db.Create(&users.User{ID: id, Email: id + "@example.invalid", PasswordHash: "synthetic", DisplayName: &name}).Error; err != nil {
			t.Fatal(err)
		}
	}
	registeredAt := time.Date(2023, time.March, 14, 11, 30, 0, 0, time.UTC)
	if err := db.Exec("UPDATE users SET bio=?,created_at=? WHERE id=?", "О себе\nСсылка https://example.invalid", registeredAt, peer).Error; err != nil {
		t.Fatal(err)
	}
	direct, _, err := repo.GetOrCreate(ctx, owner, peer)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := repo.ConversationProfile(ctx, owner, direct.ID, peer)
	if err != nil || profile.ID != peer || profile.Email != peer+"@example.invalid" || profile.Bio != "О себе\nСсылка https://example.invalid" || profile.DisplayName != "Profile "+peer {
		t.Fatalf("wrong scoped profile: %+v, %v", profile, err)
	}
	if !profile.CreatedAt.Equal(registeredAt) {
		t.Fatalf("profile registration date = %s, want %s", profile.CreatedAt, registeredAt)
	}
	if _, err := repo.ConversationProfile(ctx, outsider, direct.ID, peer); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("unrelated caller accessed profile", err)
	}
	if _, err := repo.ConversationProfile(ctx, owner, direct.ID, outsider); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("unrelated target exposed", err)
	}
	// Search results remain the minimal directory projection.
	peers, err := repo.Search(ctx, owner, "Profile")
	encoded, marshalErr := json.Marshal(peers)
	if err != nil || marshalErr != nil || strings.Contains(string(encoded), "email") || strings.Contains(string(encoded), "bio") {
		t.Fatal("directory projection changed", err, marshalErr)
	}
	for _, field := range []string{"is_blocked", "ldap_disabled"} {
		if err := db.Exec("UPDATE users SET "+field+"=true WHERE id=?", peer).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := repo.ConversationProfile(ctx, owner, direct.ID, peer); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("inactive target (%s) exposed: %v", field, err)
		}
		if _, err := repo.ConversationProfile(ctx, peer, direct.ID, owner); !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatalf("inactive actor (%s) allowed: %v", field, err)
		}
		if err := db.Exec("UPDATE users SET "+field+"=false WHERE id=?", peer).Error; err != nil {
			t.Fatal(err)
		}
	}
	group, _, err := repo.CreateGroup(ctx, owner, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Profiles", MemberIDs: []string{peer}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ConversationProfile(ctx, peer, group.ID, owner); err != nil {
		t.Fatal("member cannot see group owner", err)
	}
	if err := repo.LeaveGroup(ctx, peer, group.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ConversationProfile(ctx, owner, group.ID, peer); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("left member profile exposed", err)
	}
	if _, err := repo.ConversationProfile(ctx, peer, group.ID, owner); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("left member still has access", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	auth := func(c *gin.Context) { c.Set("authenticated_user_id", owner); c.Next() }
	httptransport.RegisterPersonalRoutes(router, &personalapp.Handler{Repo: repo}, &chatapp.Handler{}, auth, nil)
	for _, tc := range []struct {
		path string
		code int
	}{
		{"/api/v1/conversations/" + direct.ID + "/profiles/" + peer, 200},
		{"/api/v1/conversations/" + direct.ID + "/profiles/" + outsider, 404},
		{"/api/v1/conversations/" + direct.ID + "/profiles/not-a-uuid", 422},
		{"/api/v1/conversations/" + direct.ID + "/materials?kind=unknown", 422},
		{"/api/v1/conversations/" + direct.ID + "/materials?kind=media&limit=101", 422},
		{"/api/v1/conversations/" + direct.ID + "/materials?kind=media&limit=0", 422},
		{"/api/v1/conversations/" + direct.ID + "/materials?kind=media&before=garbage", 422},
	} {
		r := httptest.NewRecorder()
		router.ServeHTTP(r, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if r.Code != tc.code || r.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("%s: %d %s", tc.path, r.Code, r.Body.String())
		}
	}
	if _, err := repo.HideDirectConversation(ctx, owner, direct.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ConversationProfile(ctx, owner, direct.ID, peer); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("hidden chat still exposes profile", err)
	}
}

func TestConversationMaterialsPaginationAndVisibility(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	owner, peer, outsider := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, id := range []string{owner, peer, outsider} {
		if err := db.Create(&users.User{ID: id, Email: id + "@example.invalid", PasswordHash: "synthetic"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := NewPersonalRepository(db)
	chatRepo := NewDirectChatRepository(db)
	direct, _, err := repo.GetOrCreate(ctx, owner, peer)
	if err != nil {
		t.Fatal(err)
	}
	send := func(id, text string) chat.Message {
		t.Helper()
		req, fingerprint, err := chat.NormalizeSend(chat.SendRequest{ClientRequestID: uuid.NewString(), Text: text})
		if err != nil {
			t.Fatal(err)
		}
		message, _, err := chatRepo.Send(ctx, owner, id, req, fingerprint)
		if err != nil {
			t.Fatal(err)
		}
		return message
	}
	old := send(direct.ID, "Old https://example.invalid/old")
	if _, err := repo.ClearDirectHistory(ctx, peer, direct.ID); err != nil {
		t.Fatal(err)
	}
	message := send(direct.ID, "Ссылки https://example.invalid/first, HTTPS://example.invalid/second.")
	fileIDs := make([]string, 0, 4)
	for _, mime := range []string{"image/png", "image/jpeg", "video/mp4", "application/pdf"} {
		id := uuid.NewString()
		fileIDs = append(fileIDs, id)
		if err := db.Exec(`INSERT INTO conversation_attachments(id,conversation_id,owner_user_id,client_request_id,filename,mime_type,size,status,message_id,expires_at)
			VALUES(?,?,?,?,?,?,1,'attached',?,now()+interval '1 day')`, id, direct.ID, owner, uuid.NewString(), "material", mime, message.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	var mediaIDs []string
	cursor := ""
	for i := 0; i < 4; i++ {
		page, err := repo.ConversationMaterials(ctx, peer, direct.ID, "media", cursor, 1)
		if err != nil || len(page.Items) != 1 || page.Items[0].Attachment == nil || page.Items[0].Message.ID != message.ID {
			t.Fatalf("media page %d: %+v %v", i, page, err)
		}
		mediaIDs = append(mediaIDs, page.Items[0].Attachment.ID)
		if i == 0 {
			if _, err := repo.ConversationMaterials(ctx, peer, direct.ID, "file", page.NextCursor, 1); !errors.Is(err, apperrors.ErrInvalidInput) {
				t.Fatal("cross-tab cursor accepted", err)
			}
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if len(mediaIDs) != 3 || strings.Join(mediaIDs, ",") != strings.Join(fileIDs[:3], ",") {
		t.Fatal("media pagination lost order or repeated files", mediaIDs)
	}
	files, err := repo.ConversationMaterials(ctx, peer, direct.ID, "file", "", 50)
	if err != nil || len(files.Items) != 1 || files.Items[0].Attachment.ID != fileIDs[3] {
		t.Fatal("file tab includes media or loses files", files, err)
	}
	links, err := repo.ConversationMaterials(ctx, peer, direct.ID, "link", "", 1)
	if err != nil || len(links.Items) != 1 || links.Items[0].URL != "https://example.invalid/first" || links.NextCursor == "" {
		t.Fatal("first link page", links, err)
	}
	links, err = repo.ConversationMaterials(ctx, peer, direct.ID, "link", links.NextCursor, 1)
	if err != nil || len(links.Items) != 1 || links.Items[0].URL != "HTTPS://example.invalid/second" || links.NextCursor != "" {
		t.Fatal("link pagination crossed cleared history", links, err)
	}
	ownerLinks, err := repo.ConversationMaterials(ctx, owner, direct.ID, "link", "", 50)
	if err != nil || len(ownerLinks.Items) != 3 || ownerLinks.Items[2].Message.ID != old.ID {
		t.Fatal("one user's history clear affected another user", ownerLinks, err)
	}
	for _, kind := range []string{"media", "file", "link"} {
		if _, err := repo.ConversationMaterials(ctx, outsider, direct.ID, kind, "", 50); !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatal("outsider materials allowed", kind, err)
		}
	}
	if _, err := chatRepo.Delete(ctx, owner, direct.ID, message.ID); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"media", "file", "link"} {
		page, err := repo.ConversationMaterials(ctx, peer, direct.ID, kind, "", 50)
		if err != nil || len(page.Items) != 0 || page.NextCursor != "" {
			t.Fatal("deleted message still has materials", kind, page, err)
		}
	}
	group, _, err := repo.CreateGroup(ctx, owner, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Media", MemberIDs: []string{peer}})
	if err != nil {
		t.Fatal(err)
	}
	send(group.ID, "https://example.invalid/group")
	if page, err := repo.ConversationMaterials(ctx, peer, group.ID, "link", "", 50); err != nil || len(page.Items) != 1 {
		t.Fatal("group materials missing", page, err)
	}
	if err := repo.LeaveGroup(ctx, peer, group.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ConversationMaterials(ctx, peer, group.ID, "link", "", 50); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("left group member materials allowed", err)
	}
}
