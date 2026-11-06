package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/records"
	"github.com/janickiy/meet-space/internal/domain/users"
	"github.com/janickiy/meet-space/internal/infrastructure/security"
	authusecase "github.com/janickiy/meet-space/internal/usecase/auth"
)

func TestAuditAdminResourceMetadataAndSafeDelete(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	repo := NewAdminRepository(db)
	actor := admin.Administrator{ID: uuid.NewString(), Username: "operator", Name: "Operator", PasswordHash: "synthetic", Status: "active"}
	if _, err := repo.Bootstrap(ctx, actor); err != nil {
		t.Fatal(err)
	}
	audit := admin.AuditInput{AdministratorID: actor.ID, Username: actor.Username, IP: "127.0.0.1"}
	owner, meeting, ready, busy := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO users(id,email,password_hash,display_name) VALUES(?,?,'synthetic','Owner')", owner, owner+"@example.invalid")
	exec("INSERT INTO conferences(id,owner_id,title,invite_code,status,started_at) VALUES(?,?,'Планирование продукта',?,'active',now())", meeting, owner, strings.Repeat("z", 32))
	exec("INSERT INTO conference_participants(id,conference_id,user_id,display_name,role,status,admission_state,joined_at) VALUES(?,?,?,'Owner','owner','joined','admitted',now())", uuid.NewString(), meeting, owner)
	for _, row := range []struct{ id, status string }{{ready, "ready"}, {busy, "recording"}} {
		exec("INSERT INTO record(uuid,conference_id,platform_conference_id,mode,source_type,transport_type,status,quality_mode,size_bytes,duration_sec) VALUES(?,?,?,'composite','conference','webrtc',?,'auto',1024,42)", row.id, meeting, meeting, row.status)
	}
	exec("INSERT INTO record_file(record_id,file_type,bucket,object_key,mime_type,size_bytes) SELECT id,'final_mp4','private','records/'||uuid::text||'/private-name.mp4','video/mp4',1024 FROM record WHERE uuid=?", ready)
	// Uploaded segments consume storage too. A segment referencing the final
	// object must not count that same object a second time.
	exec("INSERT INTO record_segment(record_id,seq_no,status,object_key,size_bytes) SELECT id,1,'uploaded','records/'||uuid::text||'/segment.webm',512 FROM record WHERE uuid=?", ready)
	exec("INSERT INTO record_segment(record_id,seq_no,status,object_key,size_bytes) SELECT id,2,'uploaded','records/'||uuid::text||'/private-name.mp4',1024 FROM record WHERE uuid=?", ready)
	conferences, err := repo.Conferences(ctx, admin.Query{Q: "Планирование", Sort: "title", Order: "asc"})
	if err != nil {
		t.Fatal(err)
	}
	if conferences.Total != 1 || len(conferences.Items) != 1 || conferences.Items[0].ParticipantCount != 1 {
		t.Fatalf("conference page: %+v", conferences)
	}
	page, err := repo.Recordings(ctx, admin.Query{PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 1 {
		t.Fatalf("record page: %+v", page)
	}
	files, err := repo.FileSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if files.Count != 2 || files.Bytes == nil || *files.Bytes != 1536 || files.CapacityBytes != nil {
		t.Fatalf("file metrics: %+v", files)
	}
	dash, err := repo.Dashboard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if dash.Users.Total != 1 || dash.Conferences.Active != 1 || dash.Recordings.Active != 1 || len(dash.Activity) != 7 || dash.Recordings.Bytes == nil || *dash.Recordings.Bytes != 1536 {
		t.Fatalf("dashboard: %+v", dash)
	}
	chats, err := repo.ChatSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if chats.Messages != 0 {
		t.Fatal(chats)
	}
	encoded, _ := json.Marshal([]any{page, files, dash, chats})
	if strings.Contains(string(encoded), "private-name") || strings.Contains(string(encoded), "objectKey") || strings.Contains(string(encoded), "passwordHash") {
		t.Fatal("private persistence fields leaked")
	}
	called := false
	_, err = repo.DeleteAdminRecording(ctx, busy, audit, func(context.Context, records.Record) error { called = true; return nil })
	if !errors.Is(err, apperrors.ErrConflict) || called {
		t.Fatalf("active recording deletion: %v called=%t", err, called)
	}
	_, err = repo.DeleteAdminRecording(ctx, ready, audit, func(context.Context, records.Record) error { return errors.New("storage unavailable") })
	if err == nil {
		t.Fatal("storage failure was ignored")
	}
	files, err = repo.FileSummary(ctx)
	if err != nil || files.Count != 2 {
		t.Fatalf("failed cleanup removed metadata: %+v %v", files, err)
	}
	_, err = repo.DeleteAdminRecording(ctx, ready, audit, func(_ context.Context, r records.Record) error {
		called = true
		if r.UUID != ready {
			t.Fatal("wrong record")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	files, err = repo.FileSummary(ctx)
	if err != nil || files.Count != 0 {
		t.Fatalf("successful cleanup: %+v %v", files, err)
	}
	log, err := repo.AuditLog(ctx, admin.Query{Action: "recording.delete"})
	if err != nil || log.Total != 1 {
		t.Fatalf("deletion audit: %+v %v", log, err)
	}
	repeated, err := repo.DeleteAdminRecording(ctx, ready, audit, func(context.Context, records.Record) error { t.Fatal("duplicate delete reached storage"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if repeated.UUID != "" {
		t.Fatal("duplicate delete would emit another deletion event")
	}
}

func TestAuditUserBlockPublicAuthAndGuestCompatibility(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	repo := NewAdminRepository(db)
	actor := admin.Administrator{ID: uuid.NewString(), Username: "operator", Name: "Operator", PasswordHash: "synthetic", Status: "active"}
	if _, err := repo.Bootstrap(ctx, actor); err != nil {
		t.Fatal(err)
	}
	audit := admin.AuditInput{AdministratorID: actor.ID, Username: actor.Username}
	id := uuid.NewString()
	email := id + "@example.invalid"
	if err := db.Create(&users.User{ID: id, Email: email, PasswordHash: "synthetic-not-a-login-hash"}).Error; err != nil {
		t.Fatal(err)
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
	login := func() (users.LoginResponse, error) {
		return auth.Login(ctx, users.LoginRequest{Email: email, Password: "synthetic-password"})
	}
	session, err := login()
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := tokens.Issue(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.SetUserStatus(ctx, id, "blocked", audit); err != nil {
		t.Fatal(err)
	}
	if _, err = login(); !errors.Is(err, apperrors.ErrAccountBlocked) {
		t.Fatalf("blocked login: %v", err)
	}
	if err = auth.PasswordRecovery(ctx, " "+strings.ToUpper(email)+" "); !errors.Is(err, apperrors.ErrAccountBlocked) {
		t.Fatalf("blocked recovery: %v", err)
	}
	if _, err = auth.Register(ctx, users.RegisterRequest{Email: strings.ToUpper(email), Password: "replacement-password"}); !errors.Is(err, apperrors.ErrConflict) || err.Error() != "email is already registered" {
		t.Fatalf("blocked email registration: %v", err)
	}
	var sameEmail int64
	if err = db.Table("users").Where("email=? AND is_blocked", email).Count(&sameEmail).Error; err != nil || sameEmail != 1 {
		t.Fatalf("registration changed blocked account: count=%d err=%v", sameEmail, err)
	}
	if _, err = auth.Refresh(ctx, session.SessionToken); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatalf("blocked refresh: %v", err)
	}
	for _, token := range []string{session.AccessToken, legacy} {
		if _, _, _, _, err = auth.VerifyAuthorization(ctx, token); !errors.Is(err, apperrors.ErrAccountBlocked) {
			t.Fatalf("blocked bearer: %v", err)
		}
	}
	if _, err = repo.SetUserStatus(ctx, id, "active", audit); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{session.AccessToken, legacy} {
		if _, _, _, _, err = auth.VerifyAuthorization(ctx, token); !errors.Is(err, apperrors.ErrUnauthorized) {
			t.Fatalf("unblock revived old token: %v", err)
		}
	}
	newSession, err := login()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err = auth.VerifyAuthorization(ctx, newSession.AccessToken); err != nil {
		t.Fatal(err)
	}
	// Guest accounts use !guest hashes and remain valid until individually blocked.
	meeting, guest := uuid.NewString(), uuid.NewString()
	if err = db.Exec("INSERT INTO conferences(id,owner_id,title,invite_code,status,started_at) VALUES(?,?,'Guest test',?,'active',now())", meeting, id, strings.Repeat("g", 32)).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&users.User{ID: guest, Email: guest + "@guest.invalid", PasswordHash: "!guest", GuestConferenceID: &meeting}).Error; err != nil {
		t.Fatal(err)
	}
	guestToken, err := tokens.IssueGuest(guest, meeting)
	if err != nil {
		t.Fatal(err)
	}
	if _, scope, _, _, err := auth.VerifyAuthorization(ctx, guestToken); err != nil || scope != meeting {
		t.Fatalf("guest authorization regressed: %s %v", scope, err)
	}
}
