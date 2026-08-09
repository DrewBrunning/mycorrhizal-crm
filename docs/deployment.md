---
title: Deployment
nav_order: 7
has_children: false
---

# Deployment

For initial setup see [Getting Started](getting-started.html). This page covers production-specific concerns.

## How the Docker Setup Works

Mycorrhizal CRM runs as a single all-in-one container. Inside it, nginx serves the React SPA and proxies all `/api/`, `/carddav/`, and `/.well-known/carddav` requests to the Go backend on `127.0.0.1:8080`; the backend is never exposed to the host directly. Only the nginx port is published (default `7300`). This same-origin proxy is built in, so nothing extra is required.

Rate limiters (auth, API, CardDAV, account lockout) are in-memory and per-process; they reset on restart and are not shared across replicas if you run more than one backend instance.

You only need an external reverse proxy for TLS termination. Point it at the published port (default `7300`):

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
| `JWT_SECRET_KEY` | Generate with `openssl rand -base64 32` |


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

### Security notes

- The backup file contains **all** user data at its full sensitivity (including `private`/`secret`
  fields, email addresses, and anything attached). Treat it like a copy of the database: protect it
  in transit and at rest (e.g. `age`/`gpg`/encrypted volume) and rotate it off the server.
- Test restores regularly. The cheapest way is the restore procedure above against a throwaway
  instance or copy of the data.
