-- folder_conferences: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE folder_conferences (
    folder_id uuid NOT NULL,
    conference_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

-- migrate:constraints

ALTER TABLE ONLY folder_conferences
    ADD CONSTRAINT folder_conferences_pkey PRIMARY KEY (folder_id, conference_id);

CREATE INDEX idx_folder_conferences_target ON folder_conferences USING btree (conference_id, folder_id);

-- migrate:references

ALTER TABLE ONLY folder_conferences
    ADD CONSTRAINT folder_conferences_conference_id_fkey FOREIGN KEY (conference_id) REFERENCES conferences(id) ON DELETE CASCADE;

ALTER TABLE ONLY folder_conferences
    ADD CONSTRAINT folder_conferences_folder_id_fkey FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE CASCADE;
