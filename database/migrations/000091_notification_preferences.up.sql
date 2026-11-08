-- notification_preferences: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE notification_preferences (
    user_id uuid NOT NULL,
    invitation boolean DEFAULT true NOT NULL,
    reminder boolean DEFAULT true NOT NULL,
    recording boolean DEFAULT true NOT NULL,
    email boolean DEFAULT false NOT NULL,
    push boolean DEFAULT false NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

-- migrate:constraints

ALTER TABLE ONLY notification_preferences
    ADD CONSTRAINT notification_preferences_pkey PRIMARY KEY (user_id);

-- migrate:references

ALTER TABLE ONLY notification_preferences
    ADD CONSTRAINT notification_preferences_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;
