ALTER TABLE users
    ADD COLUMN auth_source text NOT NULL DEFAULT 'local' CHECK (auth_source IN ('local','ldap')),
    ADD COLUMN deleted_at timestamptz,
    ADD COLUMN ldap_disabled boolean NOT NULL DEFAULT false,
    ADD COLUMN auth_version bigint NOT NULL DEFAULT 0;

-- Пустой хеш до этой миграции обозначал удалённый аккаунт. LDAP не хранит локальный пароль.
UPDATE users SET deleted_at=updated_at WHERE password_hash='';

CREATE TABLE ldap_identities (
    provider_id text NOT NULL DEFAULT 'ldap' CHECK (provider_id='ldap'),
    external_id text NOT NULL,
    user_id uuid NOT NULL UNIQUE REFERENCES users(id),
    login text NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(provider_id, external_id)
);
CREATE INDEX ldap_identities_last_seen_idx ON ldap_identities(last_seen_at);

CREATE TABLE ldap_directory_state (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    last_snapshot_at timestamptz
);
INSERT INTO ldap_directory_state(singleton) VALUES(true);

DROP INDEX idx_users_admin_status_created;
DROP INDEX admin_users_registered;
DROP INDEX admin_users_search_trgm;
CREATE INDEX idx_users_admin_status_created ON users((is_blocked OR ldap_disabled),created_at DESC,id) WHERE deleted_at IS NULL;
CREATE INDEX admin_users_registered ON users(created_at DESC,id) WHERE guest_conference_id IS NULL AND deleted_at IS NULL;
CREATE INDEX admin_users_search_trgm ON users USING gin(display_name gin_trgm_ops,email gin_trgm_ops) WHERE guest_conference_id IS NULL AND deleted_at IS NULL;
