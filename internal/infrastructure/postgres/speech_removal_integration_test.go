package postgres

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	domain "github.com/janickiy/meet-space/internal/domain/search"
	"gorm.io/gorm"
)

var removedSpeechTables = []string{
	"transcripts", "transcript_segments", "meeting_summaries", "live_transcription_sessions",
	"live_caption_segments", "participant_analytics", "conference_analytics",
	"content_search_indexes", "content_embeddings", "embedding_cache",
}

func assertSpeechSchemaRemoved(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, table := range removedSpeechTables {
		var present bool
		if err := db.Raw("SELECT to_regclass(?) IS NOT NULL", table).Scan(&present).Error; err != nil {
			t.Fatal(err)
		}
		if present {
			t.Errorf("removed table %s remains", table)
		}
	}
	var leftovers int64
	if err := db.Raw(`SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname IN
 ('trg_stage_seven_recording','trg_screen_analytics','trg_transcript_index')`).Scan(&leftovers).Error; err != nil || leftovers != 0 {
		t.Fatalf("speech triggers=%d error=%v", leftovers, err)
	}
	if err := db.Raw(`SELECT count(*) FROM pg_proc WHERE pronamespace=current_schema()::regnamespace AND proname IN
 ('enqueue_stage_seven_recording','track_screen_duration','enqueue_transcript_index')`).Scan(&leftovers).Error; err != nil || leftovers != 0 {
		t.Fatalf("speech functions=%d error=%v", leftovers, err)
	}
	if err := db.Raw(`SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='notification_preferences' AND column_name='summary'`).Scan(&leftovers).Error; err != nil || leftovers != 0 {
		t.Fatalf("summary preference remains: %d %v", leftovers, err)
	}
	var preserved int64
	if err := db.Raw(`SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='conferences' AND column_name='search_vector'`).Scan(&preserved).Error; err != nil || preserved != 1 {
		t.Fatalf("conference search column missing: %d %v", preserved, err)
	}
	for _, table := range []string{"record", "record_file", "recording_outbox", "chat_messages", "background_jobs", "notification_jobs"} {
		var present bool
		if err := db.Raw("SELECT to_regclass(?) IS NOT NULL", table).Scan(&present).Error; err != nil || !present {
			t.Fatalf("preserved table %s missing: %v", table, err)
		}
	}
}

func TestAuditSpeechRemovalFreshInstall(t *testing.T) {
	db := auditDatabase(t)
	assertSpeechSchemaRemoved(t, db)
	var counts struct{ Tables, Columns, Sequences int64 }
	if err := db.Raw(`SELECT
	 (SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_type='BASE TABLE' AND table_name<>'release_schema_migrations') AS tables,
	 (SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name<>'release_schema_migrations') AS columns,
	 (SELECT count(*) FROM information_schema.sequences WHERE sequence_schema=current_schema()) AS sequences`).Scan(&counts).Error; err != nil {
		t.Fatal(err)
	}
	var constraint string
	if err := db.Raw("SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='notifications'::regclass AND conname='notifications_type_check'").Scan(&constraint).Error; err != nil {
		t.Fatal(err)
	}
	t.Logf("schema without migration ledger: tables=%d columns=%d sequences=%d; notifications=%s", counts.Tables, counts.Columns, counts.Sequences, constraint)
	if err := RunMigrations(db, filepath.Join("..", "..", "..", "database", "migrations")); err != nil {
		t.Fatalf("idempotent startup: %v", err)
	}
}

// This is deliberately a trait fixture, not a replay of the removed SQL chain:
// a populated database with an incomplete old ledger and obsolete speech data
// must be rejected before either its supported or deprecated data is changed.
func TestAuditSpeechHistoryRejectedWithoutDeletingOldData(t *testing.T) {
	db := auditDatabase(t, 49)
	exec := func(sql string, args ...any) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	user, other, meeting, recording, transcript := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec("INSERT INTO users(id,email,password_hash) VALUES(?,?,'synthetic'),(?,?,'synthetic')", user, user+"@example.invalid", other, other+"@example.invalid")
	exec("INSERT INTO conferences(id,owner_id,title,invite_code,status,started_at) VALUES(?,?,'Searchable planning meeting',?,'active',now())", meeting, user, strings.Repeat("m", 32))
	exec("INSERT INTO conference_participants(id,conference_id,user_id,display_name,role,status,admission_state,joined_at) VALUES(?,?,?,'Owner','owner','joined','admitted',now())", uuid.NewString(), meeting, user)
	exec("INSERT INTO record(uuid,conference_id,platform_conference_id,mode,source_type,transport_type,status,quality_mode) VALUES(?,?,?,'composite','conference','webrtc','ready','auto')", recording, meeting, meeting)
	exec("CREATE TABLE transcripts(id uuid PRIMARY KEY,conference_id uuid REFERENCES conferences(id),text text NOT NULL)")
	exec("INSERT INTO transcripts(id,conference_id,text) VALUES(?,?,'Preserve obsolete speech data')", transcript, meeting)
	exec("INSERT INTO background_jobs(kind,entity_id,conference_id,dedup_key) VALUES('content.transcribe',?,?,'incomplete-history-speech-job')", recording, meeting)
	exec("DELETE FROM release_schema_migrations WHERE name >= '000037_'")
	before, ledger := migrationDataSnapshot(t, db), migrationLedgerSnapshot(t, db)
	t.Setenv("AUTO_MIGRATE", "false")
	for _, run := range []func(*gorm.DB, string) error{RunMigrations, RunStartupMigrations} {
		if err := run(db, copyTableMigrations(t)); err == nil {
			t.Fatal("incomplete historical database was silently adopted")
		}
	}
	if !reflect.DeepEqual(before, migrationDataSnapshot(t, db)) || !reflect.DeepEqual(ledger, migrationLedgerSnapshot(t, db)) {
		t.Fatal("rejected migration changed recordings, jobs, settings or the old ledger")
	}
	var preserved string
	if err := db.Raw("SELECT text FROM transcripts WHERE id=? AND conference_id=?", transcript, meeting).Scan(&preserved).Error; err != nil || preserved != "Preserve obsolete speech data" {
		t.Fatalf("obsolete user data was deleted by the rejected upgrade: %q %v", preserved, err)
	}
	repo := NewConferenceSearchRepository(db)
	page, err := repo.Search(context.Background(), user, domain.Query{Query: "planning", Limit: 20})
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ConferenceID != meeting {
		t.Fatalf("ordinary search after rejected update: %+v %v", page, err)
	}
	page, err = repo.Search(context.Background(), other, domain.Query{Query: "planning", Limit: 20})
	if err != nil || page.Total != 0 {
		t.Fatalf("unauthorized search: %+v %v", page, err)
	}
}
