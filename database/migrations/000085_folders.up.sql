-- folders: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE folders (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    name text NOT NULL,
    name_key text NOT NULL,
    "position" integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT folders_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 50))),
    CONSTRAINT folders_name_key_check CHECK ((name_key <> ''::text)),
    CONSTRAINT folders_position_check CHECK (("position" >= 0))
);

-- migrate:constraints

ALTER TABLE ONLY folders
    ADD CONSTRAINT folders_owner_name_key UNIQUE (user_id, name_key);

ALTER TABLE ONLY folders
    ADD CONSTRAINT folders_owner_position UNIQUE (user_id, "position") DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY folders
    ADD CONSTRAINT folders_pkey PRIMARY KEY (id);

CREATE INDEX idx_folders_owner_order ON folders USING btree (user_id, "position", id);

-- migrate:references

ALTER TABLE ONLY folders
    ADD CONSTRAINT folders_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
