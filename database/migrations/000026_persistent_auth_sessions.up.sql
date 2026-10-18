-- Сессии учётных записей завершаются при явном выходе из системы, а не по истечении времени или
-- при бездействии.
CREATE TABLE IF NOT EXISTS auth_sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ NULL
);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_user ON auth_sessions(user_id);

-- Параллельные обновления устаревших токенов используют один SID, но получают разные файлы
-- cookie.
CREATE TABLE IF NOT EXISTS auth_session_tokens (
    token_hash CHAR(64) PRIMARY KEY CHECK (token_hash ~ '^[a-f0-9]{64}$'),
    session_id UUID NOT NULL REFERENCES auth_sessions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_auth_session_tokens_session ON auth_session_tokens(session_id);

-- Сохраняйте маркеры выхода из системы, чтобы при параллельном обновлении устаревших токенов не
-- происходил повторный вход в систему.
CREATE TABLE IF NOT EXISTS auth_legacy_tokens (
    token_hash CHAR(64) PRIMARY KEY CHECK (token_hash ~ '^[a-f0-9]{64}$'),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id UUID NULL REFERENCES auth_sessions(id) ON DELETE CASCADE,
    revoked_at TIMESTAMPTZ NULL
);
