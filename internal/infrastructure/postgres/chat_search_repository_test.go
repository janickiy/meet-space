package postgres_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	chatapp "github.com/janickiy/meet-space/internal/app/chat"
	personalapp "github.com/janickiy/meet-space/internal/app/personal"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/personal"
	"github.com/janickiy/meet-space/internal/domain/users"
	pg "github.com/janickiy/meet-space/internal/infrastructure/postgres"
	transport "github.com/janickiy/meet-space/internal/transport/http"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	chatusecase "github.com/janickiy/meet-space/internal/usecase/chat"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func searchDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MEETSPACE_SEARCH_TEST_DSN")
	if dsn == "" {
		t.Skip("set MEETSPACE_SEARCH_TEST_DSN for isolated PostgreSQL integration tests")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil || config.Database != "meetspace_search_test" || (config.Host != "127.0.0.1" && config.Host != "localhost") {
		t.Fatal("integration DSN must target loopback database meetspace_search_test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	searchMust(t, err)
	pool, err := db.DB()
	searchMust(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	_, path, _, _ := runtime.Caller(0)
	searchMust(t, pg.RunMigrations(db, filepath.Join(filepath.Dir(path), "../../../database/migrations")))
	// Every fixture mutation rolls back; migrations remain only in the explicit test database.
	tx := db.Begin()
	searchMust(t, tx.Error)
	t.Cleanup(func() { _ = tx.Rollback().Error })
	return tx
}
func searchMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func searchUser(t *testing.T, db *gorm.DB, name string) string {
	t.Helper()
	id := uuid.NewString()
	searchMust(t, db.Create(&users.User{ID: id, Email: id + "@example.invalid", PasswordHash: "!test", DisplayName: &name}).Error)
	return id
}
func searchSend(t *testing.T, repo *pg.ChatRepository, user, scope, text, reply string) chat.Message {
	t.Helper()
	request, fingerprint, err := chat.NormalizeSend(chat.SendRequest{ClientRequestID: uuid.NewString(), Text: text, ReplyTo: reply})
	searchMust(t, err)
	message, _, err := repo.Send(context.Background(), user, scope, request, fingerprint)
	searchMust(t, err)
	return message
}
func searchCursor(scope string, sequence int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(scope + ":" + strconv.FormatInt(sequence, 10)))
}
func searchIDs(items []chat.Message) []string {
	result := []string{}
	for _, item := range items {
		result = append(result, item.ID)
	}
	return result
}
func searchHas(items []chat.Message, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func TestConversationMessageSearchAndContext(t *testing.T) {
	db := searchDB(t)
	ctx := context.Background()
	repo := pg.NewDirectChatRepository(db)
	personalRepo := pg.NewPersonalRepository(db)
	a, b, outsider := searchUser(t, db, "Поиск автора"), searchUser(t, db, "Собеседник"), searchUser(t, db, "Посторонний")
	direct, _, err := personalRepo.GetOrCreate(ctx, a, b)
	searchMust(t, err)
	first := searchSend(t, repo, a, direct.ID, "Привет, Вселенная", "")
	second := searchSend(t, repo, b, direct.ID, "ПРИВЕТ, второй", "")
	literal := searchSend(t, repo, b, direct.ID, `Скидка 50% a_b x!y C:\path 😀😀`, "")
	searchSend(t, repo, b, direct.ID, `Скидка 500 aXb xyy C:path`, "")
	deleted := searchSend(t, repo, b, direct.ID, "привет удалённый", "")
	_, err = repo.Delete(ctx, b, direct.ID, deleted.ID)
	searchMust(t, err)

	t.Run("literal unicode and descending cursor", func(t *testing.T) {
		page, err := repo.SearchMessagePage(ctx, a, direct.ID, "прИвет", "", 1)
		searchMust(t, err)
		if len(page.Items) != 1 || page.Items[0].ID != second.ID || page.NextCursor == "" || page.UnreadCount != 3 || page.LastReadMessageID != nil {
			t.Fatalf("wrong search page: %+v", page)
		}
		next, err := repo.SearchMessagePage(ctx, a, direct.ID, "прИвет", page.NextCursor, 1)
		searchMust(t, err)
		if len(next.Items) != 1 || next.Items[0].ID != first.ID || next.NextCursor != "" {
			t.Fatalf("cursor skipped/repeated messages: %+v", next)
		}
		for _, query := range []string{"50%", "a_b", "x!y", `\path`, "😀😀"} {
			found, err := repo.SearchMessagePage(ctx, a, direct.ID, query, "", 20)
			searchMust(t, err)
			if len(found.Items) != 1 || found.Items[0].ID != literal.ID {
				t.Fatalf("query %q was not literal: %v", query, searchIDs(found.Items))
			}
		}
		page, err = repo.SearchMessagePage(ctx, a, direct.ID, "Поиск автора", "", 20)
		searchMust(t, err)
		if len(page.Items) != 0 {
			t.Fatal("sender name must not match a text search")
		}
		for _, cursor := range []string{"!", searchCursor(uuid.NewString(), second.Sequence), searchCursor(direct.ID, -1), strings.Repeat("x", 129), base64.RawURLEncoding.EncodeToString([]byte(direct.ID + ":999999999999999999999"))} {
			_, err := repo.SearchMessagePage(ctx, a, direct.ID, "привет", cursor, 20)
			if !errors.Is(err, apperrors.ErrInvalidInput) {
				t.Fatalf("unsafe cursor accepted: %v", err)
			}
		}
		if _, err := repo.ChatMessageContext(ctx, a, direct.ID, deleted.ID); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("deleted target visible: %v", err)
		}
		if _, err := repo.SearchMessagePage(ctx, outsider, direct.ID, "привет", "", 20); !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatalf("outsider search allowed: %v", err)
		}
		if _, err := repo.ChatMessageContext(ctx, outsider, direct.ID, first.ID); !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatalf("outsider context allowed: %v", err)
		}
	})

	t.Run("personal cleared history and hidden reply protection", func(t *testing.T) {
		_, err := personalRepo.ClearDirectHistory(ctx, a, direct.ID)
		searchMust(t, err)
		reply := searchSend(t, repo, b, direct.ID, "привет после очистки", first.ID)
		page, err := repo.SearchMessagePage(ctx, a, direct.ID, "привет", "", 20)
		searchMust(t, err)
		if len(page.Items) != 1 || page.Items[0].ID != reply.ID || page.Items[0].ReplyTo != nil || page.Items[0].ReplyPreview != nil || page.UnreadCount != 1 {
			t.Fatalf("cleared history leaked through results/reply: %+v", page)
		}
		older, err := repo.SearchMessagePage(ctx, a, direct.ID, "привет", searchCursor(direct.ID, second.Sequence), 20)
		searchMust(t, err)
		if len(older.Items) != 0 {
			t.Fatal("forged old sequence bypassed cutoff")
		}
		window, err := repo.ChatMessageContext(ctx, a, direct.ID, reply.ID)
		searchMust(t, err)
		if len(window.Items) != 1 || window.Items[0].ID != reply.ID || window.NextCursor != "" || window.Items[0].ReplyPreview != nil {
			t.Fatalf("context crossed clear frontier: %+v", window)
		}
		if _, err := repo.ChatMessageContext(ctx, a, direct.ID, first.ID); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("cleared target visible: %v", err)
		}
		peer, err := repo.SearchMessagePage(ctx, b, direct.ID, "привет", "", 20)
		searchMust(t, err)
		if len(peer.Items) != 3 {
			t.Fatal("one user's clear affected peer")
		}
		_, err = personalRepo.HideDirectConversation(ctx, a, direct.ID)
		searchMust(t, err)
		if _, err := repo.SearchMessagePage(ctx, a, direct.ID, "привет", "", 20); !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatalf("hidden chat allowed: %v", err)
		}
		if _, err := repo.ChatMessageContext(ctx, a, direct.ID, reply.ID); !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatalf("hidden context allowed: %v", err)
		}
		_, _, err = personalRepo.GetOrCreate(ctx, a, b)
		searchMust(t, err)
		empty, err := repo.SearchMessagePage(ctx, a, direct.ID, "привет", "", 20)
		searchMust(t, err)
		if len(empty.Items) != 0 {
			t.Fatal("unhide restored cleared messages")
		}
	})

	t.Run("group membership join remove leave and deletion", func(t *testing.T) {
		group, _, err := personalRepo.CreateGroup(ctx, a, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Search group", MemberIDs: []string{b}})
		searchMust(t, err)
		message := searchSend(t, repo, a, group.ID, "группа ищет сообщение", "")
		denied := func(user string) {
			t.Helper()
			if _, err := repo.SearchMessagePage(ctx, user, group.ID, "группа", "", 20); !errors.Is(err, apperrors.ErrForbidden) {
				t.Fatalf("group search membership bypass: %v", err)
			}
			if _, err := repo.ChatMessageContext(ctx, user, group.ID, message.ID); !errors.Is(err, apperrors.ErrForbidden) {
				t.Fatalf("group context membership bypass: %v", err)
			}
		}
		denied(outsider)
		_, _, err = personalRepo.AddGroupMembers(ctx, a, group.ID, []string{outsider})
		searchMust(t, err)
		page, err := repo.SearchMessagePage(ctx, outsider, group.ID, "группа", "", 20)
		searchMust(t, err)
		if len(page.Items) != 1 || page.Items[0].ID != message.ID {
			t.Fatal("joined member must have the same history policy as List")
		}
		_, _, err = personalRepo.RemoveGroupMember(ctx, a, group.ID, outsider)
		searchMust(t, err)
		denied(outsider)
		_, _, err = personalRepo.AddGroupMembers(ctx, a, group.ID, []string{outsider})
		searchMust(t, err)
		_, err = repo.ChatMessageContext(ctx, outsider, group.ID, message.ID)
		searchMust(t, err)
		searchMust(t, personalRepo.LeaveGroup(ctx, outsider, group.ID))
		denied(outsider)
		// Shared cutoff handling also applies to a group membership if a frontier is set.
		searchMust(t, db.Table("conversation_members").Where("conversation_id=? AND user_id=?", group.ID, b).Update("history_cleared_through", message.Sequence).Error)
		page, err = repo.SearchMessagePage(ctx, b, group.ID, "группа", "", 20)
		searchMust(t, err)
		if len(page.Items) != 0 {
			t.Fatal("group cutoff ignored")
		}
		if _, err = repo.ChatMessageContext(ctx, b, group.ID, message.ID); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatal("group cleared target allowed")
		}
		_, err = personalRepo.DeleteGroup(ctx, a, group.ID)
		searchMust(t, err)
		denied(a)
	})

	t.Run("focused bounded context and ordinary older paging", func(t *testing.T) {
		conversation, _, err := personalRepo.GetOrCreate(ctx, a, outsider)
		searchMust(t, err)
		messages := []chat.Message{}
		for i := 0; i < 80; i++ {
			messages = append(messages, searchSend(t, repo, a, conversation.ID, fmt.Sprintf("needle %02d", i), ""))
		}
		_, err = repo.Delete(ctx, a, conversation.ID, messages[40].ID)
		searchMust(t, err)
		window, err := repo.ChatMessageContext(ctx, outsider, conversation.ID, messages[42].ID)
		searchMust(t, err)
		if len(window.Items) != 50 || window.Items[0].ID != messages[18].ID || window.Items[49].ID != messages[67].ID || !searchHas(window.Items, messages[42].ID) || window.NextCursor == "" {
			t.Fatalf("context is not a bounded focused window: %v", searchIDs(window.Items))
		}
		for i, message := range window.Items {
			if i > 0 && window.Items[i-1].Sequence >= message.Sequence {
				t.Fatal("context not chronological")
			}
			if message.ID == messages[40].ID && (message.DeletedAt == nil || message.Text != "" || len(message.Attachments) != 0) {
				t.Fatal("deleted neighbor not redacted")
			}
		}
		earlier, err := repo.List(ctx, outsider, conversation.ID, window.NextCursor, 100)
		searchMust(t, err)
		if len(earlier.Items) != 18 || earlier.Items[17].ID != messages[17].ID || earlier.NextCursor != "" {
			t.Fatal("context cursor is incompatible with ordinary paging")
		}
		for _, message := range earlier.Items {
			if searchHas(window.Items, message.ID) {
				t.Fatal("context older page overlaps")
			}
		}
		boundary, err := repo.ChatMessageContext(ctx, outsider, conversation.ID, messages[0].ID)
		searchMust(t, err)
		if len(boundary.Items) != 26 || boundary.NextCursor != "" {
			t.Fatal("first-message context must not emit a cursor before available history")
		}
		if _, err := repo.ChatMessageContext(ctx, a, conversation.ID, first.ID); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("foreign target escaped scope: %v", err)
		}
	})

	t.Run("conversation HTTP routes and validation", func(t *testing.T) {
		message := searchSend(t, repo, b, direct.ID, "Новый HTTP результат", "")
		gin.SetMode(gin.TestMode)
		router := gin.New()
		handler := &personalapp.Handler{Repo: personalRepo}
		auth := middleware.Authenticate(searchTokens{})
		transport.RegisterPersonalRoutes(router, handler, chatapp.NewHandler(nil).ForConversations(), auth, nil)
		transport.RegisterPersonalSearchRoutes(router, &chatapp.SearchHandler{Service: &chatusecase.SearchService{Repo: repo}}, auth, handler.AccountOnly, nil)
		base := "/api/v1/conversations/" + direct.ID + "/messages"
		request := func(user, path string, want int) map[string]json.RawMessage {
			t.Helper()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			if user != "" {
				req.Header.Set("Authorization", "Bearer "+user)
			}
			out := httptest.NewRecorder()
			router.ServeHTTP(out, req)
			if out.Code != want {
				t.Fatalf("%s: got %d want %d: %s", path, out.Code, want, out.Body.String())
			}
			if out.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("private response cache headers missing")
			}
			body := map[string]json.RawMessage{}
			searchMust(t, json.Unmarshal(out.Body.Bytes(), &body))
			return body
		}
		result := request(a, base+"/search?q="+url.QueryEscape("  HTTP "), 200)
		for _, key := range []string{"status", "items", "nextCursor", "unreadCount", "lastReadMessageId"} {
			if _, ok := result[key]; !ok {
				t.Fatalf("ChatPage field %s missing", key)
			}
		}
		contextPage := request(a, base+"/"+message.ID+"/context", 200)
		if !strings.Contains(string(contextPage["items"]), message.ID) {
			t.Fatal("focused target missing")
		}
		request("", base+"/search?q=HTTP", 401)
		request(outsider, base+"/search?q=HTTP", 403)
		request(a, base+"/search?q=%00HTTP", 422)
		request(a, base+"/search?q=%FF%FE", 422)
		request(a, base+"/search?q=x", 422)
		request(a, base+"/search?q=HTTP&limit=0", 422)
		request(a, base+"/search?q=HTTP&limit=51", 422)
		request(a, base+"/search?q=HTTP&limit=no", 422)
		request(a, base+"/search?q=HTTP&before=bad", 422)
		request(a, base+"/invalid/context", 422)
		request(a, base+"/"+first.ID+"/context", 404)
	})

	t.Run("conference search and context remain compatible", func(t *testing.T) {
		conferenceRepo := pg.NewConferenceRepository(db)
		conferenceID := uuid.NewString()
		started := time.Now().UTC()
		conference, err := conferenceRepo.Create(ctx, conferences.Conference{ID: conferenceID, OwnerID: a, Kind: "meeting", Title: "Search conference", InviteCode: strings.ReplaceAll(uuid.NewString(), "-", ""), Status: conferences.Active, StartedAt: &started}, conferences.Participant{ID: uuid.NewString(), ConferenceID: conferenceID, UserID: &a, DisplayName: "Owner", Role: conferences.Owner, Status: conferences.Joined, AdmissionState: conferences.AdmissionAdmitted, JoinedAt: &started})
		searchMust(t, err)
		conferenceChat := pg.NewChatRepository(db)
		message := searchSend(t, conferenceChat, a, conference.ID, "Проверка конференции 50%", "")
		searchMust(t, conferenceChat.SetChatPin(ctx, a, conference.ID, message.ID, true))
		found, err := conferenceChat.SearchMessages(ctx, a, conference.ID, "конференции 50%", "", 20)
		searchMust(t, err)
		if len(found.Items) != 1 || found.Items[0].ID != message.ID || !found.Items[0].Important || found.Items[0].ConferenceID != conference.ID || found.Items[0].ConversationID != "" {
			t.Fatal("conference search projection regressed")
		}
		window, err := conferenceChat.ChatMessageContext(ctx, a, conference.ID, message.ID)
		searchMust(t, err)
		if len(window.Items) != 1 || !window.Items[0].Important {
			t.Fatal("conference context projection regressed")
		}
		if _, err := conferenceChat.SearchMessages(ctx, outsider, conference.ID, "конференции", "", 20); !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatalf("conference outsider search allowed: %v", err)
		}
		if _, err := conferenceChat.ChatMessageContext(ctx, outsider, conference.ID, message.ID); !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatalf("conference outsider context allowed: %v", err)
		}
		// Even a forged conversation membership cannot turn a guest into an account reader.
		guest := searchUser(t, db, "Guest")
		searchMust(t, db.Table("users").Where("id=?", guest).Update("guest_conference_id", conference.ID).Error)
		group, _, err := personalRepo.CreateGroup(ctx, a, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Private", MemberIDs: []string{b}})
		searchMust(t, err)
		searchMust(t, db.Exec("INSERT INTO conversation_members(conversation_id,user_id,role) VALUES(?,?,'member')", group.ID, guest).Error)
		if _, err := repo.SearchMessagePage(ctx, guest, group.ID, "конференции", "", 20); !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatal("guest gained conversation search access")
		}
	})
}

type searchTokens struct{}

func (searchTokens) Verify(token string) (string, error) {
	id, err := uuid.Parse(token)
	if err != nil || id == uuid.Nil {
		return "", apperrors.ErrUnauthorized
	}
	return id.String(), nil
}
