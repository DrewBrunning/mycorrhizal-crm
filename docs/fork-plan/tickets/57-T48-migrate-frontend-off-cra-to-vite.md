# T48 — Migrate the frontend off Create React App (`react-scripts`) to Vite

| | |
|---|---|
| **Rating** | 3 — health, not features (same framing as [T22](19-T22-legacy-audit.md)) |
| **Size** | XL — first ticket at this size; every current L ticket is narrower (two Dockerfiles, CI-adjacent config, docs, and a genuinely uncertain PWA/service-worker risk area, not just one surface) |
| **Depends on** | — |
| **Alpha** | n/a — real data exists, but this is build-tooling only: no schema change, no API contract change |
| **Source** | v0.3.0 post-release security review, 2026-08-06 |

## Why this exists

`react-scripts` has had no stable release since `5.0.1` on 2022-04-12 — confirmed against npm's
registry metadata, not assumed. The React team formally deprecated it on 2025-02-14
([react.dev/blog/2025/02/14/sunsetting-create-react-app](https://react.dev/blog/2025/02/14/sunsetting-create-react-app)):
no active maintainers, no security patches, no dependency updates. For an existing SPA with no SSR
need — which is exactly what this app is, confirmed during the T-router-8 work that it uses plain
declarative-mode routing, no data router, no RSC — their own announcement names Vite as the
recommended replacement over a full framework.

This isn't hypothetical cost; it showed up twice in the same session that produced this ticket:

- The react-router 7→8 bump required bumping the all-in-one Docker image's Node version for an
  unrelated reason (react-router 8's `engines` floor), because CRA's own toolchain has no bearing
  on that kind of constraint — it just happened to collide.
- Three "trivial" patch-level Dependabot fixes (`postcss`, `js-yaml`, `brace-expansion`) turned out
  to be unfixable cleanly: Yarn Classic has no safe way to bump one of two co-resolved major-line
  instances of a transitive `react-scripts` dependency without either polluting `package.json` with
  a fake direct dependency or hand-scoping fragile `resolutions` paths.
- Eight further alerts (`svgo`, `webpack-dev-server`, `uuid`, `nth-check`, `serialize-javascript`
  ×2, `underscore`, `@tootallnate/once`) are permanently stuck the same way — `react-scripts` pins
  them below their fix versions and will never bump short of ejecting. None are reachable in
  production (confirmed: all are build/dev/test-only transitive deps, verified with `yarn why` per
  package), but they're a standing drag on every future Dependabot scan.

Migrating to Vite removes `react-scripts` and its entire locked dependency tree in one shot,
closing all eleven of those alerts at once. It's also a continuation of work already begun:
`vitest` + `@vitejs/plugin-react` are already `devDependencies`, and `test`/`test:watch` already
run through Vite's own tooling — only `start` and `build` still shell out to `react-scripts`.

## Full scope (verified against the actual codebase during scoping, not assumed)

**Scripts**: `package.json`'s `start`/`build` go through `react-scripts`; `test` already doesn't.
`eject` becomes dead weight to delete.

**Env vars — 7 `process.env.*` sites across 4 files**: `src/index.tsx` (`NODE_ENV`),
`src/pushSubscription.ts` (`PUBLIC_URL`), `src/auth.ts` (`REACT_APP_API_URL`),
`src/api/client.ts` (`REACT_APP_REQUEST_TIMEOUT`), `src/serviceWorkerRegistration.ts`
(`NODE_ENV`, `PUBLIC_URL` ×3). `.env`/`.env.example` declare `PORT` (read directly by the CRA dev
server) and `REACT_APP_API_URL`. No `"proxy"` field in `package.json` — nothing to migrate there.

**`public/index.html`** uses `%PUBLIC_URL%` three times — CRA's own template-substitution
convention. Vite's `index.html` lives at the project root and *is* the real entry point (a
`<script type="module">` tag, not a webpack-injected template) — assets in `public/` are just
served at `/` directly, no placeholder needed.

**TypeScript**: `src/react-app-env.d.ts` has `/// <reference types="react-scripts" />` — swaps to
`/// <reference types="vite/client" />`. `tsconfig.json` already uses `moduleResolution: "bundler"`
and has a `tsconfig.node.json` reference (today covering only `playwright.config.ts`/`e2e/**`) —
extend the existing pattern, don't invent a new one. Zero `import x from '*.svg'`-style asset
imports anywhere in `src/` — no asset-resolution risk to verify.

**PWA / service worker — the highest-risk area.** Confirmed via `react-scripts`' own webpack
config (`node_modules/react-scripts/config/webpack.config.js`) that this app uses Workbox's
**`InjectManifest`** strategy (hand-written service worker, manifest injected at build time), not
the simpler auto-generated `GenerateSW` mode. `vite-plugin-pwa` (latest `1.3.0` as of scoping) has
a directly matching `injectManifest` strategy — confirmed against its own docs. Needs
`"WebWorker"` added to a `lib` array and `declare let self: ServiceWorkerGlobalScope` in the
service worker source; [service-worker.ts](../../../frontend/src/service-worker.ts) currently uses
`declare const self` + `/// <reference lib="webworker" />`, which needs reconciling with
vite-plugin-pwa's expected shape. This is the app's Web Push delivery path (N9) and the
service-worker registration fix that shipped in v0.3.0 — must be re-verified against the exact same
`e2e/serviceWorker.spec.ts` assertions, not just "the app loads."

**Docker — two Dockerfiles build the frontend, both need updating**:
- Root `Dockerfile` (all-in-one image) sets `REACT_APP_API_URL=""` deliberately empty so the
  bundle uses same-origin relative URLs through nginx's proxy — needs the renamed var, same
  empty-string behavior.
- `frontend/Dockerfile` (split image) takes `REACT_APP_API_URL` as a build `ARG` — same rename,
  same mechanism (Vite bakes `VITE_*` vars into the bundle at build time exactly like CRA did with
  `REACT_APP_*`).
- `.gitignore` ignores `/build` (CRA's output dir). Configure Vite's `build.outDir` to `build` too
  rather than switching to Vite's default `dist/` — keeps the Dockerfile `COPY` paths and
  `.gitignore` unchanged, smaller diff, no functional downside.

**CI**: `unit-tests.yml` already runs only `yarn test` (vitest) — no `react-scripts` coupling to
remove there. `docker-build-check.yml`/`e2e-tests.yml` build the Docker image directly —
downstream of the Dockerfile fix, no separate script changes needed.

**Local dev launch**: `.claude/launch.json`'s `frontend-dev` config runs
`FAST_REFRESH=false npx --yes yarn start` bound to port 7300. `yarn start` needs to become Vite's
dev command. Whether `@vitejs/plugin-react` has a clean `fastRefresh: false` equivalent is
genuinely unresolved from scoping-time research — several open upstream issues suggest it may not
be, and could cause HMR flicker if forced. Whatever originally motivated disabling it under CRA may
not even apply to Vite's different HMR implementation.

**Docs referencing the old convention**: `docs/development/frontend.md:15,25,38` and
`README-developer.md:16,32` reference `REACT_APP_API_URL`/`REACT_APP_REQUEST_TIMEOUT` — update to
the `VITE_` names. `docs/fork-plan/70-environment.md:34` mentions a `react-scripts`
peer-dependency conflict from original setup — that's a dated grooming-log entry ("why a past
decision was made"), leave as historical record, don't rewrite it.

**Dependencies**: add `vite`, `vite-plugin-pwa` (both bring Workbox transitively, same as today).
Remove `react-scripts` and `cra-template-pwa-typescript` (confirmed vestigial at scoping time —
grepped, nothing in `src/` imports it; it's a leftover scaffold-time dependency).

## What to build

Can't ship in independently-mergeable slices — the app either builds with `react-scripts` or Vite,
not both at once — so one branch, staged into checkpoints each fully verified before the next (so
a regression is easy to bisect to its checkpoint):

1. **Core swap.** Add `vite` + `vite-plugin-pwa`; write `vite.config.ts` (dev server on port 7300
   to match `launch.json`; `build.outDir: 'build'`). Move `public/index.html` to the project root,
   strip `%PUBLIC_URL%`, add the module script tag. Swap `react-app-env.d.ts`'s ambient-types
   reference. Rename the 7 env-var sites and both `.env` files. Verify: `yarn dev` boots and the
   app is usable; `yarn build` produces a working static bundle (PWA/push not expected to work yet
   at this checkpoint).
2. **PWA/service-worker parity.** Configure `vite-plugin-pwa`'s `injectManifest` strategy pointed
   at `src/service-worker.ts`; reconcile the webworker type declarations. Verify specifically:
   `e2e/serviceWorker.spec.ts` passes unmodified against a real build (registration survives a
   reload is the regression-sensitive assertion), plus a manual browser check of the Web Push
   enable flow from Settings.
3. **Docker, CI, launch config.** Update both Dockerfiles' env var names and confirm both still
   build. Update `.claude/launch.json`'s dev command. Resolve the `FAST_REFRESH` question — try
   Vite's default (HMR on) first and confirm in the browser preview whether it actually causes a
   problem before spending effort on a workaround. Remove `react-scripts`/
   `cra-template-pwa-typescript` from `package.json`, fresh `yarn install`. Update the docs lines
   above.
4. **Full verification pass.** `npx tsc --noEmit` clean, `npx vitest run` (557 tests at
   scoping time) green. Rebuild **both** Docker images — the all-in-one and the split
   `frontend/Dockerfile` (don't infer the split image works from the all-in-one build; it has its
   own build path and needs its own confirmation). Full Playwright suite against the rebuilt
   all-in-one image. Manual browser pass: deep-linked route, sidebar navigation, back/forward, zero
   console errors/warnings. After merge: re-query Dependabot alerts via `gh api
   repos/DrewBrunning/mycorrhizal-crm/dependabot/alerts` and confirm the eight previously-blocked
   alerts are actually gone — verify the claim, don't just assert it.

## Traps

- Don't switch `build.outDir` to Vite's default `dist/` without also updating both Dockerfiles and
  `.gitignore` — keeping it as `build` avoids that diff entirely and there's no functional reason
  to prefer `dist/`.
- The PWA/service-worker checkpoint is the one place "it built and the app loads" is not sufficient
  proof — `e2e/serviceWorker.spec.ts` exists specifically because a prior regression here (CRA's
  scaffolded `unregister()` silently defeating Web Push) shipped undetected by every other check in
  the suite. Re-run it, don't just trust a clean `yarn build`.
- Don't force a `fastRefresh: false`-equivalent workaround speculatively — confirm there's an
  actual problem in the browser preview first; the research at scoping time found no clean,
  confidently-documented option for this in current `@vitejs/plugin-react`.
- `docs/fork-plan/70-environment.md:34`'s `react-scripts` mention is a historical record, not a
  living doc — leave it alone even though it will look stale once this lands.

## Done when

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` unaffected/still green (backend
  isn't touched by this ticket, but the Docker rebuild proves the whole image still assembles).
- `npx tsc --noEmit` clean, `npx vitest run` green.
- Both Docker images (all-in-one and split `frontend/Dockerfile`) build and boot.
- Full Playwright e2e suite green against the rebuilt all-in-one image, including
  `e2e/serviceWorker.spec.ts` unmodified.
- Hand-verified in a real browser: app navigation, Web Push enable flow from Settings, a PWA
  install prompt/manifest still resolves correctly.
- `react-scripts` and `cra-template-pwa-typescript` fully removed from `package.json`/`yarn.lock`.
- Post-merge: Dependabot alert count for the eight previously-blocked frontend-toolchain packages
  confirmed at zero via the API, not assumed from the dependency removal alone.
