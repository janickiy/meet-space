package postgres

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/records"
	"github.com/janickiy/meet-space/internal/domain/users"
	postgresdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This test owns only a fresh schema in an explicitly supplied, empty loopback
// database. Keep it self-contained so the feature can be tested on older servers.
func recordingLibraryDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	raw := os.Getenv("MEETSPACE_AUDIT_POSTGRES_DSN")
	if raw == "" {
		t.Skip("set MEETSPACE_AUDIT_POSTGRES_DSN to a disposable loopback meetspace_audit database")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "postgres" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || u.Path != "/meetspace_audit" {
		t.Fatal("test database must be postgres://...@127.0.0.1:<port>/meetspace_audit")
	}
	for key := range u.Query() {
		if key != "sslmode" {
			t.Fatal("test DSN only allows sslmode")
		}
	}
	admin, err := gorm.Open(postgresdriver.Open(raw), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal("cannot open disposable test database")
	}
	adminSQL, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminSQL.Close() })
	var existing int64
	if err := admin.Raw(`SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname NOT LIKE 'pg_%' AND n.nspname <> 'information_schema'
		AND c.relkind IN ('r','p','v','m','f')`).Scan(&existing).Error; err != nil || existing != 0 {
		t.Fatal("test requires an empty disposable database")
	}
	schema := "recording_library_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Error(err)
		}
	})
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	db, err := gorm.Open(postgresdriver.Open(u.String()), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal("cannot open test schema")
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(2)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := RunMigrations(db, filepath.Join("..", "..", "..", "database", "migrations")); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRecordingLibraryAuthorizationLifecycleAndPagination(t *testing.T) {
	db := recordingLibraryDatabase(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	owner, joined, left, waiting, kicked, rejected, outsider := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, id := range []string{owner, joined, left, waiting, kicked, rejected, outsider} {
		if err := db.Create(&users.User{ID: id, Email: id + "@example.invalid", PasswordHash: "synthetic-not-a-login-hash"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	meetingIDs := []string{}
	for _, status := range []conferences.Status{conferences.Active, conferences.Finished, conferences.Cancelled} {
		id := uuid.NewString()
		meeting := conferences.Conference{ID: id, OwnerID: owner, Kind: "meeting", Title: "Library " + string(status), InviteCode: uuid.NewString()[:32], Status: status, CreatedAt: now, UpdatedAt: now}
		started, finished := now.Add(-time.Minute*2), now.Add(-time.Minute)
		if status != conferences.Cancelled {
			meeting.StartedAt = &started
		}
		if status != conferences.Active {
			meeting.FinishedAt = &finished
		}
		if err := db.Create(&meeting).Error; err != nil {
			t.Fatal(err)
		}
		meetingIDs = append(meetingIDs, id)
		for _, member := range []struct {
			id        string
			status    conferences.ParticipantStatus
			admission conferences.AdmissionState
			role      conferences.Role
		}{
			{owner, conferences.Joined, conferences.AdmissionAdmitted, conferences.Owner},
			{joined, conferences.Joined, conferences.AdmissionAdmitted, conferences.ParticipantRole},
			{left, conferences.Left, conferences.AdmissionAdmitted, conferences.ParticipantRole},
			{waiting, conferences.Waiting, conferences.AdmissionWaiting, conferences.ParticipantRole},
			{kicked, conferences.Kicked, conferences.AdmissionKicked, conferences.ParticipantRole},
			{rejected, conferences.Rejected, conferences.AdmissionRejected, conferences.ParticipantRole},
		} {
			userID := member.id
			participant := conferences.Participant{ID: uuid.NewString(), ConferenceID: id, UserID: &userID, DisplayName: "Library fixture", Status: member.status, AdmissionState: member.admission, Role: member.role, JoinedAt: &started}
			if member.status == conferences.Left {
				participant.LeftAt = &finished
			}
			if err := db.Create(&participant).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	// Leaving only the chat must not revoke the separate recording read permission.
	if err := db.Exec("INSERT INTO conference_chat_preferences(conference_id,user_id,left_at) VALUES(?,?,?)", meetingIDs[0], left, now).Error; err != nil {
		t.Fatal(err)
	}
	createRecord := func(meeting string, mode string, at, ended time.Time) records.Record {
		t.Helper()
		platform := meeting
		record := records.Record{UUID: uuid.NewString(), Mode: mode, ConferenceID: meeting, PlatformConferenceID: &platform, SourceType: "conference", TransportType: "sfu", Status: records.StatusReady, QualityMode: "auto", CreatedAt: at, UpdatedAt: at, EndedAt: &ended}
		if mode == records.ModeLegacy {
			record.PlatformConferenceID = nil
		}
		if err := db.Create(&record).Error; err != nil {
			t.Fatal(err)
		}
		return record
	}
	expected := []records.Record{}
	for i, mode := range []string{records.ModeComposite, records.ModeAudioOnly, records.ModeIndividualTracks, records.ModeScreenFocus} {
		expected = append(expected, createRecord(meetingIDs[i%3], mode, now, now))
	}
	deleted := createRecord(meetingIDs[0], records.ModeComposite, now, now)
	if err := db.Exec("UPDATE record SET deleted_at=? WHERE id=?", now, deleted.ID).Error; err != nil {
		t.Fatal(err)
	}
	createRecord(meetingIDs[0], records.ModeComposite, now.Add(-8*24*time.Hour), now.Add(-8*24*time.Hour))
	createRecord(meetingIDs[0], records.ModeLegacy, now, now)
	privateID := uuid.NewString()
	privateStart := now.Add(-time.Minute)
	if err := db.Create(&conferences.Conference{ID: privateID, OwnerID: owner, Kind: "direct_call", Title: "Private room", InviteCode: uuid.NewString()[:32], Status: conferences.Active, StartedAt: &privateStart}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&conferences.Participant{ID: uuid.NewString(), ConferenceID: privateID, UserID: &owner,
		DisplayName: "Private room owner", Role: conferences.Owner, Status: conferences.Joined,
		AdmissionState: conferences.AdmissionAdmitted, JoinedAt: &privateStart}).Error; err != nil {
		t.Fatal(err)
	}
	createRecord(privateID, records.ModeComposite, now, now)
	r := NewConferenceRecordingRepository(db)
	for _, actor := range []string{owner, joined, left} {
		rows, err := r.Library(ctx, actor, records.LibraryQuery{Limit: 100})
		if err != nil || len(rows) != 4 {
			t.Fatalf("authorized %s: count=%d err=%v", actor, len(rows), err)
		}
		states := map[conferences.Status]bool{}
		for i, row := range rows {
			if row.Recording.ID != expected[len(expected)-1-i].ID {
				t.Fatalf("same-time record order: %d", row.Recording.ID)
			}
			states[row.Conference.Status] = true
		}
		if !states[conferences.Active] || !states[conferences.Finished] || !states[conferences.Cancelled] {
			t.Fatalf("missing lifecycle state: %v", states)
		}
	}
	for _, actor := range []string{waiting, kicked, rejected, outsider} {
		rows, err := r.Library(ctx, actor, records.LibraryQuery{Limit: 30})
		if err != nil || len(rows) != 0 {
			t.Fatalf("denied member returned data: count=%d err=%v", len(rows), err)
		}
	}
	first, err := r.Library(ctx, owner, records.LibraryQuery{Limit: 2})
	if err != nil || len(first) != 3 {
		t.Fatalf("lookahead page: %d %v", len(first), err)
	}
	cursor := records.EncodeLibraryCursor(first[1].Recording)
	// A newly published record before the boundary cannot shift/duplicate page two.
	createRecord(meetingIDs[0], records.ModeComposite, now, now)
	second, err := r.Library(ctx, owner, records.LibraryQuery{Limit: 2, Cursor: cursor})
	if err != nil || len(second) != 2 || second[0].Recording.ID != expected[1].ID || second[1].Recording.ID != expected[0].ID {
		t.Fatalf("second page: %+v %v", second, err)
	}
	if _, err := r.Library(ctx, owner, records.LibraryQuery{Limit: 2, Cursor: "bad!"}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatalf("invalid cursor: %v", err)
	}
}
