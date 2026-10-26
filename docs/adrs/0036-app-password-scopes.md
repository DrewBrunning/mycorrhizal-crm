# ADR 0036: App-password scopes and the CardDAV/CalDAV account-password policy

- **Status:** accepted
- **Date:** 2026-10-25
- **Decides:** issue #2010

## Context

`ApiToken` was a long-lived bearer credential with one legacy `scope` string:
`full` (the REST API and CardDAV/CalDAV) or `carddav` (DAV only). A `full`
token reached every authenticated REST route, including token creation, 2FA and
passkey enrollment, session revocation, device-grant creation, and feed
management. It could therefore mint replacement credentials or enroll a second
factor on an account that had none.

CardDAV/CalDAV Basic Auth also accepted the account password with no second
factor check. A user who enabled 2FA still had their password accepted by every
DAV client, so the password alone reached all contacts and calendars.

## Decision

### One credential type, explicit scopes

Extend `api_tokens` rather than adding an `AppPassword` table. The credential
already has a hashed secret, name, expiry, revocation, rotation, last-used and
an in-memory-only plaintext return. Add an explicit `scopes` set:

| Scope | Grants |
|---|---|
| `carddav` | CardDAV read/write |
| `caldav` | CalDAV read |
| `api:read` | Safe-method REST requests |
| `api:write` | Mutating REST requests |
| `mcp` | The read-only MCP surface |

The legacy `scope` column remains for old clients and old rows. A legacy `full`
row expands to all five scopes; a legacy `carddav` row expands to `carddav` and
`caldav`. New requests may send `scopes`; `scope` is still accepted and ignored
when both are present. `full` remains the default when neither is sent, so an
old client's create request does not silently narrow. A DAV-only scope set may
also set `never_expires: true`, because a 90-day expiry silently breaks
background sync; API/MCP tokens still always expire.

### Credential management requires an interactive session

No API token reaches password change, 2FA/passkey management, session
management, API-token management, feed management, device-grant management,
self-deletion, or the DAV password policy. Those routes reject
`isAPIToken=true` with `403`. This is the boundary that stops a leaked token
from extending or escalating itself. All other REST routes keep the existing
scope rule: `api:read` for safe methods, `api:write` for mutations.

### DAV has per-surface scopes and a password policy

CardDAV requires `carddav`; CalDAV requires `caldav`. A token scoped to one is
refused on the other.

The account password is accepted on DAV only when the user has opted in
(`users.allow_dav_password`) **and** has no second factor enrolled. 2FA always
wins: a TOTP or passkey enrollment rejects the account password regardless of
the setting, and enabling a factor persists `allow_dav_password = false` so the
DB and UI agree. The migration backfills users with no second factor to
opted-in so an upgrade does not break their clients; users that already hold
TOTP or a passkey, and users created afterwards, start opted out. A
rejected password is a `403` with
`X-Mycorrhizal-Auth-Error: account-password-not-allowed-for-dav` and a message
telling the user to create an app password.

## Consequences

- **Breaking (MAINT-02):** a `full` token can no longer manage credentials or
  enroll a factor. The PR/release note must say so. Scripts that create or
  revoke tokens with a token need an interactive session or a different admin
  path. Existing data is preserved; the migration only adds columns and
  backfills them.
- Existing `full` tokens keep their data-plane access, including both DAV
  surfaces and MCP. Existing `carddav` tokens keep both DAV surfaces.
- Existing users keep account-password DAV access until they enroll 2FA or
  disable the setting. New users must opt in or create an app password.
- Tests: `backend/models/api_token_scope_test.go`,
  `backend/carddav/auth_test.go`, `backend/middleware/api_scope_test.go`,
  `backend/controllers/api_token_controller_test.go`,
  `backend/controllers/dav_password_policy_test.go`,
  `backend/routes/authorization_matrix_credentials_test.go`, and
  `backend/database/migrate_app_password_scopes_test.go`.

## v2.0.0 follow-up

The legacy `scope` field and the `full`/`carddav` aliases remain in 1.x only for
compatibility. Their removal is tracked in issue #2023: deprecate `scope` with
the MAINT-01 runtime signal in a 1.x release, backfill every remaining `scopes`
row from `scope`, then drop the column and the API field in 2.0.0. The security
enforcement in this ADR does not depend on that cleanup.
