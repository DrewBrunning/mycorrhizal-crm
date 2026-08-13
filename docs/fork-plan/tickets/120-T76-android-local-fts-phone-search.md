# T76 — Android offline search can't find a contact by phone number either

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 3 — same bug as T69, smaller blast radius (offline search only) |
| **Size** | S — one Room entity + the DAO query |
| **Depends on** | [T69](113-T69-phone-search-tokenization.md) conceptually (same normalization rule), not technically — this is a different database in a different module and can land independently. |
| **Status** | TO BE DONE |
| **Source** | Found during the 2026-08-11 grooming pass, tracing [T69](113-T69-phone-search-tokenization.md)'s root cause across every search path. Not in the original testing notes. |

## Why this exists

Android has its own local full-text index over the cached contact list, entirely separate from the
backend's — `CachedContactFts`
(`android/core/data/src/main/kotlin/com/mycorrhizal/crm/data/local/CachedContactFts.kt`) is a Room
`@Fts4(contentEntity = CachedContact::class)` mirror indexing `fn`, `firstname`, `lastname`,
`primaryEmail`, `primaryPhone`, `org`. `CachedContactDao.searchFts` (`CachedContactDao.kt:46-59`)
runs `WHERE cached_contacts_fts MATCH :query || '*'`.

FTS4's default tokenizer splits on punctuation exactly as FTS5's does, so `primaryPhone` stored as
`(800) 555-1234` indexes as three tokens `800`/`555`/`1234`, and a query of `8005551234` prefix-
matches none of them. Identical bug to [T69](113-T69-phone-search-tokenization.md), and fixing the
backend does nothing for it: this index is built on-device from cached rows and is what powers
offline search.

Two secondary gaps come along with it, both mirroring T69's:

- Only `primaryPhone` is indexed, so a contact's second or third number is unfindable offline even
  when typed exactly.
- The query is passed to `MATCH` with a bare `|| '*'` and no sanitization, so a term containing FTS
  syntax characters (a quote, `*`, `-`, `NEAR`) is a malformed-query risk. The backend guards this
  in `ftsQuery`; the Android side has no equivalent. Worth fixing in the same pass since it is the
  same three lines.

## What to build

1. Add a normalized phone field to `CachedContact` (and therefore to the FTS mirror), holding the
   same two tokens per phone that [T69](113-T69-phone-search-tokenization.md) indexes server-side:
   the full digit string, plus the last-10-digits key when it differs. Cover **all** of the
   contact's phone numbers, not just `primaryPhone`.
2. Implement the key function in Kotlin to the same rule as T68's `PhoneKey` (digits only, keep at
   most the last 10, empty below 7 significant digits). The two implementations must agree — add a
   comment on each pointing at the other, the same way `/CLAUDE.md`'s frontend trap #4 requires for
   hand-mirrored enum lists.
3. Normalize a phone-shaped query the same way before it reaches `MATCH`.
4. Sanitize the `MATCH` term generally (quote it and escape embedded quotes, as `ftsQuery` does)
   rather than concatenating raw user input.
5. This changes a Room schema. **Decided: bump the version and use a destructive migration**
   (`fallbackToDestructiveMigration`), not a hand-written one. Every table in this database is a
   cache of server state, re-fetchable on the next sync — the same "derived data, rebuildable at any
   time" argument migration `000010` uses for `contacts_fts` server-side. Writing a real migration
   would be effort spent preserving data the app can re-download. **Verify the premise before
   relying on it**: confirm nothing in the Room database is offline-authored and not yet synced
   (e.g. a queued outbox), because that would make the destructive path data loss rather than a
   cache refill.
6. Unit tests for the key function and for the DAO query returning a cross-punctuation match.
   Hand-verify per `/CLAUDE.md`: confirm the new test fails before the fix.

## Traps

- **Don't assume the backend fix covers this.** Offline search is the whole point of this index; it
  answers from cached rows without contacting the server.
- **The `@Fts4(contentEntity=...)` triggers are Room-generated** — the mirror stays in sync
  automatically, but only over the columns actually declared on the FTS entity. Adding the field to
  `CachedContact` without adding it to `CachedContactFts` silently indexes nothing.

## Done when

- Searching `8005551234` in the Android app's contact search finds a contact whose number is stored
  as `(800) 555-1234`, offline.
- A non-primary phone number is findable.
- A query containing FTS syntax characters returns results or nothing, but never crashes or errors.
- New unit tests cover the key function and a cross-punctuation DAO match, hand-verified to fail
  first.
- `./gradlew testDebugUnitTest`, `./gradlew lintDebug` and `./gradlew assembleDebug` green —
  the three steps `.github/workflows/android-tests.yml` actually runs.

## Landed 2026-08-12

Implemented largely as scoped, with two real deviations from the ticket's own suggested
approach, both found by following its own "verify before relying on" instructions:

- **Not a destructive migration.** `pending_interactions` (device call/SMS tracking staged for
  server sync) is a real not-yet-synced outbox, not rebuildable cache data —
  `fallbackToDestructiveMigration` would have silently deleted it on this version bump. Wrote a
  hand-migration (`MIGRATION_13_14`, `Migrations.kt`) instead: `ALTER TABLE` on `cached_contacts`
  for the new column, drop+recreate `cached_contacts_fts` (FTS4 can't `ALTER ADD COLUMN`) with an
  FTS `'rebuild'` to reindex from existing rows. Room's own migration pipeline
  (`onPreMigrate`/`onPostMigrate`) drops and recreates every `@Fts4` entity's sync triggers
  around any registered migration automatically, so the migration itself only needed the
  table-level DDL delta. `Migration13To14Test` builds a real v13-shaped database (every table,
  not just the two that changed — Room validates the whole schema post-migration) and proves both
  halves: the outbox row survives, and the new phone index actually works after migrating.
- **The FTS4 `column:"term"*` filter syntax silently matches nothing when the term is quoted**
  (confirmed empirically against Robolectric's real SQLite, not documented anywhere found).
  `phoneMatchExpr` uses `column:term*` unquoted instead — safe without quoting since
  `PhoneKey.digits`/`PhoneKey.key` are always pure `0`-`9` by construction, so there's no FTS
  syntax character that would need escaping in the first place. The backend's Go equivalent
  (`phoneFTSMatch`) does quote — the two aren't required to match syntactically, only in what
  they find.
- The ticket's third gap — "passes unsanitized input to MATCH" — turned out to already be fixed:
  `ContactRepositoryImpl.searchLocal` already sanitized (strip FTS operator characters, quotes,
  boolean keywords) before hitting `MATCH`, plus a try/catch falling back to the LIKE scan, from
  work that landed after this ticket was filed. Left as-is; no regression to fix.

`PhoneKey.kt` mirrors backend `models.PhoneKey`/`NormalizePhoneDigits`/`FlattenPhones`
(`backend/models/phonekey.go`) and `services.PhoneQueryTokens`/`phoneFTSMatch`
(`backend/services/search_service.go`) line-for-line. `CachedContact`/`CachedContactFts` gain
`phonesNormalized` (every phone's full digits + its last-10-digit key, space-joined); a list-page
row only ever knows `primaryPhone`, so `mergePreservingDetail` now also preserves a richer
multi-phone index from a previously cached full detail fetch across a plain list refresh, the
same way it already does for `card`/`crm`.

Test coverage: `PhoneKeyTest` (pure unit), `CachedContactDaoTest` (cross-punctuation + non-primary
phone FTS match), `ContactRepositoryImplTest` (the ticket's literal scenario, plus a
country-code-prefix test that specifically pins the digits-vs-key duality — the literal-scenario
test alone would still pass with the phone-routing removed, since `phonesNormalized` is a
generally-indexed FTS column). All hand-verified per `/CLAUDE.md`: broke the phone-routing,
confirmed exactly the discriminating test failed; separately swapped the migration test to the
ticket's originally-suggested `fallbackToDestructiveMigration`, confirmed the outbox-preservation
assertion failed (`expected:<1> but was:<0>`) — the exact hazard the hand-migration exists to
avoid.

**Hand-verified on a real device** (Pixel 8a): installed over the existing debuggable build
(schema still at v13 from earlier T67/T81 installs) — the migration ran live against a real
on-device database with no crash and no data loss, confirmed via logcat. The offline-search UI
path itself (airplane mode + search) was not separately hand-verified — the migration and unit
test coverage were judged sufficient.

Landed via [PR #105](https://github.com/DrewBrunning/mycorrhizal-crm/pull/105).
