# Data retention & deletion lifecycle

The answer to "where does every copy of this piece of data live, and when does the last one go away?"
for every data type this app produces. `docs/security/asvs-l2.md` V8.3.8/V8.3.2 cite into this doc rather
than re-deriving the same evidence; `docs/security/threat-model.md` covers what "compromised" means for
each asset, not how long it survives.

| | |
|---|---|
| **Last updated** | 2026-08-25 (issue [#414](https://github.com/DrewBrunning/mycorrhizal-crm/issues/414)) |
| **Scope** | Backend (Go/Gin + SQLite), CardDAV/CalDAV (server role), Android client, browser/frontend, operator backups. |
| **Companion docs** | `docs/security/asvs-l2.md` V8 (Data Protection), `docs/deployment.md` (Backups section — the authoritative backup/restore runbook), `docs/security/masvs-l1.md` (Android storage controls). |

Every row below answers the same four questions from the issue: **where** it's stored and who can reach
it, **how long** it's retained, **how** deletion happens and whether it propagates to other copies, and
**whether it survives in backups**.

## The pattern this doc mostly confirms, not invents

[CLAUDE.md](/CLAUDE.md)'s backend trap #7 already fixes the *policy* per model (soft-delete for
user-authored content, hard-delete for edge/join rows), and the T26 purge job
(`backend/services/purge_service.go`) already hard-deletes soft-deleted rows past a retention window.
What follows enumerates every *copy* of the data — not just the primary table row — and checks each one
against the same four questions. Most rows below turn out to already be correct, wired years before this
doc; a handful of genuine gaps are called out explicitly in [Known gaps](#known-gaps).

## 1. Primary application data (user-authored content)

`Contact`, `Note`, `Activity`, `Reminder`, `LifeEvent`, `Preference`, `CadencePolicy`,
`ConversationAgenda`, `Gift`, `ImmichConfig`/`PaperlessConfig`/`SeafileConfig`/`WebDAVConfig`,
`Attachment` (metadata row only — see [§5](#5-attachments--profile-photos-files-on-disk)).

- **Where / who**: `mycorrhizal.db`, scoped by `user_id` in every query (CLAUDE.md trap #5). Reachable
  only via the authenticated owner's API session.
- **Retention**: live until the user deletes it; soft-deleted (`deleted_at` set) for
  `DELETE_RETENTION_DAYS` (default 30, `config/config.go:70,150`) as an undo window (audit `Undo`,
  `audit_controller.go`) and a sync tombstone (T17 `?since=` feed).
- **Deletion / propagation**: `DeleteContact` (`backend/controllers/contact_controller.go:829-886`)
  cascades every dependent row via `deleteContactAssociations`
  (`backend/controllers/contact_controller.go:686+`) inside one transaction; `DeleteUser`
  (`backend/controllers/admin_user_controller.go`) does the account-wide equivalent. After the retention
  window, `PurgeSoftDeletedRows` (`backend/services/purge_service.go:25-135`) hard-deletes the row and
  its remaining edge references, run daily by cron and on-demand via the admin `TriggerPurge` endpoint
  (`admin_user_controller.go:37-42`). A `?since=` cursor older than the window gets `410 Gone`
  (`controllers/helpers.go:360-370`) — deliberately the *same* `DeleteRetentionDays` config the purge job
  reads, so a client can never observe a tombstone gap; propagation to CardDAV/CalDAV and the Android
  mirror is covered in §7/§8, both of which key off this same soft-delete state.
- **Backups**: yes, full row (including still-in-window soft-deleted rows) — see [§10](#10-backups).
- **Verification**: `backend/services/purge_service_test.go` (`TestPurgeSoftDeletedRows_*`, 8 cases
  including idempotency and "never touches live rows"); `admin_user_controller_test.go` M1/M1b/M5
  (window-pinned purge, live rows untouched, `TriggerPurge` executes).

### ContactShare snapshots (issue #574)

`ContactShare` (`backend/models/contact_share.go`) is a **frozen, independent copy** of a filtered
JSContact export of a contact, serialized once at share-creation time — no `deleted_at`, no FK back to
the original `Contact` (by design: the snapshot must survive the original being edited or deleted).
It sits outside the soft-delete model above precisely because it is a copy, not the live row.

- **Where / who**: `contact_shares` table, scoped to `from_user_id`/`to_user_id`. Reachable only via the
  authenticated sender's or recipient's API session.
- **Retention**: `CONTACT_SHARE_RETENTION_DAYS` (default 30, `config/config.go:72,153`) for **every
  status**, anchored on the moment the share stopped being actionable: `pending` ages from `created_at`
  (an abandoned invite cannot materialize years later); `accepted`/`declined` age from `responded_at`
  (the recipient has already imported or rejected the snapshot).
- **Deletion / propagation**: `PurgeExpiredContactShares`
  (`backend/services/contact_share_purge_service.go`) hard-deletes rows past the window, run under the
  same daily cron + job lock as the T26 purge (`PurgeDeletedRows`, `backend/services/purge_service.go`)
  and via the admin `TriggerPurge` endpoint. Deleting the snapshot destroys no one's real data: the
  sender's original `Contact` is untouched (no FK, by design), and an accepted share's payload was
  already imported into the recipient's account. `CONTACT_SHARE_RETENTION_DAYS<=0` is treated as
  "disabled", never "delete everything".
- **Backups**: yes — a share sits in every backup taken before the purge removes it; restoring such a
  backup resurrects a share whose purpose (and both parties' copy of it) is long since served.
- **Verification**: `backend/services/contact_share_purge_service_test.go`
  (`TestPurgeExpiredContactShares_*` — window-pinned per status, ages-responded-from-responded-at,
  `NULL responded_at` never purged, disabled-when-zero, idempotent).

## 2. Edge- and join-shaped rows

`RelationshipEdge`, `CircleMember`, `ContactTag`, `HouseholdMember`, `ContactSyncLink`,
`CalendarEventLink`, `FieldValue`, `activity_contacts`, `NotificationDelivery`, `ReachOutSuggestion`,
`ContactSyncConflict`.

- **Where / who**: same DB, same `user_id` scoping.
- **Retention**: none — CLAUDE.md trap #7 hard-deletes these immediately (a natural-key unique index
  would otherwise block re-creating the same edge after a soft-delete ghost).
- **Deletion / propagation**: removed synchronously in the same transaction as the owning entity's delete
  (`deleteContactAssociations`, `PurgeSoftDeletedRows`'s `cleanups` slice for anything that outlives a
  purged contact as defense-in-depth).
- **Backups**: no — gone before any subsequent backup is taken (unless a backup predates the delete, in
  which case restoring it resurrects the edge along with its endpoints — see §10).

## 3. Audit trail (`AuditEvent`)

- **Where / who**: `audit_events` table, append-only, tamper-evident hash chain (issue #381,
  `models/audit_chain.go`). Deny-listed fields never enter it (`models/audit.go`).
- **Retention**: `AUDIT_RETENTION_DAYS` (default 90, `config/config.go:71,151`) — independent from the
  30-day content-deletion window, since audit's job is post-hoc investigation, not undo.
- **Deletion / propagation**: `PurgeExpiredAuditEvents` (`backend/services/audit_purge_service.go:26-45`)
  hard-deletes rows older than the window and re-links the surviving hash chain
  (`models.RecomputeAuditChain`) so tamper-evidence isn't broken by the purge itself. Runs daily via cron
  (`backend/main.go:184-190`); `AUDIT_RETENTION_DAYS<=0` is treated as "disabled", never "delete
  everything". No external mirror — audit never syncs to CardDAV/CalDAV/Android.
- **Backups**: yes, and a restored backup's audit trail is only as fresh as the snapshot.
- **Verification**: `backend/services/audit_purge_service_test.go` (`TestPurgeExpiredAuditEvents*`, 3
  cases including the re-link and a swallowed-recompute-failure case).

## 4. Sessions & short-lived secrets

Session/JWT cookies, TOTP recovery codes, password-reset tokens, API tokens.

- **Where / who**: httpOnly cookie (session) or DB row (API tokens, hashed); never in `localStorage`
  (pinned by the #419 Playwright regression, `frontend/e2e/`).
- **Retention**: token expiry per type (session TTL, reset-token TTL, recovery-code single-use — deleted
  in the same `WHERE` that consumes them, `services/twofactor.go:171-175`). API tokens live until
  revoked/rotated (issue #413).
- **Deletion / propagation**: logout clears the cookie server-side and `USER_INFO_KEY` client-side
  (`frontend/src/auth.ts:172`); revoke-all/rotate invalidate DB rows immediately.
- **Backups**: API token hashes yes; session cookies no (never persisted server-side to begin with).

## 5. Attachments & profile photos (files on disk)

- **Where / who**: `PROFILE_PHOTO_DIR` / `ATTACHMENTS_DIR` on the host filesystem, one file per
  attachment/photo, filenames server-generated (never user-controlled — path-traversal-safe).
- **Retention**: the metadata row (`Attachment`) follows §1's soft-delete/purge window, **but the file
  itself is deleted immediately**, not window-delayed (N7, comment at
  `backend/services/purge_service.go:48-49`): "an attachment's content IS the file; a tombstone that
  outlives a vanished file is acceptable for the change feed, an orphaned file is a leak."
- **Deletion / propagation**: `DeleteAttachment` (`backend/controllers/attachment_controller.go:244-263`)
  and `DeleteContact`'s `deleteContactPhotos`/`deleteContactAttachmentFiles`
  (`contact_controller.go:888-939`) remove the file from disk right after the DB transaction commits (file
  deletion can't be rolled back, so it happens after, not inside, the transaction). A failed file removal
  is logged as a leak but never fails the request.
- **Backups**: yes, if the operator's backup includes the photo/attachment directories — `docs/
  deployment.md`'s Backups section is explicit that a DB-only backup is *not* a complete backup for
  exactly this reason, and ships `rsync` commands for both directories alongside `make backup`.

## 6. Full-text search index (SQLite FTS5)

`contacts_fts`, `notes_fts`, `activities_fts` — derived, rebuildable indexes
(`services.RebuildSearchIndex`), not a second source of truth.

- **Where / who**: same SQLite file, `user_id UNINDEXED` column scopes every query to one user without a
  join.
- **Retention**: zero lag past the base table — `AFTER UPDATE`/`AFTER DELETE` triggers on `contacts`/
  `notes`/`activities` remove the FTS row the instant `deleted_at` is set (soft delete) or the row is hard
  deleted (`backend/database/migrations/000007_search_fts5.up.sql`,
  `000020_search_phones.up.sql`). A soft-deleted row is therefore unsearchable immediately, not merely
  after the purge window.
- **Backups**: yes, but irrelevant — a restore always includes the matching FTS state for that snapshot
  (same file), and a corrupted/missing index is always rebuildable from the base tables.

## 7. CardDAV / CalDAV (this app as the *server*)

External DAV clients (phones, desktop DAV apps) sync against `backend/carddav`, `backend/caldav`.

- **Where / who**: served straight from the live `contacts`/`activities`/`life_events` tables — no
  separate DAV-side copy exists.
- **Retention**: matches §1 exactly — a DAV client sees whatever the primary table currently shows.
- **Deletion / propagation**: `go-webdav` v0.7.0 (this project's DAV library) has **no RFC 6578
  sync-collection REPORT support** — there is no incremental delta protocol to push a tombstone through.
  Instead, `ListAddressObjects`/`ListCalendarObjects` query with GORM's default scope
  (`backend/carddav/backend.go:266-284`, no `.Unscoped()`), which — like every other query in this
  codebase — silently excludes soft-deleted rows. A deleted contact/event simply stops appearing in the
  *next* full listing a client requests (PROPFIND/REPORT), which is how these clients already detect
  removal without a native delta protocol. `DeleteAddressObject` (`backend/carddav/backend.go:385-413`)
  performs the same soft delete a REST client would, so a delete initiated *from* a DAV client propagates
  back into §1's normal lifecycle (undo window, purge, etc.) identically.
  - **Practical implication**: propagation isn't "instant" in the push sense (there is no push) — it is
    "as fresh as the client's next poll", which is the existing, already-shipped design (see the ETag
    comment at `backend/caldav/backend.go:314`), not a gap this ticket introduces or needs to close.
- **Backups**: not a separate store — see §1.

## 8. Android offline mirror (Room)

- **Where / who**: `AppDatabase`, SQLCipher-encrypted end to end (issue #385,
  `android/core/data/src/main/kotlin/.../local/RoomCacheEncryption.kt`), keyed via Android Keystore
  (`RoomPassphraseStore.kt`). Local to the device; unreadable without the app's key even with root file
  access.
- **Retention**: a rebuildable cache, not a second source of truth — rows persist only until the next
  sync disagrees with them.
- **Deletion / propagation**: `ContactRepositoryImpl.applySync` (`android/core/data/src/main/kotlin/.../
  repository/ContactRepositoryImpl.kt:248-256`) reads the T17 `sync.incremental` id list from every list
  response and calls `dao.deleteByIds(ids)` — the same tombstone mechanism §1/§7 rely on, consumed
  correctly here too. A direct on-device delete (`deleteContact`) also removes the cached row immediately
  on success (`ContactRepositoryImpl.kt:137-141`). On logout or an invalidated session,
  `LocalDataCleaner.clear()` (`android/core/data/src/main/kotlin/.../local/LocalDataCleaner.kt`) calls
  `AppDatabase.clearAllTables()` **and** deletes `context.cacheDir` recursively — wiping the FTS4 index,
  Coil's photo disk cache, and the vCard-share `FileProvider` staging area together, so a "stolen device
  after logout" story leaves nothing recoverable outside the (encrypted) DB the OS itself controls.
- **Backups**: none — this is a device-local cache with no server-visible backup; Android's own
  Auto Backup is out of scope for app-internal DB files of this kind and isn't configured for it.

## 9. Browser-side storage (frontend SPA)

- **`localStorage`**: holds only `user_info` (id/username/admin flag/self-contact UID) and UI
  preferences — no auth token, no contact PII (`frontend/src/auth.ts:15,120`; pinned as a regression by
  the #419 Playwright spec asserting no long-lived credential ever lands in `localStorage`/
  `sessionStorage`/`IndexedDB`/URL). Cleared on logout (`auth.ts:172`).
- **Auth token**: httpOnly session cookie — never readable by JS or the service worker, so there is
  nothing for either to leak or need to clear.
- **Service worker cache**: `frontend/src/service-worker.ts` precaches only the static build (app shell)
  and runtime-caches same-origin `.png` requests (CRA's default scaffold route, `StaleWhileRevalidate`,
  LRU-capped at 50 entries). It never intercepts API calls — contact photos ship as base64 data URIs
  *inside* JSON API responses (`models/contact_record.go`'s `buildMedia`/`photostore`), not as separate
  image requests, so no contact PII ever passes through this cache. **No IndexedDB usage anywhere in the
  frontend.**
- **Backups**: not applicable — device-local, ephemeral, not part of any server backup.

## 10. Backups

- **Where / who**: operator-controlled, off-instance storage the operator chooses (`docs/deployment.md`
  Backups section). `make backup` (`backend/cmd/backup/main.go` → `database.BackupSnapshot`,
  `backend/database/backup.go`) takes a live `VACUUM INTO` snapshot — safe under WAL, verified with
  `PRAGMA integrity_check` before it's considered valid — of the SQLite file; photos/attachments are
  backed up separately (`rsync`, since they're plain files, not DB rows).
- **Retention**: **entirely operator-owned** — this app has no built-in backup rotation, expiry, or
  auto-deletion of old backup files. A cron-scheduled `make backup` will accumulate snapshots forever
  unless the operator adds their own retention (e.g. a `find -mtime +N -delete` outside this app).
- **Deletion / propagation**: **does not happen automatically, ever** — deleting/purging live data has no
  effect on already-taken backup files. This is the one place in the whole lifecycle where "deletion
  propagates" is false by design, and `docs/deployment.md:165-167` already documents the direct
  consequence: restoring a backup **resurrects** anything that was soft-deleted (but not yet purged) as
  of that snapshot, since a file-level restore has no concept of "these rows were mid-undo-window".
  There is no partial/selective restore.
- **Backups of backups**: N/A — recursion stops here by definition.
- **Verification**: `frontend/e2e/backupRestore.spec.ts` (full backup → destroy → restore round trip);
  `backend/services/restore_drill_service.go` + issue #275 (scheduled restore-drill job that proves a
  backup actually restores, not just that the file exists).

## 11. Exports (CSV / vCard3 / vCard4 / jSContact / audit log)

- **Where / who**: generated per-request and streamed directly to the HTTP response
  (`backend/controllers/export_controller.go`) — never written to disk server-side (no `os.WriteFile`
  anywhere in the export path). Once downloaded, the file is the requesting user's own device/browser —
  outside this app's retention control by definition (same boundary as any file a user saves from any
  web app). Issue #416 added a fifth export in this same category: `GET /audit/export`
  (`ExportAuditLog`), an unbounded CSV of the caller's own audit trail — same generation/no-server-copy
  shape as the other four.
- **Retention**: nothing server-side to retain.
- **Deletion / propagation**: nothing to delete — there is no export artifact that outlives the request.
  Sensitive-above-`normal` fields and CSV-formula-injection payloads are filtered/neutralized *before*
  the export leaves the server (`sensitivity` classification, ASVS 8.3.4). The audit-log export's
  `before_snapshot` column is omitted unless the caller explicitly passes `?include_snapshots=true`: it
  is already credential-redacted at write time (`auditDenyList`, `models/audit.go`) but is **not**
  filtered by contact-field sensitivity the way the other four exports are, so it is gated behind its own
  explicit opt-in rather than reusing `include_sensitive`.
- **Backups**: not applicable.

## 12. Import wizard staging

- **Where / who**: `ImportSessionManager` (`backend/services/import_session.go`) — **in-memory only**,
  never persisted to disk or DB.
- **Retention**: `sessionExpiry = 15 * time.Minute` (`import_session.go:18`). A tiny post-confirm
  tombstone (`confirmedImport`) survives a little longer so a client that lost the confirm response can
  retry idempotently, but it holds only the result/owner/expiry, not the original uploaded rows.
- **Deletion / propagation**: expires on its own timer; also fully lost on process restart (in-memory,
  by design — an import mid-flight during a restart must be redone, which is an acceptable trade for
  never writing uploaded-but-unconfirmed contact data to disk).
- **Backups**: never — can't be backed up if it was never persisted.

## 13. External integration credentials (WebDAV / Paperless / Immich / Seafile)

- **Where / who**: one config row per user per integration, app-password/API-key encrypted at rest
  (`services/credential_crypto.go`); the plaintext credential is never returned by the read endpoint.
- **Retention**: soft-delete with the T26 partial-unique-index pattern (a user may remove and
  re-add a connection without a soft-deleted ghost blocking it).
- **Deletion / propagation**: hard-deleted on account removal (`admin_user_controller.go:705-720`).
  **Deleting the connection here never deletes anything in the external service** — Immich/Paperless/
  Seafile/WebDAV content lives entirely under the user's own account on their own external service; this
  app only ever stores a reference/credential, never a durable mirror of that content. That boundary is
  deliberate, not a gap: this app has no authority to delete data the user manages in a separate product.
- **Backups**: only the encrypted credential row; the external content is that service's own backup
  story.

## Known gaps

One item surfaced by walking every data type through the four questions above. It does not block this
ticket's "at minimum" bullets (T26 purge + CardDAV/CalDAV + Android propagation are all already correct,
per §1/§7/§8), but it is a genuine, named gap rather than a silently-accepted one.

1. **API responses carry no `Cache-Control: no-store`.** Already tracked and accepted as `partial` at
   ASVS 8.1.1/8.2.1 (`docs/security/asvs-l2.md`) — noted here only because it's the one place a stale
   *cached* copy of contact PII could theoretically outlive a delete, on a shared/proxied browser. Not
   re-litigated here; that row already owns the gap and its own fix.

## Verification summary

| Data type | Retention pinned by test? |
|---|---|
| Soft-deleted rows / purge window | `backend/services/purge_service_test.go` (8 cases) |
| ContactShare snapshot purge window | `backend/services/contact_share_purge_service_test.go` (9 cases) |
| Audit retention + re-link | `backend/services/audit_purge_service_test.go` (3 cases) |
| Admin purge trigger + window | `backend/controllers/admin_user_controller_test.go` M1/M1b/M5 |
| Sync-horizon 410 Gone matches purge window | `backend/controllers/cursor_feed_test.go` |
| FTS index follows soft/hard delete | `backend/database/migrate_test.go`, FTS trigger coverage |
| Android mirror wiped on logout | `LocalDataCleaner` — see Android test suite |
| Android mirror deletes tombstoned ids | `ContactRepositoryImpl` sync tests (`core/data/src/test/.../repository/`) |
| No PII/credential in browser storage | `frontend/e2e/` (#419 Playwright regression) |
| Backup restore actually restores | `frontend/e2e/backupRestore.spec.ts`, restore-drill job (#275) |
