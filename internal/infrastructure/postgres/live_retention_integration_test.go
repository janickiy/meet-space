package postgres

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/records"
	"github.com/janickiy/meet-space/internal/domain/settings"
	"gorm.io/gorm"
	"reflect"
	"sort"
	"testing"
)

type liveRetentionSettings struct {
	days int
	err  error
}

func (s *liveRetentionSettings) Snapshot(context.Context) (settings.Values, error) {
	return settings.Values{RecordingRetentionDays: s.days}, s.err
}

func TestAuditLiveRetentionReadSurfacesAndSafeCleanup(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	user, conference, record := uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec("INSERT INTO users(id,email,password_hash) VALUES(?,?,'synthetic')", user, user+"@example.invalid")
	exec("INSERT INTO conferences(id,owner_id,title,invite_code,status,started_at,finished_at) VALUES(?,?,'Retention',?,'finished',now()-interval '10 days',now()-interval '9 days')", conference, user, uuid.NewString()[:32])
	exec("INSERT INTO conference_participants(id,conference_id,user_id,display_name,role,status,admission_state,joined_at,left_at) VALUES(?,?,?,'Owner','owner','left','admitted',now()-interval '10 days',now()-interval '9 days')", uuid.NewString(), conference, user)
	exec("INSERT INTO record(uuid,conference_id,platform_conference_id,mode,source_type,transport_type,status,quality_mode,ended_at) VALUES(?,?,?,'composite','conference','sfu','ready','auto',now()-interval '9 days')", record, conference, conference)
	policy := &liveRetentionSettings{days: 30}
	repo := NewRecordRepository(db).WithSettings(policy)
	meetings := NewConferenceRecordingRepository(db).WithSettings(policy)
	history := NewConferenceRepository(db).WithSettings(policy)
	check := func(want int) {
		t.Helper()
		rows, err := meetings.Library(ctx, user, records.LibraryQuery{Limit: 20})
		if err != nil || len(rows) != want {
			t.Fatalf("library: %d expected %d: %v", len(rows), want, err)
		}
		listed, err := meetings.List(ctx, user, conference, 20, 0)
		if err != nil || len(listed) != want {
			t.Fatalf("list: %d expected %d: %v", len(listed), want, err)
		}
		summary, err := history.History(ctx, conference, user)
		if err != nil || summary.Recordings.Total != int64(want) {
			t.Fatalf("history: %d expected %d: %v", summary.Recordings.Total, want, err)
		}
		_, err = repo.FindDetailsByUUID(ctx, record)
		if (want == 1 && err != nil) || (want == 0 && !errors.Is(err, gorm.ErrRecordNotFound)) {
			t.Fatalf("direct read expected %d: %v", want, err)
		}
	}
	check(1)
	removed := 0
	remove := func(context.Context, records.Record) error { removed++; return nil }
	if id, err := repo.ExpireNextRecording(ctx, nil, remove); err != nil || id != 0 || removed != 0 {
		t.Fatal("unexpired recording removed", id, err)
	}
	policy.days = 7
	check(0)
	policy.days = 30
	check(1) // A read-side policy change itself never destroys data.
	policy.err = errors.New("settings offline")
	if _, err := repo.ExpireNextRecording(ctx, nil, remove); !errors.Is(err, apperrors.ErrUnavailable) || removed != 0 {
		t.Fatal("cleanup did not fail closed", err)
	}
	if _, err := repo.FindDetailsByUUID(ctx, record); !errors.Is(err, apperrors.ErrUnavailable) {
		t.Fatal("read silently used old retention", err)
	}
	policy.err = nil
	policy.days = 7
	if id, err := repo.ExpireNextRecording(ctx, nil, remove); err != nil || id == 0 || removed != 1 {
		t.Fatal("expired recording was not cleaned", id, err)
	}
	policy.days = 30
	check(0) // Extending retention cannot resurrect deleted artifacts.
}

func TestAuditAttachmentSizeCeilingMigration(t *testing.T) {
	db := auditDatabase(t)
	users := []string{uuid.NewString(), uuid.NewString()}
	sort.Strings(users)
	meeting, conversation := uuid.NewString(), uuid.NewString()
	exec := func(sql string, args ...any) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, user := range users {
		exec("INSERT INTO users(id,email,password_hash) VALUES(?,?,'synthetic')", user, user+"@example.invalid")
	}
	exec("INSERT INTO conferences(id,owner_id,title,invite_code) VALUES(?,?,'Attachments',?)", meeting, users[0], uuid.NewString()[:32])
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("INSERT INTO conversations(id,user_low_id,user_high_id) VALUES(?,?,?)", conversation, users[0], users[1]).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO conversation_members(conversation_id,user_id) VALUES(?,?),(?,?)", conversation, users[0], conversation, users[1]).Error
	}); err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO chat_attachments(id,conference_id,owner_user_id,client_request_id,filename,mime_type,size,expires_at) VALUES(?,?,?,?,'large.txt','text/plain',104857600,now()+interval '1 hour')", uuid.NewString(), meeting, users[0], uuid.NewString())
	exec("INSERT INTO conversation_attachments(id,conversation_id,owner_user_id,client_request_id,filename,mime_type,size,expires_at) VALUES(?,?,?,?,'large.txt','text/plain',104857600,now()+interval '1 hour')", uuid.NewString(), conversation, users[0], uuid.NewString())
	for _, table := range []string{"chat_attachments", "conversation_attachments"} {
		if err := db.Exec("UPDATE " + table + " SET size=104857601").Error; err == nil {
			t.Fatalf("%s accepted above absolute ceiling", table)
		}
	}
	// A future change must fail atomically rather than discard existing large
	// attachment metadata to satisfy a narrower constraint.
	dir := copyTableMigrations(t)
	query := ""
	for _, table := range []string{"chat_attachments", "conversation_attachments"} {
		query += "ALTER TABLE " + table + " DROP CONSTRAINT " + table + "_size_check; ALTER TABLE " + table + " ADD CONSTRAINT " + table + "_size_check CHECK(size BETWEEN 1 AND 10485760);\n"
	}
	writeNumberedMigration(t, dir, 104, "tighten_attachments", query)
	before, ledger := migrationDataSnapshot(t, db), migrationLedgerSnapshot(t, db)
	if err := RunMigrations(db, dir); err == nil {
		t.Fatal("migration narrowed the safety ceiling despite existing larger metadata")
	}
	if !reflect.DeepEqual(before, migrationDataSnapshot(t, db)) || !reflect.DeepEqual(ledger, migrationLedgerSnapshot(t, db)) {
		t.Fatal("failed constraint change altered attachments, other data, sequences or the ledger")
	}
	for _, table := range []string{"chat_attachments", "conversation_attachments"} {
		// Validation failure must restore the original 100MiB constraint too.
		if err := db.Exec("UPDATE " + table + " SET size=104857601").Error; err == nil {
			t.Fatalf("failed upgrade removed the %s safety constraint", table)
		}
	}
}
