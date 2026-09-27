# ADR 0028: Local-only Android mode and server profiles

- **Status:** proposed — Decisions 1, 3, 4 and 5 are adopted; Decision 2 names an embedded Go backend
  as the preferred shape, **conditional on the spike** it defines. The spike's result flips this
  ADR to `accepted` (shape A confirmed) or amends Decision 2 (shape B).
- **Date:** 2026-09-26
- **Implements:** issue #1108 (Android: offline-only mode — no server required, login optional). The
  issue asked for a design pass covering the server-configuration model, the shape of a "local
  server", and the later-sync story, before any implementation. This ADR is that design pass; the
  implementation tickets it splits into are indexed on #1108.
- **Depends on:** `docs/adrs/0007-source-import-mapping.md` (the import framework and ledger the
  attach path reuses), `docs/adrs/0009-rest-conflict-policy.md` (reject-and-return; why live
  two-way sync is out of scope), `docs/adrs/0011-scheduled-job-catchup.md` (what makes an
  intermittently-running scheduler correct), `docs/adrs/0014-local-app-lock-and-biometric-resume.md`
  (the local gate that must now also cover a token-free local profile),
  `docs/adrs/0022-distribution-variants.md` (flavors — the embedded backend ships in all three).

## Context

Today the Android app is a thin, online-first client of a remote Mycorrhizal server, with a
read-mostly offline cache. Offline-only is therefore not a toggle:

- **The whole UI is gated on a server login.** `rootSurface()` in
  `android/app/src/main/kotlin/com/mycorrhizal/crm/MycorrhizalApp.kt` routes `!isLoggedIn` to
  `RootSurface.Auth`, and `isLoggedIn` is exactly "a JWT is stored"
  (`android/core/data/src/main/kotlin/com/mycorrhizal/crm/data/session/DefaultSessionManager.kt`).
  The server URL is entered once, at login.
- **Every write is online-first.** The ~30 repository implementations bound in `DataBindsModule`
  (`android/core/data/src/main/kotlin/com/mycorrhizal/crm/data/di/DataModule.kt`) call the server and
  only mirror into Room on success. Eleven classes bypass the repository seam and inject `ApiClient`
  directly (`ContactListViewModel`, `DashboardViewModel`, `PrepViewViewModel`, the four import view
  models, `NotificationWorkers`, `InteractionSyncWorker`, `DeviceRegistrationManager`,
  `MainActivity`).
- **Room is a cache, not a store.** `AppDatabase` documents itself as a cache, runs
  `fallbackToDestructiveMigration`, and is wiped wholesale on logout
  (`android/core/data/src/main/kotlin/com/mycorrhizal/crm/data/local/LocalDataCleaner.kt`). Only
  contacts are ever read back; notes/activities/reminders are cached without a `contactId`; contacts,
  notes, activities and reminders are keyed by server-assigned `Int` IDs. The only offline write queue
  is `pending_interactions` (call/SMS capture).
- **Much of the product is computed server-side** — dashboard, prep/briefing, cadence, reach-out,
  score, duplicates, address/relationship suggestions, and the reminder/birthday/cadence
  notifications (`android/feature/tracking/src/main/kotlin/com/mycorrhizal/crm/feature/tracking/NotificationWorkers.kt`
  polls the API).
- **Nothing on-device parses or writes vCard**, and `allowBackup=false` — there is no device-side
  backup.

On the backend side, the relevant facts:

- **The server is a plausible embed target.** SQLite is pure Go (`glebarez/sqlite` → `modernc.org/sqlite`,
  built `CGO_ENABLED=0`), migrations are embedded, and the HEIC decoder is WASM via wazero — no cgo
  anywhere. But `main()` (`backend/main.go`) is one ~360-line function ending in `ListenAndServe`,
  all configuration is environment variables (`backend/config/config.go`, with `JWT_SECRET_KEY` and
  an absolute `PROFILE_PHOTO_DIR` required), a pre-migration snapshot is written before every migration
  (`backend/database/premigration_backup.go`), and `registerScheduledJobs`
  (`backend/scheduled_jobs.go`) starts ~17 gocron jobs, several of which (restore drill, alert
  evaluation, webhook retries) are meaningless on a phone.
- **There is no per-user, re-importable export.** `GET /export` CSV is full-fidelity but is not an
  import format; backups (`VACUUM INTO`) are whole-database and multi-user; restore is a file swap.
- **The import framework already has what an attach path needs**: `ExecuteSourceImportWithActions`
  (`backend/services/import_source.go`) over an `ImportSourcePlan` covering contacts, relationships,
  notes, activities, reminders, gifts, preferences, households, circles, tags and custom fields, with
  per-contact add/skip/merge and the re-run-safe `import_source_links` ledger
  (`backend/models/import_source_link.go`, ADR 0007).
- **Identity is mostly portable.** Most entities have UUID primary keys generated in `BeforeCreate`
  only when empty; contacts carry a stable `VCardUID` that a create request can supply (`card.uid`).
  `Note`, `Reminder` and `ReminderCompletion` are the exceptions — uint-only, no stable portable ID.

Issue #1108 proposed reframing "Android needs an offline mode" as "how does the app decide which server
it talks to" — i.e. an on-device local server plus a server-selection model — and asked for the costs of
that reframing to be written down. This ADR does that and makes the call.

## Decision

### 1. Server profiles are first-class

The one-time "enter a URL, then log in" flow is replaced by **server profiles**:

```
ServerProfile { id: UUID, kind: Remote(url) | Local, label }
```

- **Exactly one profile is active at a time.** No simultaneous multi-account UI; switching is an
  explicit action from Settings (and from the Auth screen).
- **Storage.** The profile list and active-profile ID live in DataStore (alongside today's
  `session_prefs`); each profile's credential lives in EncryptedSharedPreferences keyed by profile ID
  (today's single `secure_session`/`jwt` entry becomes the active profile's slot, migrated in place —
  an existing install becomes one `Remote` profile with its current URL and token, no re-login).
- **The Room mirror belongs to the active profile** and is wiped on a switch. It is a cache
  (`AppDatabase`'s own contract), so this is correct, and it keeps one profile's data out of another's
  UI. The `pending_interactions` outbox is the exception: it is drained or explicitly discarded
  (with a confirmation that names the count) before a switch, never silently dropped.
- **A `Local` profile's data never lives in that cache.** Whatever the local store is (Decision 2), it
  is a separate, profile-owned file set that `LocalDataCleaner` does not touch. For a `Local` profile
  there is no "Log out"; the destructive action is an explicit **"Delete local data"** with a
  typed-confirmation, because it is the only copy.
- **The Auth screen gains "Use on this device only"**, which creates (or re-activates) the `Local`
  profile and lands in `RootSurface.Main` with no credentials entered. `isLoggedIn` keeps meaning
  "the active profile has a usable session"; for `Local` that session is minted on-device (Decision 2).
- **App lock (ADR 0014) covers `Local` profiles.** `DefaultAppLockController` gates on "a resumable
  session exists", which a `Local` profile always has — arguably more important there, since there
  is no server-side revocation to fall back on.
- **Base-URL resolution needs no redesign.** `BaseUrlInterceptor`
  (`android/core/network/src/main/kotlin/com/mycorrhizal/crm/network/BaseUrlInterceptor.kt`) already
  rewrites the `http://mycorrhizal.invalid` placeholder per request from `BaseUrlProvider`; it now
  reads the active profile. The compatibility gate (`MainViewModel`, `ServerCapabilities`) runs
  against whichever profile is active.

This decision stands on its own — it is worth doing even if local-only mode never ships, because it
also gives "switch servers" and "re-point after a domain move" a real home.

### 2. The local store: embedded Go backend preferred, gated on a spike

Two shapes were evaluated.

**A — Embedded Go backend (preferred).** The real backend runs on-device, and the app's `Local`
profile talks to it exactly as a `Remote` profile talks to a server.

- *Backend:* a library entry point (`embedded.Start(cfg) / Stop()`) that `main()` is refactored onto,
  with a programmatic `Config` instead of environment variables; and an **embedded deployment mode**:
  - one user, auto-provisioned on first start; registration, OIDC, 2FA, password reset, email, CORS,
    API tokens, shares, webhooks, CardDAV/CalDAV **serving**, device grants and push all disabled;
  - listens on a **Unix domain socket in app-private storage, never TCP loopback** — any app on the
    device can connect to `127.0.0.1`, and only the app's own UID can open a socket under its data
    directory;
  - the JWT signing secret and the `atrest` master key are generated by the app on first run, wrapped
    by an Android Keystore key, and passed in through `Config` (never environment, never written in
    plaintext);
  - the scheduler runs only while the app process lives; ADR 0011's fire-missed-runs-once catch-up is
    exactly the semantics an intermittently-running process needs. The restore drill, alert evaluation,
    webhook retries and FCM are not registered at all; the pre-migration snapshot is kept (it is the
    only upgrade safety net for the only copy) but capped to the most recent one;
  - `/health` reports `deployment: "embedded"` plus a capability list, which the client's existing
    `ServerCapabilities` gate consumes to hide server-only features (the list above) — no Android
    feature code branches on "is local", only on capabilities.
- *Android:* the app hosts the server (in-process via `gomobile bind`, or as an executable shipped in
  `nativeLibraryDir` — the Syncthing-Android precedent; the spike picks), starts it lazily from the
  first request or worker that needs it, and routes `Local`-profile traffic over the socket through a
  profile-aware OkHttp `SocketFactory`. **The ~30 repositories, the 11 direct `ApiClient` users and
  the notification workers are unchanged** — they are already written against the API.
- *Costs:* APK size (a Go server binary per ABI), cold-start latency for the first local request,
  process/battery behaviour of an in-app server, and SQLite without SQLCipher (Decision 4).
- *Why preferred:* parity for free. Dashboard, cadence, suggestions, duplicates, import parsing,
  exports, the correspondence oracle (ADR 0002) and every invariant (ADR 0012) are the same code the
  server runs and tests. Local-only users get the whole product, and every backend fix reaches them.

**B — Native Room local-first (fallback).** Local implementations of the repository interfaces,
swapped in through `DataBindsModule` when the active profile is `Local`.

- *Requires:* a UUID/vCard-UID identity scheme for every entity (today four are server `Int`s); new
  repository seams for the 11 direct `ApiClient` users; a non-destructive Room migration policy for a
  database that is now the source of truth (today's is destructive by design); a Kotlin vCard
  parser/serializer; and either Kotlin re-implementations of every server-computed feature or those
  features hidden in local mode.
- *Cost:* a permanent second implementation of the domain logic, drifting from the tested one — the
  opposite of ADR 0002's single mapping truth — or a visibly lesser local product.

**Decision rule.** Shape A is adopted **unless** the spike (the first implementation ticket) measures
any of the following on the `obtainium` release build, arm64, on the maintainer's Pixel 8a:

| Criterion | Threshold that rejects A |
|---|---|
| Release APK size increase (arm64 split) | > 35 MB |
| Cold start → first contact list rendered, `Local` profile | > 1.5 s (median of 10) |
| Idle battery: app backgrounded 8 h, server stopped per lifecycle | any wakeups attributable to the server |
| Build | `GOOS=android GOARCH=arm64` (either packaging) cannot build or link the backend |

If A is rejected, Decision 2 is amended to B and the A-specific tickets are superseded by a Room
local-first ticket set; Decisions 1, 3, 4 and 5 are unaffected (Decision 3's bundle is the contract
that makes that true).

### 3. Attaching to a remote server is a one-time migration, not a sync

A `Local` user who later wants a server **moves** their data; the two stores are never kept in sync.

- **A native account bundle.** `GET /api/v1/export/account` returns a versioned JSON document shaped
  like `ImportSourcePlan`: every entity the plan covers, with stable IDs (contact `vcard_uid`, entity
  UUIDs). It is full-fidelity — every sensitivity level and `status: suggested` included — for the same
  reason the flat CSV export is (issue #861): it is the user's own data going to the user's own
  destination, and withholding is silent data loss. `Note`, `Reminder` and `ReminderCompletion` gain a
  UUID column (data-preserving migration with backfill) so the bundle has a stable ID for everything.
- **A `mycorrhizal` import source.** The bundle is imported through the existing source-import
  upload → preview → confirm flow with per-contact add/skip/merge, and the `import_source_links`
  ledger keyed on the bundle's stable IDs, so an interrupted attach is simply re-run.
- **The Android flow:** add and log into a `Remote` profile → the `Local` server exports the bundle →
  the app uploads it to the remote import endpoint → the user reviews the preview → confirm → the
  `Local` profile is marked **read-only archive** (still browsable, no writes) until the user deletes
  it explicitly. Nothing is deleted automatically.
- **Live two-way sync is out of scope** and rejected for now. It would be a fourth conflict policy
  alongside REST reject-and-return (ADR 0009), CardDAV remote-overwrites-then-surface, and CalDAV
  local-wins/no-delete-propagation (T13), and the CLAUDE.md rule is that a new two-way path does not
  inherit an existing policy by accident. A future ADR can add it; the bundle and ledger would be its
  initial-sync step.
- **Shape-independence.** Under shape B, Kotlin writes the same bundle format; the remote side is
  identical. The bundle also gives server-to-server account portability, useful on its own.

### 4. Encryption at rest for the local store

The embedded backend's SQLite (modernc) cannot be SQLCipher, so the whole-database encryption
MASVS P4 records for the Room mirror does not carry over. Adopted instead:

- the backend's existing **field-level `atrest` encryption** (`backend/atrest/atrest.go`), with the
  master key generated on-device and wrapped by an Android Keystore key (Decision 2);
- **Android file-based encryption** of app-private storage (credential-encrypted storage, locked until
  first unlock);
- the residue — the plaintext FTS index and structural columns (names in `sort_name`, IDs,
  timestamps) — recorded as a **documented MASVS position** in `docs/security/masvs-l1.md`, not a
  silent gap. An encrypted-SQLite Go driver (cgo SQLCipher / SQLite3MultipleCiphers) was considered
  and declined: it reintroduces cgo and an NDK build for a marginal gain over FBE on a single-user,
  single-device store.

Under shape B, the local Room database is SQLCipher like the mirror, and this position does not apply.

### 5. Backup for local-only data

A `Local` profile's data has no server-side backup, and `allowBackup=false` stays (cloud backup of a
contact database is a different privacy decision than this ADR should make implicitly). The backup is
**user-initiated: "Export account bundle to file"** through the Storage Access Framework — the same
Decision 3 bundle. Restore is importing a bundle into a fresh `Local` profile through the same import
path. The app surfaces a periodic, dismissible reminder when a `Local` profile has never been exported
or was last exported more than 30 days ago.

## Consequences

- **Docs that change with the implementation:** `docs/security/data-retention-lifecycle.md` gains the
  local store as a new persistent copy (§8 currently says the device holds only a rebuildable cache);
  `docs/security/masvs-l1.md` gains the Decision 4 position; `docs/security/threat-model.md` gains the
  Unix-socket boundary; `docs/android-app.md` describes profiles and local mode.
- **Upgrade policy applies on-device.** The embedded backend runs the same frozen migration chain,
  so an old local database upgrades exactly as a server does (issue #529 floor included); a local
  database below the floor refuses to start and the app offers bundle export from a read-only mode.
- **Open risks the spike and tickets must address:** process death mid-write (the server's
  transactions protect the database; the client must treat a dropped socket like a network error,
  which it already does); storage use of the capped pre-migration snapshot; per-flavor APK size
  (the FOSS flavor gets the same binary — no GMS dependency is introduced).
- **What does not change:** remote profiles behave exactly as today; the Room mirror stays a cache;
  no Android feature code learns about "local", only about capabilities.
