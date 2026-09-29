# PII inventory & data-minimization review

The answer to a question the other security docs do not ask. `threat-model.md` asks *is it
protected?* `data-retention-lifecycle.md` (issue [#414](https://github.com/DrewBrunning/mycorrhizal-crm/issues/414))
asks *how long does it live and how is it deleted?* This document asks the **minimization**
question, per store: *should this data exist at all, is it more than we need, and is it kept
longer than we need?* — and records the answer.

| | |
|---|---|
| **Last updated** | 2026-09-29 (issues [#510](https://github.com/DrewBrunning/mycorrhizal-crm/issues/510), [#621](https://github.com/DrewBrunning/mycorrhizal-crm/issues/621), [#978](https://github.com/DrewBrunning/mycorrhizal-crm/issues/978), [#1316](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1316)) |
| **Scope** | Backend (Go/Gin + SQLite), CardDAV/CalDAV server role, structured + access logs, operator backups, Android offline mirror, browser storage. |
| **Companion docs** | `data-retention-lifecycle.md` (retention/deletion, cited here rather than repeated), `asvs-l2.md` V7 (logging) / V8 (data protection), `deployment-baseline.md` (operator boundary), `../privacy.md` (the plain-language operator/adopter summary), `../supported-versions.md` "The deployment shape" (the multi-user isolation guarantee and admin-capability statement, issue [#558](https://github.com/DrewBrunning/mycorrhizal-crm/issues/558)). |
| **Method** | Schema walked table-by-table from `backend/database/migrations/*.up.sql`; logs checked against **real captured output**, not by reading the logging code (see [How this was verified](#how-this-was-verified)). |

## Why this review exists

The data here is unusually sensitive even by CRM standards. It is a record of one person's
relationships, and much of it is **about third parties who never consented to being in it and
cannot see what is written about them** — free-text notes, inferred preferences, relationship
labels. Self-hosting means the data never leaves the operator's machine, which is a strong
starting point; it is not the same property as *"we store the minimum,"* and only the second one
survives scrutiny.

Two structural facts shape every row below:

- **Multi-user-per-instance is a supported `1.0.0` configuration** (issue
  [#558](https://github.com/DrewBrunning/mycorrhizal-crm/issues/558)). The operator is data
  controller not only over their own relationship data but over *other users'* — including notes
  those users wrote about people twice removed from the operator. Any **instance-wide** store
  (logs, audit trail, metrics) that attributes personal data to a specific user is a cross-user
  disclosure even though every API handler is correctly `user_id`-scoped.
- **There is no telemetry.** No analytics SDK, no crash reporter, no "usage statistics", no
  phone-home of any kind — `grep -rniE 'analytics|telemetry|sentry|posthog|mixpanel|phone.?home'
  backend/` is empty, and nothing in the frontend or Android client does it either. The only data
  that leaves the instance is what the operator explicitly configures: CardDAV/CalDAV sync,
  outbound email, push, webhooks, and the optional external integrations. This is the single
  biggest minimization result and it is a deliberate, defended position.

## How to read the tables

Each row carries a **necessity verdict**:

| Verdict | Meaning |
|---|---|
| `necessary` | The feature cannot work without it; the data is the minimum the feature needs. |
| `deliberate, documented` | More than the strict minimum, but a considered product decision with a written reason (here or in a linked doc). |
| `kept-longer-than-necessary → #NNN` | Retention exceeds need; remediation filed. |
| `more-than-necessary → #NNN` | Collection exceeds need; remediation filed. |

and a **scope**: `per-user` (rows carry `user_id`, reachable only by the owner's session) or
`instance-wide` (one store for the whole deployment).

---

## 1. Per-user application data

The contact graph and everything hung off it. All of this is content a user authored or imported;
all of it is `user_id`-scoped in every query (CLAUDE.md backend trap #5) and served only to the
authenticated owner.

| Store | Personal data | Subject | Necessity | Retention → lifecycle doc |
|---|---|---|---|---|
| `contacts` (+ nested `card`/`crm`/`passthrough` JSON) | Names, nicknames, emails, phones, addresses, birthdays, org/title, photos, free-text `how_we_met` / `work_information` / `contact_information`, gender, IM handles, URLs | The contact (a third party) | `necessary` — this is the product | Soft-delete + `DELETE_RETENTION_DAYS` (30) purge — §1 |
| `notes` | Free-text notes about a contact — the highest-sensitivity field in the system; written about someone who cannot see it | Third party | `necessary` | §1 |
| `activities` (+ `activity_contacts`) | `title` / `description` / `location` of meetings, calls, events; which contacts were present | Third party + user | `necessary` | §1 |
| `reminders`, `reminder_completions` | `message` free text, cadence, which contact | Third party + user | `necessary` | §1 |
| `attachments` | Metadata row for a file a user uploaded against a contact: `original_name` (a user-chosen filename, often the person's name), `content_type`, `size_bytes`, server-generated `stored_name`. The bytes live on disk (§5 below) | Third party | `necessary` | Soft-delete + `DELETE_RETENTION_DAYS` purge — §1; **the file itself is deleted immediately**, not window-delayed (§5) |
| `occasion_obligations` | An obligation tied to a contact (a birthday card, an anniversary gift): `label` and `notes` free text, `kind`, month/day anchor, lead time, `linked_life_event_id`, `sensitivity` | Third party | `necessary` | Soft-delete + purge — §1 (added to the purge list by issue [#1310](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1310); a deleted contact's obligations are removed with it) |
| `occasion_events` / `occasion_event_attendees` | An event the user is hosting or attending: `title`, `location`, `starts_at`/`ends_at`, free-text `notes`, `sensitivity`; the attendee rows list which contacts were invited and their `rsvp` | Third party + user | `necessary` | Events: soft-delete + purge — §1. Attendee rows are join-shaped and hard-delete (with the contact via `DeleteContact`, with the account via `DeleteUser`) — §2. Contact merge repoints them to the survivor rather than dropping them (issue [#1309](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1309)) |
| `cadence_policies`, `data_decay_policies` | A per-contact opt-in policy: target contact-interval in days (`cadence_policies`, with the qualifying activity types) and a data-freshness interval + `last_verified_at` (`data_decay_policies`). Keyed by the contact's UID, no free text | Third party (which contacts the user tracks, and how closely) | `necessary` | Soft-delete + purge — §1; removed with the contact by `DeleteContact` |
| `life_event_suggestion_resolutions` | Which suggested life event the user accepted or dismissed for a contact: `source_kind`, `source_entry_id`, `event_type`, `resolution` — no event content | Third party (UID only) | `necessary` — remembers a dismissal so it is not re-suggested | Hard-delete; removed with the contact by `DeleteContact` and with the account — §2 |
| `link_field_types` | User-defined link kinds for contact links: `name`, `protocol`, `category`, `icon`, ordering — a label vocabulary, no per-contact values | User | `necessary` | Soft-delete + purge — §1 (added by issue [#978](https://github.com/DrewBrunning/mycorrhizal-crm/issues/978)) |
| `life_events` | `type` / `date` / `description` of a contact's life events (birth, marriage, death, job change) | Third party | `necessary` | §1 |
| `preferences` | Inferred/observed likes, dislikes, dietary, gift ideas, free-text `notes`; `sensitivity` column (`normal`/`private`/`secret`) | Third party | `necessary` — but see `sensitivity` filtering below | §1 |
| `conversation_agenda` | Free-text things to raise next time; `reference_url` | Third party | `necessary` | §1 |
| `gifts` | Gift ideas/history, `occasion`, `value_cents`, free-text `description`/`notes` | Third party | `necessary` | §1 |
| `households` / `household_members`, `circles` / `circle_members`, `tags` / `contact_tags` | Grouping labels; `households.address`; membership by contact UID | Third party | `necessary` | §2 (edge rows hard-delete) |
| `relationship_edges` | Relationship type between two contacts, `metadata`, `sensitivity`; only `status=confirmed` is fact | Two third parties | `necessary` | §2 |
| `field_definitions` / `field_values` | Operator-defined custom fields and their per-entity values — arbitrary user-chosen data, `sensitivity` + `projection` columns | Third party | `necessary` (open-ended by design; `sensitivity` is the control) | §2 |
| `contact_sync_conflicts` | `local_value` / `remote_value` (encrypted) of a field that diverged during CardDAV sync | Third party | `necessary` for conflict resolution | §2 |
| `reach_out_suggestions` (+ `reach_out_cursors`) | `old_value` / `new_value` of an org/title/address change that triggered a "reach out" nudge; references an `audit_event_id` | Third party | `deliberate, documented` — derived from the audit trail, ages with it | `AUDIT_RETENTION_DAYS` (90) — §3; both `pending` and `dismissed` rows are hard-deleted by `PurgeExpiredReachOutSuggestions` once past the window (issue [#978](https://github.com/DrewBrunning/mycorrhizal-crm/issues/978)) |
| `dismissed_household_suggestions` | `address_hash` + `member_hash` — **hashed**, not the address or the members | — | `necessary` and already minimized (a good example) | hard-delete with user |
| `dismissed_duplicate_pairs` | Two contact UIDs the user said "not a duplicate" | Third party (UIDs only) | `necessary` | hard-delete with user |

**`sensitivity` classification.** `preferences`, `relationship_edges`, `field_definitions`, and
`contacts`' nested fields carry a `normal`/`private`/`secret` marker. Anything above `normal` is
excluded from external sync, contact shares, and the neutral-`Card` exports (vCard 3/4, JSContact)
**in the query, not in the caller** (CLAUDE.md backend conventions). This is the mechanism that
keeps the most sensitive third-party data from leaving the instance even when the user turns on
CardDAV. It governs copies that leave the instance or reach another party — it is not an
access-control tier against the owning user, so the flat CSV backup (`GET /export`) deliberately
carries every sensitivity, labelled by column; see `data-retention-lifecycle.md` §11 and issue
[#861](https://github.com/DrewBrunning/mycorrhizal-crm/issues/861).

**At-rest encryption (protection note, not minimization).** Contact free-text, `preferences`,
`conversation_agenda`, `gifts`, `reminders.message`, `life_events.description`, and the audit
`before_snapshot` travel as `encv1:` ciphertext when at-rest encryption is armed (issue
[#380](https://github.com/DrewBrunning/mycorrhizal-crm/issues/380)). `notes.content`,
`activities.title/description/location`, and `webhooks.secret` are **not** yet on that list — a
known gap owned by #380, out of scope here.

---

## 2. Identity & authentication

| Store | Personal data | Scope | Necessity | Notes |
|---|---|---|---|---|
| `users` | `username`, `email` (both unique), `language`, `date_format`, `is_admin` | per-user row, but see §4 | `necessary` | `email` is the reset/notification address and the login identifier |
| `users.password` | bcrypt hash (cost 10) | per-user | `necessary` | never logged, never exported, deny-listed from audit snapshots (`models/audit.go`) |
| `users.password_reset_token_hash` / `*_expires_at` / `*_requested_at` | SHA-256 of a reset token + timing | per-user | `necessary` | single-use; consumed in the same `WHERE` that reads it |
| `users.totp_secret_encrypted` / `totp_enabled` / `totp_confirmed_at`, `recovery_codes.code_hash` | 2FA secret (encrypted) and recovery-code hashes | per-user | `necessary` | recovery codes deleted in the `WHERE` that consumes them (`services/twofactor.go`) |
| `users.oidc_subject` / `oidc_provider` | External IdP subject identifier | per-user | `necessary` when OIDC is configured; `NULL` otherwise | |
| `users.self_contact_vcard_uid` | Which contact row *is* the user | per-user | `necessary` | |
| `api_tokens` | `name`, `token_hash`, `last_used_at`, `scope` | per-user | `necessary` | hash only; plaintext shown once at creation |
| `feeds` | `name` (user label), `entity_id` (a contact's VCardUID for a contact feed), `kind`, `detail`, `token_hash`, `last_accessed_at` | per-user | `necessary` | hash only; the plaintext exists once, in the create/rotate URL. No expiry (ADR 0030 decision 6); revoked per `data-retention-lifecycle.md` §7a. The reader's fetched copy outlives revocation |
| `sessions` | One row per interactive login: `user_agent` and client `ip` captured at login, `created_at` / `last_seen_at` / `expires_at` / `revoked_at`. The `id` is an opaque 256-bit lookup key, useless without the separately-signed JWT | per-user | `necessary` (server-side revocation, the "active sessions" list) | `user_agent` + `ip` are the deliberate minimum for that list and for spotting a stolen session. Hard-deleted 24 h after revocation and once past the absolute expiry (`PurgeExpiredSessions`, `data-retention-lifecycle.md` §23); removed with the account. In the embedded (Android local-only) shape the single local session is minted once per start (each start revokes the previous one, issue [#1340](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1340)) with idle enforcement off and a one-year absolute expiry (issue [#1312](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1312)) — there is no network surface or login screen there to protect |
| `webauthn_credentials` | One row per enrolled passkey / security key: the **public** key (never a secret), raw `credential_id`, `aaguid` (identifies the authenticator model), `sign_count`, `transports`, `backup_eligible`/`backup_state`, the user-supplied `name` label, `created_at` / `last_used_at`. No biometric data ever reaches the server | per-user | `necessary` (the second factor) | Hard-delete when the user removes the passkey (gated on a live second-factor proof), on an admin 2FA reset, and with the account (`data-retention-lifecycle.md` §4). Bound to the `FRONTEND_URL` hostname: a hostname change strands every row (`deployment.md` "Moving to a new hostname"). Revocation history is the `webauthn_register`/`webauthn_revoke` `audit_events` rows |
| `device_grants` | `label` (a device name the user chose), `token_hash`, `last_used_at`, `revoked_at` — the revocable Android biometric-resume grant | per-user | `necessary` | Hash only; the plaintext lives in the device Keystore. Revoked on password / 2FA change and on admin 2FA reset; hard-deleted with the account (`data-retention-lifecycle.md` §8) |

---

## 3. Derived & incidental stores

The stores a conventional privacy review forgets, because nobody *decided* to keep the data —
it accumulated as a side effect.

### 3.1 Full-text search index (`contacts_fts`, `notes_fts`, `activities_fts`)

- **Contents**: tokenized copy of contact names/emails/phones/addresses, note bodies, and activity
  text — held **independently of the source rows**, and **in plaintext even when at-rest
  encryption is armed** (the FTS5 index cannot be encrypted column-wise; documented in
  `data-retention-lifecycle.md` §6/§10).
- **Scope**: per-user (`user_id UNINDEXED` column scopes every query).
- **Necessity**: `necessary` for search; it is a rebuildable index, not a second source of truth.
- **Deletion**: `AFTER UPDATE`/`AFTER DELETE` triggers remove the FTS row the instant `deleted_at`
  is set — a soft-deleted contact is unsearchable **immediately**, before the 30-day purge. This
  is the one place deletion is *faster* than the primary table. Verified below.

### 3.2 Audit trail (`audit_events`)

- **Contents**: `entity_type` / `entity_id` / `operation` / `user_id`, plus `before_snapshot` — a
  **redacted JSON snapshot** of the pre-change row. `auditDenyList` (`models/audit.go`) strips
  passwords, TOTP secrets, token hashes, OIDC codes, SMTP/Resend credentials at any depth. Contact
  snapshots include the nested card (T82). No credential data; full contact PII, yes.
- **Scope**: per-user (`user_id` column, indexed). Instance-wide *table*, per-user *rows*.
- **Necessity**: `deliberate, documented`. The snapshot is what powers the Undo button and
  post-hoc investigation. #381 made it tamper-evident, which also makes it **effectively
  immutable within its window** — a deleted contact's data survives here for
  `AUDIT_RETENTION_DAYS` (default 90) after it is gone everywhere else. This is intentional and is
  called out to the operator in `../privacy.md`.
- **Deletion**: `PurgeExpiredAuditEvents` hard-deletes past the window and re-links the hash chain
  (`data-retention-lifecycle.md` §3). No external mirror — audit never syncs anywhere.

### 3.3 Request / access logs

Checked against **real captured output** (`GIN_MODE=release`, `LOG_LEVEL=info`). This review
found and **fixed four instance-wide leaks** in the same PR:

| # | Store | What leaked (before) | Fix (this PR) |
|---|---|---|---|
| F1 | zerolog, `services/mailer.go` + `services/password_reset_service.go` | Recipient email address verbatim (`"email":"alice@example.test"`) at info/warn on every send and every "no channel configured" no-op | `logger.MaskEmail` — local part reduced to `a***`, domain kept for delivery diagnostics (`backend/logger/mask.go`) |
| F2 | zerolog request log, `middleware/logging.go` `query` field | Full query string, including `?search=<a contact's name>` and `?q=<words from a private note>` | `logger.RedactQueryValues` reworked from a credential deny-list to an **allow-list** (`backend/logger/redact.go`): pagination/sort/enum params logged verbatim, everything else — search terms, ids, OIDC `state` — `[REDACTED]` |
| F3 | gin's own `Logger()` (`gin.Default()` in `main.go`) | A **second, entirely unredacted** request line: `GET /api/v1/contacts?search=<name>` | `gin.New()` + `gin.Recovery()` — the app's own redaction-aware `LoggingMiddleware` is the only request logger now |
| F4 | GORM default logger (`database/migrate.go`) | Full SQL with **interpolated literal values** on any errored/slow/not-found query: `SELECT ... WHERE email = "<address>"`. Not gated by `LOG_LEVEL` | **Fixed (issue [#621](https://github.com/DrewBrunning/mycorrhizal-crm/issues/621))**: every connection through `database/migrate.go` uses `newGormLogger` — `ParameterizedQueries: true` logs `?` placeholders, `IgnoreRecordNotFoundError: true` drops the benign not-found SELECTs — pinned by `database/migrate_test.go::TestGormLoggerDoesNotInterpolatePII` |

After F1–F4, a full scripted exercise of the API (register, login, create contact with
name+email+phone, note, search, list, password-reset, delete) produces **no personal data in the
captured logs**. Evidence in [How this was verified](#how-this-was-verified).

What the logs legitimately retain: `user_id`, `request_id`, method, path (no query), status,
duration, client IP, User-Agent. `user_id` + IP is the deliberate minimum for rate-limit and
abuse investigation. **Correlation IDs (issue [#425](https://github.com/DrewBrunning/mycorrhizal-crm/issues/425), in progress):**
this review's position is that a correlation ID may carry **only** low-cardinality identifiers and
enums — never a contact id, name, email, or raw URL — into the standardized field set. Recorded
here so #425 lands against a written rule.

The **retention** of that stream is operator-owned, not an app setting: the app writes to stdout and
ships no in-app log rotation or TTL, so bounding it is a deployment decision (Docker `max-size`/
`max-file`, or `journald` retention). That lifecycle entry is
`data-retention-lifecycle.md` §24 (issue [#978](https://github.com/DrewBrunning/mycorrhizal-crm/issues/978)).

### 3.4 Delivery bookkeeping

| Store | Personal data | Necessity | Notes |
|---|---|---|---|
| `notification_deliveries` | `channel`, `status`, `error` string, `reminder_id` | `necessary` (dedupe: "was this reminder sent on this channel?") | `error` can echo a provider message; low risk, no address column |
| `webhook_deliveries` | `payload` — the **full serialized entity** (a `contact.created` delivery carries the whole contact record), plaintext, plus `error` | `deliberate, documented` — 30-day window (`WEBHOOK_DELIVERY_RETENTION_DAYS`), and successful deliveries store only the event envelope, never the entity body | issue [#622](https://github.com/DrewBrunning/mycorrhizal-crm/issues/622) closed; `data-retention-lifecycle.md` §3 |
| `carddav_sync`, `contact_sync_links`, `calendar_event_links` | Sync tokens, `href`s, content hashes — no PII beyond an opaque UID/URL | `necessary` | hard-delete with parent (`data-retention-lifecycle.md` §2/§7) |
| `job_executions`, `server_settings` | None (job names, lock holder hostname; instance settings) | `necessary` | `locked_by` is a hostname, not a person |
| `push_subscriptions` | Web Push `endpoint` (a per-browser push-service URL — a device identifier), the `p256dh` / `auth` keys, `device_label` | `necessary` (it is the delivery address for push reminders) | Stored in plaintext (not on the at-rest-encryption list) and returned only to the owning user by `GET /notifications/push-subscriptions`; the endpoint identifies a browser installation and lets the server push to it. Dropped on a 404/410 from the push service, removable by the user, hard-deleted with the account (`DeleteUser`). Not in any export or sync projection. `data-retention-lifecycle.md` §25 |
| `device_registrations` | FCM/APNs device `token`, `client`, `device_label` | `necessary` (delivery address for native push) | Same posture as `push_subscriptions` (plaintext, owner-only list at `GET /notifications/devices`): user-removable, dropped on a permanent push-service rejection, hard-deleted with the account (§25). Push is absent from the embedded (Android local-only) shape |
| `idempotency_keys` | The opaque client key, method + route template, a request fingerprint, and — on a 2xx — a **copy of the response body**, so a retried create echoes the serialized new entity | `deliberate, documented` — the copy is what makes a retry replay instead of double-write | 24-hour window (`IDEMPOTENCY_KEY_RETENTION_HOURS`), 6-hourly purge; hard-deleted with the account. `data-retention-lifecycle.md` §22 |
| `import_runs` | Format token + five counts per confirmed file import — **no** names, values or error strings | `necessary` (the import-history list) | No purge job (self-bounding: a handful of rows per user per year); hard-deleted with the account. `data-retention-lifecycle.md` §17 |
| `import_source_links` | Source system token, the source row's own id, and the local entity kind/UID it was mapped to — no field values | `necessary` (idempotency ledger for Monica/Meerkat re-runs) | Immutable, self-bounding; hard-deleted with the account (`ON DELETE CASCADE`); contact merge repoints it to the survivor. `data-retention-lifecycle.md` §21 |
| `system_events`, `job_runs` | Operational timeline (event/job name, result, duration, correlation id, sanitized + length-capped `error`/`detail`); `system_events.user_id` is nullable and set only for user-attributed events | `necessary` (the admin health surface) | **No PII by construction** — the writers sanitize and the models carry no contact ids, names or URLs. 30-day windows (`SYSTEM_EVENT_RETENTION_DAYS`, `JOB_RUN_RETENTION_DAYS`), daily purge. `data-retention-lifecycle.md` §3 / §16 |
| `alert_states`, `operational_check_results`, `storage_samples` | Alert condition key + state + a sanitized detail line; latest self-check outcome per check; byte counts of the database / filesystem / photo and attachment directories | `necessary` (alerting, self-check, storage-growth trend) | **None — no `user_id`, no contact data.** The first two are upserted in place (one row per key); `storage_samples` is pruned to `STORAGE_SAMPLE_RETENTION_DAYS` (180). `data-retention-lifecycle.md` §15 / §14 / §20 |
| `data_backfills`, `data_encryption_keys` | The one-shot startup-backfill ledger (name, version, completed-at); the **wrapped** at-rest data-encryption key (`key_id`, `wrapped_dek`, `rotated_at`) | `necessary` | **No personal data.** `wrapped_dek` is key material sealed under the master key — useless without it, and lost with it by design (no escrow, issue [#380](https://github.com/DrewBrunning/mycorrhizal-crm/issues/380)); it is in every backup, so the master key must be stored apart from the backup (`deployment.md` "Backup confidentiality & retention") |

---

## 4. Instance-wide stores & cross-user attribution

The #510 question: *can an instance-wide store name a specific user's personal data?*

| Store | Instance-wide? | Attributes PII to a user? | Recorded reason |
|---|---|---|---|
| Structured logs (zerolog) | yes | `user_id` + client IP, and (until this PR) email/search terms — **now fixed**, F1–F3 | `user_id` + IP is the minimum for rate-limit/abuse response; no contact data after the fix |
| GORM SQL echo | yes | yes, on error/slow queries — **now fixed** [#621](https://github.com/DrewBrunning/mycorrhizal-crm/issues/621) | parameterized + not-found suppressed (`backend/database/migrate.go` `newGormLogger`, pinned by `database/migrate_test.go::TestGormLoggerDoesNotInterpolatePII`) |
| `audit_events` | table is instance-wide; every row has `user_id` | yes, by design | Undo + investigation; ages out at 90 days (§3.2) |
| `users` directory (`GET /api/v1/users/directory`, `ListUserDirectory`) | yes | **usernames are visible to every other authenticated user** on the instance | `deliberate, documented` — required for contact-sharing (issue [#574](https://github.com/DrewBrunning/mycorrhizal-crm/issues/574)); the operator must know usernames are not private between co-tenants. Stated in `../privacy.md` |
| `contact_shares` | table instance-wide; rows scoped to `from_user_id`/`to_user_id` | `contact_display_name` + frozen `payload` cross a user boundary **by the sending user's explicit action** | `deliberate, documented` — 30-day window (`CONTACT_SHARE_RETENTION_DAYS`), `data-retention-lifecycle.md` §1 |
| `job_executions`, `server_settings` | yes | no | — |
| Admin API | — | **Admin cannot read another user's contacts, notes, or activities** — the admin routes (`routes/routes.go:524-544`) are user-account CRUD, 2FA reset, and job triggers only. Admin *can* delete a user (cascade) and, as operator, has filesystem/DB/backup access | This is the #371 admin-capability answer; stated plainly in `../privacy.md` |

No metrics endpoint, no Prometheus exporter, no per-user counters that outlive a request.

---

## 5. Off-database copies

Fully enumerated in `data-retention-lifecycle.md`; summarized here with the minimization verdict.

| Copy | Personal data | Verdict | Reference |
|---|---|---|---|
| Attachments & profile photos on disk | Files a user uploaded against a contact | `necessary`; file deleted immediately on contact/attachment delete (not window-delayed) | lifecycle §5 |
| Operator backups (`VACUUM INTO` snapshot + photo `rsync`) | **A complete copy at full sensitivity** — `private`/`secret` fields, email/hashes, audit trail, still-in-window soft-deleted rows, FTS plaintext | `deliberate, documented` — retention and deletion are **entirely operator-owned** (the app deliberately cannot expire backups, issue [#505](https://github.com/DrewBrunning/mycorrhizal-crm/issues/505)); restoring one resurrects soft-deleted data | lifecycle §10, `deployment.md` "Backup confidentiality & retention" |
| Android offline mirror (Room, SQLCipher) | Cached contact list + FTS for offline read | `necessary`; encrypted end to end (issue [#385](https://github.com/DrewBrunning/mycorrhizal-crm/issues/385)); wiped on logout; rows drop on next sync disagreement | lifecycle §8 |
| Browser `localStorage` | `user_info` (id/username/admin flag/self-contact UID) + UI prefs — **no auth token, no contact PII** | `necessary`; pinned by the #419 Playwright regression | lifecycle §9 |
| Exports (CSV / vCard3 / vCard4 / jSContact / audit CSV) | Whatever the user requests; sensitivity-filtered before it leaves the server | `necessary`; streamed, never written to disk server-side; no artifact outlives the request | lifecycle §11 |
| Import wizard staging | Uploaded rows pre-confirmation | `necessary`; **in-memory only**, 15-minute expiry, lost on restart | lifecycle §12 |
| External integrations (`webdav_configs`, `paperless_configs`, `immich_configs`, `seafile_configs`, `notification_configs`, `calendar_subscriptions`, `contact_subscriptions`) | One encrypted credential row per user per integration; `external_identities` / `external_activities` cache remote `payload` / `metadata` | `necessary`; credentials encrypted (`services/credential_crypto.go`), plaintext never returned by the read endpoint; deleting the connection never deletes anything in the external service (boundary, not gap) | lifecycle §13 |

---

## 6. The third-party dimension

The people described in `contacts`, `notes`, `preferences`, `life_events`, and
`relationship_edges` did not consent and cannot see their record. What the product owes them, and
what it actually provides:

| Owed | Provided? |
|---|---|
| The operator can **find everything about one person** | Yes — FTS search (`contacts_fts` + `notes_fts` + `activities_fts`) covers names, note bodies, and activity text; the contact detail view aggregates every hung-off entity |
| The operator can **remove everything about one person** | Yes for a person who is a `Contact` — `DeleteContact` cascades every dependent row in one transaction (`contact_controller.go`, the canonical checklist in CLAUDE.md trap #6), then the 30-day purge hard-deletes. **Partial** for a person mentioned only inside a free-text note or activity belonging to a *different* contact: they are findable by search but not individually deletable — the operator must edit the note. Stated as a known limit in `../privacy.md` |
| Data about them does not silently outlive a delete | Mostly — see [§8](#8-deletion-completeness). The deliberate exceptions (audit window, backups) are documented, not silent |
| Data about them is not propagated further than they'd expect | `sensitivity` ≥ `private` is excluded from CardDAV/CalDAV, contact shares, and the vCard/JSContact exports **in the query**; `suggested` (unconfirmed) relationship edges are never projected to standards or graphed. Both classifications ride along uncensored in the user's own flat CSV backup (`GET /export`), labelled by column — that file stays on the data subject's own controller's device rather than propagating outward, so the boundary it crosses is custody, not disclosure (§11 of `data-retention-lifecycle.md`, issue [#861](https://github.com/DrewBrunning/mycorrhizal-crm/issues/861)) |

This is issue [#414](https://github.com/DrewBrunning/mycorrhizal-crm/issues/414)'s deletion path
viewed from the data subject's side rather than the account owner's.

---

## 7. Operator responsibilities (multi-user)

Stated in full, for adopters, in [`../privacy.md`](../privacy.md) and
[`../supported-versions.md`](../supported-versions.md) ("The deployment shape"). In brief: with more than one
user on an instance, the operator holds **other users' relationship data and their notes about
third parties twice removed from the operator**. The software does not discharge the controller's
duties for them — it provides scoping, per-user deletion, export, and a `sensitivity` filter, and
the operator owns everything the deployment boundary
(`deployment-baseline.md`) hands them: the filesystem, the database, and the backups.

---

## 8. Deletion completeness

Checked by walking a contact delete against **this inventory**, not against `DeleteContact`.
Procedure: create a contact with an email, a phone, a note, an activity, and an attachment; note
the sentinel values; `DELETE` it; then inspect each store the inventory lists.

| Store | State immediately after delete | State after `DELETE_RETENTION_DAYS` (30) |
|---|---|---|
| `contacts` + hung-off rows (`notes`, `activities`, `reminders`, `life_events`, `preferences`, `gifts`, `conversation_agenda`, `attachments`, `occasion_obligations`, `cadence_policies`, `data_decay_policies`) | `deleted_at` set (soft) — invisible to every API query | hard-deleted by `PurgeSoftDeletedRows` (its coverage of every soft-deletable table is enforced by `services/purge_completeness_test.go`, issue [#1310](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1310)) |
| Edge/join rows (`relationship_edges`, `circle_members`, `contact_tags`, `household_members`, `activity_contacts`, `contact_sync_links`, `occasion_event_attendees`, `life_event_suggestion_resolutions`) | **hard-deleted synchronously** in the same transaction | — |
| `contacts_fts` / `notes_fts` / `activities_fts` | **row gone immediately** via trigger — unsearchable at once | — |
| Attachment file on disk | **deleted immediately** after the transaction commits | — |
| `audit_events.before_snapshot` | **retained** — holds a redacted snapshot of the deleted contact | hard-deleted at `AUDIT_RETENTION_DAYS` (90), i.e. ~60 days *after* the row itself is purged. **Deliberate**, documented in `../privacy.md` |
| `webhook_deliveries.payload` | **retained** if a webhook fired on this contact — a plaintext copy of the deleted contact | hard-deleted at `WEBHOOK_DELIVERY_RETENTION_DAYS` (30), i.e. ~30 days after the delivery attempt; successful (2xx) deliveries no longer hold the entity body at all (only the event envelope) |
| Operator backups | **retained** in every snapshot predating the delete; a restore resurrects the (soft-deleted, not yet purged) contact | ages out of *new* snapshots after the purge; survives in old snapshots until the operator deletes them |
| Android Room mirror | dropped on the next `?since=` change-feed sync (T17 tombstone) or on logout wipe | — |
| CardDAV/CalDAV clients | contact stops appearing in the next full listing (no delta protocol; `data-retention-lifecycle.md` §7) | — |

Deliberate retention: **audit snapshots** and **backups** only. Both documented. Everything else
is gone at or before the 30-day mark.

---

## 9. Known over-collection & dispositions

| Finding | Verdict | Disposition | Issue |
|---|---|---|---|
| Recipient email address logged verbatim (mailer + password-reset) | `more-than-necessary` | **Fixed in this PR** — `logger.MaskEmail` | — |
| Full query string (search terms, ids) in the request log | `more-than-necessary` | **Fixed in this PR** — allow-list in `logger.RedactQueryValues` | — |
| gin's duplicate unredacted request logger | `more-than-necessary` | **Fixed in this PR** — `gin.New()` + `gin.Recovery()` | — |
| GORM SQL echo with interpolated PII on error/slow queries | `more-than-necessary` | **Fixed** — `newGormLogger` (`ParameterizedQueries: true` + `IgnoreRecordNotFoundError: true`) wired into `InitDB`/`OpenMigratedFile`, pinned by `database/migrate_test.go` | [#621](https://github.com/DrewBrunning/mycorrhizal-crm/issues/621) |
| `webhook_deliveries.payload` retained forever, full entity body | `kept-longer-than-necessary` | **Fixed** — 30-day window (`WEBHOOK_DELIVERY_RETENTION_DAYS`) + purge job; successful deliveries store only the event envelope, never the entity body | [#622](https://github.com/DrewBrunning/mycorrhizal-crm/issues/622) |
| `notes.content` / `activities.*` / `webhooks.secret` not at-rest-encrypted | protection gap, not minimization | Owned elsewhere | [#380](https://github.com/DrewBrunning/mycorrhizal-crm/issues/380) |
| User directory exposes usernames to all co-tenants | `deliberate, documented` | Stated in `../privacy.md` | — |
| Audit snapshots outlive the deleted row by ~60 days | `deliberate, documented` | Stated in `../privacy.md` and §8 | [#381](https://github.com/DrewBrunning/mycorrhizal-crm/issues/381) |
| Backups are a full-sensitivity copy with operator-owned retention | `deliberate, documented` | `deployment.md` + `../privacy.md` | [#420](https://github.com/DrewBrunning/mycorrhizal-crm/issues/420) |

Everything not listed here was walked and judged `necessary`.

## How this was verified

- **Schema**: every table in a freshly migrated database was enumerated; each is a row above or is
  recorded as holding no personal data (`job_executions`, `server_settings`, `data_encryption_keys`,
  `data_backfills`, the `dismissed_*` hash tables, sync-token tables). This is no longer a manual
  walk: `go run ./cmd/docscheck` (the `docs-citations` CI job, and its Go test) migrates a scratch
  database and fails if any table is neither a code span in a table row or heading of this document
  nor in the reasoned exclusion list in `backend/cmd/docscheck/pii_inventory.go` (issue
  [#1316](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1316) — 23 tables had drifted out
  of the inventory before it existed). A new table therefore needs a row here — its personal data,
  necessity and retention/deletion — in the same PR.
- **Logs**: the backend was built and run twice with `GIN_MODE=release LOG_LEVEL=info` (the
  production profile) — once before the F1–F3 fixes, once after — with stdout captured to a file.
  A scripted run exercised `register`, `login`, `contacts` create (name + email + phone),
  `contacts/:id/notes`, `contacts?search=`, `search?q=`, `password-reset/request` (known and
  unknown address), `admin/trigger-reminders`, and `contacts/:id` delete, using sentinel values
  (`Zephyrina Testsubjectson`, `zephyrina.contact.sentinel@…`, `+15550009999`,
  `SECRET_NOTE_BODY_SENTINEL…`, `alice.pii.sentinel@…`). The captured files were then `grep`'d for
  every sentinel.
  - **Before**: recipient address in a warn line (`password_reset_service.go:51`); `query="search=Zephyrina"`
    and `query="q=SECRET_NOTE_BODY_SENTINEL…"` in the zerolog request line; the same two as raw
    `[GIN]` lines; `SELECT … WHERE email = "…"` from GORM.
  - **After**: `"email":"a***@example.test"`; `query="search=[REDACTED]"` / `query="q=[REDACTED]"`;
    no `[GIN]` lines at all; and after **#621** no GORM SELECT is logged with a literal value
    (`?` placeholders instead, and not-found lookups not at all — pinned by
    `database/migrate_test.go::TestGormLoggerDoesNotInterpolatePII`).
  - The contact name, phone, and note body from the create/note/list/delete calls appeared in
    **no** log line, before or after — only the search-term and SQL-echo paths carried them.
- **Deletion** (§8): verified by the FTS trigger coverage in `backend/database/migrate_test.go`,
  the purge-window tests in `backend/services/purge_service_test.go`, and the audit-retention
  tests in `backend/services/audit_purge_service_test.go`; the `webhook_deliveries` gap was
  found by `grep` showing no purge call site and is closed by
  `backend/services/webhook_delivery_purge_service_test.go` (issue #622).
- **Regression pins added in this PR**: `backend/logger/mask_test.go` (local part never echoed),
  `backend/logger/redact_test.go` (allow-list semantics; search terms / ids / OIDC state
  redacted), `backend/services/mailer_test.go::TestSendEmail_LogsMaskRecipientAddress`
  (hand-verified to fail before the fix), `backend/middleware/logging_test.go` (query allow-list).

## Changelog

| Date | Change |
|---|---|
| 2026-09-29 | Inventory brought back in line with the schema (issue #1316): rows added for `webauthn_credentials`, `sessions`, `device_grants`, `push_subscriptions`, `device_registrations`, `attachments`, `occasion_obligations` / `occasion_events` / `occasion_event_attendees`, `cadence_policies`, `data_decay_policies`, `life_event_suggestion_resolutions`, `link_field_types`, `idempotency_keys`, `import_runs`, `import_source_links`, `system_events`, `job_runs`, `alert_states`, `operational_check_results`, `storage_samples`, `data_backfills`, `data_encryption_keys`; §8 updated for the purge-coverage guarantee (#1310). `docscheck` now fails when a migrated table is missing from the inventory. |
| 2026-08-27 | GORM SQL echo leak (F4) closed (issue #621): every connection through `database/migrate.go` uses `newGormLogger` — `ParameterizedQueries: true` logs `?` placeholders instead of interpolated values, `IgnoreRecordNotFoundError: true` drops benign not-found SELECTs. Pinned by `database/migrate_test.go::TestGormLoggerDoesNotInterpolatePII` (hand-verified to fail against the pre-fix default logger). §3.3 / §4 / §9 dispositions updated from "pending/filed" to fixed. |
| 2026-08-27 | `webhook_deliveries` retention gap closed (issue #622): `WEBHOOK_DELIVERY_RETENTION_DAYS` (default 30) + daily purge job + admin trigger, and successful deliveries now store only the event envelope (never the entity body). Disposition updated `kept-longer-than-necessary → deliberate, documented`. |
| 2026-08-26 | Created (issue #510). Log leaks F1–F3 fixed in the same PR; F4 → #621, `webhook_deliveries` retention → #622. |
