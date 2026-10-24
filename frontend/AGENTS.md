# Frontend — React 19, TypeScript, MUI, vitest, Playwright

Read the root `AGENTS.md` first for the dev loop, safety boundaries, and implementation workflow.
This file is the frontend contract. Android shares the trap IDs below.

## Frontend traps

1. <a id="trap-1"></a>**vitest here has no auto-cleanup and no `globals: true`.** Add
   `afterEach(cleanup)` explicitly in component test files or you get "multiple elements found".

2. <a id="trap-2"></a>**MUI appends `" *"` to a required field's accessible label.**
   `getByLabelText('Name')` fails; `getByLabelText('Name *')` works.

3. <a id="trap-3"></a>**Do not nest a `<Chip>` (renders `<div>`) inside
   `<Typography variant="body2">` (renders `<p>`).** Invalid HTML; React warns. Put both in a
   sibling flex `Box`.

4. <a id="trap-4"></a>**Frontend enum/registry lists are hardcoded mirrors of backend `oneof`
   validators.** There is no dynamic type-list endpoint, by design. Add a backend token → update the
   frontend/Android copies by hand, with a comment saying they must stay in sync.

5. <a id="trap-5"></a>**All five locale files get real translations** (`en`, `de`, `es`, `fr`, `it`),
   not English placeholders. `src/i18n/locales.test.ts` enforces identical key sets both directions,
   identical interpolation placeholders, and no *namespace* byte-identical to English. It cannot
   assert per-key difference — proper nouns and cognates are legitimately identical in bulk.

6. <a id="trap-6"></a>**Translate to a leaf key, never to an object node.**
   `t('contacts.personalInfo.kindOptions')` where `kindOptions` is a parent renders i18next's
   diagnostic (`key '…' returned an object instead of string`) as visible UI text.

7. <a id="trap-7"></a>**Playwright's `e2e/global-setup.ts` hardcodes `http://localhost:7300`** for
   both the app origin and its direct API calls, separately from `playwright.config.ts`'s `baseURL`.
   The e2e suite cannot run against a different port without editing shared test infra.

8. <a id="trap-8"></a>**A Go `omitempty` response field paired with a required TS field is a crash
   waiting to happen.** `db.Find` leaves a slice nil when nothing matches, `omitempty` drops the key,
   and nobody guards `.length`. Collection fields on response DTOs must not carry `omitempty`; a test
   asserting it must read the **raw JSON** (decoding into the Go struct makes absent and `[]`
   indistinguishable). `backend/internal/omitemptyguard` enforces the backend half.

## Conventions

- The contact model is nested (`Card`/`CRMEnvelope`/`Passthrough` via `ContactRecordResponse`). The
  flat `Contact` type survives **only** for the list endpoint (genuinely flat on the wire) and for
  `MultiValueField`/`AddressFields`' editing contract. Do not reintroduce a flat adapter.
- API modules live in `src/api/<entity>.ts`, hooks in `src/hooks/use<Entity>.ts`, dialogs/lists in
  `src/components/`. `relationshipEdges.ts` + `useRelationshipEdges.ts` +
  `RelationshipEdgeDialog/List` is the most complete example of the full pattern — copy that shape.
- Generated TS types: `src/generated/openapi.ts` is generated from `openapi.yaml` by
  `cd backend && go run ./cmd/gentsapi` — never hand-edited. The hand-written types in `src/api/*.ts`
  are pinned to it by `src/api/contractConformance.ts` (types only; imported by nothing): `tsc` fails
  if a hand-written response type invents a required field, mistypes one, or drifts an enum.
  Deliberate differences go in that file's commented `DeliberateException` register.
- **TypeScript is intentionally capped at 6.x** (`typescript ^6.0.3`). TypeScript 7 is the Go-native
  rewrite (`tsgo`): no classic JS compiler API (`lib/typescript.js` gone, only `./unstable/*`) and no
  `tsserver`, so `typescript-eslint`'s typed linting — which `eslint.config.js`'s `projectService`
  promise-safety rules depend on — cannot load it. Latest `typescript-eslint@8.68.0` declares peer
  `typescript >=4.8.4 <6.1.0`. The cap is load-bearing; revisit when the toolchain supports TS 7.
- **`nanoid` is locked to `^3.3.18` in `resolutions`** (security pin for GHSA-2v37-7h3g-55p8 /
  CVE-2026-67213, which hit 3.3.17 and the whole 4.x–5.1.5 line). It cannot move to 4/5/6: vite →
  postcss 8.5.26 requires `nanoid ^3.3.17` and loads it via CommonJS `require('nanoid/non-secure')`,
  while nanoid 4+ is ESM-only with no `.cjs` files. 3.3.18 is the last CommonJS-capable *and* patched
  line.

## Web performance budgets

`docs/development/web-perf-budgets.md` governs. `frontend/bundle-budget.json` is the committed
per-chunk **gzip** baseline, generated from a clean `yarn build` by
`frontend/scripts/check-bundle-budget.mjs` — never hand-edited; regenerate with
`cd frontend && yarn build && yarn budget:update` and commit the diff (the diff is the review). The
`Frontend bundle-size budget` job in `unit-tests.yml` fails on growth past tolerance. The
network-graph render ceiling lives in `frontend/src/utils/graphBudget.ts`
(`GRAPH_RENDER_NODE_CEILING` / `GRAPH_RENDER_EDGE_CEILING`); above it `NetworkGraph` renders a notice
plus the always-present `NetworkListView` instead of the force-graph canvas. Keep the constants in
sync with the doc table and `networkGraphScale.spec.ts`. Two tiers: bundle budget + unit tests +
`@perf`-tagged route-stubbed Playwright specs per-PR; `@perf-heavy` seeded specs (`listPerf`,
`networkGraphScale`) nightly only via `e2e-tests.yml` with `RUN_PERF_HEAVY=1`.

## Dev server and service workers

`frontend-dev` (`yarn start`) never compiles `service-worker.ts` into a real `/service-worker.js` —
requests fall through to the SPA's `index.html` fallback. Registering a service worker against that
wrong-MIME-type response fails (Firefox: "The operation is insecure"), blocking push notifications
and every other service-worker feature. Use the `frontend-prod` launch config (`yarn build` +
`serve -s build`, same port) for those. Playwright and E2E are covered by trap 7 and
`docs/development/testing.md`.
