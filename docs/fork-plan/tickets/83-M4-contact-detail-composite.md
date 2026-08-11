# M4 — `GET /contacts/:id/detail` contact-detail composite endpoint

| | |
|---|---|
| **Rating** | 4 — the mobile contact-detail screen is the highest-traffic surface in the app; 21 round-trips is unacceptable on a phone |
| **Size** | L |
| **Depends on** | — |
| **Source** | 2026-08-10 mobile-API work session — the frontend-per-screen audit found `ContactDetailPage.tsx` fires ~21 distinct endpoints to render one screen |
| **Status** | **DONE** (2026-08-11) |

## Why this exists

`ContactDetailPage.tsx` composes ~21 endpoints client-side to render one contact's screen:
the record itself, notes, activities, reminder completions, reminders, relationship edges +
other-party names, life events + related-contact names, conversation agenda, gifts, custom field
values, external identities + activities, circles, tags, field definitions, immich config +
summary, profile picture, current user (for enabled fields). On a phone this is many sequential
and parallel HTTPS round-trips to render one person — slow, battery-heavy, and fragile (any one
failure can drop a whole section).

The web app has the `ContactBriefing` composite for the *prep view* already. The contact-detail
screen is the same shape of problem at a larger scale, and M1 §4.2's `ContactDetailScreen` is
built directly off it. A single `GET /contacts/:id/detail` that returns everything the screen
renders (minus the profile-picture blob, which is a separate image request by nature) is the
obvious mobile shape.

## What exists today

- `GET /contacts/:id` → `ContactRecordResponse` (`contact_controller.go:410`,
  `models.NewContactRecordResponse`).
- Per-resource endpoints for each section of the detail screen (all documented in `routes.go`):
  notes, activities, completions, reminders, relationship-edges, life-events, conversation-agenda,
  gifts, field-values, external-identities, external-activities, circles, tags, link-field-types.
- `GET /contacts/:id/briefing` — the reference composite (`briefing_controller.go`), with the
  `normalizeBriefingSlices` discipline for empty blocks.
- The `use*` hooks + `ContactDetailPage` orchestration that currently does the fan-out
  (`frontend/src/hooks/useLifeEvents.ts`, `useRelationshipEdges.ts`, `useGifts.ts`, etc.).
- T17 cursor pagination on the list endpoints (used by the per-resource fetches).

## Design decisions — locked 2026-08-10

1. **One read-only `GET /contacts/:id/detail`, no new tables, no writes.** A server-side
   aggregation of existing data, scoped by `user_id`, following the briefing composite's
   pattern. The profile picture stays a separate request (`GET /contacts/:id/profile_picture`
   returns a blob; you cannot inline a photo into a JSON envelope sanely).

2. **Return the real `ContactRecordResponse` for the contact block** — the exact payload
   `GET /contacts/:id` returns, not a reduced shadow. The mobile client caches this in Room and
   edits against it; a different shape here would force a second adapter. Reuse
   `NewContactRecordResponse` verbatim.

3. **Collection blocks re-use the existing per-resource DTO shapes** (the same arrays the
   individual endpoints return), not bespoke reduced projections. That keeps the frontend types
   unchanged and makes the composite a drop-in for the existing hooks. The one deliberate
   enrichment: relationship edges and life events carry their **other-party / related-contact
   display names** (the hooks today make a second `getContactsByUid` call each — the composite
   must fold that in, or the phone still does the N+1).

4. **`enabled_contact_fields` is included** (the web fetches `/users/me` for it). Add a
   `user` block with the current user's enabled-fields so the client can render the correct
   field set without a second call. Keep it minimal — only what the screen needs
   (`enabled_contact_fields`), not the whole admin user object.

5. **Immich is a special case.** The web fires `getImmichConfig` + `getImmichContactSummary` on
   every detail load even when Immich isn't configured. The composite should include an
   `immich` block that is **absent when not configured** (config presence gate, cheap one-row
   lookup), rather than always composing it. When configured, include the contact summary. This
   is the one block that may legitimately be absent rather than `[]` (it's not a collection).

6. **Every collection block serializes as `[]` when empty, never `null`/absent** — the
   `normalizeBriefingSlices` rule, applied to the larger set. This is the single most likely
   regression (it broke the prep view once; `CLAUDE.md` frontend trap 8).

## What to build

1. **DTO** `models.ContactDetailResponse` in a new `models/contact_detail.go`: `contact
   ContactRecordResponse`, `user` (enabled fields), `notes`, `activities`, `completions`,
   `reminders`, `relationship_edges` (+ names), `life_events` (+ names), `agenda`, `gifts`,
   `field_values`, `external_identities`, `external_activities`, `circles`, `tags`,
   `immich` (optional). No `omitempty` on the collection blocks.

2. **Composer service/controller function** `buildContactDetail(db, userID, contactID, cfg)` —
   a sibling of `buildContactBriefing`. Reuse the existing controller query logic where it is
   shared (notes/activities/completions/reminders/agenda/gifts per-contact queries, the
   relationship + life-event name resolution from `briefing_controller.go`'s `attachBriefingRelationships`
   pattern). Scope every query by `user_id`. Batch other-party/related-contact names in single
   `WHERE vcard_uid IN ?` queries.

3. **Controller + route** `GetContactDetail`: ownership check (404 on another user's contact,
   same as `GetContact`), then compose. Register `GET /contacts/:id/detail` **after**
   `GET /contacts/:id` in `routes.go` so the literal `/detail` path is never captured as a
   contact ID (same note as the briefing route).

4. **Web**: add `frontend/src/api/contactDetail.ts` (`getContactDetail(id)`) with the response
   types. **The web page itself is NOT rewired in this ticket** — `ContactDetailPage`'s
   incremental per-hook loading is deliberate web UX (sections appear as they load) and rewiring
   it is a separate frontend-only ticket with real risk. The api module + OpenAPI contract exist
   so the Android client (M1 §4.2 `ContactDetailScreen`) has a stable target. (If a later
   session wants the web page on it too, that's its own ticket.)

## Traps

- **`omitempty` on collections = the prep-view crash, all over again.** Test on raw JSON.
- **Route ordering**: `/contacts/:id/detail` must not be shadowed by `/contacts/:id`. Gin
   matches literals before params, but the briefing route sets the precedent of registering the
   literal after the param route with a comment — follow it.
- **Sensitivity (91.13)**: exclude `secret` relationship edges exactly as `attachBriefingRelationships`
   does (in the query, not the caller). Preferences/custom-fields keep their own projection
   paths' sensitivity filtering.
- **Soft-deleted rows**: the notes/activities/reminders queries must keep their existing
   `deleted_at IS NULL` semantics (GORM default). The composite must NOT surface deleted rows
   just because it's a new query — mirror the existing per-resource handlers' filters.
- **N+1 discipline**: one batch query for other-party names, one for related-contact names, one
   for reminder contact names. No per-row fetches.
- **`NewContactRecordResponse` already does projection queries** (relationship edges, tags,
   keywords) — do not double-query those in the composer; reuse its output.
- **Ownership scoping** on every sub-query (CLAUDE.md trap 5).
- **Large payload**: the full detail of one contact can be big. That's fine (it's one contact,
   and the briefing is the same pattern); do NOT paginate sub-blocks in v1 — the client wants
   the whole screen in one shot. Note this explicitly in the OpenAPI description so it's a
   deliberate contract, not an accident.

## Done when

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- A real-DB controller test (`database.InitDB`) asserts: full envelope round-trips; every
  collection block is `[]` (raw JSON) for a fresh contact with no data; other-party names on
  relationship edges and related-contact names on life events resolve; `secret` edges are
  excluded; cross-user access is 404; `immich` absent when unconfigured and present (summary)
  when configured. OpenAPI drift test green.
- `frontend/src/api/contactDetail.ts` ships; `npx tsc --noEmit` and `npx vitest run` green.
- Hand-verify: `curl` the endpoint for a populated contact and diff the blocks against the
  individual endpoints' output; confirm the profile picture is the only thing not in the
  envelope.

## Landing note

**Shipped 2026-08-11** (branch `feature/m2-m3-m4-mobile-composites`, alongside M2's frontend
finish and M3). Landed per spec, with one deliberate deviation: **controller tests use the
existing `setupRouter()` AutoMigrate harness** (`controllers/contact_detail_controller_test.go`),
not `database.InitDB`, for the same reason recorded on M3's landing note — `briefing_controller_
test.go`, the reference pattern this ticket names explicitly, already uses `setupRouter()`, and
this composite adds no new persisted columns for a real-schema test to catch drift on.

`attachBriefingRelationships` (briefing_controller.go) was refactored into a shared
`resolveConfirmedRelationships` returning `[]models.BriefingRelationship` directly, so the
briefing composite and this one call the exact same confirmed-only, secret-excluded,
other-party-name-resolved query instead of maintaining two copies. Verified the refactor didn't
change behavior: `TestGetContactBriefing_*` all stayed green afterward.

Circles/Tags were built as **this contact's memberships** (via the `circle_members`/
`contact_tags` join tables), not the global per-user lists `GET /circles`/`GET /tags` return —
the ticket's own DTO field list didn't disambiguate this, but returning the full per-user circle/
tag universe on every single contact-detail fetch would contradict the ticket's own stated
purpose ("21 round-trips is unacceptable on a phone"). Documented explicitly in both the Go doc
comments and the OpenAPI schema description.

All five hand-verification passes (empty-slice normalization, full composition including both
name-resolution enrichments, secret-edge exclusion, cross-user 404, and all three Immich states)
were confirmed to actually fail when the corresponding code was temporarily broken, then restored
— per CLAUDE.md's "hand-verify your tests." Hand-verified against the real dev backend/DB: `GET
/contacts/5/detail` returns 200 with every collection block present as `[]`; the `immich` key is
absent, consistent with `GET /immich/config`'s own view that this account has no config; the
`contact` block's `id`/`uid`/`card.name` match `GET /contacts/5` exactly. The dev DB only has one
contact with no sub-resource data, so a diff against individual endpoints for a *populated*
contact wasn't possible live — the Go test suite's `TestGetContactDetail_ComposesAllBlocks`
covers that ground instead, asserting exact field values (contact_name, other-party name,
related-entity name) against real query results for one row per block.
