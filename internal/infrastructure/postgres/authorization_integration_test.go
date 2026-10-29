package postgres

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/personal"
	"github.com/janickiy/meet-space/internal/domain/users"
	"github.com/janickiy/meet-space/internal/infrastructure/security"
	authusecase "github.com/janickiy/meet-space/internal/usecase/auth"
	assets "github.com/janickiy/meet-space/internal/usecase/personal"
	postgresdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// auditDatabase accepts only an explicit loopback-only disposable database.
// It creates a uniquely named schema, applies real migrations and drops only
// that schema afterwards; it never loads the application's environment file.
func auditDatabase(t *testing.T, throughVersion ...int) *gorm.DB {
	t.Helper()
	raw := os.Getenv("MEETSPACE_AUDIT_POSTGRES_DSN")
	if raw == "" {
		t.Skip("set MEETSPACE_AUDIT_POSTGRES_DSN to a disposable loopback meetspace_audit database")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "postgres" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || u.Path != "/meetspace_audit" {
		t.Fatal("audit database must be postgres://...@127.0.0.1:<port>/meetspace_audit")
	}
	for key := range u.Query() {
		if key != "sslmode" {
			t.Fatal("audit DSN only allows sslmode; connection overrides are forbidden")
		}
	}
	admin, err := gorm.Open(postgresdriver.Open(raw), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal("cannot open disposable audit database")
	}
	adminSQL, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminSQL.Close() })
	var existing int64
	if err := admin.Raw(`SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname NOT LIKE 'pg_%' AND n.nspname <> 'information_schema'
		AND c.relkind IN ('r', 'p', 'v', 'm', 'f')`).Scan(&existing).Error; err != nil || existing != 0 {
		t.Fatal("audit requires an empty disposable database without user relations")
	}
	schema := "audit_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Errorf("remove audit schema: %v", err)
		}
	})
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	db, err := gorm.Open(postgresdriver.Open(u.String()), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal("cannot open audit schema")
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(2)
	t.Cleanup(func() { _ = sqlDB.Close() })
	directory := filepath.Join("..", "..", "..", "database", "migrations")
	if len(throughVersion) > 0 {
		files, err := migrationFiles(directory)
		if err != nil {
			t.Fatal(err)
		}
		directory = t.TempDir()
		for _, file := range files {
			version, err := strconv.Atoi(strings.SplitN(file.name, "_", 2)[0])
			if err != nil {
				t.Fatal(err)
			}
			if version <= throughVersion[0] {
				if err := os.WriteFile(filepath.Join(directory, file.name), []byte(file.sql), 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if err := RunMigrations(db, directory); err != nil {
		t.Fatal(err)
	}
	return db
}

type auditPasswords struct{}

func (auditPasswords) Hash(string) (string, error) { return "synthetic-not-a-login-hash", nil }
func (auditPasswords) Verify(password, hash string) (bool, error) {
	return password == "synthetic-password" && hash == "synthetic-not-a-login-hash", nil
}

// TestAuditSessionRevocation validates durable session binding and logout with
// real PostgreSQL repositories and signed JWTs, using no real account secrets.
func TestAuditSessionRevocation(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	user, other := uuid.NewString(), uuid.NewString()
	for _, id := range []string{user, other} {
		if err := db.Create(&users.User{ID: id, Email: id + "@example.invalid", PasswordHash: "synthetic-not-a-login-hash"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	tokens, err := security.NewTokenService("synthetic-audit-secret-32-bytes-long")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := authusecase.NewService(NewUserRepository(db), auditPasswords{}, tokens)
	if err != nil {
		t.Fatal(err)
	}
	auth.WithSessions(NewAuthSessionRepository(db))
	login, err := auth.Login(ctx, users.LoginRequest{Email: user + "@example.invalid", Password: "synthetic-password"})
	if err != nil {
		t.Fatal(err)
	}
	identity, _, session, _, err := auth.VerifyAuthorization(ctx, login.AccessToken)
	if err != nil || identity != user || session == "" {
		t.Fatalf("initial session verification: identity=%q session=%q error=%v", identity, session, err)
	}
	if _, err := auth.Refresh(ctx, login.SessionToken); err != nil {
		t.Fatal(err)
	}
	foreign, err := tokens.IssueSession(other, session)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := auth.VerifyAuthorization(ctx, foreign); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatalf("cross-user session binding error=%v, want unauthorized", err)
	}
	if err := auth.Logout(ctx, login.SessionToken, login.AccessToken); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := auth.VerifyAuthorization(ctx, login.AccessToken); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatalf("revoked access token error=%v, want unauthorized", err)
	}
	if _, err := auth.Refresh(ctx, login.SessionToken); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatalf("revoked refresh cookie error=%v, want unauthorized", err)
	}
}

// TestAuditRepositoryAuthorization exercises server-side ownership and revoked
// membership boundaries against actual PostgreSQL queries, not mocked SQL.
func TestAuditRepositoryAuthorization(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	owner, member, outsider := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for i, id := range []string{owner, member, outsider} {
		user := users.User{ID: id, Email: []string{"owner@example.invalid", "member@example.invalid", "outsider@example.invalid"}[i], PasswordHash: "synthetic-not-a-login-hash"}
		if err := db.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
	}
	meeting := uuid.NewString()
	if err := db.Exec("INSERT INTO conferences(id,owner_id,title,invite_code,status,started_at) VALUES(?,?,'Audit meeting',?,'active',now())", meeting, owner, strings.Repeat("a", 32)).Error; err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{owner, member} {
		role := []string{"owner", "participant"}[i]
		if err := db.Exec("INSERT INTO conference_participants(id,conference_id,user_id,display_name,role,status,admission_state,joined_at) VALUES(?,?,?,'Audit user',?,'joined','admitted',now())", uuid.NewString(), meeting, id, role).Error; err != nil {
			t.Fatal(err)
		}
	}
	recordings := NewConferenceRecordingRepository(db)
	record, _, err := recordings.Start(ctx, owner, meeting, 5)
	if err != nil {
		t.Fatal(err)
	}
	meetingChat := NewChatRepository(db)
	request, fingerprint, err := chat.NormalizeSend(chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "Synthetic private message"})
	if err != nil {
		t.Fatal(err)
	}
	message, _, err := meetingChat.Send(ctx, owner, meeting, request, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	checkDenied := func(t *testing.T, err error) {
		t.Helper()
		if !errors.Is(err, apperrors.ErrForbidden) && !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("access error = %v, want forbidden or not found", err)
		}
	}
	t.Run("recording_member_allowed", func(t *testing.T) {
		if _, err := recordings.Accessible(ctx, member, meeting, record.UUID); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("recording_outsider_denied", func(t *testing.T) {
		_, err := recordings.Accessible(ctx, outsider, meeting, record.UUID)
		checkDenied(t, err)
	})
	t.Run("recording_stop_non_owner_denied", func(t *testing.T) {
		_, err := recordings.Stop(ctx, member, meeting, record.UUID)
		checkDenied(t, err)
	})
	t.Run("meeting_outsider_cannot_finish", func(t *testing.T) {
		_, err := NewConferenceRepository(db).Transition(ctx, outsider, meeting, conferences.Finished)
		checkDenied(t, err)
	})
	t.Run("chat_outsider_denied", func(t *testing.T) {
		_, err := meetingChat.List(ctx, outsider, meeting, "", 20)
		checkDenied(t, err)
	})
	t.Run("chat_other_sender_edit_denied", func(t *testing.T) {
		_, err := meetingChat.Edit(ctx, member, meeting, message.ID, "changed")
		checkDenied(t, err)
	})
	people := NewPersonalRepository(db)
	direct, _, err := people.GetOrCreate(ctx, owner, member)
	if err != nil {
		t.Fatal(err)
	}
	group, _, err := people.CreateGroup(ctx, owner, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Audit group", MemberIDs: []string{member}})
	if err != nil {
		t.Fatal(err)
	}
	for _, conversation := range []personal.Conversation{direct, group} {
		t.Run(conversation.Type+"_outsider_denied", func(t *testing.T) {
			_, err := NewDirectChatRepository(db).List(ctx, outsider, conversation.ID, "", 20)
			checkDenied(t, err)
		})
	}
	t.Run("group_member_cannot_promote_self", func(t *testing.T) {
		_, err := people.ChangeGroupRole(ctx, member, group.ID, member, personal.Admin)
		checkDenied(t, err)
	})
	t.Run("group_avatar_outsider_denied", func(t *testing.T) {
		err := NewPersonalAssetRepository(db).BeginAvatar(ctx, outsider, group.ID, assets.Asset{Version: uuid.NewString()})
		checkDenied(t, err)
	})
	t.Run("attachment_other_owner_denied", func(t *testing.T) {
		a, _, err := meetingChat.InitAttachment(ctx, owner, meeting, chat.InitRequest{ClientRequestID: uuid.NewString(), Filename: "audit.txt", MimeType: "text/plain", Size: 5})
		if err != nil {
			t.Fatal(err)
		}
		_, err = meetingChat.ClaimUpload(ctx, member, meeting, a.ID, uuid.NewString())
		checkDenied(t, err)
		_, err = meetingChat.DownloadAttachment(ctx, outsider, meeting, a.ID)
		checkDenied(t, err)
	})
	t.Run("folder_other_owner_denied", func(t *testing.T) {
		folders := NewFolderRepository(db)
		folder, err := folders.Create(ctx, owner, "Audit folder")
		if err != nil {
			t.Fatal(err)
		}
		_, err = folders.Get(ctx, outsider, folder.ID)
		checkDenied(t, err)
		_, err = folders.Rename(ctx, outsider, folder.ID, "Changed")
		checkDenied(t, err)
	})
	t.Run("group_removed_member_denied", func(t *testing.T) {
		if _, _, err := people.RemoveGroupMember(ctx, owner, group.ID, member); err != nil {
			t.Fatal(err)
		}
		_, err := NewDirectChatRepository(db).List(ctx, member, group.ID, "", 20)
		checkDenied(t, err)
		_, _, err = NewPersonalAssetRepository(db).GroupAttachment(ctx, member, group.ID, uuid.NewString())
		checkDenied(t, err)
	})
	t.Run("kicked_meeting_member_denied", func(t *testing.T) {
		if err := db.Exec("UPDATE conference_participants SET status='kicked',admission_state='kicked' WHERE conference_id=? AND user_id=?", meeting, member).Error; err != nil {
			t.Fatal(err)
		}
		_, err := meetingChat.List(ctx, member, meeting, "", 20)
		checkDenied(t, err)
		_, err = recordings.Accessible(ctx, member, meeting, record.UUID)
		checkDenied(t, err)
	})
}
