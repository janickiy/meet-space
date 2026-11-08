CREATE TABLE error_events (
 id UUID PRIMARY KEY,
 fingerprint CHAR(64) NOT NULL UNIQUE,
 service VARCHAR(32) NOT NULL,
 instance VARCHAR(120) NOT NULL,
 level VARCHAR(8) NOT NULL CHECK(level IN ('ERROR','WARN')),
 code VARCHAR(80) NOT NULL,
 message VARCHAR(300) NOT NULL,
 route VARCHAR(200) NOT NULL,
 request_id VARCHAR(36) NOT NULL DEFAULT '',
 release VARCHAR(120) NOT NULL,
 first_seen TIMESTAMPTZ NOT NULL,
 last_seen TIMESTAMPTZ NOT NULL,
 occurrences BIGINT NOT NULL CHECK(occurrences BETWEEN 1 AND 1000000000000000),
 browser VARCHAR(16) NOT NULL DEFAULT '',
 stack JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(stack)='array' AND jsonb_array_length(stack)<=5),
 CHECK(first_seen<=last_seen)
);
CREATE INDEX error_events_recent_idx ON error_events(last_seen DESC,id DESC);
CREATE INDEX error_events_retention_idx ON error_events(first_seen);
CREATE INDEX error_events_service_recent_idx ON error_events(service,last_seen DESC,id DESC);
CREATE INDEX error_events_request_idx ON error_events(request_id) WHERE request_id<>'';
-- Receipts make bounded retries idempotent even if a commit response was lost.
CREATE TABLE error_event_batches(id UUID PRIMARY KEY,created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp());
CREATE INDEX error_event_batches_created_idx ON error_event_batches(created_at);
