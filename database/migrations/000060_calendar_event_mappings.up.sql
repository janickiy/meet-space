-- calendar_event_mappings: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE calendar_event_mappings (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    conference_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    provider text NOT NULL,
    external_calendar_id text NOT NULL,
    external_event_id text DEFAULT ''::text NOT NULL,
    sync_status text NOT NULL,
    source_version bigint DEFAULT 0 NOT NULL,
    last_synced_at timestamp with time zone,
    CONSTRAINT calendar_event_mappings_sync_status_check CHECK ((sync_status = ANY (ARRAY['pending'::text, 'synced'::text, 'cancelled'::text, 'failed'::text])))
);

-- migrate:constraints

ALTER TABLE ONLY calendar_event_mappings
    ADD CONSTRAINT calendar_event_mappings_conference_id_connection_id_key UNIQUE (conference_id, connection_id);

ALTER TABLE ONLY calendar_event_mappings
    ADD CONSTRAINT calendar_event_mappings_pkey PRIMARY KEY (id);

-- migrate:references

ALTER TABLE ONLY calendar_event_mappings
    ADD CONSTRAINT calendar_event_mappings_conference_id_fkey FOREIGN KEY (conference_id) REFERENCES conferences(id) ON DELETE RESTRICT;

ALTER TABLE ONLY calendar_event_mappings
    ADD CONSTRAINT calendar_event_mappings_connection_id_fkey FOREIGN KEY (connection_id) REFERENCES calendar_connections(id) ON DELETE RESTRICT;
