-- Пароль bind хранится отдельно от JSON настроек и шифруется приложением.
ALTER TABLE platform_settings
    ADD COLUMN ldap_bind_password_ciphertext TEXT NOT NULL DEFAULT '';
