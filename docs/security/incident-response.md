# Incident response & credential rotation runbook

The rest of the security posture assumes the application resists attack. This document assumes it
didn't. It is written to be followed under stress, by one person, on their own instance, at a bad
time — so it is commands and decisions, not background reading.

| | |
|---|---|
| **Last updated** | 2026-08-27 (issue [#509](https://github.com/DrewBrunning/mycorrhizal-crm/issues/509)) |
| **Scope** | A self-hosted single instance (the shipped all-in-one image). Contain, assess, rotate, recover, notify — plus a per-credential rotation reference. |
| **Companion docs** | `docs/deployment.md` (backup/restore/upgrade *procedures*), `docs/security/threat-model.md` (assets/actors), `docs/security/deployment-baseline.md` (operator boundary), `docs/security/data-retention-lifecycle.md` (what lives where, for how long). Migration recovery is [#440](https://github.com/DrewBrunning/mycorrhizal-crm/issues/440); disaster-recovery boundaries are [#455](https://github.com/DrewBrunning/mycorrhizal-crm/issues/455). |
| **Verified** | The rotation procedures below were exercised against a real build on 2026-08-27 — see [What this was verified against](#what-this-was-verified-against). |

## Using this under stress

On most instances the operator, the only user, and the incident responder are the same person, and
there is no security team to escalate to. That shapes this document:

- **Fewer decisions.** Where there is a safe default, this runbook takes it and says so.
- **Preserve evidence before you change anything.** Rotation and restore both destroy the state you
  would want to look at later. The capture step (below) takes two minutes.
- **When in doubt, rotate `JWT_SECRET_KEY` and restore from a known-good backup.** It is a
  self-inflicted outage (every session and the stored integration credentials die — see the
  procedure), but it is complete, and "complete but disruptive" beats "targeted but uncertain"
  mid-incident.

If you received a vulnerability report rather than finding a compromise, skip to
[Scenario: a vulnerability is reported through SECURITY.md](#scenario-a-vulnerability-is-reported-through-securitymd).

## Running the helper commands

The shipped Docker image contains only the server binary. `audit-verify`, `rotate-at-rest-key`,
`backup`, and the `migrate` CLI are run with `go run ./cmd/<name>` (or `make <target>`) from a
checkout of this repo with a Go toolchain — the same "a clone of this repo used to operate the
instance" assumption `docs/deployment.md` already makes. Check out the tag your instance is running.
All of them read `SQLITE_DB_PATH`, so point that at the live DB (or a copy). Where a step has a
toolchain-free alternative — `sqlite3 … VACUUM INTO` for the snapshot, a direct `UPDATE` for token
revocation — it is given inline.

## First 15 minutes: capture before you touch

Do this **before** rotating anything or restoring a backup. Both are destructive to evidence.

```bash
# 1. Snapshot the database as-is (safe while the server runs).
make backup                      # checkpoint + VACUUM INTO → <db-dir>/<name>-<timestamp>.db
# or, outside the container:
sqlite3 "$SQLITE_DB_PATH" "VACUUM INTO '/path/off-host/evidence-<timestamp>.db'"

# 2. Prove the audit trail hasn't been tampered with, and keep the result.
SQLITE_DB_PATH=/path/to/mycorrhizal.db go run ./cmd/audit-verify | tee evidence-audit-verify-<timestamp>.txt
#   → "Audit hash chain is intact on <path>"  (exit 0)
#   → a non-zero exit and a reported first-gap row means rows were edited or deleted; keep that output.

# 3. Capture logs and the container state.
docker compose logs --no-color --timestamps mycorrhizal > evidence-logs-<timestamp>.txt
docker inspect mycorrhizal > evidence-inspect-<timestamp>.json

# 4. Copy the on-disk files that back the DB, photos, and attachments off-host, read-only.
```

Notes:

- `cmd/audit-verify` is the tamper-evidence check for the hash-chained audit log
  ([#381](https://github.com/DrewBrunning/mycorrhizal-crm/issues/381)). Run it *first* — once you
  restore or rotate, you can no longer prove what the log said at the time of the incident.
- `GET /api/v1/audit/export` gives a CSV of **the calling user's own** audit events, not the whole
  instance. For an instance-wide record, the database snapshot from step 1 is the artifact.
- Move every `evidence-*` file off the host. If the host itself is suspect, treat everything on it as
  read-only from now on and work from copies.

## Containment quick reference

Two blast radii. Know which one you need before you act.

| You want to… | Do this | What it kills | What it does **not** kill |
|---|---|---|---|
| Lock out **one** compromised account | Admin `PATCH /api/v1/admin/users/:id` with a new `password` | That user's web sessions (token-version bump) **and** all their API tokens (revoke-all), and their CardDAV/CalDAV Basic-auth (same password) | Other users; the instance |
| Kill **one** leaked API token | Owner: `DELETE /api/v1/api-tokens/:id`, or `POST /api/v1/api-tokens/:id/rotate` | Just that token | Sessions; other tokens |
| Kill **all** of one user's API tokens | Owner: `POST /api/v1/api-tokens/revoke-all` (also fires on the admin password reset above) | Every active API token for that user | Their web session (unless triggered via the password reset) |
| Invalidate **every session on the instance** | Rotate `JWT_SECRET_KEY` (see procedure — has a boot-time footgun) | Every web session; every 2FA challenge in flight; **every stored integration credential and TOTP secret** | **API tokens** — they are not JWTs; revoke them separately |
| Take the instance offline now | `docker compose down` | Everything | Nothing — data on disk is untouched |

There is deliberately **no** single "log every user out" button short of rotating `JWT_SECRET_KEY`.
Per-user session invalidation is the token-version bump, which the admin password-reset path does.

Admin endpoints (`/api/v1/admin/*`) reject API-token auth outright
(`403 "API tokens cannot access admin endpoints"`) — containment actions that need admin rights need
a session cookie from an admin login.

## Credential & key rotation reference

Each procedure states what the secret protects, its blast radius **up front**, the commands, and how
to verify. Rotations are ordered by how much they hurt.

### `JWT_SECRET_KEY`

**Protects:** the signature on every session JWT and every 2FA-challenge JWT; and, as an HKDF root,
the AES key that encrypts every stored integration credential (CardDAV/CalDAV passwords, Gotify/
Seafile/Paperless/Immich tokens, WebDAV app passwords) **and every user's TOTP secret**; and, when no
dedicated `DATA_ENCRYPTION_KEY` is set, the at-rest master key that unwraps the field-encryption DEK.

**Blast radius — rotating this key:**

1. Every web session is invalidated. All users must log in again.
2. Every 2FA challenge in flight is invalidated (users mid-login retry).
3. Every stored integration credential becomes undecryptable and must be **re-entered** by each user
   (CardDAV/CalDAV subscriptions, notification tokens, Immich/Paperless/Seafile/Nextcloud).
4. Every user's **TOTP secret** becomes undecryptable — 2FA no longer verifies. Users re-enrol, or an
   admin runs `POST /api/v1/admin/users/:id/reset-2fa` per user.
5. **API tokens are *not* affected** (they are random tokens, hashed at rest — not JWTs). Revoke them
   separately if the incident calls for it.
6. **If `DATA_ENCRYPTION_KEY` / `DATA_ENCRYPTION_KEY_FILE` is not set**, the field-encryption master
   key is derived from `JWT_SECRET_KEY`. Rotating the JWT secret without first decoupling the at-rest
   key makes the server **refuse to boot**:

   ```
   FTL Failed to initialize at-rest encryption
       error="atrest: failed to unwrap data-encryption key
       (wrong DATA_ENCRYPTION_KEY? lost key = lost data): cipher: message authentication failed"
   ```

**Procedure**

If you have not already, decouple the at-rest key first (one-time, do it now even outside an
incident):

```bash
# With the CURRENT (old) JWT_SECRET_KEY still in the environment, rewrap the
# field-encryption DEK under a fresh, explicit master key. Payload bytes are
# never touched — this only rewrites the one wrapped-DEK row.
NEWKEY="$(openssl rand -base64 32)"
JWT_SECRET_KEY="<current value>" go run ./cmd/rotate-at-rest-key -new "$NEWKEY" -db "$SQLITE_DB_PATH"
#   → "Master key rotated: DEK rewrapped under the new key."

# Persist it so the at-rest key no longer follows the JWT secret.
#   DATA_ENCRYPTION_KEY=<NEWKEY>   (or DATA_ENCRYPTION_KEY_FILE=/path, mode 0400)
```

Then rotate the JWT secret:

```bash
NEW_JWT="$(openssl rand -base64 32)"        # ≥ 32 bytes, random — boot validation rejects short/low-entropy/placeholder values
#   set JWT_SECRET_KEY=<NEW_JWT> in the environment / compose file
docker compose up -d                        # restart with the new value
```

If the server is **already failing to boot** because the JWT secret was rotated with no
`DATA_ENCRYPTION_KEY`: put the **old** `JWT_SECRET_KEY` back in the environment just for the
`rotate-at-rest-key` run above (it resolves the old master key from it), set `DATA_ENCRYPTION_KEY` to
the fresh key, then bring the server up with the **new** `JWT_SECRET_KEY`. You must still have the old
value to recover — if it is lost, the field-encrypted columns are lost (by design; there is no
escrow).

**Verify**

```bash
# Old session cookie → rejected:
curl -s -b old-cookies.txt https://<host>/api/v1/users/me      # {"error":"Invalid token signature"} [401]
# API token → still works (rotate/revoke it separately if needed):
curl -s -H "Authorization: Bearer mycorrhizal_…" https://<host>/api/v1/users/me   # [200]
# A stored integration credential → now fails until re-entered, e.g. a calendar sync:
#   "authentication failed - check username and password"  (was a connection error before)
```

Then: tell users to log in again and re-enter integration credentials; have 2FA users re-enrol or
reset them.

### `DATA_ENCRYPTION_KEY` (at-rest field-encryption master key)

**Protects:** wraps the single data-encryption key (DEK) that AES-256-GCM-encrypts the sensitive
non-searchable columns at rest ([#380](https://github.com/DrewBrunning/mycorrhizal-crm/issues/380)).

**Blast radius:** none to users. Rotation only unwraps the DEK with the old master key and rewraps it
with the new one — payload bytes are untouched, no re-encryption, no downtime beyond the restart.
Lost old **and** new key = the DEK cannot be unwrapped and every encrypted column is unrecoverable.

**Procedure**

```bash
NEWKEY="$(openssl rand -base64 32)"
make rotate-at-rest-key NEW="$NEWKEY"
#   equivalently: go run ./cmd/rotate-at-rest-key -new "$NEWKEY" [-old <old-base64>] -db "$SQLITE_DB_PATH"
#   -old is only needed when rotating from one explicit key to another without
#   changing the running environment; otherwise the old key is resolved the way
#   the server resolves it (DATA_ENCRYPTION_KEY → _FILE → HKDF(JWT_SECRET_KEY)).
#   set DATA_ENCRYPTION_KEY=<NEWKEY> and restart
```

**Verify:** the server boots (a wrong key fails closed at startup, before any data is served), and the
next scheduled restore drill — or a manual `atrest.VerifyBackupDecryptable` against a fresh snapshot —
passes. Back up the new key the same way as the old one.

### API tokens

**Protects:** programmatic and CardDAV-scoped access. Stored as a SHA-256 hash; never recoverable in
plaintext after creation.

**Blast radius:** the token(s) revoked. No effect on sessions or other tokens.

**Procedure** (token owner, or admin via the password reset which revokes all of a user's tokens):

```bash
# One token:
curl -s -X DELETE  https://<host>/api/v1/api-tokens/<id>            -b cookies.txt
# One token, replaced in place (old dies immediately, new returned once):
curl -s -X POST    https://<host>/api/v1/api-tokens/<id>/rotate     -b cookies.txt
# All of the caller's tokens:
curl -s -X POST    https://<host>/api/v1/api-tokens/revoke-all      -b cookies.txt   # → {"revoked":N}
```

Direct DB fallback if the API is unavailable:

```sql
UPDATE api_tokens SET revoked_at = CURRENT_TIMESTAMP WHERE revoked_at IS NULL;   -- all
```

**Verify:** the old token returns `401 "Invalid token"`. Token create/revoke both write audit events.

### Account password (admin-driven reset)

**Protects:** interactive login for one account, and that account's CardDAV/CalDAV Basic-auth (same
credential).

**Blast radius:** that user only — but for that user it is thorough: web sessions **and** API tokens
**and** Basic-auth all drop.

**Procedure**

```bash
# Admin session cookie required (API tokens are refused on /admin/*).
curl -s -X PATCH https://<host>/api/v1/admin/users/<id> \
     -b admin-cookies.txt -H 'Content-Type: application/json' \
     -d '{"password":"<a strong temporary value>"}'
```

This bumps the user's token version (all their JWT sessions → `401 "Session expired, please sign in
again"`), runs revoke-all on their API tokens (→ `401 "Invalid token"`), and — on the self-service
recovery path — sends a "your password was changed" email. Hand the temporary password to the real
account owner over a channel the attacker doesn't control; they change it on next login.

The user-initiated flow (`POST /api/v1/password-reset/request` → emailed token →
`/password-reset/confirm`) has the same session/token effect and is the right path when the owner
still controls the account's email.

### TOTP secret & recovery codes

**Protects:** the second factor.

**Blast radius:** one user's 2FA.

**Procedure**

- **User still has a working TOTP code:** they regenerate recovery codes
  (`POST /api/v1/users/2fa/recovery-codes/regenerate`, gated on a live code) — the old set dies. To
  fully re-key, disable (`/users/2fa/disable`) and set up again (`/users/2fa/setup` → `/confirm`).
- **User is locked out, or the TOTP secret was invalidated by a `JWT_SECRET_KEY` rotation:** an admin
  runs `POST /api/v1/admin/users/:id/reset-2fa`
  ([#560](https://github.com/DrewBrunning/mycorrhizal-crm/issues/560) /
  PR [#595](https://github.com/DrewBrunning/mycorrhizal-crm/pull/595)). The user re-enrols from
  scratch on next login.

TOTP secrets and recovery-code material are generated from `crypto/rand` and stored encrypted (via
the `JWT_SECRET_KEY`-derived key) — which is why a JWT-secret rotation forces a re-enrol for everyone.

### OIDC client secret

**Protects:** the confidential-client credential for the SSO handshake with your identity provider.

**Blast radius:** the SSO login path only. Existing sessions ride the same JWT cookie every other
login produces, so they are unaffected (unless you are *also* rotating `JWT_SECRET_KEY`).

**Procedure**

1. Rotate the client secret at the identity provider; get the new value.
2. Set `OIDC_CLIENT_SECRET=<new value>` in the environment / compose file.
3. `docker compose up -d`.
4. Verify one SSO login end to end.

## Scenario playbooks

Each is: **contain → assess → rotate → recover → notify.**

### Scenario: a user account is compromised

1. **Contain.** Admin `PATCH /admin/users/:id` with a fresh password (kills that user's sessions,
   API tokens, and Basic-auth in one call). If the account is an admin, do this from a *different*
   admin session, or from the DB.
2. **Assess.** From the capture snapshot, read that user's `audit_events` rows for the incident
   window: what did they create/update/delete, and from what IPs (`docker logs`). Note anything to
   undo (`POST /api/v1/audit/:id/undo` exists for reversible operations).
3. **Rotate.** Password is already done. If the account had API tokens for external systems, treat
   those systems' credentials as exposed too and rotate them there.
4. **Recover.** Undo unauthorized changes. If the damage is wide, restore the whole DB from the
   last backup *before* the incident window (see `docs/deployment.md`); you lose everything since
   that backup.
5. **Notify.** Give the owner the temporary password over a trusted channel. On a multi-user
   instance, see [Multi-user instances](#multi-user-instances).

### Scenario: `JWT_SECRET_KEY` is leaked

Anyone with the key can forge a session for any user and decrypt any stored integration credential.

1. **Contain.** `docker compose down` if you can afford the outage while you work; otherwise proceed
   straight to rotation — a forged session survives until you rotate.
2. **Assess.** You generally cannot tell a forged session from a real one in the logs. Assume every
   session since the leak is suspect.
3. **Rotate.** Follow the [`JWT_SECRET_KEY` procedure](#jwt_secret_key) exactly — including the
   at-rest-key decoupling step, or the server won't boot. This also invalidates the leaked-key
   attacker's ability to decrypt credentials going forward, but **credentials encrypted under the old
   key were already readable to them** — treat every stored integration credential as exposed and
   have users rotate them at the source (their Nextcloud app password, Gotify token, etc.).
4. **Recover.** Users log in again, re-enter integration credentials, re-enrol or reset 2FA. Rotate
   API tokens as well (`revoke-all` per user or the DB `UPDATE`) — the JWT rotation did not touch
   them.
5. **Notify.** Tell every user: re-login required, re-enter integration credentials, 2FA reset, and
   that any credential they had stored in the app should be considered exposed and rotated upstream.

### Scenario: an API token is leaked

1. **Contain.** Revoke it: `DELETE /api-tokens/:id`, or `revoke-all` if you're not sure which one, or
   the DB `UPDATE`.
2. **Assess.** `api_tokens.last_used_at` and the request logs show whether and when it was used.
   Audit events attribute actions to the user, not the token — correlate by time and IP.
3. **Rotate.** Issue a fresh token for the legitimate integration (`POST /api-tokens` or
   `/:id/rotate`).
4. **Recover.** Undo anything the token did that shouldn't stand.
5. **Notify.** If the token was `carddav`-scoped, the synced client holds a copy of the address book —
   nothing to do there, but note it.

### Scenario: the host is compromised

Assume the attacker has read everything on disk: the SQLite database, `JWT_SECRET_KEY`,
`DATA_ENCRYPTION_KEY` (if it's on that host), backups within reach, TLS material at the proxy.

1. **Contain.** Take the instance offline. Do not "clean" the host — rebuild it.
2. **Assess.** Preserve evidence (the capture step) from a *copy* of the disk, not the live host.
   `cmd/audit-verify` on the copied DB tells you whether the audit log was edited.
3. **Rotate — everything, on the rebuilt host:**
   - New `JWT_SECRET_KEY` (with the at-rest decoupling).
   - New `DATA_ENCRYPTION_KEY` (`rotate-at-rest-key`), because the old one was on the box.
   - New TLS certificate/key at the proxy.
   - Every user's password (admin `PATCH` per user), which also drops their tokens and Basic-auth.
   - Every 2FA enrolment (`reset-2fa` per user).
   - `OIDC_CLIENT_SECRET`, `RESEND_API_KEY` / SMTP creds, and any other secret from the environment,
     rotated at their sources.
   - Every stored integration credential is exposed — users rotate them upstream.
4. **Recover.** Restore application data from a backup you are confident predates the compromise.
   Verify the restore boots and `audit-verify` passes.
5. **Notify.** Full disclosure to all users: what was exposed (all contact data on the instance,
   given the searchable-plaintext columns), what you rotated, what they must rotate.

### Scenario: a backup is leaked

A database snapshot carries the *wrapped* DEK but never the master key, so the field-encrypted
columns in a leaked backup are not readable without `DATA_ENCRYPTION_KEY`
([#420](https://github.com/DrewBrunning/mycorrhizal-crm/issues/420), P5). But the **searchable
plaintext columns** (`notes.content`, `activities.*`, the flat contact search fields — the documented
at-rest exception) *are* readable in a raw snapshot.

1. **Contain.** Work out the blast radius: which snapshot, how old, what it contained. Find out how it
   leaked (misconfigured bucket, lost drive, backup host breach) and close that.
2. **Assess.** Treat the plaintext columns in that snapshot as disclosed. If `DATA_ENCRYPTION_KEY`
   could *also* have leaked (same host, same bucket), treat the encrypted columns as disclosed too.
3. **Rotate.** If the key is also suspect: `rotate-at-rest-key` to a new master key, so future
   backups are sealed under a key the leaked snapshot's holder doesn't have. (This does not un-leak
   the old snapshot.) Rotate the backup storage's own credentials.
4. **Recover.** Nothing to restore — the live instance is intact. Fix backup storage confidentiality
   per `docs/deployment.md` "Backup confidentiality & retention".
5. **Notify.** Disclose to affected users that a backup containing their contact data was exposed.

### Scenario: a vulnerability is reported through SECURITY.md

1. **Acknowledge** within the window `SECURITY.md` states (5 business days).
2. **Triage.** Reproduce it. Decide severity: does it expose data, bypass auth, or allow IDOR? Is it
   remotely exploitable without credentials?
3. **Contain if it's being exploited.** If the report includes evidence of active exploitation,
   switch to the relevant scenario above (compromise, leaked key) in parallel with the fix.
4. **Fix** on a private branch. Add a regression test. If it's a class of bug, check the
   neighbours.
5. **Disclose.** Coordinate a timeline with the reporter. Ship the fix in a tagged release (only the
   latest tag gets fixes — `SECURITY.md` "Supported Versions"). Credit the reporter if they want it.
6. **Update** `docs/security/asvs-l2.md` / `threat-model.md` for anything the fix changed, in the
   same release.

## Multi-user instances

If other people have accounts on your instance, an incident is not only yours to absorb.

- **Tell them promptly** what happened, what data was potentially exposed, and what they must do
  (re-login, re-enter integration credentials, reset 2FA, rotate upstream credentials).
- **A forced instance-wide logout is visible** — rotating `JWT_SECRET_KEY` logs everyone out at once
  with no warning. Send the notice *before or with* the rotation, not after, or people will assume
  the instance is broken.
- **Per-user rotation is manual.** There is no bulk "reset every user's password / 2FA" action —
  loop `PATCH /admin/users/:id` and `POST /admin/users/:id/reset-2fa` over the user list from
  `GET /admin/users`.
- Contact data on this instance includes other people who never had accounts (the contacts
  themselves). A host-level compromise exposes their personal data too; a serious breach notice
  should say so.

## After the incident

- Write down the timeline while it's fresh: first sign, what you found, what you rotated, what you
  restored, what you told whom.
- If a step in this runbook was wrong, missing, or slower than it should have been — fix it here now.
  A runbook only improves right after it's been used.
- Re-run `cmd/audit-verify` on the recovered instance and keep the output.
- If the root cause was a code or config weakness, file it and reference this incident.

## What this was verified against

The rotation and revocation procedures above were exercised on 2026-08-27 against a build of the
backend at the then-current `main`, with SQLite, using the shipped CLIs and HTTP API. Observed:

| Procedure | Observed |
|---|---|
| `JWT_SECRET_KEY` rotated, **no `DATA_ENCRYPTION_KEY` set** | Server refused to boot: `FTL "Failed to initialize at-rest encryption" … "cipher: message authentication failed"`. Recovered by running `rotate-at-rest-key -new <fresh>` with the old `JWT_SECRET_KEY` in the environment, persisting `DATA_ENCRYPTION_KEY`, then starting with the new `JWT_SECRET_KEY`. |
| `JWT_SECRET_KEY` rotated (at-rest key decoupled) | Old session cookie → `401 "Invalid token signature"`. API token → still `200` (not a JWT). Stored calendar-subscription password → sync failed with `"authentication failed - check username and password"` where the same call pre-rotation failed only at the network layer — i.e. `DecryptCredential` no longer recovers the stored secret. |
| `POST /api-tokens/:id/rotate` | Old token → `401 "Invalid token"`; new token works. |
| `POST /api-tokens/revoke-all` | `{"revoked":N}`; the token used to make the call is itself rejected immediately afterward. |
| Admin `PATCH /admin/users/:id` with `password` | Target's session cookie → `401 "Session expired, please sign in again"`; target's API token → `401 "Invalid token"`. |
| API token against `/admin/*` | `403 "API tokens cannot access admin endpoints"` — admin actions require a session cookie. |
| `cmd/audit-verify` on an untampered DB | `"Audit hash chain is intact on <path>"`, exit 0. |

Not exercised end to end (mechanism confirmed from the code, not run live): the full TOTP re-enrolment
ceremony, and identity-provider-side OIDC client-secret rotation. Both are mechanically simple and
their session/credential effects are covered by the rows above.
