-- Issue #2888 (v1.7.0 half of #2071): a per-webhook choice over whether
-- contact payloads carry private/secret addresses and their coordinates.
--
-- NULL means "follow the server default". In v1.x the default is full
-- fidelity, so every existing webhook keeps its behaviour untouched. An
-- explicit 1/0 pins the choice now; when the v2.0.0 default flips to
-- filtered (#2071), only rows that never chose (NULL) change.
ALTER TABLE webhooks ADD COLUMN include_sensitive INTEGER;
