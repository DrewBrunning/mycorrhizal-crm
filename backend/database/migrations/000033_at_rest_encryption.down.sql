-- Field-level data-at-rest encryption (issue #380) — rollback.
--
-- WARNING: dropping this table destroys the only copy of the wrapped
-- data-encryption key. Every column encrypted by at-rest encryption becomes
-- permanently undecryptable — this is "lost key = lost data, by design".
-- The down migration is therefore destructive to already-encrypted data and
-- is documented as such (per the repo rule that a down migration that cannot
-- re-derive plaintext must be explicitly irreversible and documented).
DROP TABLE data_encryption_keys;
