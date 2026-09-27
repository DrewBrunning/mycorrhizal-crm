# ADR 0030: Feeds — private Atom emission; consumption deferred; no ActivityPub

- **Status:** accepted
- **Date:** 2026-09-26
- **Implements:** issue #382 ("Design doc settles A/B scope and the auth + sensitivity model; a first cut
  is the read-only Atom emission (per-contact + aggregate) with private tokens, sensitivity-filtered and
  rate-limited"). This ADR is that design pass.
- **Depends on:** `docs/adrs/0004-soft-vs-hard-delete-semantics.md` (revocation and cascade shape),
  `docs/adrs/0015-temporal-semantics.md` (instants on the wire), the T66 contact timeline
  (`controllers/timeline_controller.go`, `models/timeline.go`) as the query layer, the `ApiToken`
  model and lifecycle (`models/api_token.go`, `controllers/api_token_controller.go`,
  `services/api_token_service.go`) as the credential template, and CLAUDE.md's sensitivity rule.
- **Feeds:** the implementation tickets on the v1.3.0 milestone listed under "Implementation" below.

## Context

The app has no RSS, Atom or ActivityPub support. Issue #382 proposed two directions: (A) emitting feeds a
user subscribes to in their own reader, and (B) consuming a contact's external feed into their timeline.
It ruled out ActivityPub unless a concrete need appears.

Three facts about the current codebase shaped the design:

1. **The data to emit already exists as one merged view.** The T66 contact timeline merges six entity
   types (`note`, `activity`, `completion`, `life_event`, `external_activity`, `gift`) behind one
   `timelineComposer`, with a deterministic `(date, type, id)` sort. A feed is that timeline in another
   serialization. The composer lives in the controller package and is scoped to one contact.
2. **None of the six timeline types has a `Sensitivity` column.** Only `RelationshipEdge`, `Preference`,
   `FieldDefinition`, `OccasionObligation` and `OccasionEvent` have one, and none of them is on the
   timeline. The issue's first risk ("reuse the same query-level `normal|private|secret` exclusion") has
   nothing to filter today. `caldav/backend.go`'s `ListCalendarObjects` is in the same position and
   leaves the obligation to a comment. A comment is not enforcement.
3. **Most feed readers cannot send an `Authorization` header.** Hosted readers such as Feedly, Inoreader
   and NewsBlur, and most local ones, subscribe to a bare URL. Any credential has to travel in the URL.
   GitHub's private dashboard feed (`…private.atom?token=…`) is the familiar precedent.

The threat that matters most is not a stranger guessing a URL. A 256-bit token makes that negligible.
The real threat is that **every item the feed serves is copied to wherever the reader runs**. For a
hosted reader, that is a third party's database, outside this instance's deletion, retention and
sensitivity guarantees. The design limits what leaves the instance and makes each feed revocable. It
cannot make a copy that has already left come back.

## Decision

### 1. Scope: emit Atom only (A); defer consumption (B); ActivityPub out of scope

- **A — emission ships in v1.3.0.** It adds a read-only surface and no new write path or outbound client,
  and it reuses the timeline query layer.
- **B — consumption is deferred.** It is not on v1.3.0 and has no ticket yet. Decision 9 records the
  constraints so a future ticket is not a fresh design pass.
- **ActivityPub is out of scope.** Federation needs an actor inbox and outbox, HTTP Signatures,
  follower state and moderation. That is a large inbound attack surface on a single-owner, self-hosted
  instance, and no concrete need has appeared. A new ADR would be needed to reopen it.

### 2. Format: Atom 1.0 (RFC 4287) only, `text` constructs only

- **The app serves only Atom; there is no RSS 2.0 variant.** Every reader in common use supports Atom.
  Atom has mandatory, unambiguous `id` and `updated` elements, RFC 3339 timestamps, and a defined
  `type` attribute on every text construct. RSS 2.0 has RFC 822 dates, optional GUIDs and ambiguous HTML
  in `<description>`. A second format doubles the test surface for no reader it would unlock.
- **Every `title`, `summary` and `content` element is `type="text"`, never `html` or `xhtml`.** The feed
  never tells a reader to render markup. Feed readers that render HTML are a known XSS vector, and
  user-authored note bodies are untrusted text. Rendering uses `encoding/xml` marshalling of plain
  structs, which escapes everything and replaces XML-invalid runes with U+FFFD. No feed library is added;
  emission is a handful of structs.
- **Entry `id`:** `urn:uuid:<UUIDv5(feedNamespace, "<timeline type>:<entity id>")>`. The value is stable
  across renders, rotations and hostname changes. It does not expose sequential database IDs or the
  instance hostname. The namespace is a fixed constant in code.
- **Entry `published`** is the timeline date (`timelineLifeEventDate` for life events, as the timeline
  already resolves it). **Entry `updated`** is the entity's `UpdatedAt`. **Feed `updated`** is the
  maximum entry `updated`, or the feed's `CreatedAt` when the feed has no entries. It is never the time
  of the request, so an unchanged feed renders byte-identically.
- **Entry `link rel="alternate"`** points to `FRONTEND_URL + "/contacts/<contact id>"`. The link is
  omitted when `FRONTEND_URL` is `*` (dev), because no absolute URL exists then.
- Titles are localized with the backend `i18n` package, in the owning user's language. The
  `{type label}: {contact names}` keys are added to all five locale files.

### 3. Two feed kinds, both built on the timeline composer

| Kind | Source | Contacts |
|---|---|---|
| `contact` | That contact's merged timeline, the same six types as T66 | The one contact, whether or not it is archived |
| `aggregate` | The same six types across **every non-archived contact** the user owns | All live, non-archived contacts |

Both kinds follow these rules:

- **Window:** the 50 most recent entries by timeline date, descending. A feed is a recent-changes
  surface, not an archive, and readers keep their own history. The window has no pagination
  (RFC 5005 paging is not implemented).
- **Future-dated entries are excluded:** any entry whose timeline date is after `now`, for example a
  scheduled activity or this year's not-yet-reached life-event anniversary. A feed records what has
  happened, and readers sort by date.
- **A multi-contact `activity` appears once in `aggregate`,** deduplicated by activity ID, with every
  live participant named in the title.
- **The composer moves from `controllers/timeline_controller.go` into `services`,** with the contact
  filter made optional. `GetContactTimeline` then calls the service. The existing timeline tests are the
  regression guard for the move. The feed does not maintain a second query path over the six tables.

### 4. Detail level is chosen per feed: `headlines` (default) or `full`

A feed's content is fixed when it is created:

- **`headlines` (default):** the localized type label, the contact display name(s) and the date.
  **This level contains no user-authored free text beyond contact display names.** There are no note
  bodies, activity titles, descriptions or locations, no gift descriptions, no life-event descriptions and
  no reminder-completion messages.
- **`full`:** `headlines`, plus the entity's user-authored text in a `type="text"` `content` element:
  - note `content`;
  - activity `title`, `description` and `location`;
  - reminder-completion `message`;
  - life-event `description`;
  - gift `description`.

  Two of these (`Gift.Description`, `ReminderCompletion.Message`) are at-rest-encrypted columns
  (`serializer:encrypted`, P4/#380). A `full` feed serves them decrypted, the same as the REST API
  does; the at-rest envelope protects the database file, not what the owner chooses to export.

**`external_activity` is always rendered at the `headlines` level, at either setting.** That means its
type and `source_system` only, never its `Payload`. The payload is an arbitrary map from an integration
(for example, Immich asset references) and may carry internal URLs that must not leave the instance.

The rule is "no free text unless the user asked for it" rather than a per-field allowlist. That makes
the default auditable in one sentence and keeps the choice with the user. The web UI states plainly
that `full` copies note text into whatever reader they use (see ticket 3).

### 5. Sensitivity: a structural guarantee, not a comment

CLAUDE.md's sensitivity rule covers copies that leave the instance. A feed is such a copy, so the rule
applies:

- **Rows above `normal` are excluded in the query, with no `include_sensitive` opt-in for feeds.** The
  opt-in exists for exports the user downloads to their own device. A feed's copy lands in a reader the
  instance cannot see, which is the case the rule exists for.
- Today none of the six timeline tables has a `Sensitivity` column, so the filter is empty.
- **A completeness test keeps it honest:** a test in `services` reflects over the model type behind
  each `models.TimelineTypes` entry. It fails if any of them has a `Sensitivity` field while the feed
  composer's query has no `sensitivity = 'normal'` predicate for that table. That test is the
  enforcement the `caldav` comment lacks.
- If a future change adds a sensitivity level to notes, for example, the feed fails CI until it filters
  that level. The fix is to add the filter, not to allowlist the finding.
- `status: suggested` does not apply: no timeline type has it.

### 6. Credential: a per-feed token, hashed at rest, on the `ApiToken` lifecycle

**The model is `Feed`**, a new file in `backend/models/`: a new UUID-PK entity whose ID is generated in
`BeforeCreate`, with this migration:

| Column | Notes |
|---|---|
| `id` | UUID PK |
| `user_id` | owner, not null, indexed |
| `name` | user label, 1–100 characters |
| `kind` | `contact` or `aggregate` (`oneof`) |
| `entity_id` | contact `VCardUID`; required for `contact`, must be empty for `aggregate` |
| `detail` | `headlines` or `full` (`oneof`), default `headlines` |
| `token_hash` | SHA-256 hex of the plaintext, `unique`, never serialized |
| `last_accessed_at` | nullable |
| `revoked_at` | nullable |
| `created_at` / `updated_at` | |

- **The token is `mycorrhizal_feed_` + 32 bytes of `crypto/rand`, base64url-encoded,** minted the way
  `generateApiToken` mints API tokens.
- **The plaintext appears once,** in the create or rotate response, as the full feed URL. After that
  the server keeps only the hash, as `ApiToken` does.
- **The prefix keeps the credential spaces separate:**
  - `AuthMiddleware` routes every `mycorrhizal_`-prefixed bearer to `LookupAPIToken`, which only
    searches `api_tokens`. A feed token presented to the REST API therefore misses and returns 401.
  - The feed endpoint looks up only `feeds.token_hash`, so an API token or JWT presented there misses
    and returns 404.
  - No scope check is involved. The two tables are disjoint.
  - This is deliberately **not** a new `ApiToken.Scope` value. Each scope must be rejected by hand at
    every surface; `AuthMiddleware`'s `carddav` check is one such line. A new scope that someone forgot
    to reject would silently grant REST access. A separate table cannot fail that way.
- **Feed tokens do not expire.** An expiring token breaks a reader subscription silently, weeks later,
  with no error the user sees. Revocation and rotation are the controls, and the web UI shows
  `last_accessed_at` so a stale feed is visible. This departs from the `ApiToken` policy that every new
  token gets an `ExpiresAt`. The ASVS row records the departure (ticket 2).
- **At most 50 active feeds per user.** Creating another returns 422.

**Revocation follows the `ApiToken` lifecycle exactly** (ADR 0004 is satisfied: a credential is neither
authored content nor a join row, and `ApiToken` already sets the precedent of keeping revoked rows for
the audit trail):

- `DELETE /feeds/:id` sets `revoked_at`. A revoked feed returns 404 and drops out of the list.
- `POST /feeds/:id/rotate` revokes the old row and mints a new one with the same `name`, `kind`,
  `entity_id` and `detail`, following `RotateApiToken`'s shape.
- **Account-takeover responses revoke feeds too.** A new `services.RevokeAllFeeds(db, userID)` is called
  next to `services.RevokeAllAPITokens` at the two compromise-response sites:
  - the recovery-path password reset (`controllers/user_controller.go`);
  - the admin password reset (`controllers/admin_user_controller.go`).

  A feed URL grants read access to the same data a leaked API token would, so a suspected compromise
  ends both. The self-service `POST /api-tokens/revoke-all` stays API-token-only; feeds get their own
  `POST /feeds/revoke-all` (decision 8), and self-service `ChangePassword` leaves both alone, for the
  reason its existing comment gives for API tokens.
- **`DeleteContact` revokes that contact's `contact` feeds** by setting `revoked_at` (backend trap 6).
  Undoing the contact deletion does **not** reinstate them. Deleting a contact is a signal to stop
  exporting that person, so a credential is never re-armed implicitly.
- **`DeleteUser` hard-deletes every `Feed` row** in `controllers/user_delete_cascade.go`, next to the
  `ApiToken` row delete.
- **Audit:** create, rotate and revoke each record an audit event under a new `AuditEntityFeed`, one per
  affected row, mirroring the T18 API-token events.

### 7. Serving endpoint: `GET /api/v1/feeds/atom?token=<plaintext>`

- **The endpoint is under `/api/v1`, registered on the unauthenticated `v1` group, not `protected`.**
  The token is the only credential. Placing it under `/api/v1` has three effects:
  - Both nginx configs (`docker/nginx.conf`, `frontend/nginx.conf`) and every operator's reverse proxy
    already forward `/api/`, so no proxy change is needed.
  - The URL falls under MAINT-02's breaking-change policy. That matters because a changed feed URL
    silently kills every reader subscription.
  - The endpoint is documented in `openapi.yaml` with an `application/atom+xml` response.
- **The token goes in the query string, not the path,** because `LoggingMiddleware` already redacts
  query values (`logger.RedactQueryValues`, allowlist-based) but logs the path verbatim. A `token`
  query value is therefore redacted from app logs with no new code. Ticket 2 adds a test that pins
  this. Proxy access logs sit outside the app's control; the operator docs say so.
- **Every miss is the same 404 with an empty body:** unknown token, revoked token, a feed whose contact
  is soft-deleted, or a feed whose owner is a soft-deleted user. The response never distinguishes the cases.
- **Response headers:**
  - `Content-Type: application/atom+xml; charset=utf-8`.
  - `ETag`: a strong SHA-256 of the rendered body. This is correct by construction because decision 2
    makes rendering deterministic.
  - `X-Robots-Tag: noindex, nofollow, noarchive`: hosted readers that expose feeds to search engines
    are told not to.
  - `Cache-Control: no-store`, which `SecurityHeadersMiddleware` sets for `/api/` and which stays
    unchanged (issue #872's reasoning holds).
  - Readers track `ETag` themselves regardless of `no-store`, so `If-None-Match` is honored with a 304.
- **Rate limit:** a new `FeedRateLimitMiddleware` on its own `IPRateLimiter`, `rate.Every(2*time.Second)`
  with a burst of 60. It is separate from `apiLimiter`, so a local reader polling 50 feeds at once
  cannot starve the web UI on the same IP, and the reverse. The issue asked for "the existing API rate
  limiter". This is the existing limiter type and middleware (`RateLimitMiddleware`), with its own
  bucket, the way CardDAV has one.
- **`last_accessed_at` is updated asynchronously, at most once an hour per feed.** Readers poll often,
  and a write per poll would be pointless write load on SQLite.
- The endpoint is read-only: no idempotency, no `If-Match`.
- It joins the six-persona authorization matrix as a `public` class, and the credentials matrix gains a
  `feed-token` column:
  - a feed token is 401 on every REST route;
  - a JWT, API token or CardDAV credential is 404 on the feed route.

### 8. Management API and web surface

- **REST routes** go on the `protected` group, following `circle_controller.go`'s idiom:
  - `GET /feeds` lists active feeds (no token, no hash; `last_accessed_at` included).
  - `POST /feeds` creates a feed with `{ name, kind, entity_id?, detail? }` and returns the feed plus
    the one-time `url`.
  - `POST /feeds/:id/rotate` rotates a feed and returns the new one-time `url`.
  - `DELETE /feeds/:id` revokes a feed.
  - `POST /feeds/revoke-all` revokes every active feed.
- Every route is scoped by `user_id`, and `entity_id` must be a live contact of the caller (404
  otherwise).
- **The `url` field is built from `FRONTEND_URL`.** When that is `*`, the response returns the path and
  query only, and the UI prefixes `window.location.origin`.
- **Web:** a "Feeds" section in settings, next to API tokens, with the list, create, rotate, revoke and
  revoke-all actions. The contact page gets a "Subscribe via feed" action that opens the create dialog
  pre-filled with `kind: contact`. The create dialog:
  - defaults to `headlines`;
  - states that a feed reader keeps its own copy of everything the feed contains, beyond this app's
    reach once fetched;
  - recommends a self-hosted or local reader;
  - shows an extra warning before `full` is chosen.
- **Android:** no management UI in v1. Feeds are a desktop-reader feature and web covers creation, the
  same web-first split ADR 0027 decision 5a took.

### 9. Consumption (B), when it is picked up: the constraints already decided

This section files no ticket. Whoever picks B up starts from these constraints, not from scratch:

- **Storage:** consumed entries land as `ExternalActivity` rows with `source_system = "feed"` and
  `external_id` set to the entry's Atom `id` or RSS `guid`, falling back to its link. The existing
  `(source_system, external_id)` identity, the timeline's `external_activity` type and the provenance
  and sync-state fields already fit. No new timeline type is needed. T66 decision 1 anticipated
  "a second external-activity source".
- **Subscription:** a `ContactFeedSubscription` per contact holds the URL, the last fetch, the
  `ETag`/`Last-Modified` pair and a failure count, following `ContactSubscription`/`CalendarSubscription`,
  including `SyncHealthFields`.
- **Transport:** fetches go through the guarded SSRF dialer (`httputil.SafeDialContext`), with bounded
  redirects, a response cap (1 MiB) and a timeout declared in `integrations.Registry()`. That is a new
  INT-01 matrix row, a `faults.Hook` seam, a failure-behavior test and a `docs/integration-ownership.md`
  section.
- **Parsing:** use a parser that never resolves external entities or DTDs. Go's `encoding/xml` does
  neither, which is why it is acceptable. Input is capped before parsing, entry count is bounded per
  fetch, and stored text is stripped to plain text, never kept as HTML.
- **Scheduling:** a scheduled job under ADR 0011's catch-up semantics, with a poll floor of 1 hour.
- **Records:** a `data-retention-lifecycle.md` row for the ingested copies and a `pii-inventory.md`
  entry.

## Implementation (v1.3.0 milestone)

1. **Backend: `Feed` entity and management API.** The migration, model and `schema_parity` registry
   entry; the REST routes in decision 8; token minting and hashing; the revocation sites and cascades
   in decision 6; audit events; openapi, regenerated `gencontract`/`gentsapi`/`genapibaseline`
   artifacts, and authorization-matrix rows.
2. **Backend: Atom serving endpoint.** The composer move into `services` and its aggregate mode
   (decision 3); the Atom renderer (decisions 2 and 4); the endpoint, headers, conditional GET and
   rate limiter (decision 7); the sensitivity completeness test (decision 5); credentials-matrix rows;
   PERF-02 registry and `budgets.json` entries; i18n keys; and the security-doc updates (ASVS rows,
   threat model, data-retention lifecycle, PII inventory).
3. **Web: feed management UI.** The settings section and contact-page action (decision 8), in all five
   locales.

## Consequences

- One new table (`feeds`) and one new unauthenticated route whose only credential is a URL secret. The
  security docs gain the corresponding rows. A leaked feed URL exposes, until revoked, only what its
  `kind` and `detail` cover. It never exposes the REST API.
- The timeline composer moves to `services` and gains an optional contact filter. This is a refactor of
  a working, well-tested path, guarded by its existing tests.
- Anything a reader fetched stays in that reader after revocation, after contact deletion and after
  account deletion. `data-retention-lifecycle.md` gets a feeds section stating this, alongside its §7
  (CardDAV/CalDAV, this app as the server). The `headlines` default is the main mitigation.
- A future `Sensitivity` column on any timeline table breaks the feed's CI until the feed filters it.
  That is intended.
- No feed library, no consumption path, no outbound client and no ActivityPub surface. Each of those,
  if ever wanted, is a new decision on top of this one.
