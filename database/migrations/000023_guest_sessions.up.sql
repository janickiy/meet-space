-- Идентификационные данные гостей сохраняют историю посещений, но не могут стать сессиями
-- учётных записей.
ALTER TABLE users ADD COLUMN guest_conference_id UUID NULL;
ALTER TABLE users ADD CONSTRAINT users_guest_not_admin
    CHECK (guest_conference_id IS NULL OR is_admin = FALSE);
