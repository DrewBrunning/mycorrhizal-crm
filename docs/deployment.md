---
title: Deployment
nav_order: 8
has_children: false
---

# Deployment

For initial setup see [Getting Started](getting-started.html). This page covers production-specific concerns.

## How the Docker Setup Works

Mycorrhizal CRM runs as a single all-in-one container. Inside it, nginx serves the React SPA and proxies all `/api/`, `/carddav/`, and `/.well-known/carddav` requests to the Go backend on `127.0.0.1:8080`; the backend is never exposed to the host directly. Only the nginx port is published (default `7300`). This same-origin proxy is built in, so nothing extra is required.

Rate limiters (auth, API, CardDAV, account lockout) are in-memory and per-process; they reset on restart and are not shared across replicas if you run more than one backend instance.

You only need an external reverse proxy for TLS termination. Point it at the published port (default `7300`):

See [Deployment security baseline](security/deployment-baseline.md) for the full recommended
hardening checklist (HSTS, secure cookies, container capabilities, resource limits, firewall/DNS)
and an explicit statement of what this application does not secure (host OS, Docker daemon, reverse
proxy, TLS certs, DNS, firewall, host admins).

```nginx
server {
    listen 443 ssl;
    server_name mycorrhizal.example.com;

    location / {
        proxy_pass http://localhost:7300;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto https;
    }
}
```

## Production Environment

Set these variables in `.env` when running over HTTPS:

| Variable | Value |
|---|---|
| `FRONTEND_URL` | Exact origin, e.g. `https://mycorrhizal.example.com` (never `*`) |
| `COOKIE_SECURE` | `true` |
| `COOKIE_DOMAIN` | Your domain |
| `JWT_SECRET_KEY` | Generate with `openssl rand -base64 32`; the server refuses to start with the `.env.example` placeholder or a weak secret |


## Single Sign-On (OIDC)

Mycorrhizal CRM supports SSO via any OpenID Connect provider (Keycloak, Google, Authentik, Authelia, etc.). When enabled, a **Sign in with provider** button appears on the login page.

### Setup

1. Register a new OAuth2 client with your provider. Set the redirect URI to:
   ```
   https://mycorrhizal.example.com/api/v1/auth/oidc/callback
   ```
   This is derived automatically from `FRONTEND_URL`, no separate variable needed. If your provider
   supports RP-Initiated Logout and requires a `post_logout_redirect_uri` to be pre-registered too, it's
   likewise derived from `FRONTEND_URL`:
   ```
   https://mycorrhizal.example.com/login
   ```

2.  Set the OIDC environment variables in the docker compose. See [Getting-Started → Environment variables](getting-started.html#environment-variables) for details. SSO is disabled if any of the first three variables are missing.

### Account linking

On first SSO login, the backend attempts to match the OIDC identity to an existing account in this order:

1. **Subject match** — if the user has logged in via this provider before, their account is found directly.
2. **Email match** — if the provider returns a *verified* email that matches an existing account, the OIDC identity is linked to that account automatically. Unverified emails are ignored to prevent account takeover (except if `OIDC_TRUST_EMAIL=true` is set).
3. **Auto-provision** — if `OIDC_AUTO_PROVISION=true` and no account matched, a new account is created using the email/name from the provider.

If auto-provisioning is disabled and no match is found, the user sees an error and must be registered manually first.

### Passwords

Accounts created through SSO have no password and can only log in via SSO. Existing password-based accounts that get linked retain their password.

## Upgrades

```sh
docker compose pull
docker compose up -d
```

Database migrations run automatically on startup.

## Verifying release artifacts

Every published Docker image is signed and carries SBOM + SLSA provenance attestations; the
Android release APK is additionally cosign co-signed and carries its own build provenance. Before
deploying a new release, or any time you want to confirm a downloaded binary/container actually
came from this project's official build pipeline unmodified, see
`docs/security/release-verification.md` for exact commands (`cosign`, `gh attestation verify`) —
no supply-chain background required.

## Health, liveness & readiness endpoints

The server exposes three unauthenticated, secret-free health endpoints (issue #421). They answer
different questions, and wiring the wrong one to the wrong consumer causes outages (e.g. a monitor
restarting a healthy app because an optional integration is down).

| Endpoint | Question | Cost | Who should hit it |
|---|---|---|---|
| `GET /health/live` | Is the process running? | Instant — touches nothing | Container / orchestrator **restart policy** (`HEALTHCHECK`, Kubernetes `livenessProbe`) |
| `GET /health/ready` | Can this instance serve? | DB ping + migration state + filesystem write probe | **Load balancer / traffic gate** (Kubernetes `readinessProbe`) |
| `GET /health` | Is the CRM actually operational? | Deep — persisted check outcomes, job locks, integration reachability (30 s cached) | **Humans and monitoring aggregators** (dashboards, alerting) |

- `/health/live` returns `200 {"status":"live"}` and never does I/O. It must not fail because a
  dependency is slow — that would make the orchestrator kill a working process.
- `/health/ready` returns `200 {"status":"ready", ...}` or `503 {"status":"not_ready", ...}`. It is
  `not_ready` while the database or a configured file directory is unreachable, or while migrations
  are pending or dirty. Gate inbound traffic on this, not on `/health`.
- `/health` returns `healthy`, `degraded`, or `unhealthy` with a per-facet `checks` breakdown
  (database read/write, migration lag, last DB-integrity-check and restore-drill outcomes,
  background-job locks, and reachability of server-scoped integrations — OIDC, email, FCM).
  **`degraded` is still HTTP 200** — an unreachable optional integration or a stale scheduled job is
  degraded-but-alive, not down. Only a database read failure returns `503`. This is the endpoint
  that reports the running build's version/commit.

The bundled Docker image's `HEALTHCHECK` and the CI boot-wait probes all use `/health/live`. The
single-container nginx is not a load balancer, so `/health/ready` is not wired to anything by
default — put it in front of your own proxy/LB if you run more than one instance.

All three endpoints are unauthenticated (like the original `/health`) and deliberately carry no
secrets. The deep endpoint's `reason`/`detail` strings are generic categories only — the underlying
errors, absolute paths, the SMTP host/OIDC URL, and any table names or row counts from a failed
integrity check / restore drill go to the **server log and the failure webhook**, never the
response body. It does expose the build version/commit, the applied migration number, and
scheduled-job names — same class of operational metadata the pre-split `/health` already returned.
If you consider even that too much for an unauthenticated endpoint, put `/health` behind your proxy's
auth and leave `/health/live` + `/health/ready` open. The all-in-one image's nginx serves all three
with `X-Robots-Tag: noindex` so they are never crawled.

## Diagnostics & logging

When something is failing in production, `docs/operations/observability.md` is the map: the
structured log field vocabulary, how a correlation ID threads one UI action through every background
step and outbound call it triggers (and how to grep for it), and the admin **System events**
timeline (`/system-events` on web, Settings → System events on Android) — a persisted, restart-
surviving record of scheduled job runs, sync runs, backup/restore drills, and migrations.

Quick reference: `LOG_LEVEL` (`info` default), `LOG_PRETTY` (JSON off in release), and
`SYSTEM_EVENT_RETENTION_DAYS` (`30` default) for how long the timeline keeps rows.

## Backups

A backup is two (or three) independent pieces, and missing any of them means the backup is not a
backup:

| Piece | Where it lives | Docker volume |
|---|---|---|
| SQLite database | `SQLITE_DB_PATH` (default `mycorrhizal.db` / `/app/data/mycorrhizal.db` in the image) | `DATA_PATH` (default `./data`) |
| Profile photos | `PROFILE_PHOTO_DIR` (default `/app/static/photos`) | `PHOTOS_PATH` (default `./photos`) |
| Attachments (N7) | `ATTACHMENTS_DIR` (default a sibling of the photos) | `ATTACHMENTS_PATH` (default `./attachments`) |

Photos and attachments live **outside** the SQLite file, so backing up only the `.db` silently loses
them. They are plain directories; a file-level copy (`rsync`/`cp`) is exactly right for them.

### Why you cannot just copy the `.db` file while the server runs

The database runs in WAL mode. A running server has committed writes sitting in a sidecar
`-wal` file; copying only `mycorrhizal.db` misses them, and a copy taken mid-write can be torn.
`VACUUM INTO` (below) produces a single self-contained snapshot that is valid even while the server
is running — that is the recommended online procedure.

### Online backup (no downtime)

If the backend directory and a Go toolchain are available on the host (e.g. a clone of this repo
used to operate the instance), `make backup` reads `SQLITE_DB_PATH` (exactly like `make migrate-up`)
and writes a timestamped `VACUUM INTO` snapshot beside the database:

```sh
# inside backend/ with the same environment the server uses
make backup
# → Backed up /path/to/data/mycorrhizal.db to /path/to/data/mycorrhizal-20260809-120000.db
```

It runs a best-effort WAL checkpoint first (a tidy-up, not a requirement —
`VACUUM INTO` reads through the WAL, so the snapshot is complete regardless),
refuses to overwrite an existing file, and verifies the result with
`PRAGMA integrity_check` before reporting success. Set `BACKUP_PATH` to choose
the output location instead of the timestamped default:

```sh
BACKUP_PATH=/backups/mycorrhizal-$(date +%F).db make backup
```

No Makefile/Go? Any SQLite client works — the operation is a plain SQL statement:

```sh
sqlite3 /path/to/data/mycorrhizal.db "PRAGMA wal_checkpoint(TRUNCATE); VACUUM INTO '/backups/mycorrhizal.db';"
```

(If the target already exists, `VACUUM INTO` errors rather than overwriting —
remove it or pick a fresh path first.)

Then back up the two directories (they cannot be snapshotted via SQLite):

```sh
rsync -a /path/to/photos/ /backups/photos/
rsync -a /path/to/attachments/ /backups/attachments/
```

For the all-in-one Docker image, the host paths are whatever `DATA_PATH`/`PHOTOS_PATH`/
`ATTACHMENTS_PATH` resolve to (defaults `./data`, `./photos`, `./attachments` next to your
`docker-compose.yml`), so `make backup` from the host against `SQLITE_DB_PATH=./data/mycorrhizal.db`
backups up the same file the container writes.

### Offline backup (downtime, simplest)

1. Stop the server: `docker compose stop`. A clean stop checkpoints the
   WAL, so the `.db` file is then complete on its own.
2. Copy the database, photos, and attachments:
   ```sh
   cp /path/to/data/mycorrhizal.db /backups/mycorrhizal.db
   rsync -a /path/to/photos/ /backups/photos/
   rsync -a /path/to/attachments/ /backups/attachments/
   ```
3. Start the server again: `docker compose start`.

### Restore

A restore is a deliberate **point-in-time rollback**: it replaces the whole instance with the
snapshot, so anything created or edited *after* the backup was taken is lost — and anything that had
been soft-deleted (but not yet purged, see T26) before the backup is resurrected. That is the
expected meaning of restoring a file-level backup; there is no partial/merge restore.

1. Stop the server: `docker compose stop`.
2. Replace the three pieces from backup. For a database backup produced by `VACUUM INTO`, the
   snapshot file is self-contained — drop any `-wal`/`-shm` files that may sit beside the live
   database first:
   ```sh
   rm -f /path/to/data/mycorrhizal.db /path/to/data/mycorrhizal.db-wal /path/to/data/mycorrhizal.db-shm
   cp /backups/mycorrhizal.db /path/to/data/mycorrhizal.db
   rsync -a --delete /backups/photos/ /path/to/photos/
   rsync -a --delete /backups/attachments/ /path/to/attachments/
   ```
   `rsync --delete` matters: it removes files that were added to the photo/attachment directories
   after the backup, so the directories match the snapshot instead of blending old and new.
3. Start the server: `docker compose start`. Migrations run automatically on startup.
4. **Verify** — a restore that has never been tested is a hypothesis. Log in and check that a known
   contact, a recent note, and a reminder are present; for the photo/attachment directories, open a
   contact's photo and download an attachment. The automated check behind this page's procedure is
   `frontend/e2e/backupRestore.spec.ts`, which backs up a populated instance, destroys the database
   and both directories, restores, and asserts every entity type survived.

The JWT secret key lives in your environment (`.env`), not in the database — restoring a
backup does not change which key the server uses. If you rotated `JWT_SECRET_KEY` between backup and
restore, any session tokens issued under the old key are unrecognized, so users simply have to log
in again. That is harmless.

### Automated integrity & restore checks

Two scheduled background jobs automate the two checks above so a silent failure is caught rather
than discovered during a real disaster:

- **Live DB integrity check** (issue #273) — runs `PRAGMA integrity_check` against the live
  database on a schedule (`DB_INTEGRITY_CHECK_ENABLED`, default on; `DB_INTEGRITY_CHECK_INTERVAL_HOURS`,
  default `24`).
- **Restore drill** (issue #275) — takes a fresh backup snapshot, restores it into a scratch
  database, compares every table's row count against the live database, and — since issue #420 —
  verifies the snapshot's wrapped DEK unwraps under the current master key, so a rotated/lost
  at-rest key is caught here rather than at the moment of need
  (`DB_RESTORE_DRILL_ENABLED`, default on; `DB_RESTORE_DRILL_INTERVAL_HOURS`, default `168`, i.e.
  weekly).

Either job logs and fires a webhook (`db.integrity_check_failed` / `db.restore_drill_failed`, see
Settings → Webhooks) on failure, so "test restores regularly" above is now something the app does
for you rather than a manual chore.

Since issue #428 these failures also flow through **alerting on state transitions**: a scheduled
evaluator watches the per-subsystem health, disk usage, and scheduled-job liveness, and dispatches
one notification when a condition breaks (`alert.raised`) and one when it clears
(`🟢 Backup recovered after 3 failures` — `alert.cleared`), de-duplicated so a persistent failure
does not page you on every run. Webhooks go to all subscribers; email/ntfy/Gotify/push go to admin
users. Full condition list and the `ALERT_*` env vars are in
`docs/operations/observability.md` → "Alerting on state transitions".

### Backup confidentiality & retention

A backup is a **complete copy of the CRM's sensitive data** — not a sanitized extract — so the rules
below are the same ones that apply to the database itself. This section is the confidentiality,
retention, and access statement for backups (issue #420); the per-data-type retention/deletion story,
including whether each type survives in backups, is §10 of `docs/security/data-retention-lifecycle.md`.

**What a backup actually contains.** The DB snapshot holds every user's data at full sensitivity:
`private`/`secret` contact fields, email addresses, notes, lifecycle data, attachment metadata, the
audit trail, password hashes, API-token hashes, and TOTP recovery-code hashes — plus soft-deleted
rows still inside their undo window (see below). The two plaintext directories (photos, attachments)
complete the backup. The snapshot does **not** contain the secrets that unlock it: `JWT_SECRET_KEY`
and `DATA_ENCRYPTION_KEY` live in the operator's environment, never in the database. Access to a
backup is operator access — whoever can read the filesystem or backup store it lands in can read the
backup, and the app itself never re-reads one (only the restore drill creates its own fresh
snapshot, into a scratch directory it deletes afterwards).

**Encryption & key management.** The DB snapshot inherits the database's field-level at-rest
encryption (issue #380): encrypted columns travel as `encv1:` ciphertext, and the wrapped data
encryption key (DEK) travels inside the snapshot (`data_encryption_keys`). Restoring therefore
requires the **same master key** the live instance uses — a restore under a different key fails
closed at boot (GCM authentication rejects the unwrap) instead of serving garbage. Two operator
consequences:

- Prefer a dedicated `DATA_ENCRYPTION_KEY` over the `JWT_SECRET_KEY` fallback. If the at-rest master
  key is derived from `JWT_SECRET_KEY` (the zero-config fallback), rotating that secret changes the
  derived key, so the stored wrapped DEK — in the live database *and* in every snapshot taken under
  the old key — can no longer be unwrapped: the server fails closed at boot, and restoring an old
  snapshot fails the same way. With a dedicated key, rotating the JWT secret never affects at-rest
  data or backups.
- Encrypted columns are only part of the story. The FTS-indexed columns (`notes.content`,
  `activities.*`, flat contact search fields) are plaintext by design — they must be, for FTS5
  indexing (`docs/security/asvs-l2.md` P4) — and the photos/attachments directories are plain files.
  **Protecting those at rest is the operator's job** (`age`/`gpg`/encrypted volume), exactly as it
  was before #380, and off-host movement should use the same protection in transit (`rsync` over SSH,
  TLS to object storage).

**Retention & deletion.** Retention is entirely operator-owned: this app has no backup rotation,
expiry, or auto-deletion — and deliberately won't, because an app that can expire backups gives an
attacker running as the app the same power (issue #505). A cron-scheduled `make backup` accumulates
snapshots until the operator imposes a lifecycle, e.g.:

```sh
find /backups -name 'mycorrhizal-*.db' -mtime +30 -delete   # keep 30 days of daily snapshots
```

**Deleting live data never deletes backups.** Purging contacts, notes, or any other data has no
effect on already-taken backup files. A restore is a point-in-time rollback: anything soft-deleted
(but not yet purged, T26) as of the snapshot is resurrected, and anything created or edited after
the snapshot is lost. Soft-deleted data therefore ages out of backups in two steps: it stops
appearing in *new* snapshots once the purge window (default 30 days, `DELETE_RETENTION_DAYS`) passes,
but it survives in every snapshot that predates that purge until the operator deletes the snapshot.
There is no partial/selective restore.

**Restore-environment security.** A restore is a real, destructive change, so the safe order is:
(1) let the automated restore drill (issue #275) do it into a throwaway first — it restores a fresh
snapshot into a scratch database weekly by default, compares every table's row count with the live
database, and since issue #420 also verifies the snapshot's wrapped DEK unwraps under the current
master key, so a rotated/lost key is caught weekly instead of at the moment of need; (2) for a real
restore, stop the server, replace the three pieces, and verify a known contact/note/reminder plus a
photo and an attachment (or run the `frontend/e2e/backupRestore.spec.ts` round trip on a throwaway
copy first). Restoring into an environment whose master key differs from the snapshot's fails closed
at boot — that is the intended signal to check keys before touching the live instance. The audit
trail in a restored database is only as fresh as the snapshot.
