# T60 — Audit trail UI (frontend only)

| | |
|---|---|
| **Rating** | 3 |
| **Size** | M |
| **Depends on** | — |
| **Beta** | after (backend is shipped; no schema/model change, additive) |
| **Source** | T18 (34-T18-audit-trail.md) — landed backend-only; this ticket is the UI half |

## What already exists

T18 shipped a full audit trail backend in v0.4.0:

| Layer | What's there |
|---|---|
| **Recording** | Every create/update/delete across 9 entity types (Contact, Note, Activity, LifeEvent, Gift, Circle, Tag, Household, Reminder) fires an async, non-blocking `AuditEvent` write — `models/audit_hooks.go`. The recording goroutine can never roll back the real write. |
| **API** | `GET /audit` — list the caller's events, newest first, filterable by `entity_type`/`entity_id`, `limit` 100–500. `POST /audit/{id}/undo` — revert an update event from its `before_snapshot` (Contact only, updates only; deletes and past-retention events are rejected). Both endpoints are user-scoped — no IDOR surface. |
| **OpenAPI** | Both endpoints and the `AuditEvent` schema are documented in `backend/openapi.yaml:5017–5091` and line 7248: `entity_type` enum, `operation` enum (create/update/delete), `before_snapshot` (redacted JSON), all typed. Ready for a mobile client. |
| **Undo** | `undoContact` (`controllers/audit_controller.go:103`) restores a contact through the canonical `ApplyRecordToContact` path — the before snapshot's flat fields rebuild a neutral Record, so Card/CRM/Passthrough are reconstructed rather than diverging. Undo for entities beyond Contact is a follow-up ticket. |
| **Purge** | `services/audit_purge_service.go` expires events past `AUDIT_RETENTION_DAYS`. |

**What does not exist: any frontend UI.** Zero React components, zero routes, zero navigation entries. A user has no way to see their audit history or trigger an undo from the app — it is a backend-only feature today.

## Why this matters beyond the browser

The mobile app (M1) needs a documented, stable API for every surface it surfaces. The audit API already has OpenAPI docs — the last thing needed is to exercise it end-to-end from a real client so the contract gets battle-tested before M1 starts. This ticket is also the lowest-overhead way to surface an undo button to the user (the API already works; it just needs a caller).

## What to build

### 1. New page: Audit Log (read-only event list)

A route under a sensible parent — either a standalone page (`/audit`) or a tab/section within a settings/data area. Shows the caller's audit events as a reverse-chronological list:

- Each row: timestamp, entity type (icon or badge), operation (create/update/delete, colour-coded), entity ID (tappable — navigating to the entity if it still exists and the user can view it).
- Filter toolbar at top: `entity_type` dropdown (the 9 types from the OpenAPI enum), `entity_id` free-text field. Respects `limit` (100 default, scroll-to-load more is fine — the API doesn't support cursor pagination today, and it's not in scope for this ticket).
- Empty state when no events match.
- API module: `src/api/audit.ts` with `AuditEvent` type, `getAuditEvents(params)` function — call `GET /audit` with query params.

### 2. Undo affordance on update events

On each `operation: "update"` row (contact only for now — other entity types return 400 from the undo endpoint, so the button should be gated on `entity_type === "contact"`), show an "Undo" button. On click:

- Confirmation dialog showing the timestamp and entity type being undone.
- Calls `POST /audit/{id}/undo`.
- On success: show a resolved toast that the contact was restored, and refresh the audit list.
- On 400: show the error message (delete event, unsupported entity). On 410: show "this event is past its retention window."

### 3. Contact-page integration (stretch — scoped separately if it grows)

The most natural place for an undo button is the entity the undo affects. A future follow-up could add a "recent changes" card on the contact detail page with the last N update events and inline undo — but that has a real layout-design cost (where does it go? does it show on narrow viewports?). Scoped out of this ticket to keep it buildable — the standalone audit page is the complete deliverable.

## Traps

- **The `before_snapshot` is redacted** at recording time (`models/audit.go`'s `redactJSON` strips credential fields). The UI must never attempt to display it or read it as user-facing content — it's opaque infrastructure. The list row should show entity type + operation + timestamp, not the JSON blob.
- **Undo is Contact-only.** The API's `switch` (`audit_controller.go:94–98`) only routes `AuditEntityContact` to `undoContact`; everything else returns 400. The frontend must match this gate — an "Undo" button on a Note/LifeEvent/Gift/... row must be hidden or show the server's error.
- **Five locale files, real translations** — `src/i18n/locales.test.ts` enforces this. Every user-facing string (page title, column headers, filter labels, confirmation dialog, toast messages, error messages) needs translation in `en`, `de`, `es`, `fr`, `it`. They cannot be English placeholders.
- **vitest no auto-cleanup** — add `afterEach(cleanup)` in any component test file.
- **No schema/migration/model change.** This is pure frontend — no `backend/` changes at all.
- **The API uses the user's session cookie** (or bearer token for the mobile app). All calls are user-scoped — the API does the IDOR gating. The frontend just calls it.

## Done when

- `cd frontend && npx tsc --noEmit && npx vitest run` green.
- A new audit page at a reasonable route shows the caller's events, newest first.
- The entity_type dropdown filters the list; clear clears it.
- An "Undo" button appears on update events for contacts and is hidden for all other entity types.
- Clicking "Undo" shows a confirmation, calls the API, shows a success toast on 200, shows the server error on 400/410, and refreshes the list.
- Undo a contact update, re-fetch the contact, and confirm the field reverted — hand-verified, not just component-in-isolation.
- All five locale files carry real translations; `locales.test.ts` passes.
- The audit page is reachable from app navigation (no dead URL-only page).

## Landing note — 2026-08-09

**DONE.** The audit backend already had a real e2e consumer in one session: the undo round-trip (create a contact, update it, undo via the page, re-fetch the contact, confirm the field reverted) ran green against the dockerised stack, and the undo endpoint's Contact-only gate was exercised from the UI as well (a circle update's row renders without an Undo button).

Delivered as one new `/audit` page plus the usual API module + hook pair:

- `src/api/audit.ts` — `getAuditEvents` / `undoAuditEvent`, with `AuditEntityType` + `AUDIT_ENTITY_TYPES` hardcoded mirrors of `backend/models/audit.go`'s tokens (per the codebase's enum-mirror convention, kept in sync by hand).
- `src/hooks/useAudit.ts` — server-side filtering, the 100→500 limit window for load-more, and `handleUndo` which refreshes the list after a successful undo.
- `src/AuditPage.tsx` — the table, the filter toolbar (entity_type dropdown + debounced entity_id, cleared together), the empty state, the undo confirmation dialog, and a contact-only tappable entity cell: event vcard_uids are batch-resolved via `getContactsByUid`, so a still-existing contact links to `/contacts/:id` and a since-deleted one degrades to its raw uid as plain text (archived contacts still resolve — `getContactsByUid` passes `include_archived`).
- Route + nav: `/audit` added to `App.tsx`, reachable via a new "Audit log" entry in the nav drawer (secondary destination — not a primary AppBar icon, not collapsed into the account menu).
- i18n: a new `audit.*` namespace in all five locale files, plus `nav.audit`.

Decisions worth recording:

- **Contact-only entity linking.** Only contacts have a per-entity route today (notes/activities/gifts/life events are contact-scoped; circles/households/tags have no per-entity page), so the tappable-entity requirement is implemented for contacts and the other entity types render their ID as plain text. Expanding the entity cell later is a small change to `EntityIdCell`.
- **Timestamps use `toLocaleString()`, not `formatDate()`.** The audit log needs date *and* time (multiple events share a day), which the date-only `DateFormat` preference doesn't express. Precedent: the API-tokens table in `SettingsPage`.
- **No load-more cursor.** The API is pageless (`limit` only); the page exposes a Load-more that grows the window 100 → 500 and hides once the response no longer fills it. This is the ticket's sanctioned "scroll-to-load-more is fine" reading.
- **410 is translated, everything else shows the server's message.** The undo endpoint's 410 ("past retention") maps to a localized string; 400/404/500 keep `getDisplayMessage()` so the server's exact diagnosis (e.g. "undo is not supported for …") is what the user sees.
- **Tests.** `src/api/audit.test.ts` (query building, error propagation, the enum mirror pinned to the backend's exact token set), `src/AuditPage.test.tsx` (rendering, the contact-only undo gate, uid→link resolution + deleted-contact fallback, filter round-trips, and the 200/400/410 undo paths), and `e2e/audit.spec.ts` (nav reachability, server-side filtering, the hand-verified undo round-trip, the non-contact no-undo gate, and the filtered empty state). The e2e polls the API before driving the UI because audit hooks write fire-and-forget.
- **No backend changes** — the ticket's frontend-only scope held.

Pre-existing note, not from this branch: `e2e/immich.spec.ts`'s shared-config tests flake under fully-parallel local runs (each `beforeEach`/`afterEach` deletes `/immich/config`, so parallel workers race each other's just-saved config). Reproduced on `main` with this ticket's changes stashed; passes 11/11 standalone. If it keeps biting, the spec should be `test.describe.configure({ mode: 'serial' })`.
