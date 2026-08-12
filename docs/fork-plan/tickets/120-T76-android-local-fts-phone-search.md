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
