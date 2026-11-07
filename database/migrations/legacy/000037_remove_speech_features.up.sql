-- Remove speech recognition and all derived content. Existing migration bytes remain
-- immutable for release_schema_migrations checksum verification and fresh installations.
-- Stop/drain old API, product-worker and live-worker before applying this migration.
-- Recording, chat search and conferences.search_vector are intentionally preserved.
DROP TRIGGER IF EXISTS trg_stage_seven_recording ON record;
DROP FUNCTION IF EXISTS enqueue_stage_seven_recording();
DROP TRIGGER IF EXISTS trg_screen_analytics ON conference_participants;
DROP FUNCTION IF EXISTS track_screen_duration();
DROP TRIGGER IF EXISTS trg_transcript_index ON transcripts;
DROP FUNCTION IF EXISTS enqueue_transcript_index();

-- Remove delivery records before their parent jobs, including already completed jobs.
DELETE FROM integration_deliveries WHERE job_id IN (
 SELECT id FROM background_jobs
 WHERE kind IN ('content.transcribe','content.summarize','content.embed','analytics.aggregate')
 OR payload->>'event' IN ('transcript.ready','summary.ready','transcript.failed','summary.failed','processing.failed')
 OR entity_id IN (SELECT id FROM notifications WHERE type IN ('transcript.ready','summary.ready','processing.failed'))
 OR payload->>'notificationId' IN (SELECT id::text FROM notifications WHERE type IN ('transcript.ready','summary.ready','processing.failed'))
);
DELETE FROM background_jobs
 WHERE kind IN ('content.transcribe','content.summarize','content.embed','analytics.aggregate')
 OR payload->>'event' IN ('transcript.ready','summary.ready','transcript.failed','summary.failed','processing.failed')
 OR entity_id IN (SELECT id FROM notifications WHERE type IN ('transcript.ready','summary.ready','processing.failed'))
 OR payload->>'notificationId' IN (SELECT id::text FROM notifications WHERE type IN ('transcript.ready','summary.ready','processing.failed'));
DELETE FROM notifications WHERE type IN ('transcript.ready','summary.ready','processing.failed');
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_type_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_type_check CHECK(type IN (
 'conference.soon','admission.decided','recording.ready','conference.invited',
 'conference.rescheduled','conference.cancelled','chat.message'
));
ALTER TABLE notification_preferences DROP COLUMN IF EXISTS summary;

-- Explicit dependency order also supports installations without optional pgvector.
DROP TABLE IF EXISTS content_embeddings;
DROP TABLE IF EXISTS embedding_cache;
DROP TABLE IF EXISTS content_search_indexes;
DROP TABLE IF EXISTS meeting_summaries;
DROP TABLE IF EXISTS transcript_segments;
DROP TABLE IF EXISTS transcripts;
DROP TABLE IF EXISTS live_caption_segments;
DROP TABLE IF EXISTS live_transcription_sessions;
DROP TABLE IF EXISTS participant_analytics;
DROP TABLE IF EXISTS conference_analytics;
-- Do not DROP EXTENSION vector: it may be used by objects outside MeetSpace.
