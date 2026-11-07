-- error_event_batches: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE error_event_batches (
    id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

-- migrate:constraints

ALTER TABLE ONLY error_event_batches
    ADD CONSTRAINT error_event_batches_pkey PRIMARY KEY (id);

CREATE INDEX error_event_batches_created_idx ON error_event_batches USING btree (created_at);

-- migrate:references


