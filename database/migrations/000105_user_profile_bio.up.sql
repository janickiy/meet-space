-- Optional user-owned profile text; existing accounts keep an empty biography.
ALTER TABLE users
    ADD COLUMN bio text NOT NULL DEFAULT '',
    ADD CONSTRAINT users_bio_length_check CHECK (char_length(bio) <= 1000);
