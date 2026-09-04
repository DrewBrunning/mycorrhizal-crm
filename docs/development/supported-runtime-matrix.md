# Supported runtime matrix (issue #472)

This is the engineering source of truth for every supported-version claim this project makes.
A minimum with no reason gets raised or lowered arbitrarily the next time someone finds it
inconvenient; every row below states the minimum **and** why it is there, grounded in an
actual dependency, feature, or file in this repo rather than a round number.

This ticket (COMPAT-01) produces the matrix and wires the two rows that can be mechanically
enforced (`browserslist`, `engines`) into the real build/CI. It does not add CI jobs that test
every row at its stated minimum — that is COMPAT-02 (issue #473), which consumes this table
directly. It does not publish an operator-facing page either — that is DOC-01 (issue #486),
which is expected to summarize this table for `docs/deployment.md`'s audience rather than
duplicate it. The dependency-*upgrade policy* (how/when a floor is allowed to move) is
COMPAT-03 (issue #474) and [breaking-change-policy.md](../breaking-change-policy.md)
(MAINT-02, issue #491) — raising any row below is a breaking change under that policy, not a
routine edit.

| Component | Minimum | Why |
|---|---|---|
| **Go** | `1.26.0` (toolchain `1.27.1`, `backend/go.mod`) | Deliberately pinned per the security posture (see CLAUDE.md) — this row states the current pin, it does not float it. Contributors building from source need a matching Go install; the shipped Docker image does not (it's built inside a `golang:1.27.1-alpine` build stage). |
| **Node.js** (contributor/CI, not the shipped image) | `>=22.13.0` (`frontend/package.json` `engines.node`) | The binding constraint is **not** the toolchain's own floor — vite@8.2.2 and eslint@10 both declare `^20.19.0 \|\| >=22.13.0` (verified against each package's published `engines` field), which would nominally allow Node 20.19+. But `frontend/vitest.config.ts` unconditionally passes `--no-experimental-webstorage` to the test worker, a flag that does not exist before Node 22.4 (added in 22.4.0) — on Node 20.x that is a fatal "bad option" error, not a slow-but-working test run. So the declared floor drops the 20.x branch entirely and is just the 22.x line. The all-in-one Docker image builds the frontend itself inside a pinned `node:26-alpine` stage (`Dockerfile`), so an operator running the published image never needs a local Node at all — this row is for anyone building from source. |
| **Yarn** | Classic v1, `>=1.22.0` (`frontend/package.json` `engines.yarn`) | Matches the committed `yarn.lock` v1 format and the version already used in CI/dev; nothing in this repo needs a newer Yarn Classic release, and migrating to Yarn Berry is a separate, undecided change (see the `nanoid`/postcss CommonJS constraint in CLAUDE.md, which is unrelated but shows the toolchain is deliberately conservative). `frontend/.yarnrc`'s `engine-strict true` makes this enforced, not decorative — Yarn Classic does not check `engines` by default. |
| **Browsers** | Chrome/Edge/Firefox ≥ 111, Safari/iOS ≥ 16.4 (`frontend/package.json` `browserslist`) | The binding constraint is Web Push: `frontend/src/pushSubscription.ts` uses `PushManager`/`applicationServerKey`, and Safari only shipped Web Push support in **16.4** (March 2023). Every other evergreen engine has supported Web Push and service workers for far longer, so the other three floors are pinned to the same release window (~March 2023) for one coherent, testable statement rather than a false sense of a lower floor nothing else in the PWA feature set was ever exercised against. `frontend/vite.config.ts`'s `build.target: browserslistToEsbuild()` reads this array directly, so it constrains the actual build output — not just documentation. |
| **SQLite** | Ships via the pure-Go `glebarez/sqlite` driver | No separate host SQLite install; the version travels with the Go module, governed by the Go row above. **Storage constraint, not a version number: local filesystem only.** SQLite's WAL mode depends on advisory byte-range locks that NFS, SMB/CIFS, and similar network filesystems do not reliably implement across clients — running the database over one is a documented corruption risk, and it is the most likely self-hosted mistake (mounting a NAS share and pointing `SQLITE_DB_PATH` at it). `backend/internal/fsguard` warns loudly at startup when it detects a known network-filesystem type under the database path (see "Fail-clearly behavior" below) but cannot catch every case — see its own doc comment for the FUSE caveat. |
| **Docker Engine** | `>=23.0` | Every doc and script in this repo invokes `docker compose` (the Compose **V2** CLI plugin) — never the deprecated hyphenated `docker-compose` v1 binary. Compose V2 became the Engine-bundled default at 23.0. Nothing in the committed `docker-compose*.yml` files uses syntax newer than that (checked this session: no `develop:`, `include:`, or other recent top-level keys — just `services`, `healthcheck`, `environment`, `volumes`). |
| **Docker Compose** | V2 (any release bundled with Engine `>=23.0`) | Same reasoning as above; there is no independent Compose-only floor beyond "whatever ships with the Engine minimum." |
| **Host OS / architecture** | Linux, x86_64 or arm64, running Docker | The only deployment shape this project builds, tests, and documents (`docker-publish.yml` publishes multi-arch images). Anything else — bare-metal install, Windows/macOS host, a non-Docker container runtime — is unsupported, not merely undocumented. |
| **Android** | `minSdk 26` (Android 8.0 Oreo; `android/build-logic/src/main/kotlin/com/mycorrhizal/crm/buildlogic/AndroidConfig.kt`) | `MycorrhizalApplication.kt`/`MycorrhizalApp.kt` create `NotificationChannel`s, an API-26 concept the app's notification features (push, ntfy, Gotify) depend on — there is no lower-API code path for notifications, so `minSdk` below 26 would need one built first. **What would justify raising it:** a feature that needs a platform API above 26 with no reasonable fallback. Raising the floor drops real devices still on Android 8–9 and is a breaking change under [breaking-change-policy.md](../breaking-change-policy.md) (MAINT-02, issue #491) — it needs that process, not an incidental bump alongside an unrelated change. |

## Fail-clearly behavior (issue #472 action 6 / #540's "unsupported environments fail clearly")

What is checkable at runtime, and what happens when the floor isn't met:

- **Storage filesystem type** — `backend/internal/fsguard.NetworkFilesystemWarning`, called from
  `main.go` right after `database.InitDB` succeeds. Detects NFS/SMB/CIFS/SMB2/NCP/Coda/AFS under
  the configured `SQLITE_DB_PATH` and logs a clear, actionable warning naming the filesystem —
  deliberately **advisory, not fatal** (see the function's doc comment): this is a corruption
  *risk*, not a certainty, and real production data already exists on deployments this check has
  never run against. Refusing to boot could brick a currently-working instance with no escape
  hatch; a v0.6.0-upgrade-floor-style hard refusal was considered and rejected for that reason.
- **Node/Yarn** — `frontend/.yarnrc`'s `engine-strict true` makes `yarn install` refuse outright
  on a Node/Yarn outside `engines`, both locally and in CI (`actions/setup-node`'s
  `node-version-file: frontend/package.json` reads the same `engines.node` value, so CI always
  runs the declared floor rather than an independently hardcoded number).
- **Browsers** — `build.target` (derived from `browserslist`) constrains what syntax the bundle
  can emit; a browser genuinely below the floor fails to parse/execute the bundle rather than
  running a silently broken app. This is a build-time enforcement, not a runtime "your browser is
  unsupported" banner — a friendlier in-browser message for that case is future work, not part of
  this ticket.
- **Go, Docker/Compose, host OS** — not checkable from inside this codebase (Go is a build-time
  toolchain choice; Docker/Compose/host-OS versions are outside any process this project controls
  at boot). Documented here instead, per the issue's "a startup check for the things that are
  checkable, and a documented statement for the things that are not."
