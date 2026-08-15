# M14 — Network graph on Android

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 3 — real design work, not a port; T10's graph on a 6" screen is a genuinely different problem |
| **Size** | M — smaller than it looked once the design settled on `/graph/connections` |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing — T10's graph traversal backend already exists and serves web today |
| **Status** | **DONE 2026-08-15** — see the landing note. |

`MycorrhizalApp.kt:498` wraps the `"network"` drawer route in `PlaceholderScreen`; no graph
implementation exists anywhere in `android/`. M8's own ticket flagged this as "the clearest case" of
a web surface that shouldn't be a straight port — a force-directed graph with drag/zoom/pan doesn't
translate to touch well, and the sign-off target still calls for it to be built (no exclusion was
recorded for this one), so the deliverable is a **mobile-appropriate design**, not a pixel port of
`NetworkGraph.tsx`.

## The design — decided 2026-08-11

**Ego-centric and list-first, over `/graph/connections`. Not a force-graph.**

### The finding that decides it

There are **two** graph endpoints, and web only leans on the first:

- `GET /graph` — the whole graph as nodes + edges. This is what `NetworkGraph.tsx` renders, and it
  is the one that needs a layout engine.
- `GET /graph/connections?from=<uid>&depth=&relation=` — T10's traversal. From one contact it
  returns every reachable contact within *depth* hops, each with its **chain of relation steps,
  names already resolved, and the inverse relation already applied** when a hop walked against the
  edge's stored direction (`frontend/src/api/graph.ts:18-60`).

The second endpoint returns, essentially, a pre-rendered readable path per contact. That is a list.
It needs no layout engine, no canvas, no gesture arbitration — and the hard part (walking the graph
and resolving names and inverse relations) is already done server-side.

### Why not the force-graph

1. **Gesture conflict.** Pan/zoom on a canvas fights the navigation drawer's edge swipe and the
   parent scroll container. Every one of those is resolvable, and each resolution is a papercut.
2. **Accessibility.** A force-directed canvas is opaque to TalkBack — nodes are drawn, not
   traversable. [M5](84-M5-android-polish-and-hardening.md) already carries a deferred accessibility
   audit; adding the single least accessible surface in the app right before that audit is choosing
   to fail it.
3. **New dependency.** No Compose graph-layout library is vendored today, and Android has no
   equivalent of the web's `react-force-graph`.
4. **Web itself already reaches for ego-centric focus.** `NetworkPage.tsx:50` keeps a
   `centeredNodeId`, and the contact detail page has a `ConnectionsPanel` — the whole-graph view is
   the *exploration* surface, not the everyday one. On a phone, only the everyday one earns space.

### The design

**Entry points.** Two, and the second matters more:

- The drawer's `network` route (`MycorrhizalApp.kt:498`, currently `PlaceholderScreen`) opens on a
  "start from…" contact picker, defaulting to the Self contact when one is set.
- **"Explore connections" from a contact's own detail screen** — the higher-value entry, and the one
  that matches how the feature is actually reached. Coordinate with
  [M21](103-M21-android-relationships-depth.md), which is doing that screen's relationship section.

**Primary surface: a depth-grouped list.** Section headers per hop ("Direct", "2 hops away",
"3 hops away"); each row is the contact's name plus the chain rendered as a readable path — *"Bob →
sister of → Alice"* collapses to "Alice's sister". Tapping a row opens that contact's detail screen.
Every row is a real, focusable list item, so TalkBack reads it for free.

**Controls:** a depth stepper (1–3, defaulting to 2), a relation filter passing the registry token
or synonym straight through to the endpoint's `relation` param, and the circle filter web has.

**Deliberately excluded from v1 — recorded explicitly**, since M8's sign-off rule is that exclusions
must be decided rather than inferred:

- **Node dragging, hover tooltips, pan/zoom.** Replaced by the list, not ported.
- **Activity nodes.** Web's `/graph` mixes activity nodes in with contacts;
  `/graph/connections` is contacts-only. On a phone, "what happened with this person" is already
  answered far better by the contact timeline, and adding activity nodes would mean falling back to
  `/graph` and re-inheriting the whole layout problem. This is the one real parity gap the design
  accepts.

**Deferred, not rejected: a radial visualization** as a second tab once the list works. If a visual
is wanted, radial-from-ego is the shape to build — it is bounded (one center, one or two rings),
needs no force simulation, and can be laid out with trigonometry in a `Canvas`. Do not start here.

### The one thing to revisit if this proves wrong

If the list turns out to feel like a worse relationships screen rather than a distinct *network*
feature, that is the signal that the whole-graph view was carrying real value and the radial tab
should be pulled forward — not that the force-graph should be ported.

## Done when

- The `network` route is no longer a `PlaceholderScreen`.
- From a chosen starting contact, connections are listed grouped by hop depth, each showing a
  readable relation chain, each tappable through to that contact.
- The depth stepper, relation filter, and circle filter all work.
- "Explore connections" is reachable from a contact's detail screen.
- Every row is reachable and announced under TalkBack.
- The activity-node exclusion above is restated in the landing note, so the parity matrix records it
  as a decision rather than an oversight.
- Hand-verified on-device against a contact with a non-trivial relationship graph.
- New strings translated in all five locales.

---

## Implementation contract (added 2026-08-12)

### Endpoints

| Need | Route | In `ApiClient`? |
|---|---|---|
| Ego-centric traversal | `GET /graph/connections?from=&depth=&relation=` | **No.** Add `getConnections`. |
| Whole graph | `GET /graph` | **No** — and deliberately not needed; see the design above. |

One new client method. `from` is a `Contact.VCardUID`, not a numeric id.

### Response shape (already resolved server-side)

`{from_vcard_uid, from_name, depth, chains: [{target_id, target_vcard_uid, target_name, depth,
steps: [{contact_id, contact_vcard_uid, contact_name, relation}]}]}`.

`target_name` and each step's `contact_name` are **already resolved**, and `relation` already has the
inverse applied where a hop walked against the edge's stored direction. Do not re-resolve names or
re-derive inverses on-device — that is the work this endpoint exists to have already done.

### Test cases

1. **Grouping by depth** — a response mixing 1-hop and 2-hop chains renders under the right headings.
2. **Empty graph** — a contact with no relationships renders an empty state, with the `chains` key
   **absent** from the JSON as well as `[]` (`/CLAUDE.md` frontend trap #8).
3. **Filters** — `depth` and `relation` go on the query string; a registry synonym ("brother") is
   passed through rather than being resolved on-device.
4. **Navigation** — tapping a row opens that contact's detail screen by UID.
5. **Accessibility** — every row exposes a content description; the list is traversable under
   TalkBack. This is a stated reason for choosing a list over a canvas, so it needs a test rather
   than an intention.

### Gate

- `./gradlew testDebugUnitTest`, `./gradlew lintDebug`, `./gradlew assembleDebug` — the exact three
  steps `.github/workflows/android-tests.yml` runs. CI has been green since M1's review pass; keep it.
- Every new user-facing string in all five locales (`values`, `values-de/es/fr/it`). M1's review pass
  had to retrofit ~80 unlocalized strings — don't rebuild that debt.

### Test conventions (this repo, not generic)

JUnit4 + MockK (`mockk`/`coEvery`) + Turbine + `runTest` with `MainDispatcherRule`. ViewModel tests
mock the repository — `feature/contacts/.../ContactListViewModelTest.kt` is the reference. New
`ApiClient` methods get a MockWebServer test in `core/network` — `ApiClientTest.kt` is the reference.
Hand-verify per `/CLAUDE.md`: break the code, confirm the new test fails, restore.

---

## Landing note (2026-08-15)

Implemented per the design: an ego-centric, list-first network screen over
`GET /graph/connections` — no layout engine, no canvas.

**What landed** (new `:feature:network` module + `GraphRepository`):
- `ApiClient.getConnections(from, depth, relation)` — `from` is a
  Contact.VCardUID; depth/relation are optional query params; a registry
  synonym like `"brother"` is passed through verbatim, never resolved
  on-device.
- `GraphConnectionsResponse`/`GraphChain`/`GraphChainStep` models with
  nullable lists + `chainsOrEmpty`/`stepsOrEmpty` accessors (trap #8 — Moshi
  rejects an explicit JSON `null` for a non-nullable list; raw-JSON
  MockWebServer tests pin both the absent-key and explicit-null cases).
- `NetworkScreen`/`NetworkViewModel`: a "start from" picker (search-based,
  defaulting to the self contact via the `self_contact_vcard_uid` the backend
  now returns on `/users/me`), a depth stepper (1–3, default 2 — the design's
  mobile range), a verbatim relation filter, a client-side circle filter
  (the endpoint has no circle param; web filters the whole graph client-side
  too), and chains grouped under "Direct" / "N hops away" headers with each
  row rendering the readable path (`Sister (sibling of) → Bob (spouse of)`).
- Every row merges its descendants into a single semantics node with a full
  content description — the TalkBack-traversable guarantee that justified
  choosing a list over a canvas; rows whose `target_id` is 0 (a soft-deleted
  intermediate the server degrades) render identically but aren't tappable,
  and blank names fall back to the backend's own "Unknown" convention.
- Entry points: the drawer's `network` route (no longer a
  `PlaceholderScreen` — that composable and its `coming_soon` string were
  deleted) and an "Explore connections" item on the contact detail's ⋮ menu.

**Deliberate exclusions restated for the parity matrix:** activity nodes are
NOT in the v1 list (the contact timeline answers "what happened with this
person" better on a phone, and adding them would force a fall back to
`/graph` and the whole layout problem). The radial visualization is deferred,
not rejected. Pan/zoom/hover are replaced by the list, not ported.

**Review pass (2026-08-15):** an independent review found four real defects,
all fixed with regression tests — a dead error path (a starting contact with
no VCardUID set `errorRes` the screen never read), an out-of-order traversal
race (an uncancelled coroutine per call meant the last-to-complete request
won; a stored `connectionsJob` is now cancelled before every relaunch and a
delay-based test pins it), ghost rows rendering blank names (fixed via the
"Unknown" fallback), and stale chains surviving a failed depth/relation
change (chains are cleared on error).

**Test coverage:** `ApiClientTest` (query building, parse, absent/null chains,
404), `GraphRepositoryImplTest` (pass-through + the circle→member map),
`NetworkViewModelTest` (13 cases: grouping, self-contact default, empty graph,
relation pass-through/clear, circle filtering, no-uid error, search exclusion,
from-selection, circle-failure isolation, the race, stale-clear), and
`NetworkScreenTest` (Robolectric: depth grouping, empty state, the
content-description + click-action accessibility assertions, tap-through,
depth chip, relation apply, circle select, missing-from prompt, picker
search/select/empty). Hand-verified per `/CLAUDE.md` (the null-chains and
accessibility tests were each confirmed to fail against a reintroduced bug).

**Gate:** `testDebugUnitTest`/`lintDebug`/`assembleDebug` all green.

**On-device verification: outstanding** — no device in the build environment
(same gap as M11/M17/M19/M20). The traversal endpoint, name resolution and
inverse application are all server-side and already tested there; what remains
is a visual pass on the Pixel 8a.
