CREATE TABLE platform_settings (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    revision BIGINT NOT NULL DEFAULT 0 CHECK (revision >= 0),
    values JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(values) = 'object'),
    smtp_password_ciphertext TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO platform_settings(singleton) VALUES(TRUE);
