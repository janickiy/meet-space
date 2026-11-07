-- error_events: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE error_events (
    id uuid NOT NULL,
    fingerprint character(64) NOT NULL,
    service character varying(32) NOT NULL,
    instance character varying(120) NOT NULL,
    level character varying(8) NOT NULL,
    code character varying(80) NOT NULL,
    message character varying(300) NOT NULL,
    route character varying(200) NOT NULL,
    request_id character varying(36) DEFAULT ''::character varying NOT NULL,
    release character varying(120) NOT NULL,
    first_seen timestamp with time zone NOT NULL,
    last_seen timestamp with time zone NOT NULL,
    occurrences bigint NOT NULL,
    browser character varying(16) DEFAULT ''::character varying NOT NULL,
    stack jsonb DEFAULT '[]'::jsonb NOT NULL,
    CONSTRAINT error_events_check CHECK ((first_seen <= last_seen)),
    CONSTRAINT error_events_level_check CHECK ((level IN ('ERROR', 'WARN'))),
    CONSTRAINT error_events_occurrences_check CHECK (((occurrences >= 1) AND (occurrences <= '1000000000000000'::bigint))),
    CONSTRAINT error_events_stack_check CHECK (((jsonb_typeof(stack) = 'array'::text) AND (jsonb_array_length(stack) <= 5)))
);

-- migrate:constraints

ALTER TABLE ONLY error_events
    ADD CONSTRAINT error_events_fingerprint_key UNIQUE (fingerprint);

ALTER TABLE ONLY error_events
    ADD CONSTRAINT error_events_pkey PRIMARY KEY (id);

CREATE INDEX error_events_recent_idx ON error_events USING btree (last_seen DESC, id DESC);

CREATE INDEX error_events_request_idx ON error_events USING btree (request_id) WHERE ((request_id)::text <> ''::text);

CREATE INDEX error_events_retention_idx ON error_events USING btree (first_seen);

CREATE INDEX error_events_service_recent_idx ON error_events USING btree (service, last_seen DESC, id DESC);

-- migrate:references


