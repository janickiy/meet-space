-- user_avatars: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE user_avatars (
    user_id uuid NOT NULL,
    version uuid NOT NULL,
    data bytea NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT user_avatars_data_check CHECK (((octet_length(data) >= 1) AND (octet_length(data) <= 262144)))
);

-- migrate:constraints

ALTER TABLE ONLY user_avatars
    ADD CONSTRAINT user_avatars_pkey PRIMARY KEY (user_id);

-- migrate:references

ALTER TABLE ONLY user_avatars
    ADD CONSTRAINT user_avatars_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
