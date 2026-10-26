-- App-password scopes and the CardDAV/CalDAV account-password policy
-- (issue #2010).
--
-- api_tokens.scopes is the explicit access set. Existing rows are backfilled
-- from their legacy single scope so no credential changes meaning:
--   full     -> carddav, caldav, api:read, api:write, mcp
--   carddav  -> carddav, caldav
-- The legacy scope column stays in place for older clients and older rows.
ALTER TABLE api_tokens ADD COLUMN scopes TEXT;

UPDATE api_tokens
SET scopes = CASE
    WHEN scope = 'carddav' THEN '["carddav","caldav"]'
    ELSE '["carddav","caldav","api:read","api:write","mcp"]'
END;

-- users.allow_dav_password: whether the account password may authenticate
-- CardDAV/CalDAV when no second factor is enrolled. Existing users keep
-- today's behaviour (allowed); users created after this migration default to
-- off and must opt in. The password is always rejected once a second factor
-- is enrolled, regardless of this column.
ALTER TABLE users ADD COLUMN allow_dav_password BOOLEAN NOT NULL DEFAULT 0;
UPDATE users SET allow_dav_password = 1;
UPDATE users SET allow_dav_password = 0
WHERE totp_enabled = 1
   OR EXISTS (SELECT 1 FROM webauthn_credentials w WHERE w.user_id = users.id);
