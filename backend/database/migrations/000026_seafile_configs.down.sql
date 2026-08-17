-- Reverses 000026. Destructive by nature: the per-user Seafile connection
-- (server URL + encrypted API token) is discarded. Linked ExternalIdentity
-- rows survive (they carry their own URL), but the connection that let a user
-- re-open/refresh them is gone.

DROP TABLE seafile_configs;
