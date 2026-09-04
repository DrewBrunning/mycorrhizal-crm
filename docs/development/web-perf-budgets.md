# Web client performance budgets (issue #556)

The web counterpart to the backend PERF suite (#468–#471) and to the Android
macrobenchmark (#263). Nothing in `frontend/` measured client cost before this;
CI ran `tsc --noEmit`, vitest and lint and nothing else.

The stance is the same as #261/#471 for the backend: **deterministic metrics are
hard gates, wall-clock is a trend.** Bundle bytes, graph node/edge ceilings and
request counts fail CI on regression; render timings are recorded and watched, not
asserted on a noisy runner.

Two tiers, matching #447 and the backend `MYCORRHIZAL_LARGE_TESTS` split:

| Tier | What | Where |
|---|---|---|
| **Per-PR** | bundle-size budget; `graphBudget` + `NetworkGraph` fallback unit tests; route-stubbed Playwright specs tagged `@perf` (`networkPerf`, `searchPerf`) | `unit-tests.yml` (`Frontend bundle-size budget` job) + `e2e-tests.yml` |
| **Nightly** | seeded / large-payload Playwright specs tagged `@perf-heavy` (`listPerf`, `networkGraphScale`) | `e2e-tests.yml` schedule, `RUN_PERF_HEAVY=1` |

Run the heavy tier locally with:

```sh
cd frontend && RUN_PERF_HEAVY=1 npx playwright test --grep @perf-heavy
```

---

## 1. Bundle size

`frontend/bundle-budget.json` records the **gzipped** size of every emitted
chunk plus the total, generated from a clean `yarn build` by
`scripts/check-bundle-budget.mjs`. `yarn budget` (run after `yarn build`) fails
when a chunk, or the total, grows past

```
baseline * (1 + tolerancePct) + absoluteSlackBytes
```

(`tolerancePct` 5%, `absoluteSlackBytes` 2 KiB), or when a **new** unlisted chunk
exceeds `newChunkGzipLimit` (50 KiB). The growth is the signal — there is no
absolute ceiling on the app itself, which legitimately ships the whole `@mdi/js`
set (see `vite.config.ts`). Every run prints a per-chunk table (baseline /
current / Δ / status); in CI it is also written to the job summary so a jump is
attributable to a chunk, and from there to a dependency.

The chunk names come from `vite.config.ts`'s `VENDOR_CHUNKS` split
(`react-vendor`, `mui-core`, `mui-icons`, `mdi`, `i18n-vendor`, `graph-vendor`)
plus the `index` entry chunk and `index.css`.

**Updating the baseline** is deliberate and reviewable: after an intentional
dependency change run

```sh
cd frontend && yarn build && yarn budget:update
```

and commit the `bundle-budget.json` diff — the diff *is* the review. The pure
comparison (`compareBudget`) is unit-tested in
`scripts/check-bundle-budget.test.mjs`.

## 2. Network graph render ceiling

`NetworkGraph.tsx` lays the relationship graph out with `react-force-graph-2d` /
`d3-force` on the main thread, running every tick until it settles. The `/graph`
endpoint returns the **entire** graph with no server-side cap, so a dense,
long-lived personal CRM is the one place in the web client that can freeze a
browser tab.

**Defined behaviour past the ceiling** (`src/utils/graphBudget.ts`):

| Constant | Value | Meaning |
|---|--:|---|
| `GRAPH_RENDER_NODE_CEILING` | 2000 | filtered node count above which the canvas is not mounted |
| `GRAPH_RENDER_EDGE_CEILING` | 6000 | filtered edge count, same |

Above either, `NetworkGraph` renders a `warning` notice
(`network.tooLargeToRender`, `data-testid="graph-over-budget"`) carrying the
node/edge counts and pointing at the circle / centre-on-contact filters and the
list view — instead of `<ForceGraph2D>`. `NetworkListView` (rendered
unconditionally by `NetworkPage`, #189) remains the full view. The check runs on
the **filtered** graph, so a filter that brings the set back under the ceiling
restores the canvas with no reload.

The ceilings are chosen so the #468 `typical` profile (900 contacts + a few
dense hubs + a depth-12 chain → well under 2000 nodes) always renders and `large`
always degrades. `networkGraphScale.spec.ts` (`@perf-heavy`) drives the synthetic
#468 profiles (`e2e/graphScaleProfiles.ts`, a hand-synced mirror of
`backend/internal/largedata/profiles.go`) through the real component, records the
worst `requestAnimationFrame` gap per size (a main-thread-blocking proxy) while
the canvas is up, and asserts the ceiling behaviour. If that benchmark shows the
real cliff has moved, update the two constants **and this table** together.

Measured (synthetic profiles, one run on a developer laptop — indicative, not a
gate):

| profile | nodes | edges | rendered | worst rAF gap |
|---|--:|--:|---|--:|
| smoke | 150 | 172 | canvas | ~25 ms |
| typical | 900 | 1037 | canvas | ~20 ms |
| typical+ | 1500 | 1836 | canvas | ~40 ms |
| mid | 3000 | 3744 | notice (degraded) | — |
| large | 15000 | 18790 | notice (degraded) | — |

Interaction stays smooth (rAF gap well under one frame) through ~1500 nodes; the
2000/6000 ceiling degrades from `mid` up, before the cliff. Note the fallback
still renders the full `NetworkListView` — at `large` that is 15k list rows, ~3.5 s
to paint, but not a frozen tab.

## 3. Contact list and detail

T17 cursor pagination (`api/contacts.ts`) bounds the API — a page at a time
(`pageSize` 10), opaque resume token, no total. `listPerf.spec.ts`
(`@perf-heavy`) seeds ~150 contacts and pins that the **UI honours it**:

- every `GET /contacts` request asks for a bounded page (`limit` ≤ 100) and only
  the first is cursor-less — *Load More* resumes from the cursor, never refetches
  page one;
- the rendered `contact-card` count grows linearly with pages loaded and every
  row is unique. **There is no list virtualization today** — this is a pinned
  property, not an oversight; a regression to "hold everything and re-render" or
  duplicate-append fails the spec;
- JS heap growth over the loaded rows is logged and sanity-checked (`< 60 MB`) —
  a trend guard for a gross regression such as retaining every page's full
  payload, not a precise budget.

Wall-clock render time for the list and the detail page is logged by the specs
and not asserted (CI-runner variance, per #261).

## 4. Search-as-you-type

`ContactsPage.tsx` debounces the search field 300 ms and gates on two runes.
`searchPerf.spec.ts` (`@perf`) pins that a burst of keystrokes collapses to a
single trailing `GET /contacts?...search=` request and a one-character query
sends none.

`useContacts` carries a request-sequence guard (`requestRef`, mirroring
`useGraph`) so a slow response for an earlier query cannot land after — and
overwrite — a newer one. This is unit-tested in `useContacts.test.ts`
("a stale search response that settles after a newer query does not clobber the
list").

## Out of scope / possible follow-ups

- Server-side `/graph` pagination or a node cap — the ceiling here is a
  client-only guard.
- Moving force layout to a Web Worker — larger and riskier than the fallback;
  not done.
- Contact-list virtualization — this budget documents and pins the current
  linear growth; windowing is a separate change.
