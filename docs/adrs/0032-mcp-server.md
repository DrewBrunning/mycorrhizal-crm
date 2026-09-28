# ADR 0032: MCP server — streamable-HTTP, read-only high-level tools, opt-in sensitivity

- **Status:** proposed
- **Date:** 2026-09-28
- **Implements:** issue #176 ("MCP server: expose the CRM over Model Context Protocol"). #176's own
  "Done when" states this design pass, resolved and written down, as its v1.4.0 deliverable — this
  ADR is that design pass.
- **Depends on:** none.
- **Related:** `docs/security/asvs-l2.md` (new client/access-surface row this ADR requires).

## Context

`api_token_controller.go` already issues per-user API tokens; `backend/middleware/auth.go` already
validates them (`AuthMiddleware`, `LookupAPIToken` at line 218) and scopes every request to
`userID`. `routes.go` already exposes a fully `user_id`-scoped REST API. The idea behind #176 is
that an MCP server is a thin adapter over what already exists, not new logic — the design pass only
needs to settle *how thin*, and where sensitivity fits.

Research into the codebase found:
- No MCP dependency exists yet (`go.mod`/`go.sum` clean).
- There is no single reusable "sensitivity query-scope" function. Each domain threads its own
  `includeSensitive bool` through its own query/projection: `projectRelationshipEdges`,
  `projectPreferences`, `projectCustomFields` (`backend/models/contact_record.go:140,268,326`), and
  `GetUpcomingOccasions`/`GetGiftShoppingList` (`backend/services/occasion_service.go:93,241`). An MCP
  tool handler therefore has to call the correct per-domain function for each data type it exposes,
  not one shared filter.
- A representative REST surface to draw v1 tools from: contacts (`GET/POST /contacts`,
  `/contacts/:id`, `/contacts/:id/detail`, `/contacts/:id/briefing`, `/contacts/:id/timeline`,
  `/contacts/:id/score`), activities (`GET /contacts/:id/activities`, `GET/POST /activities`),
  occasions/cadence (`/occasion-obligations`, `/occasion-obligations/card-list`,
  `/occasion-obligations/gift-shopping-list`, `/occasion-events`, `/cadence-policies/overdue`),
  search (`GET /search`).

## Decision

### 1. Transport

**Streamable-HTTP only**, mounted on the existing Gin HTTPS server (e.g. `POST /mcp`). No stdio for
v1 — the actual use case is a self-hosted instance reached by a remote assistant, the same shape
Dawarich/GeoPulse are themselves commonly deployed in (Docker + reverse proxy). stdio would only
serve a same-host CLI agent, which isn't the target user here; add it later only if a concrete need
appears.

### 2. Auth — no new surface

MCP requests authenticate with the **same** per-user API tokens the REST API already issues, via the
**same** `AuthMiddleware`/`LookupAPIToken` path. MCP tool handlers run as the token's user, exactly
like the REST calls they wrap. No new token type, no new credential, no new IDOR surface — ownership
scoping carries over for free because it's the same middleware.

### 3. Tool set — v1, read-only

Four tools, matching #176's own originally-stated "concrete first cut":

- `search_contacts`
- `get_contact`
- `list_timeline`
- `run_cadence_report`

Each tool handler calls the **exact** service/controller function its matching REST endpoint already
uses — never a hand-rolled query. This is the mechanism that keeps sensitivity filtering and
ownership scoping from drifting between the REST and MCP surfaces: if `contact_record.go`'s
projection functions change, both surfaces change together automatically.

Writes are explicitly **out of scope for v1**, deferred to a follow-up ticket once the read surface
has run in practice — this keeps the blast radius of a leaked or misused AI-held token small, per
#176's own stated rationale.

### 4. Sensitivity — opt-in, mirroring the REST API

Per the milestone scoping decision (2026-09-28): MCP tools **may** request `include_sensitive=true`,
the same opt-in the REST API and CSV export already expose to the same token. Each of the four tool
schemas gets an optional `include_sensitive` boolean parameter, threaded straight into the
`includeSensitive` parameter the underlying service/projection function already takes. Default
`false` — an assistant has to explicitly ask for sensitive data, same as any other API client using
that token would.

### 5. Dependency

An MCP Go SDK is required to implement the server. This ADR deliberately does not pin a specific
package: the MCP tooling ecosystem moves fast enough that the implementer should confirm the current
canonical Go SDK at build time rather than trust a name written down now.

## Consequences

- An external AI assistant (Claude, Cursor, etc.) configured with a user's own API token gets
  read-only, sensitivity-respecting access to search/contact/timeline/cadence data with zero new
  backend logic beyond the MCP protocol adapter itself.
- Because tool handlers call existing functions rather than new queries, a future change to
  sensitivity filtering or ownership scoping in the REST layer automatically applies to MCP too —
  there's no second copy of that logic to keep in sync.
- Write access is a real, separate follow-up decision, not assumed by this ADR.

## Alternatives considered

- **One MCP tool per REST endpoint.** Rejected for v1: a large flat tool list is worse for an
  assistant's own tool-selection accuracy than a small set of purposeful, high-level tools, and the
  REST surface is large enough that a 1:1 mapping would be unwieldy to keep sensitivity-consistent.
- **stdio transport for local/CLI use.** Rejected for v1 as noted above — no concrete same-host CLI
  use case exists yet; streamable-HTTP alone covers the stated motivating use case.
- **Hard-blocking `include_sensitive` for MCP (never allow it).** Considered and rejected by the
  milestone scoping decision: the token already belongs to the user, and the REST API already trusts
  it with that opt-in — MCP inherits the same trust boundary rather than inventing a stricter one.

## Implementation

Filed as #176 (rewritten with this ADR's decisions), milestone v1.4.0 (#28). Backend-only; no web or
Android component. Implementation must add a new `docs/security/asvs-l2.md` row for the new inbound
protocol surface per gate #1118's standing criteria.
