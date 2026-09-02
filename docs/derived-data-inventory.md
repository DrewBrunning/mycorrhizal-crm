# Derived-data inventory

Every piece of derived, denormalized, cached, or otherwise non-canonical state this backend keeps,
classified so that none of it is "the derived state nobody has classified, which by definition has no
rebuild path and no consistency check" ([issue #497](https://github.com/DrewBrunning/mycorrhizal-crm/issues/497)).

FTS gets its own three-ticket treatment ([#461](https://github.com/DrewBrunning/mycorrhizal-crm/issues/461)
rebuild, [#462](https://github.com/DrewBrunning/mycorrhizal-crm/issues/462) consistency,
[#463](https://github.com/DrewBrunning/mycorrhizal-crm/issues/463) mutation-path tests) and a dedicated
runbook ([`operations/search-index.md`](operations/search-index.md)); this doc generalizes that shape to
everything else.

| | |
|---|---|
| **Last updated** | 2026-09-02 ([#497](https://github.com/DrewBrunning/mycorrhizal-crm/issues/497)) |
| **Companion docs** | [`adrs/0012-canonical-database-invariants.md`](adrs/0012-canonical-database-invariants.md) (INV-A5 / INV-D8 / INV-D9 — the invariants this state must satisfy), [`operations/search-index.md`](operations/search-index.md), [`operations/disaster-recovery.md`](operations/disaster-recovery.md) (post-restore rebuild steps), [`security/data-retention-lifecycle.md`](security/data-retention-lifecycle.md) (where each copy lives) |
| **Rule** | Adding a persisted derived artifact, a new client-side cache, or a new external sync target means adding a row here — and a rebuild path plus a consistency probe, or a written reason it needs neither — in the same PR. Same discipline as ADR 0004's delete cascade and the retention-lifecycle doc. |

The four questions each row answers (from the issue): **derived from what**, **what maintains it**,
**how it is rebuilt** (or why it cannot / need not be), **what happens if it is stale**.

---

## 1. Rebuildable from canonical data

Denormalized artifacts that can be dropped and recomputed from the canonical tables with an identical
result (ADR 0012 **INV-A5**).

### 1a. FTS5 search index — `contacts_fts` / `notes_fts` / `activities_fts`

| | |
|---|---|
| **Derived from** | Live (`deleted_at IS NULL`) `contacts` / `notes` / `activities` rows — the indexed columns are listed in `services.ftsConsistencySpecs`. |
| **Maintained by** | SQL triggers (migrations `000007` / `000010` / `000020`), on every ordinary INSERT/UPDATE/DELETE. |
| **Rebuild** | `services.RebuildSearchIndex` — `POST /api/v1/admin/search/rebuild` or `cmd/backfill-search-index`. One transaction, atomic swap, interruption-safe. |
| **Consistency check** | `services.CheckSearchIndexConsistency` (INV-D9, contract-aware — never flags a soft-deleted/archived row); cheap row-count probe `checkDerivedIndexes` (INV-A5). Both fold into the scheduled `db_integrity_check` job and `GET /admin/integrity-check`. |
| **If stale** | Search misses or surfaces wrong rows. Correctness for deleted/archived rows is enforced by the outer query's own filters, never by index contents. |

### 1b. Flat contact projection — `contacts.firstname` / `lastname` / `email` / `phone` / `birthday` / `fn` / `org`

| | |
|---|---|
| **Derived from** | `contacts.card` (the nested `contactmodel.Record`), via `contactmodel.DeriveProjection`. |
| **Maintained by** | `Contact.BeforeSave` → `deriveDenormalized` on every create/update, using the T75 merge (`models/contact_card_merge.go`) — flat fields are authoritative for what they can express, the loaded Card for what they cannot. |
| **Rebuild** | `services.RebuildDerivedContactColumns` — `POST /api/v1/admin/contacts/rebuild-derived` or `cmd/backfill-derived-columns`. Card-authoritative (re-derives the flat scalars straight from `Card`'s projection); commits page by page, so an interrupted run resumes cleanly. |
| **Consistency check** | `services.checkDerivedContactColumns` (INV-A5, check slug `derived_contact_column.divergent`) — recompute per column, report which drifted, per user. In the scheduled job, `GET /admin/integrity-check`, and `mycorrhizal doctor`. Plus the generative `TestDataInvariant_D8_FlatProjectionIsAFixpoint` property. |
| **If stale** | List views, CardDAV `N`/`FN`/`EMAIL`, exports, and the contacts feed show a value that disagrees with the contact's own Card. Only produced by a write that bypassed the hooks (raw-SQL migration, direct bulk INSERT, mid-write restore). |

### 1c. Searchable / sortable text columns — `contacts.addresses_flat` / `phones_normalized` / `sort_name` / `address`

| | |
|---|---|
| **Derived from** | `addresses_flat` ← `FlattenAddresses(contacts.addresses)` (T38); `phones_normalized` ← `FlattenPhones(contacts.phones)` (T69); `sort_name` ← `DeriveSortName(lastname, firstname)` (T73, migration `000021`); `address` ← `FormatAddress(addresses[0])`. The `addresses` / `phones` JSON columns are themselves a flat projection of `card` (migration `000022`). |
| **Maintained by** | `Contact.BeforeSave` → `deriveDenormalized`. Backfilled for pre-existing rows by migrations `000010` / `000020` / `000021` / `000022`. |
| **Rebuild** | Same as 1b — `services.RebuildDerivedContactColumns` covers these columns too. |
| **Consistency check** | Same as 1b — `checkDerivedContactColumns`. `sort_name` is compared case-insensitively so migration `000021`'s ASCII-only `lower()` backfill of a non-ASCII name (`Öberg`) is not perpetually flagged; a genuinely stale key still shows. |
| **If stale** | `addresses_flat` / `phones_normalized`: an address or phone number is not found by search. `sort_name`: the name-sorted contacts page orders one row wrong (pagination stays total — a single value compared under one collation). |

---

## 2. Computed on read — nothing persisted, nothing to rebuild

State that looks derived but is recomputed on every request and never written to a table. If the inputs
are correct, the output is correct; there is no stale copy to detect.

| Artifact | Computed by | Inputs |
|---|---|---|
| Cadence health — `next_due` / `overdue_by` | `services.cadence_service` | `cadence_policies` + the interaction timeline. Migration `000002`: *"Health … is derived from the timeline, never stored, so this table carries no derived columns."* |
| Reach-out **health** (the overdue signal) | `services.reach_out_trigger_service` | cadence + last-qualifying-interaction |
| Graph traversal / relationship projection (`Card.RelatedTo`) | `models.RecordForContact` → `projectRelationshipEdges` | `relationship_edges` (confirmed, normal-sensitivity only). Never written back to `Card`. |
| Household address suggestions | `services.household_service` | contact addresses + `dismissed_household_suggestions` |
| Graph suggestions | `services.graph_suggestion_service` | the confirmed edge graph |
| Dashboard / briefing aggregates | `models.dashboard.go` / `models.briefing.go` | live rows, counted per request |
| Contact list `resolved_relation`, search relation synonyms | `services.search_service` | the relation-type registry (code) |

---

## 3. Canonical state that resembles derived data but is **not** rebuildable

Per the issue: *"Anything that cannot be rebuilt from canonical data is not derived data — it is
canonical data with no backup, and that finding is worth the exercise on its own."* These carry user
decisions or event-scan positions that no canonical table can reconstruct. They are canonical; they are
covered by ordinary backup/restore and the delete cascade, **not** by a rebuild path.

| State | Why it is canonical, not derived |
|---|---|
| `reach_out_suggestions` rows + `status` | One row per detected meaningful change, plus the user's `pending`/`dismissed` decision and the linked one-off `Reminder`. The *detection* could be replayed from `audit_events`, but the dismissal decision and the reminder link cannot — replaying would resurface every suggestion the user already dismissed. Edge-shaped, hard-delete (CLAUDE.md trap #7). |
| `reach_out_cursors.last_audit_event_id` | A watermark: the last `AuditEvent.ID` scanned for reach-out triggers. Not derived from anything — it *is* the job's position. **If lost / reset to 0:** the next run re-scans history; the per-suggestion de-dup (a suggestion already exists for that `audit_event_id`) prevents double-firing, so the failure mode is wasted work, not duplicate suggestions. |
| `dismissed_household_suggestions` | A permanent "don't suggest this group again" decision, keyed by `(user_id, address_hash, member_hash)`. Purely a user choice; nothing to recompute. |
| `cadence_policies` | User-authored maintenance rules (target interval, qualifying types). Soft-delete, user content. |
| `conversation_agenda` | User-authored "bring up next time" notes. Soft-delete, user content. |
| `contacts.card` / `crm` / `passthrough` | The **canonical** neutral record. The flat columns are its projection, not the other way round. `card` integrity (valid JSON, unique element ids) is checked by `checkCanonicalRecords` (INV-D8); it is never "rebuilt". |

---

## 4. Cached state and its invalidation

*"A cache with no defined invalidation is a correctness bug with a delay."*

| Cache | Scope | Invalidated by | On a miss |
|---|---|---|---|
| `contacts` / `notes` / `activities` / `life_events` / `reminders` `.revision` + `.etag` | Per row, server-side (ADR 0006) | Every successful write bumps `revision` monotonically in `AfterSave`; `etag` is derived from it. Never wall-clock derived. | A client with a stale `etag` gets `412`/`409` on a conditional write (ADR 0008/0009) and re-fetches; a stale `If-None-Match` GET just returns `200` with the current body. |
| HTTP `ETag` / `If-Match` / `If-None-Match` | Per response, client-held | The row's `revision` (above). No intermediary/proxy cache is assumed — self-hosted, single process. | Same as above: `412` on write, full body on read. |
| CardDAV `getetag` / sync-token | Per address object / per collection, client-held | The contact's `etag`; the collection sync-token advances per change. `reconcileContactSync` is full-overwrite by design (T13) — a stale client is corrected on next sync, converging to a fixpoint (INV-A6). | Client re-pulls the changed objects. |
| Frontend React-Query cache | Per browser tab, in-memory | `src/hooks/use<Entity>.ts` invalidate the relevant query keys after every mutation; no time-based staleness is relied on for correctness. | A refetch; worst case a brief stale render until the invalidation resolves. Not a persistence layer. |
| Service worker (`frontend-prod` only) | Per browser, on-disk | App-shell precache is versioned by the build hash; API responses are **not** service-worker-cached. | Network fetch. `frontend-dev` never compiles a real service worker (CLAUDE.md frontend note). |
| `localStorage` (auth token, UI prefs) | Per browser | Explicit writes on login/logout and setting changes. | Treated as absent — the app re-authenticates or falls back to defaults. |

`localStorage` and the React-Query cache are per-viewer conveniences, never a source of truth: the
backend is always authoritative and every client re-derives from it.

---

## 5. Rebuilding after restore / migration / bulk import

The trigger-bypassing situations that leave section 1 state stale, and the follow-up:

| Situation | Follow-up |
|---|---|
| **Restore from backup** ([#453](https://github.com/DrewBrunning/mycorrhizal-crm/issues/453)) | Run **both** rebuilds — `POST /admin/search/rebuild` **and** `POST /admin/contacts/rebuild-derived` (or `cmd/backfill-search-index` + `cmd/backfill-derived-columns`), then `mycorrhizal doctor`. Documented as a step in [`operations/disaster-recovery.md`](operations/disaster-recovery.md). A `VACUUM INTO` snapshot is transactionally consistent, so in practice these are no-ops — but a snapshot taken from a database that was itself mid-repair might not be, and `doctor` proves it either way. |
| **Raw-SQL migration touching a base column** | The migration re-derives inline (as `000010` / `000020` / `000021` / `000022` do) **or** the release notes name the rebuild command to run on upgrade. The migration author owns this; `checkDerivedContactColumns` / `checkDerivedIndexes` catch an omission on the next integrity sweep. |
| **Bulk import** | The source-import framework creates contacts through the ordinary GORM path, so `BeforeSave` and the FTS triggers both fire — no rebuild needed. A future import that INSERTs rows directly must call both rebuilds and say so. |

---

## Hand-verification (CLAUDE.md — a test that has never failed has proven nothing)

- **`checkDerivedContactColumns`** — neuter it to `return nil, nil`:
  `TestDataIntegrity_TEST02_InvariantMatrix/INV-A5/denormalized_contact_column_diverges_from_Card` and
  `TestRebuildDerivedContactColumns_ConvergesToProbeClean` fail. Restore.
- **The T75 merge** — revert `mergeCardWithFlat` to the pre-T75 wholesale `return fresh`:
  `TestBeforeSave_PlainSavePreservesCardOnlyData` and
  `TestBeforeSave_EveryPlainSavePrimitivePreservesCardOnlyData/Save` fail on the dropped Card-only
  members (`SpeakToAs`, `PersonalInfo`, `CRMEnvelope.Kind`, `Passthrough`). Restore.
