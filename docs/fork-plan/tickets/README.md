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

| Rank | Ticket | R | Size | Depends on | Ready? |
|---|---|---|---|---|---|
| 1 | [N6](26-N6-backup-restore.md) · Full backup restore | 3 | S | — | Ready |
| 2 | [N7](29-N7-attachments.md) · File / document attachments | 3 | M | — | Ready (coordinate with N6 — not a hard dependency, see its ticket) |

> **N8 (2FA/TOTP) moved to Feature ideas, 2026-08-07.** For a self-hosted instance
> going through OIDC the IdP already owns 2FA, so app-level TOTP is redundant there; it only
> matters for local password accounts, which a single-operator instance rarely has. Not dropped —
> it's genuinely more likely than the live-sync/Dawarich ideas below — just not scheduled while
> N7 (the last task before the v0.4.0 alpha cut) and N6 (deferred to v0.5.0 to batch schema/model
> changes) remain.

### Deferred — not ranked, no plan to schedule

None of these are implementation-ready. Each needs its own design pass before it's even a sizeable
ticket — pulled in only when a concrete need arises, never implemented straight from its file. Split
into three categories, 2026-08-06, because "deferred" was hiding a real difference in how solidified
each idea actually is.

**Mobile clients** — a real, intended project (a native Android app), just gated on API-contract
stability rather than on demand for the idea itself.

| Ticket | Notes |
|---|---|
| [M1](67-M1-mobile-android-app.md) · Native Android app (Kotlin, Jetpack Compose) | R2. Automated call/SMS/contact-interaction tracking is the actual point, not just a native UI. Gated on API-surface stability — earliest realistic entry is the move from beta to a real v1.0.0. |
| [T57](66-T57-bulk-import-api-for-external-clients.md) · Documented/stable bulk-import API for external clients | R1–2. A named sub-piece of M1 — a repeatable contact-import contract the mobile app calls from both a first-run prompt and a standing "Import from contacts" entry point in Data, not a one-shot setup-only call. No concrete consumer until M1 starts. |
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
| [T44](53-T44-link-field-type-registry-not-in-editors.md) · Link field type registry doesn't reach the editors | **DONE** |
| [T46](55-T46-gift-add-entry-points-per-status.md) · Gift "add" entry points default to Idea everywhere | **DONE** |
| [T43](52-T43-link-field-type-custom-icons.md) · Custom link field type icons don't render | **DONE** |
| [T45](54-T45-contact-jump-nav-mobile-dropdown.md) · Contact jump nav should collapse to a dropdown on narrow viewports | **DONE** |
| [T47](56-T47-field-action-icons-layout-and-tel-link.md) · Field action icons should sit near the edit button; phone should also be a tel: link | **DONE** |
| [T52](61-T52-simplify-contact-add-flow.md) · Simplify the contact-add flow to name + contact fields | **DONE** |
| [T53](62-T53-contact-detail-delete-action.md) · Delete a contact from its own detail page, not only from the list | **DONE** |
| [T54](63-T54-contact-header-menu-fixed-position.md) · Contact header's actions menu shifts position when the name wraps | **DONE** |
| [T55](64-T55-copy-button-hover-visibility.md) · Copy button should be hidden until hover/tap, matching edit | **DONE** |
| [T58](77-T58-preferred-phone-email-ui.md) · No UI to see or set "preferred" on phone/email (and URL/IMPP) | **DONE** |
| [T48](57-T48-migrate-frontend-off-cra-to-vite.md) · Migrate frontend off Create React App to Vite | **DONE** |
| [T37](46-T37-pet-relationship-kind-default.md) · Pet relationship should default to animal kind | **DONE** |
| [T40](49-T40-household-suggestions-shared-address.md) · Suggest households from shared address | **DONE** |
| [T56](65-T56-bulk-contacts-import-flow.md) · Bulk contacts import (Google Takeout / contacts-app export) in Data Settings | **DONE** |
| [T12b](35-T12b-caldav-serve.md) · Serve Interactions/LifeEvents as CalDAV | **DONE** |
| [T13](36-T13-two-way-calendar.md) · Two-way calendar sync ⚠ | **DONE** |
| [T18](34-T18-audit-trail.md) · Event history / audit trail | **DONE** |

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
