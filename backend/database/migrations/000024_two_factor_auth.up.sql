-- N8 — 2FA / TOTP (issue #158).
--
-- Three additive columns on users:
--   * totp_secret_encrypted  — the TOTP shared secret, AES-256-GCM encrypted at
--     rest via services.EncryptCredential (HKDF-derived key from JWT_SECRET_KEY),
--     NEVER plaintext. Stored before the user is fully enrolled (pending state);
--     totp_enabled is the single source of truth for whether login demands a
--     second factor.
--   * totp_enabled           — whether interactive login requires a TOTP/recovery
--     code. Default 0 so existing users are unaffected.
--   * totp_confirmed_at      — when enrollment completed; null while pending.
--
-- Existing rows need no backfill: all three default to "no 2FA".

ALTER TABLE users ADD COLUMN totp_secret_encrypted TEXT;
ALTER TABLE users ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN totp_confirmed_at DATETIME;

-- Recovery codes: single-use login fallbacks for when the authenticator app is
-- unavailable. One row per (user, code), storing only a SHA-256 hash of each
-- code — the plaintext is shown exactly once at enrollment/regeneration and
-- never persisted. Consuming a code deletes its row, which is what makes it
-- single-use; there is no deleted_at column because a used code is gone, not
-- merely marked (hard-delete per /CLAUDE.md trap #7: a code IS its hash).
CREATE TABLE recovery_codes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    user_id INTEGER NOT NULL,
    code_hash TEXT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_recovery_codes_user_code ON recovery_codes(user_id, code_hash);
CREATE INDEX idx_recovery_codes_user_id ON recovery_codes(user_id);
