DROP TABLE email_verifications;
ALTER TABLE users DROP COLUMN email_verified_at;
ALTER TABLE users DROP COLUMN email_verification_required;
