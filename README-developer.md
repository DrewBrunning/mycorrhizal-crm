**Project Overview**
- Mycorrhizal CRM is a split Go backend and React frontend; the API sits under /api/v1 and serves a single-page app that manages contacts, activities, notes, reminders, and photos.
- The backend boots in [backend/main.go](backend/main.go) where config, database migrations, cron-style reminders, and the Gin router are wired together.
- The frontend lives in [frontend/src](frontend/src) with React 19, TypeScript, and MUI components backed by a typed API layer and custom hooks.

**Backend**
- Route definitions in [backend/routes/routes.go](backend/routes/routes.go) apply layered middleware: request IDs, structured logging, rate limiting, JWT auth, and JSON validation.
- Controllers (see [backend/controllers/contact_controller.go](backend/controllers/contact_controller.go)) expect `validated` payloads injected by [backend/middleware/validation.go](backend/middleware/validation.go); pull inputs from the context instead of decoding again.
- Custom errors from [backend/errors/errors.go](backend/errors/errors.go) plus [backend/middleware/middleware.go](backend/middleware/middleware.go) map failures to consistent JSON envelopes; prefer returning `*apperrors.AppError`.
- Database access is via GORM models in [backend/models](backend/models) with JSON arrays (contacts.circles) and manual cascade cleanup in delete flows; wrap multi-entity writes in transactions.
- Scheduled reminders run from [backend/services/reminder_service.go](backend/services/reminder_service.go) using gocron; honor `REMINDER_TIME` and Resend email toggles from [backend/config/config.go](backend/config/config.go).

**Frontend**
- All network calls go through [frontend/src/api/client.ts](frontend/src/api/client.ts) which enforces auth headers, request timeouts, and auto-logout on 401; reuse it for new endpoints.
- Resource-specific modules in [frontend/src/api](frontend/src/api) pair with hooks in [frontend/src/hooks](frontend/src/hooks); pages like [frontend/src/ContactsPage.tsx](frontend/src/ContactsPage.tsx) consume `{ data, loading, error, refetch }` contracts.
- Auth helpers in [frontend/src/auth.ts](frontend/src/auth.ts) persist JWTs in localStorage; frontend assumes `VITE_API_URL` when constructing base URLs.
- Styling blends global CSS (App.css/index.css) with MUI theming; photo uploads land in backend static storage under `static/photos`.

**Workflows**
- Source backend/my_environment.env to `.env` before running the server
- Start the backend with `go run main.go` (or `make dev`) from backend/ after `go mod tidy`; migrations are embedded in the binary and auto-run on boot. Use `make migrate-up` or cmd/migrate for manual control during development.
- Frontend uses Yarn: `yarn install` then `yarn start` from frontend/; the Vite dev server proxies nothing, so point `VITE_API_URL` at the backend in `.env`.
- Logs use zerolog via [backend/logger/logger.go](backend/logger/logger.go); set LOG_LEVEL and LOG_PRETTY for debugging, and rely on request IDs threaded through middleware.
- Rate limiting is IP-based via [backend/middleware/rate_limiter.go](backend/middleware/rate_limiter.go); respect separate auth/general buckets when adding endpoints.

**Docker (All-in-one Image)**
- The whole app ships as a single container built from the root [Dockerfile](Dockerfile): the React bundle and the Go backend served together by nginx (which proxies `/api` same-origin), managed by supervisord.
- Copy `.env.example` to `.env` and configure `JWT_SECRET_KEY`, `FRONTEND_URL`, and optionally `DATA_PATH`/`PHOTOS_PATH` for volume locations.
- Deploy using the pre-built image from GHCR: `docker compose up -d`. Set `IMAGE_TAG` in `.env` to pin a specific version (default: `latest`).
- Build and run locally instead: uncomment the `build: .` line in [docker-compose.yml](docker-compose.yml), then `docker compose up -d --build` (or plain `docker build -t mycorrhizal-crm .`).
- Container defaults (`PORT`, `SQLITE_DB_PATH`, `PROFILE_PHOTO_DIR`) are set in the root [Dockerfile](Dockerfile); override via `.env` if needed. `PORT` is the backend's internal bind port (8081) — nginx listens on 8080, which is what's actually exposed from the container.
- The frontend bundle is built with an empty `VITE_API_URL` so it calls the API on relative paths; nginx (see [docker/nginx.conf](docker/nginx.conf)) proxies `/api`, `/health`, and `/carddav` to the backend on `127.0.0.1:8081`.

**Testing**
- Backend Go tests (`go test ./...` or `make test`) spin up in-memory SQLite in helpers like [backend/controllers/activity_controller_test.go](backend/controllers/activity_controller_test.go); mirror that pattern for new suites.
- Validation and middleware behavior has dedicated coverage in [backend/middleware/validation_test.go](backend/middleware/validation_test.go) and related files—extend these before touching shared validators.
- Reminder scheduling is covered in [backend/services/reminder_service_test.go](backend/services/reminder_service_test.go) with clock control helpers; keep cron changes tested there.
- Frontend unit tests run with `yarn test` and rely on React Testing Library setup in [frontend/src/setupTests.ts](frontend/src/setupTests.ts), which already registers jest-dom.

**E2E Testing (Playwright)**
- End-to-end tests use Playwright against a real backend running in Docker; test files live in [frontend/e2e](frontend/e2e).
- Start the test stack: `docker compose -f docker-compose.test.yml up -d --build`
- Run tests: `cd frontend && npm run test:e2e` (or `test:e2e:ui` for interactive mode)
- Stop and clean up: `docker compose -f docker-compose.test.yml down -v`
- Tests run automatically in CI on push/PR to main via [.github/workflows/e2e-tests.yml](.github/workflows/e2e-tests.yml).

**Android E2E (instrumented, issue #238)**
- The instrumented suite in [android/app/src/androidTest](android/app/src/androidTest) drives the *real* app (MainActivity + Hilt graph) against the *real* `docker-compose.test.yml` backend, on an emulator or a physical device — the Android counterpart to the web Playwright suite. It covers login → list → detail → edit → list refresh, the favorites flow (issue #212), and archive/delete + the audit-undo round-trip.
- Start the test stack: `docker compose -f docker-compose.test.yml up -d --build`
- **Emulator (one command):**
  ```bash
  cd android
  ./gradlew :app:connectedDebugAndroidTest
  ```
  The suite defaults to `http://10.0.2.2:7300` (the emulator's host-loopback alias), which the debug build's cleartext allowlist permits.
- **Physical device (Pixel 8a):** the device cannot reach `10.0.2.2`, so tunnel the host backend onto the device's own loopback and point the suite at it:
  ```bash
  adb reverse tcp:7300 tcp:7300
  cd android
  ./gradlew :app:connectedDebugAndroidTest \
    -Pandroid.testInstrumentationRunnerArguments.serverUrl=http://127.0.0.1:7300
  ```
  This is exactly what the CI job does. Any `serverUrl` instrumentation arg overrides the default.
- Notes:
  - The suite registers its own seed account (`e2euser`) and creates/cleans up its own `E2E *` contacts; it never touches user data. It needs registration enabled, which `docker-compose.test.yml` already sets.
  - The seed backend data is persistent — `docker compose -f docker-compose.test.yml down -v` resets it between runs if you want a clean slate.
  - CI runs this on every push/PR to main via the `android-e2e` job in [.github/workflows/android-tests.yml](.github/workflows/android-tests.yml).

**CI: path-gated checks (issue #264)**
- Every suite is **path-gated**: a PR runs only the suites relevant to the files it changes. The mapping from paths to checks lives in one place, [.github/filters.yaml](.github/filters.yaml) — change it there, not per-workflow. Each gated workflow has a `Detect Changes` job (`dorny/paths-filter`) whose boolean outputs the suite jobs `if:` on.
  - `backend/**` → Go fmt/vet/test, govulncheck, gosec, CodeQL(go), backend image build.
  - `frontend/**` → vitest, tsc, eslint, Playwright e2e, CodeQL(js/ts), frontend image build.
  - `android/**` → Android unit/lint/assemble, instrumented e2e, CodeQL(java).
  - `backend/openapi.yaml` → backend + frontend + Android (the API contract).
  - `Dockerfile`, `docker-compose*.yml`, `docker/**` (nginx.conf etc.) → the e2e suites, which build and run the deployed artifact.
  - `.github/**` → everything (a workflow change can affect any check; this includes `filters.yaml` itself).
  - `docs/**` → nothing here; the docs deploy job is already path-filtered to `docs/**`.
- **Why a skipped job is safe to require.** A job skipped by its `if:` conditional reports **Success** (GitHub's current behavior) and satisfies a required status check — unlike a whole workflow skipped by `on: paths:`, which leaves its checks pending and blocks merges. So we gate at the job level, never at the workflow `paths:` level, and every suite check below can be marked required without unrelated pre-existing failures blocking a PR.
- **Required checks on `main`:** `Backend (Go)`, `Frontend (Vitest)`, `Run E2E Tests`, `Android (Gradle)`, `Android E2E (emulator)`. `enforce_admins` is on, so the checks apply to admins too. Set from Settings → Branches (or the REST API, e.g. `gh api`).
- **Verifying the mapping** after touching `filters.yaml`: open small PRs that touch only one area (`android/`, `backend/`, `docs/`, `backend/openapi.yaml`) and confirm each runs exactly the intended suites and reports the rest as skipped.

**CI: flake detection and test-health signal (issue #268)**
- All three suites retry a failed test once in CI and surface the flake instead of silently (or loudly) failing the required check:
  - **Go** runs through `gotestsum --rerun-fails=1` ([unit-tests.yml](.github/workflows/unit-tests.yml)); tests that failed then passed on retry are written to `rerun-fails.txt` (uploaded as an artifact) and printed as "Flaky tests" in the job log.
  - **vitest** sets `retry: 1` and the `junit` + `github-actions` reporters in CI ([vitest.config.ts](frontend/vitest.config.ts)); anything that only passed on retry lands in the job summary's "Flaky Tests" section.
  - **Android** applies `org.gradle.test-retry` to every module's `Test` tasks via the build-logic convention config ([AndroidConfig.kt](android/build-logic/src/main/kotlin/com/mycorrhizal/crm/buildlogic/AndroidConfig.kt)); the `Detect flaky unit tests` step in [android-tests.yml](.github/workflows/android-tests.yml) greps the test-results XML for retained failures after a green run. The instrumented E2E suite is retried once at the step level (the retry plugin only covers JVM `Test` tasks); a retried attempt's failing XML is preserved under `app/build/flaky-attempt-*/`.
- **PR annotations**: [test-report.yml](.github/workflows/test-report.yml) runs on `workflow_run` after the Unit Tests / Android Tests workflows, downloads each suite's JUnit XML artifact, and creates Check Runs with code annotations via `dorny/test-reporter`. It runs in the default-branch context so fork PRs (read-only token) still get annotated; the suite jobs remain the required checks and these reports are informational. A flaky test shows up as a failure in a report while its required check is green — that green-check-plus-failed-report combination *is* the flake-vs-regression distinction.

**Data & Integrations**
- SQLite lives at `SQLITE_DB_PATH` (default mycorrhizal.db); migrations in [backend/database/migrations](backend/database/migrations) are embedded into the binary and auto-run on startup.
- JWT expiry, HTTP timeouts, trusted proxies, and Resend email settings are declared in [backend/config/config.go](backend/config/config.go) and loaded based on environment variables; use Config.Validate to catch misconfigurations.
- File uploads stream through [backend/controllers/photo_controller.go](backend/controllers/photo_controller.go) and land in `static/photos`; served through protected routes to enforce auth.
- API consumers expect consistent field casing (e.g., `Firstname` in responses vs. lower-case in queries); follow existing JSON tags in [backend/models/contact.go](backend/models/contact.go).
- Deletions often clean up dependent entities manually (contacts remove reminders, notes, relationships, and activity links); mirror transaction patterns from [backend/controllers/contact_controller.go](backend/controllers/contact_controller.go).

**Dependencies & Updates**

- **Backend (Go modules)**
	1. `cd backend && go mod tidy && go mod verify` to pull new indirect deps, drop unused modules, and confirm checksums.
	2. Use `go get -u ./...` (or target a module) when you intentionally bump versions; commit both go.mod and go.sum together.
	3. Re-run `go test ./...` (or `make test`) plus `make migrate-status` if schema changes shipped with the upgrade.

- **Frontend (Yarn)**
	1. `cd frontend && yarn install --check-files` to sync lockfiles and ensure native binaries rebuild.
	2. For minor bumps run `yarn upgrade` (or `yarn up <pkg>@latest` for a specific lib); keep `yarn.lock` in the PR.
	3. After upgrades, run `yarn build` for production bundles

**Releases (only relevant for maintainers)**
- Ensure all changes are committed and pushed to `main`
- Create a tag using semantic versioning: `git tag v1.5.3`
- Push the tag to GitHub: `git push origin v1.5.3`
- This triggers a GitHub Actions workflow that automatically builds and publishes Docker images to GHCR
- Users can then deploy the new version by setting `IMAGE_TAG=v1.5.3` (or by just using `:latest`) in their `.env` and running `docker compose up -d`