-- Reverses 000024. Destructive by nature: every hashed recovery code is
-- discarded and any pending/enrolled 2FA state is dropped. Down is the only
-- lossy direction — a user who enabled 2FA here would have to re-enroll after
-- rolling back.

DROP TABLE recovery_codes;
ALTER TABLE users DROP COLUMN totp_confirmed_at;
ALTER TABLE users DROP COLUMN totp_enabled;
ALTER TABLE users DROP COLUMN totp_secret_encrypted;
