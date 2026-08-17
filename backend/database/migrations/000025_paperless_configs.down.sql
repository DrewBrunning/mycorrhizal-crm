-- Reverses 000025. Destructive by nature: the per-user Paperless connection
-- (base URL + encrypted API token) is discarded. Down is the only lossy
-- direction — linked ExternalIdentity rows survive (they carry their own URL),
-- but the connection that let a user re-open/refresh them is gone.

DROP TABLE paperless_configs;
