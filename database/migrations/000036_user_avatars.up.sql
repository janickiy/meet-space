-- Small normalized thumbnails are separate from frequently read user rows.
ALTER TABLE users ADD COLUMN avatar_version UUID NULL;
CREATE TABLE user_avatars (
 user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 version UUID NOT NULL,
 data BYTEA NOT NULL CHECK(octet_length(data) BETWEEN 1 AND 262144),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
