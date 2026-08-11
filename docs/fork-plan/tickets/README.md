# Tickets — the live status board

One file per ticket, in execution order. Each is meant to be self-contained enough to implement
from without reading anything else in `docs/fork-plan/` — but **read `/CLAUDE.md` first**, since
it carries the repo-wide conventions and recurring traps that every ticket assumes.

**This file is the single source of truth for status and ordering.** `../95-backlog-and-
priorities.md` and `../92-delivery-roadmap.md` are historical/decision records — read them for
*why* a past decision was made, never for *what's currently outstanding*. If either ever
disagrees with this file about a ticket's status, this file is right.

**Ratings** (`R`): 5 practically necessary · 4 strong, frequent use · 3 nice to have · 2 rarely
used · 1 re-evaluate whether a CRM should do this.

Real production data exists as of `v0.2.0-alpha-candidate` (2026-08-04) — see `/CLAUDE.md`'s
data-safety rules, binding on every open ticket below.

## To be done

Ranked by, in order: **rating** (highest first) → **whether it's actually ready to start** (a
ticket blocked on another open ticket ranks below ready ones at the same rating, regardless of
size) → **effort** (smaller first). Ticket numbers are stable IDs, not rank — the table order is
the rank.

**Three mobile-API tickets filed 2026-08-10** (see the Deferred table's M1 row): M1's Android app
is being built, and this session pulls the backend/web API additions it needs out as their own
ranked tickets — M2 (mobile push device registration + FCM delivery), M3 (dashboard
today/overview composite), M4 (contact-detail composite). All three are backend + web only — their
Android-side consumers are [M5](84-M5-android-polish-and-hardening.md). (The note filed with these
said "the Android client itself is external to this repo"; that was already untrue when written —
the app lives in `android/` as of M1, 2026-08-10. Corrected 2026-08-11.)

> **Numbering cleanup, 2026-08-11.** Filing M2/M3/M4 at 81/82/83 left the tickets they overlapped
> in place, so `main` briefly carried two `81-` and two `82-` files. Resolved: `81-N10-fcm-push`
> was fully superseded and is retired — its backend became M2, and its Android half (the
> `FirebaseMessagingService`, the keep-polling-for-de-Googled-devices constraint, and the
> double-notify idempotence key) moved to [M5](84-M5-android-polish-and-hardening.md) §5a rather
> than being dropped. `82-M1-missing-endpoints` was only *partly* superseded — M3 took its
> dashboard item, its other three are still live — so it is renumbered
> [85/M6](85-M6-photo-url-user-prefs-oidc.md). Old links to either old filename will not resolve.

| Ticket | Status |
|---|---|
| [M2](81-M2-fcm-mobile-push.md) · Mobile push device registration (token+client) + FCM delivery | **BACKEND DONE** — frontend Settings UI split off, not started |
| [M3](82-M3-dashboard-overview-endpoint.md) · `GET /dashboard` today/overview composite | **TO BE DONE** |
| [M4](83-M4-contact-detail-composite.md) · `GET /contacts/:id/detail` composite | **TO BE DONE** |
| [T64](90-T64-household-suggestions-null-crash.md) · "Suggest Households" crashes the whole app when there's nothing to suggest | **TO BE DONE**. R4. Nil-slice-serializes-as-`null` bug (backend `household_service.go` + frontend `AddressHouseholdSuggestions.tsx`) — reproduced live, root cause confirmed, easily reachable (fires on any empty-result scan, including a brand-new account). |
| [M8](89-M8-web-android-parity-audit.md) · Web ↔ Android parity audit — screen-by-screen matrix, then tickets from its gaps | **PROPOSED** — needs a yes on its method and its target before anyone starts. M1 was built against its own design doc, never against the web app, so "Phases 1–5 done" was never measured against the product. A short pass already found 4 drawer routes that are literal placeholders and 7 web pages with no Android equivalent (incl. the N2 prep view and the T10 graph). Carries an open question about whether literal 100% parity is the right target — parity is not one-directional. |
| [M7](88-M7-android-contact-record-coverage.md) · Android contact record: the editor covers 8 of ~30 field groups | **TO BE DONE**. R4. Addresses, organizations, titles, online services, links and personal info are *rendered on the detail screen but not editable*; `how_we_met`/`work_information`/`contact_information` appear nowhere. Not a data-loss risk — edits merge onto the loaded record — but the ceiling on Android is "don't break it". Also fixes emails/phones silently discarding type/label/preferred on edit. Needs a reusable multi-value editor decided first. |

> **N8 (2FA/TOTP) moved to Feature ideas, 2026-08-07.** For a self-hosted instance
> going through OIDC the IdP already owns 2FA, so app-level TOTP is redundant there; it only
> matters for local password accounts, which a single-operator instance rarely has. Not dropped —
> it's genuinely more likely than the live-sync/Dawarich ideas below — just not scheduled. (N7, the
> last task before the v0.4.0 alpha cut, and N6, deferred to v0.5.0 to batch schema/model changes,
> have since both shipped — the two tickets this note originally deferred N8 behind are done.)

### Deferred — not ranked, no plan to schedule

None of these are implementation-ready. Each needs its own design pass before it's even a sizeable
ticket — pulled in only when a concrete need arises, never implemented straight from its file. Split
into three categories, 2026-08-06, because "deferred" was hiding a real difference in how solidified
each idea actually is.

**Mobile clients** — a real, intended project (a native Android app). **M1's Phase 1 (core client)
shipped 2026-08-10** (see its landing note); the remaining phases and the mobile-only features
(call/SMS tracking, quick-capture, T57 import) are still deferred on API-surface stability as
later phases of the same ticket.

| Ticket | Notes |
|---|---|
| [T57](66-T57-bulk-import-api-for-external-clients.md) · Documented/stable bulk-import API for external clients | R1–2. A named sub-piece of M1 — a repeatable contact-import contract the mobile app calls from both a first-run prompt and a standing "Import from contacts" entry point in Data, not a one-shot setup-only call. No concrete consumer until M1's later phases. |
| [P4](68-P4-local-model-pilot.md) · Local-model code-gen pilot | R1. Re-enters scope specifically when M1's work begins, independent of the rest of this roadmap. |

**Planned features** — concrete, scoped-enough-to-name integrations; higher confidence they'll
actually get built than the Feature ideas below, just not scheduled yet.

| Ticket | Notes |
|---|---|
| [P2a](70-P2a-paperless-ngx-integration.md) · Paperless-ngx integration (API) | R2. Link a contact to a document that already lives in Paperless-ngx. |
| [P2b](71-P2b-seafile-integration.md) · Seafile integration (API) | R2. Same idea, for Seafile-hosted files/folders. |
| [P2c](72-P2c-nextcloud-owncloud-integration.md) · Nextcloud / ownCloud integration (WebDAV) | R2. Same idea again, reached via WebDAV rather than a bespoke API — a genuinely different integration shape from P2a/P2b. |

**Feature ideas** — real, but "might come back to" rather than planned. Lower confidence than
Planned features that these get built at all.

| Ticket | Notes |
|---|---|
| [N8](25-N8-2fa.md) · 2FA / TOTP | R3. Highest-confidence item here — moved down from the ranked table 2026-08-07 because OIDC already covers 2FA for IdP-backed instances (see the note on the To-be-done table). Pulled in if local-account instances ever matter. |
| [P1b](69-P1b-standing-contact-share.md) · Standing/live contact share + permission model (true synced contacts across users) | R1–2. XL. The closest existing formalization of "true sync," not a one-time copy like the done [P1](31-P1-contact-sharing.md). |
| [P2d](73-P2d-dawarich-geopulse-integration.md) · Dawarich / GeoPulse integration | R1–2. Location-history correlation into life-event/activity suggestions — an L4 idea, not a simple link. |
| [P2e](74-P2e-jellyfin-integration.md) · Jellyfin integration | R1. Least-defined idea in this list — not even scoped enough to say what it would do. |
| [P2f](75-P2f-audiobookshelf-integration.md) · Audiobookshelf integration | R1. Same shape of idea as P2e, for Audiobookshelf. |
| [P3](76-P3-ai-ollama-layer.md) · AI / Ollama layer | R1. Summarization, entity/relationship extraction, memory-curator suggestions. Gated on the propose-then-approve pattern; `90` D1 is explicit this is not an AI-first project. |
| [T61](80-T61-contact-picker-api.md) · W3C Contact Picker API for PWA import | R1. Lets the PWA read device contacts directly (Chrome on Android only) instead of requiring a file export first. Narrow audience — Android + PWA + no native app installed. |
| [M6](85-M6-photo-url-user-prefs-oidc.md) · Backend endpoints the Android app needs (photo URL, user prefs, OIDC native return) | R2. From the M1 Phase-5 review: expose the profile-picture URL in list/detail payloads, `PATCH /users/me`, and a `client=android` OIDC callback that returns the token via a custom-scheme deep link. Was `82-M1-missing-endpoints`; renumbered 2026-08-11 to clear the duplicate `82-`, and its fourth item (the dashboard composite) is now M3. |
| [M5](84-M5-android-polish-and-hardening.md) · Android app: deferred polish, native-endpoint consumers, and the missing test tier | R3. The **Android-client counterpart to M2/M3/M4**, which are all backend-side. The work M1 shipped without: tablet layout + accessibility audit (M1 items 31/32, explicitly deferred), the four recorded UI deviations from the web, the in-overlay quick-capture sheet, the app-side clients for M2 (unblocked now — M2's backend is merged), M3, M4 and the M1-endpoints items, and a decision about the absent instrumented-test tier. A container of independently shippable sections, not an all-or-nothing gate. Filed 2026-08-11 after a full review pass of `android/` — that pass's *defect* fixes landed separately, see M1's review-pass note. |

## Done

Kept for reference/lookup, not ranked — order below is roughly the sequence they shipped in.

| Ticket | Status |
|---|---|
| [N1](01-N1-contact-merge.md) · Contact merge / dedupe | **DONE** |
| [N4](02-N4-notes-capture-inbox.md) · Notes: dead-end journal → capture inbox | **DONE** |
| [T5](03-T5-lifeevent-frontend.md) · LifeEvent frontend + timeline | **DONE** |
| [T5b](04-T5b-lifeevent-reminders.md) · LifeEvent → reminder wiring | **DONE** |
| [T2](05-T2-circle-tag-triage.md) · Circle/Tag triage migration | **DONE** |
| [T3](06-T3-circle-tag-backend.md) · Circle/Tag backend rewiring | **DONE** |
| [T4](07-T4-circle-tag-frontend.md) · Circle/Tag frontend rewiring | **DONE** |
| [T25](08-T25-known-gaps.md) · Known small functional gaps | **DONE** |
| [T26](08b-T26-delete-semantics.md) · Delete semantics — purge job + constraint fixes | **DONE** |
| [T1](09-T1-households.md) · Household CRUD + suggestion trigger | **DONE** |
| [T20a](10-T20a-preferences.md) · Preferences migration | **DONE** |
| [T6](11-T6-custom-fields-api.md) · Custom fields v2 — API | **DONE** |
| [T7](12-T7-custom-fields-frontend.md) · Custom fields v2 — frontend + retire v1 | **DONE** |
| [T9](13-T9-selective-export.md) · Selective field export + sensitivity gating | **DONE** |
| [T12a](14-T12a-etag-primitives.md) · Activity/LifeEvent ETag primitives | **DONE** |
| [T24](15-T24-test-coverage.md) · Non-critical test-coverage expansion | **DONE** |
| [T8](16-T8-openapi.md) · OpenAPI coverage + drift test | **DONE** |
| [T17](17-T17-change-feeds.md) · Change feeds + cursor pagination | **DONE** |
| [T23](18-T23-ui-polish.md) · UI polish | **DONE** |
| [T22](19-T22-legacy-audit.md) · Legacy/dead-code audit + migration squash | **DONE** |
| [T27](20-T27-crm-kind-ui.md) · Contact CRM.Kind UI (individual/pet/animal) | **DONE** |
| [T28](21-T28-mobile-contact-layout.md) · Mobile contact view layout fixes | **DONE** |
| | **→ ALPHA v0.1.0 — shipped** |
| [T19](20-T19-cadence.md) · CadencePolicy + relationship health | **DONE** |
| [T21](21-T21-conversation-agenda.md) · ConversationAgenda | **DONE** |
| [N2](22-N2-prep-view.md) · Prep view / person briefing | **DONE** |
| [T10](23-T10-graph-traversal.md) · Graph traversal + multi-hop | **DONE** |
| [T11](24-T11-search-fts5.md) · Search synonyms, household scope, FTS5 | **DONE** |
| [N5](27-N5-bulk-operations.md) · Bulk operations | **DONE** |
| [T20b](28-T20b-gift-tracking.md) · Gift tracking | **DONE** |
| [P1](31-P1-contact-sharing.md) · Contact sharing — one-time copy | **DONE** |
| [T14](32-T14-external-link-substrate.md) · External-link substrate | **DONE** |
| [T15/T16](33-T15-T16-immich.md) · Immich level 1 + 2 | **DONE** |
| [T29](38-T29-contact-field-gaps.md) · Contact field gaps | **DONE** |
| | **→ v0.2.0-alpha-candidate — real production data begins here** |
| [T34](43-T34-contact-field-linking.md) · Tappable contact fields (tel/sms/mailto/copy/link registry) | **DONE** |
| [T38](47-T38-search-address-fields.md) · Search doesn't index address fields | **DONE** |
| [T30](39-T30-hide-empty-subtitles.md) · Hide empty section subtitles | **DONE** |
| [T31](40-T31-contact-tabs-info-architecture.md) · Contact detail — grouped cards, not tabs | **DONE** |
| [T32](41-T32-remaining-pages-mobile.md) · Mobile layout — Network/Settings/User Mgmt | **DONE** |
| [T33](42-T33-mobile-nav-restructure.md) · Mobile navigation bar restructuring | **DONE** |
| [T36](45-T36-life-event-categories.md) · Life event categories + expanded default types | **DONE** |
| [T35](44-T35-gift-tracking-gaps.md) · Gift tracking gaps (URL, notes, full-form add) | **DONE** |
| [T39](48-T39-user-management-add-user.md) · Add new users from User Management | **DONE** |
| [N9](30-N9-notification-channels.md) · Notification channels beyond email | **DONE** |
| [T41](50-T41-http-url-allowlist.md) · Web-link fields: http(s) allowlist, not a four-scheme blocklist | **DONE** |
| | **→ ALPHA v0.3.0 — shipped** |
| [T49](58-T49-vcf-import-merge-corrupts-existing-contact.md) · VCF/CSV import merge silently corrupts and orphans existing contact data ⚠ | **DONE** |
| [T50](59-T50-vcard21-import-blank-fields.md) · vCard 2.1 import produces blank phone/email/photo | **DONE** |
| | **→ ALPHA v0.3.1 — shipped** |
| [T51](60-T51-push-notification-413-payload-too-large.md) · Browser push "Test notification" fails with 413 from the push service | **DONE** |
| [T42](51-T42-immich-link-person-error-misclassification.md) · Immich "link a person" fails with "Could not reach Immich" | **DONE** |
| [T59](78-T59-immich-v041-still-broken.md) · Immich still broken in v0.4.1 testing | **DONE** (2026-08-09 — three root causes found against live Immich v3.1.0: HTTP/2 stale-session reuse, /api/people flat-array response shape, and GET /api/people/:id/assets removed in v3.x) |
| [T44](53-T44-link-field-type-registry-not-in-editors.md) · Link field type registry doesn't reach the editors | **DONE** |
| [T46](55-T46-gift-add-entry-points-per-status.md) · Gift "add" entry points default to Idea everywhere | **DONE** |
| [T43](52-T43-link-field-type-custom-icons.md) · Custom link field type icons don't render | **DONE** |
| [T45](54-T45-contact-jump-nav-mobile-dropdown.md) · Contact jump nav should collapse to a dropdown on narrow viewports | **DONE** |
| [T47](56-T47-field-action-icons-layout-and-tel-link.md) · Field action icons should sit near the edit button; phone should also be a tel: link | **DONE** |
| [T52](61-T52-simplify-contact-add-flow.md) · Simplify the contact-add flow to name + contact fields | **DONE** |
| [T53](62-T53-contact-detail-delete-action.md) · Delete a contact from its own detail page, not only from the list | **DONE** |
| [T54](63-T54-contact-header-menu-fixed-position.md) · Contact header's actions menu shifts position when the name wraps | **DONE** |
| [T55](64-T55-copy-button-hover-visibility.md) · Copy button should be hidden until hover/tap, matching edit | **REVERTED** |
| [T58](77-T58-preferred-phone-email-ui.md) · No UI to see or set "preferred" on phone/email (and URL/IMPP) | **DONE** |
| [T48](57-T48-migrate-frontend-off-cra-to-vite.md) · Migrate frontend off Create React App to Vite | **DONE** |
| [T37](46-T37-pet-relationship-kind-default.md) · Pet relationship should default to animal kind | **DONE** |
| [T40](49-T40-household-suggestions-shared-address.md) · Suggest households from shared address | **DONE** |
| [T56](65-T56-bulk-contacts-import-flow.md) · Bulk contacts import (Google Takeout / contacts-app export) in Data Settings | **DONE** |
| [T12b](35-T12b-caldav-serve.md) · Serve Interactions/LifeEvents as CalDAV | **DONE** |
| [T13](36-T13-two-way-calendar.md) · Two-way calendar sync ⚠ | **DONE** |
| [T18](34-T18-audit-trail.md) · Event history / audit trail | **DONE** |
| [N7](29-N7-attachments.md) · File / document attachments | **DONE** |
| | **→ ALPHA v0.4.0 — shipped** |
| [T59](78-T59-immich-v041-still-broken.md) · Immich still broken in v0.4.1 testing | **DONE** |
| [N6](26-N6-backup-restore.md) · Full backup restore | **DONE** (2026-08-09 — tested `VACUUM INTO` online backup via `make backup` + restore procedure; see the ticket's landing note for the two deliberate deviations from its implementation suggestions) |
| [T60](79-T60-audit-trail-ui.md) · Audit trail UI | **DONE** (2026-08-09 — new `/audit` page + API module + hook over T18's shipped backend: event list with server-side entity_type/entity_id filters, contact-only Undo with confirmation dialog, contact uid→detail-page links, all five locales; see the ticket's landing note for the decisions taken) |
| [M1](67-M1-mobile-android-app.md) · Native Android app (Kotlin, Jetpack Compose) | **Phases 1–5 core DONE** (2026-08-10 — working core client in `android/`: Gradle multi-module build, JWT/API-token auth, contacts list/detail/create/edit, activities/notes/reminders + unified timeline, tappable field actions + link-action enrichment (on-device verified), local FTS search, 349 hand-verified tests, CI workflow. Phase 3 sub-resources, Phase 4 native call/SMS tracking + notifications + quick-capture, Phase 5 T57 device-contacts import + QuickContact + custom link actions + R8. **Review pass 2026-08-11** — see the ticket's review-pass note: Android CI had never been green (two lint errors), `BootReceiver` could never fire, `:app` shadowed `:core:ui`'s resources, and ~80 strings were unlocalized; all fixed, 358 tests. Deferred polish moved to [M5](84-M5-android-polish-and-hardening.md)) |
| [T62](86-T62-badge-and-button-color-system.md) · Chip/badge color system overloads brand/status colors | **DONE** (2026-08-11 — chips go neutral, "Add X" buttons + interactive icons + gift/agenda links go brand green, dark-mode chip-flattening bug fixed) |
| [T63](87-T63-typography-roles-garamond-mono.md) · EB Garamond/IBM Plex Mono are loaded but essentially unused — apply the role split mobile testing found | **DONE** (2026-08-11 — Garamond on page headings (h5) + persistent nav; Mono on `overline` section subheadings *and*, per follow-up feedback, on per-field labels like "Birthday"/"Phone"/"Address" to contrast against the sans field content) |

### ⚠ A grooming lesson worth keeping visible

The pre-alpha ordering rules (dependency → data/contract risk → rating → size) produced an alpha
with no rating-5 *capability* in it: T19 cadence, T11 search, and N2 prep view — all R5 — landed
*after* alpha, because the risk-based rule places the cut line by risk, and all three were
additive/safe to defer. N1 contact merge was the only R5 shipped inside alpha itself. All three
have since landed anyway (see above). Noted here because it's a real lesson for the next big
milestone cut, not a live concern — rating alone doesn't set a milestone line; risk does.

### Considered and deliberately not ticketed

Reviewed against Monica's feature set and rejected, with reasons, so they don't get re-raised:

- **Tasks** (R2) — deliberately delegated to an external manager: [T19](20-T19-cadence.md) routes
  overdue cadence to Vikunja via webhook rather than building a task system. A legitimate
  architecture choice, not an omission.
- **Debts / money owed** (R1) — Monica has it; niche enough to not belong in this product.
- **Conversation/message log** (R2) — already covered: `Activity.Type` includes `message`, and
  Activities support multiple contacts.
- **Introduction chain / "who introduced us"** (R2) — mostly covered by `HowWeMet`; if wanted, it
  is a registry token in `relationship_type_registry.go`, not a feature.
- **Standalone journal as it exists today** (R1) — superseded by
  [N4](02-N4-notes-capture-inbox.md) rather than kept.

### Open questions

- **Keep i18n across 5 locales?** (unresolved) Inherited from the original Meerkat fork. Every
  user-facing string in every ticket costs 5 translations — a real, permanent drag, and every
  ticket in this folder currently assumes the answer is "yes" (each one's "Done when" requires
  real translations in all 5 locale files). Defensible for a shared fork, harder to justify for a
  one-or-two-person instance. Worth a deliberate keep-or-drop rather than continuing by default —
  flagging here since it would change the "done when" bar on every open ticket above, not just one.
