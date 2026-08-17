-- Reverses 000027. Destructive by nature: the per-user Nextcloud/ownCloud
-- connection (server URL + username + encrypted app password) is discarded.
-- Linked ExternalIdentity rows survive (they carry their own URL), but the
-- connection that let a user re-open/refresh them is gone.

DROP TABLE webdav_configs;
