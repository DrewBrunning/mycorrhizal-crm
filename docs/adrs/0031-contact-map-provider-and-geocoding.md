# ADR 0031: Contact Map — provider-agnostic engine, tile-source configuration, and in-scope geocoding

- **Status:** accepted — shipped in v1.4.0 (backend #694/PR #1424, the `GET /contacts/map` bulk read #1427/PR #1428, web #1286/PR #1426, Android #1287/PR #1429, plus the address-sensitivity and `geo:` fixes #1433/#1434).
- **Date:** 2026-09-28 (proposed); 2026-10-04 (accepted)
- **Implements:** issue #694 ("Contact Map — map contacts' addresses (web + Android,
  provider-agnostic)"). This ADR settles #694's open questions; #694 itself becomes the backend
  track, with new sibling issues for the web and Android tracks.
- **Depends on:** none.
- **Related:** `docs/security/asvs-l2.md` (sensitivity/SSRF sections this ADR adds rows to),
  `docs/security/data-retention-lifecycle.md` (new external-data-flow subsection this ADR adds),
  `docs/development/web-perf-budgets.md` (the bundle-budget process the web track must follow).

## Context

The neutral contact model already carries a coordinate: `contactmodel.Address.Coordinates`
(`backend/contactmodel/model.go:156`) is a `geo:` URI (vCard GEO) and round-trips through all three
exporters. A contact imported via CardDAV/vCard/JSContact that already carries a GEO value is
plottable today with zero backend work. But coordinates cannot be *entered*: the flat editor DTO
`ContactAddress` (`backend/models/contact.go:66-79`) has no coordinate field, and
`frontend/src/components/AddressFields.tsx` never surfaces one. No map library exists on web (only
`d3-force`/`react-force-graph-2d` for `NetworkGraph.tsx`) or on Android
(`android/gradle/libs.versions.toml`), and no geocoding code exists anywhere.

This is a self-hosted, privacy-forward CRM. A map is the first feature that sends contact geography
to a third party in two distinct flows — tile requests (viewport bbox, low sensitivity alone but
revealing in aggregate) and geocoding (the address text itself leaves the machine, the sensitive
flow). The milestone decision (2026-09-28) is that geocoding is in scope for v1, not deferred — which
means this ADR also has to settle how a postal address's sensitivity is represented and enforced,
since `Address` has no such field today (only `Contexts`, which describes what an address is *for*,
not how sensitive it is).

## Decision

### 1. Rendering engine and tile source

MapLibre GL JS (web) + `maplibre-android` (Android) — one open-source renderer on both platforms, no
Google dependency, decoupling "how we draw" from "whose tiles we draw."

Default tile source: **OpenFreeMap** (free hosted vector tiles, no API key). Configurable via a new
**instance-level** env var, `MAP_TILE_STYLE_URL` (a MapLibre style JSON URL), read in
`backend/config/config.go` alongside the existing SSRF-policy flags (e.g. `*_BLOCK_PRIVATE_URLS`,
`config.go:65-83,136-147`). This is a deliberate departure from the per-user `ImmichConfig`-style DB
model the other optional integrations use — see "Alternatives considered."

### 2. Geocoding — provider, default-off, and the sensitivity gate

In scope for v1, default **off**. Two new instance-level env vars: `GEOCODER_PROVIDER`
(`none|nominatim|maptiler`, default `none`) and `GEOCODER_API_KEY` (required when `maptiler` is
selected; ignored for `nominatim`, which needs no key but is rate-limited to 1 req/s per its usage
policy — the backend must self-throttle to that ceiling when `nominatim` is selected). All outbound
geocoder requests go through the existing SSRF-guarded dialer (`backend/httputil/safedial.go`,
`fetch.go`) — the same choke point Immich/Paperless/Seafile calls already use.

`Address` gets a new `Sensitivity` field (`normal|private|secret`, default `normal`), mirroring
`RelationshipEdge.Sensitivity` / `OccasionObligation.Sensitivity` / `Preference.Sensitivity` /
`FieldValue.Sensitivity`. Because the neutral `contactmodel.Address` is synthesized from the flat
`ContactAddress` table via `ApplyRecordToContact`/`RecordForContact` (CLAUDE.md backend traps #2/#3),
the new column is added to `ContactAddress` (`backend/models/contact.go:66-79`) by migration, and
must be threaded through those existing apply/record functions — **never** by mutating `Card`/`CRM`
fields directly, which is exactly the class of bug traps #2/#3 document as having shipped broken
before.

A new endpoint (e.g. `POST /contacts/:id/addresses/:addressId/geocode`) triggers a single, explicit,
per-address geocode lookup. It refuses (400, same shape as the existing sensitivity checks) for any
address with `Sensitivity != normal` unless the request carries `include_sensitive=true` — the same
opt-in already used for exports/shares. Geocoding is **never** triggered automatically or in bulk;
only this one explicit per-address action calls out. A small cache (normalized address text +
provider → coordinate, with a sane TTL) bounds repeat calls for the same address.

### 3. The map view itself is not sensitivity-gated

Sensitivity governs copies that leave the instance or reach another party (exports, sync, shares) —
it is not an access-control tier against the owning user (CLAUDE.md, "Sensitivity" section). The map
view is the owner looking at their own data, same as the contact detail page: it plots every contact
with a coordinate, `private`/`secret` included, with no `include_sensitive` gate. Tile requests never
carry per-marker data — a slippy-map tile request is inherently just a viewport bbox — so there is
nothing to build to keep tile traffic address-free; that property falls out of how XYZ/vector tile
requests work.

### 4. Documentation obligations

- `docs/security/data-retention-lifecycle.md`: a new numbered subsection for the map feature —
  tile requests are stateless/no retention; the geocode cache is **in-memory only** (see the
  2026-10-01 amendment) so it is not a persisted copy, but the subsection still records that
  address text is sent to the configured geocoder and that the cache vanishes on restart, in the
  doc's existing Where/who–Retention–Deletion/propagation–Backups–Verification shape (see its §13
  for the pattern).
- `docs/security/asvs-l2.md`: a new row under the SSRF section (5.2.6, alongside the existing
  Immich/Seafile per-service opt-in guard rows) covering the geocoder as a new outbound integration.
- `docs/int-01-integration-classification-matrix.md`: a `Registry()` entry for the geocoder client
  (`backend/integrations`), since `TestEveryOutboundClientIsClassified` fails otherwise.

### 5. Web and Android tracks (owned by the sibling issues, recorded here for continuity)

- **Web**: this repo has no existing lazy-loaded route (`grep -r "React.lazy" frontend/src` returns
  zero hits today) — the map view is the first one. MapLibre GL JS gets its own `map-vendor` entry in
  `vite.config.ts`'s `VENDOR_CHUNKS` (alongside the existing `graph-vendor` entry for
  `react-force-graph-2d`/`d3-force`), budgeted via `yarn build && yarn budget:update` per
  `docs/development/web-perf-budgets.md` — the diff is the review. `AddressFields.tsx` gains a
  coordinate input (manual entry) and a "find coordinates" geocode-trigger action, gated on
  `Address.Sensitivity` client-side to match the backend's 400 behavior. All five locale files need
  real translations (frontend trap #5).
- **Android**: a new `android/feature/map` module following the existing feature-module template
  (`mycorrhizal.android.library` + `mycorrhizal.android.hilt` + Compose plugin, depending on
  `:core:data`/`:core:domain`/`:core:ui`, per e.g. `android/feature/network/build.gradle.kts`), with
  `maplibre-android` added to `android/gradle/libs.versions.toml`.

## Amendment, 2026-10-01 (maintainer decisions from the pre-implementation review of #694)

The post-ADR review of #694/#1286/#1287 found the original text under-specified in five places.
Settled:

- **Stable address identifier.** `ContactAddress` gains an `ID` (migration with backfill for existing
  rows — real data exists, CLAUDE.md "Orientation"), threaded through `AddressFromContactAddress` /
  `contactAddressFromNeutral` into `contactmodel.Address.ID` and added to the schema-parity registry.
  `:addressId` in the geocode route is this ID. Array index was rejected: it shifts on reorder/delete.
- **`Coordinates` column shape.** A `geo:` URI string, matching `contactmodel.Address.Coordinates`
  exactly (no lossy lat/lng conversion on the exporter path); the editor validates lat/lng ranges.
- **Geocode cache is in-memory** (bounded size + TTL, lost on restart). No table, no migration, no new
  persisted copy of address text. Supersedes the "retained copy" wording in §4 above and in #694.
- **Route.** `POST /contacts/:id/addresses/:addressId/geocode` is final (the "e.g." is dropped).
- **Tile-style delivery.** A new unauthenticated bootstrap endpoint, `GET /api/v1/config/map`,
  mirroring `GET /api/v1/auth/oidc/config` (`routes/routes.go`), returns only
  `{"tile_style_url": "<MAP_TILE_STYLE_URL or the OpenFreeMap default>"}`. It is public because the
  value is not secret (the client fetches tiles from it directly) and it must be readable before the
  map view needs a session-scoped call. It stays deliberately narrow — no other instance setting is
  added to it — and needs a `classPublic` row in `routes/authorization_matrix_test.go`, a
  `routes/embedded_mode_test.go` entry if the embedded-mode route list requires it, and an OpenAPI
  schema + example (which regenerates the contract fixtures and TS types). The web (#1286) and
  Android (#1287) tracks both read the style from this endpoint.

## Amendment, 2026-10-05 (stateless draft geocode)

The v1.4.0 web editor (issue #1286) shipped with "Find coordinates" disabled for any address without a
saved `id`, so a newly typed address had to be saved (and the contact re-opened) before it could be
geocoded — and an existing address had to be saved before an edited street/city could be looked up at
all, because the endpoint geocodes the persisted row. It was also a write: pressing the button saved
coordinates onto the stored address, so **Discard** did not revert a lookup made mid-edit.

Settled: an additive, **stateless** route — `POST /api/v1/contacts/:id/addresses/geocode`
(`GeocodeContactAddressDraft`, `backend/controllers/contact_address_controller.go`). It reads the same
postal fields (plus the address's `sensitivity`) from the request body, applies the same sensitivity
gate (400 unless `include_sensitive=true` for `private`/`secret`), enforces the same contact-ownership
scoping, and returns `{coordinates, cached}` **without persisting anything**. The editor holds the
coordinate in its draft; the ordinary contact save persists it, so Discard correctly reverts it. The
persisted per-address route is unchanged and remains the endpoint for API clients. Both are still one
explicit lookup per action, never automatic, never bulk, and the contact id remains the ownership
anchor, so neither is a general geocoding proxy. The frontend drops the saved-id requirement from the
button's disabled state (the sensitivity gate stays).

## Amendment, 2026-10-05 (SPA CSP allows the configured tile origin)

§1 chose to fetch tiles straight from `MAP_TILE_STYLE_URL`'s host, but the shipped SPA
Content-Security-Policy pinned `connect-src 'self'` (and named no `worker-src`), so in a real
deployment the browser refused both the style/tile fetches and MapLibre's `blob:` worker — pins
rendered on a blank canvas. Fixed by deriving the CSP's tile origin from the same env var at container
start: `docker/entrypoint.sh` renders `$csp_tile_origin` (host only) into the CSP via an
`hsts.conf`-style include, and the policy adds `worker-src blob:`. `connect-src`/`img-src` therefore
allow exactly `'self'`, `data:`, `blob:` and the one configured origin — no blanket `https:`. The
split `frontend/nginx.conf` image (no entrypoint) carries the OpenFreeMap default statically.

## Consequences

- A self-hosted operator who wants OpenFreeMap needs to set nothing; anyone wanting Google-free tiles
  from a different source, or self-hosted PMTiles later, changes one env var.
- Geocoding stays fully opt-in at the instance level (`GEOCODER_PROVIDER=none` by default) and at the
  per-address level (explicit trigger, sensitivity-gated) — no address text leaves the instance
  unless an operator has turned geocoding on *and* a user has explicitly asked for that specific
  address.
- `Address.Sensitivity` is new surface area or future features to reuse (e.g. sensitivity-aware
  address sharing) — a reasonable one-time cost, mirroring an already-established pattern on four
  other entities.
- The web bundle grows by MapLibre GL JS's real weight; the lazy-route precedent this establishes
  will likely get reused by future heavy views.

## Alternatives considered

- **Per-user `MapConfig`/`GeocoderConfig` DB model**, matching `ImmichConfig`/`PaperlessConfig`
  exactly. Rejected for the tile-source half: unlike Immich/Paperless, a tile style isn't a personal
  external account — every user on a household instance looking at the same tiles is the normal case,
  and per-user tile-style rows would just duplicate the same value across every row for no benefit.
  For the geocoder half this is closer to a real judgment call (an API key is a credential), but a
  self-hosted instance typically has one operator managing one geocoding budget/key for the whole
  household, so instance-level env vars were chosen for consistency with the tile-source decision
  rather than splitting the two into different config shapes. Revisit if a genuine "different geocoder
  per user" need appears.
- **Deferring geocoding past v1** ("plot what already has coordinates + manual pin entry only").
  This was the smaller, safer default recommendation; the milestone scoping decision (2026-09-28) was
  to include geocoding now rather than defer it.
- **Self-hosted Nominatim/Photon as the only geocoder option.** Rejected as the *only* option (though
  `nominatim` — the public instance, rate-limited — is the no-key default choice) because self-hosting
  a geocoder is a heavy operational ask most single-operator instances won't take on; `maptiler` is
  offered as the batteries-included alternative for anyone who wants faster/unlimited geocoding and is
  willing to hold a key.

## Implementation

Filed as three issues, all milestone v1.4.0 (#28):

- #694 (rewritten) — backend: migration (`Sensitivity` + `Coordinates` on `ContactAddress`), geocoder
  config + SSRF-guarded client + cache, the geocode-trigger endpoint, `Registry()` entry, the two doc
  updates above.
- A new "Contact Map — Web" issue, depending on #694.
- A new "Contact Map — Android" issue, depending on #694.

Out of scope, unchanged from the original ticket: routing/directions, bulk/batch geocoding of the
whole address book, full offline maps on device.

**Shipped in v1.4.0.** All three tracks landed: the backend migration (`Sensitivity` + `Coordinates`
on `ContactAddress`), the SSRF-guarded geocoder, the geocode-trigger endpoint and
`GET /api/v1/config/map` (#694 / PR #1424); the bulk `GET /api/v1/contacts/map` read
(#1427 / PR #1428); the web map (#1286 / PR #1426) and Android map (#1287 / PR #1429). The
implementation review found and fixed two follow-ups: an above-`normal` address (and its `geo:`
coordinate) was not excluded from the neutral-`Card` outward copies (#1433), and the CardDAV card
carried the coordinate as a `GEO=` parameter that go-webdav re-encoded corruptly (#1434).
