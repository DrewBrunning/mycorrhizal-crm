-- App-password scopes and DAV account-password policy (issue #2010) --
-- rollback. Drops the explicit scope set and the per-user DAV password
-- opt-in; tokens fall back to their legacy scope and account passwords work
-- on DAV again (the pre-migration behaviour). The explicit scope choices made
-- since the upgrade are lost.
ALTER TABLE api_tokens DROP COLUMN scopes;
ALTER TABLE users DROP COLUMN allow_dav_password;
