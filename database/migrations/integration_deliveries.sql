-- integration_deliveries: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE integration_deliveries (
    job_id uuid NOT NULL,
    user_id uuid NOT NULL,
    channel text NOT NULL,
    status text NOT NULL,
    error_code text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT integration_deliveries_channel_check CHECK ((channel = ANY (ARRAY['email'::text, 'push'::text]))),
    CONSTRAINT integration_deliveries_status_check CHECK ((status = ANY (ARRAY['delivered'::text, 'skipped'::text, 'failed'::text])))
);

-- migrate:constraints

ALTER TABLE ONLY integration_deliveries
    ADD CONSTRAINT integration_deliveries_pkey PRIMARY KEY (job_id);

-- migrate:references

ALTER TABLE ONLY integration_deliveries
    ADD CONSTRAINT integration_deliveries_job_id_fkey FOREIGN KEY (job_id) REFERENCES background_jobs(id) ON DELETE RESTRICT;

ALTER TABLE ONLY integration_deliveries
    ADD CONSTRAINT integration_deliveries_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
