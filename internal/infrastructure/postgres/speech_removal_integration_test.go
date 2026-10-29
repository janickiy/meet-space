package postgres

import (
	"context"
	"path/filepath"
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

func TestAuditSpeechRemovalUpgradePreservesRecordingAndSearch(t *testing.T) {
	db := auditDatabase(t, 36)
	exec := func(sql string, args ...any) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	user, other, meeting, participant, recording, transcript, session := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec("INSERT INTO users(id,email,password_hash) VALUES(?,?,'synthetic'),(?,?,'synthetic')", user, user+"@example.invalid", other, other+"@example.invalid")
	exec("INSERT INTO conferences(id,owner_id,title,invite_code,status,started_at) VALUES(?,?,'Searchable planning meeting',?,'active',now())", meeting, user, strings.Repeat("m", 32))
	exec("INSERT INTO conference_participants(id,conference_id,user_id,display_name,role,status,admission_state,joined_at) VALUES(?,?,?,'Owner','owner','joined','admitted',now())", participant, meeting, user)
	exec("INSERT INTO record(uuid,conference_id,platform_conference_id,mode,source_type,transport_type,status,quality_mode) VALUES(?,?,?,'composite','conference','webrtc','ready','auto')", recording, meeting, meeting)
	exec("INSERT INTO transcripts(id,conference_id,recording_id,status) VALUES(?,?,?,'queued')", transcript, meeting, recording)
	segment := uuid.NewString()
	exec("INSERT INTO transcript_segments(id,transcript_id,ordinal,start_ms,end_ms,text) VALUES(?,?,0,0,100,'Synthetic speech')", segment, transcript)
	exec("INSERT INTO meeting_summaries(conference_id,transcript_id,transcript_generation,status) VALUES(?,?,1,'ready')", meeting, transcript)
	exec("INSERT INTO content_search_indexes(transcript_id,model_key,generation,state,model,model_version,dimensions) VALUES(?,? ,1,'ready','synthetic','1',1)", transcript, strings.Repeat("a", 64))
	exec("UPDATE transcripts SET status='ready' WHERE id=?", transcript)
	exec("INSERT INTO live_transcription_sessions(id,conference_id,origin,canonical_recording_id) VALUES(?,?,now(),?)", session, meeting, recording)
	exec("INSERT INTO live_caption_segments(id,session_id,conference_id,participant_id,generation,track_instance_id,utterance_id,sequence,revision,start_ms,end_ms,text,language) VALUES(?,?,?,?,1,?,'synthetic',0,0,0,100,'Synthetic caption','en')", uuid.NewString(), session, meeting, participant, uuid.NewString())
	exec("INSERT INTO participant_analytics(participant_id,conference_id) VALUES(?,?)", participant, meeting)
	exec("INSERT INTO conference_analytics(conference_id) VALUES(?)", meeting)
	exec("INSERT INTO notification_preferences(user_id,summary) VALUES(?,true)", user)
	for _, kind := range []string{"transcript.ready", "summary.ready", "processing.failed", "recording.ready"} {
		exec("INSERT INTO notifications(id,user_id,type,payload,dedup_key) VALUES(?,?,?,jsonb_build_object('conferenceId',?::text),?)", uuid.NewString(), user, kind, meeting, "audit:"+kind)
	}
	// Include delivery records whose FK prevents deleting the corresponding queued jobs first.
	exec("INSERT INTO integration_deliveries(job_id,user_id,channel,status) SELECT id,user_id,payload->>'channel','delivered' FROM background_jobs WHERE kind='integrations.delivery'")
	for _, kind := range []string{"content.transcribe", "content.summarize", "content.embed", "analytics.aggregate", "integrations.calendar"} {
		exec("INSERT INTO background_jobs(kind,entity_id,conference_id,dedup_key) VALUES(?,?,?,?)", kind, recording, meeting, "audit:"+kind)
	}
	for _, event := range []string{"transcript.ready", "summary.ready", "transcript.failed", "summary.failed", "processing.failed"} {
		exec("INSERT INTO background_jobs(kind,entity_id,conference_id,payload,dedup_key) VALUES('integrations.event',?,?,jsonb_build_object('event',?::text),?)", transcript, meeting, event, "audit:speech-event:"+event)
	}
	var vectorTables bool
	if err := db.Raw("SELECT to_regclass('content_embeddings') IS NOT NULL").Scan(&vectorTables).Error; err != nil {
		t.Fatal(err)
	}
	if vectorTables {
		exec("INSERT INTO embedding_cache(model_key,content_hash,embedding) VALUES(?,?,'[0.1]'::vector)", strings.Repeat("a", 64), strings.Repeat("b", 64))
		exec("INSERT INTO content_embeddings(id,transcript_id,conference_id,recording_id,generation,ordinal,segment_id,start_ms,end_ms,text,model_key,content_hash,embedding) VALUES(?,?,?,?,1,0,?,0,100,'Synthetic',?,?,'[0.1]'::vector)", uuid.NewString(), transcript, meeting, recording, segment, strings.Repeat("a", 64), strings.Repeat("b", 64))
	}
	if err := RunMigrations(db, filepath.Join("..", "..", "..", "database", "migrations")); err != nil {
		t.Fatal(err)
	}
	assertSpeechSchemaRemoved(t, db)
	var count int64
	if err := db.Table("record").Where("uuid=?", recording).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("recording lost: %d %v", count, err)
	}
	if err := db.Table("background_jobs").Where("dedup_key='audit:integrations.calendar'").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("integration job lost: %d %v", count, err)
	}
	if err := db.Table("notifications").Where("type='recording.ready'").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("recording notification lost: %d %v", count, err)
	}
	if err := db.Table("integration_deliveries").Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("ordinary deliveries lost or obsolete deliveries remain: %d %v", count, err)
	}
	exec("UPDATE record SET status='finalizing' WHERE uuid=?", recording)
	exec("UPDATE record SET status='ready' WHERE uuid=?", recording)
	if err := db.Table("background_jobs").Where("kind IN ('content.transcribe','content.summarize','content.embed','analytics.aggregate') OR payload->>'event' IN ('transcript.ready','summary.ready','transcript.failed','summary.failed','processing.failed')").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("speech jobs remain/recreated: %d %v", count, err)
	}
	repo := NewConferenceSearchRepository(db)
	page, err := repo.Search(context.Background(), user, domain.Query{Query: "planning", Limit: 20})
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ConferenceID != meeting {
		t.Fatalf("ordinary search: %+v %v", page, err)
	}
	page, err = repo.Search(context.Background(), other, domain.Query{Query: "planning", Limit: 20})
	if err != nil || page.Total != 0 {
		t.Fatalf("unauthorized search: %+v %v", page, err)
	}
	for _, kind := range []string{"transcript.ready", "summary.ready", "processing.failed"} {
		if err := db.Exec("INSERT INTO notifications(id,user_id,type,payload,dedup_key) VALUES(?,?,?,jsonb_build_object('conferenceId',?::text),?)", uuid.NewString(), user, kind, meeting, "rejected:"+kind).Error; err == nil {
			t.Errorf("obsolete notification %s accepted", kind)
		}
	}
	if vectorTables {
		var present bool
		if err := db.Raw("SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname='vector')").Scan(&present).Error; err != nil || !present {
			t.Fatalf("shared vector extension dropped: %v", err)
		}
	}
}
