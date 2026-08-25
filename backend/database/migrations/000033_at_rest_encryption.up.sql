-- Field-level data-at-rest encryption (issue #380, ASVS V6.4/V8.3).
--
-- This migration creates the table that stores the deployment's wrapped
-- data-encryption key. The at-rest encryption itself is applied by the
-- backend/atrest package: GORM serializers encrypt/decrypt the sensitive
-- columns transparently, and atrest.Backfill (run at startup, right after
-- migrations, the same way RebuildSearchIndex is a re-runnable Go job)
-- encrypts any rows written before encryption was enabled. SQL cannot run
-- AES-GCM, so the data transform is necessarily a Go step — this table is
-- the schema half of the feature, and the backfill is the data half.
--
-- Key layering: DATA_ENCRYPTION_KEY (or _FILE, or an HKDF derivation from
-- JWT_SECRET_KEY) is the master key; a random 32-byte DEK is wrapped by it
-- with AES-256-GCM and stored here. All field ciphertext is "encv1:<key_id>:"
-- prefixed and sealed under the DEK, so rotating the master key only rewraps
-- this single row and never re-encrypts the payloads. "Lost key = lost data,
-- by design" — if the master key is lost, this wrapped DEK cannot be
-- unwrapped and every encrypted column becomes undecryptable. There is
-- deliberately no escrow.
CREATE TABLE data_encryption_keys (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    key_id      TEXT    NOT NULL UNIQUE,
    wrapped_dek BLOB    NOT NULL,
    created_at  DATETIME NOT NULL
);

-- Deliberately no backfill in SQL: AES-GCM is not expressible in SQLite.
-- The Go backfill (atrest.Backfill) runs on every startup and encrypts any
-- remaining plaintext rows idempotently; it preserves row counts (asserted
-- by backend/atrest/backfill_test.go).
