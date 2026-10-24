# Per-ecosystem dependency update commands

Use the row for the ecosystem you are changing. The policy tiers are in
`docs/dependency-upgrade-policy.md`; this is only the mechanics.

| Ecosystem | Update command | Committed lock/pin | Ignore / exception | Generated artifact |
|---|---|---|---|---|
| Go modules | `cd backend && go get module@version && go mod tidy` | `backend/go.mod`, `backend/go.sum` | `docs/security/dependency-exceptions.ignore`; `govulncheck` findings | `THIRD_PARTY_NOTICES.md` (`gennotices`) |
| npm / Yarn | `cd frontend && yarn up package` (or `yarn add`) | `frontend/package.json`, `frontend/yarn.lock` | `docs/security/dependency-exceptions.ignore`; `yarn audit`/grype | `THIRD_PARTY_NOTICES.md` (`gennotices`) |
| Gradle / Android | edit `android/gradle/libs.versions.toml`, then the wrapper resolves | `android/gradle/libs.versions.toml`, `android/**/gradle.lockfile` | `android/.mobsf`, dependency-review | none for notices (Android is not in the npm/Go notice tree) |
| Docker base images | update the `FROM ...@sha256:<digest>` line | the three `Dockerfile`s plus the realserver stack Dockerfile | `.trivyignore`, `docker/cis-hardening.ignore` | SBOM/provenance at release |
| GitHub Actions | update `uses: owner/action@<sha> # vX.Y.Z` | the workflow file; version comment | `# zizmor: ignore[unpinned-uses]` only with a reason | none |

## Gotchas

- `go.sum` changes for a transitive module still belong in the same PR; never
  run `go mod tidy` on an unrelated branch.
- `yarn install --frozen-lockfile` is the CI path. A lockfile that needs
  resolving fails before tests. Yarn's `resolutions` block is deliberate; do not
  drop an entry without checking the advisory it pins away.
- The Gradle wrapper and version catalog move together; a lockfile-only edit is
  not an upgrade. Android tests are not part of the pre-commit hook.
- Docker Dependabot entries are per directory (`/`, `/backend`, `/frontend`,
  `/backend/integrations/realserver/stack`); a digest bump in one does not move
  the others.
- Actions must keep the human-readable version as a trailing comment. zizmor
  rejects a floating tag.
- `make gen-notices` reads `frontend/node_modules`; run
  `yarn install --frozen-lockfile` first if it is absent.
- The third-party notices gate runs only when a dependency manifest/generator
  changes (`unit-tests.yml`), so run it locally even when CI path-gates it.

Canonical: `docs/dependency-upgrade-policy.md`, `.github/dependabot.yml`,
`backend/AGENTS.md` (generated artifacts).
