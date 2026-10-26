-- App-password scopes and DAV account-password policy (issue #2010) --
-- rollback. Drops the explicit scope set and the per-user DAV password
-- opt-in; account passwords work on DAV again (the pre-migration behaviour)
-- and tokens fall back to their legacy scope.
--
-- The pre-000078 credential model can only express 'full' or 'carddav', and
-- the old AuthMiddleware treats every non-'carddav' scope as full REST access.
-- A token minted since the upgrade with a narrower explicit set carries
-- scope='custom'; leaving it in place would silently broaden it (for example,
-- a contacts-only app password would become a full REST credential). There is
-- no faithful legacy representation, so fail closed and revoke those
-- credentials rather than escalate them. The explicit scope choices made since
-- the upgrade are lost either way.
UPDATE api_tokens
SET revoked_at = COALESCE(revoked_at, CURRENT_TIMESTAMP)
WHERE scope NOT IN ('full', 'carddav');

ALTER TABLE api_tokens DROP COLUMN scopes;
ALTER TABLE users DROP COLUMN allow_dav_password;
