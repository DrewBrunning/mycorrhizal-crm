package prepr

// Command is one executable step (or a reminder when Reminder is set).
// Exactly one of Argv and Shell is set for an executable step. Argv is used
// wherever git-derived file names could appear, so a path is never
// interpolated into a shell; Shell is reserved for commands that genuinely
// need a pipeline (the coverage-package filter).
type Command struct {
	Label    string
	Dir      string // repo-relative working directory; "" is the repo root
	Argv     []string
	Shell    string
	Env      []string
	Repeat   int
	Warn     bool // a failure is reported but does not fail the gate
	Reminder string
}

// Area is the local pre-PR plan for one .github/filters.yaml filter.
//
// Fast is the lint/build build subset (the same class as the pre-commit hook);
// Full is the extra suite, coverage-ratchet, and flake-stress work that the
// pre-commit hook deliberately omits. Full mode always runs Fast first.
type Area struct {
	Name    string
	Note    string   // shown when the area is selected but cannot run locally
	Implies []string // other areas a change here requires
	Fast    []Command
	Full    []Command
}

// Areas is the single area -> command table. It must have exactly one entry
// per filter in .github/filters.yaml; TestEveryFilterHasAnArea enforces that,
// so a new filter cannot be silently unhandled.
var Areas = []Area{
	{Name: "backend", Fast: []Command{
		{Label: "backend: gofmt", Dir: "backend", Shell: `unformatted="$(gofmt -l .)"; if [ -n "$unformatted" ]; then echo "gofmt found unformatted files:"; echo "$unformatted"; exit 1; fi`},
		{Label: "backend: go build", Dir: "backend", Argv: []string{"go", "build", "./..."}},
		{Label: "backend: go vet", Dir: "backend", Argv: []string{"go", "vet", "./..."}},
		{Label: "backend: golangci-lint", Dir: "backend", Argv: []string{"go", "tool", "-modfile=tools.mod", "golangci-lint", "run", "./..."}},
		{Label: "backend: gormerrcheck", Dir: "backend", Argv: []string{"go", "run", "./cmd/gormerrcheck", "./..."}},
		{Label: "backend: rawtimecheck", Dir: "backend", Argv: []string{"go", "run", "./cmd/rawtimecheck", "./..."}},
		{Label: "backend: contract fixtures and generated TS drift", Dir: "backend", Argv: []string{"go", "test", "-run", "^TestContractFixturesMatchSpec$|^TestGeneratedTSTypesMatchSpec$", "."}},
		{Label: "backend: DATA-01/INT-01 matrix drift", Dir: "backend", Argv: []string{"go", "test", "-run", "^TestMatrixReproducesCommittedDoc$", "./correspondence/...", "./integrations/..."}},
		{Label: "backend: API baseline drift", Dir: "backend", Argv: []string{"go", "test", "-run", "^TestContractBaselineHolds$|^TestContractBaselineIsCurrent$", "./internal/apibaseline/..."}},
		{Label: "backend: mutation-scope config drift", Dir: "backend", Argv: []string{"go", "test", "./internal/mutationscope", "-run", "TestGeneratedConfigsAreCurrent|TestWorkflowMatrixMatchesScopes", "-count=1"}},
	}, Full: []Command{
		{Label: "backend: go test ./...", Dir: "backend", Argv: []string{"go", "test", "./..."}},
		{Label: "backend: cross-package coverage profile", Dir: "backend", Env: []string{"RAPID_CHECKS=200", "ADDRESS_PROPERTY_ITERATIONS=200"}, Shell: `pkgs="$(go list ./... | grep -v 'mycorrhizal/internal/schemafixture$' | tr '\n' ' ')"; go test $pkgs -coverpkg=./... -covermode=atomic -p 4 -timeout=30m -coverprofile=coverage-cross.out`},
		{Label: "backend: per-file coverage ratchet", Dir: "backend", Argv: []string{"go", "run", "./cmd/coverageratchet", "-profile", "coverage-cross.out"}},
	}},

	{Name: "frontend", Fast: []Command{
		{Label: "frontend: tsc --noEmit", Dir: "frontend", Argv: []string{"./node_modules/.bin/tsc", "--noEmit"}},
		{Label: "frontend: biome ci", Dir: "frontend", Argv: []string{"./node_modules/.bin/biome", "ci", "."}},
		{Label: "frontend: eslint", Dir: "frontend", Argv: []string{"yarn", "lint"}},
		{Label: "frontend: knip", Dir: "frontend", Argv: []string{"./node_modules/.bin/knip"}},
	}, Full: []Command{
		{Label: "frontend: vitest with coverage", Dir: "frontend", Argv: []string{"yarn", "test:coverage"}, Env: []string{"CI=true"}},
		{Label: "frontend: per-file coverage ratchet", Dir: "frontend", Argv: []string{"yarn", "coverage:ratchet"}},
	}},

	{Name: "android", Fast: []Command{
		{Label: "android: detekt + lint", Dir: "android", Argv: []string{"./gradlew", "detekt", "lintDebug", ":app:lintObtainiumDebug"}},
	}, Full: []Command{
		{Label: "android: aggregated jacoco report", Dir: "android", Argv: []string{"./gradlew", "--warning-mode=fail", "jacocoTestReportAggregated"}},
		{Label: "android: coverage ratchet", Dir: "backend", Argv: []string{"go", "run", "./cmd/androidcoverageratchet"}},
	}},

	{Name: "openapi", Note: "the API contract change re-tests every client", Implies: []string{"backend", "frontend", "android"}},

	{Name: "notices", Fast: []Command{
		{Label: "notices: third-party notice drift (needs frontend/node_modules)", Dir: "backend", Argv: []string{"go", "run", "./cmd/gennotices", "-check"}},
	}},

	{Name: "go-manifests", Fast: []Command{
		{Label: "go-manifests: go mod verify", Dir: "backend", Argv: []string{"go", "mod", "verify"}},
		{Label: "go-manifests: go mod tidy drift", Dir: "backend", Argv: []string{"go", "mod", "tidy", "-diff"}},
	}},

	{Name: "npm-manifests", Note: "lockfile/licence compliance is CI-only (license-compliance.yml); run `cd frontend && yarn install --frozen-lockfile` after changing a manifest"},
	{Name: "gradle-manifests", Note: "dependency locking/licence compliance is CI-only (license-compliance.yml); run `cd android && ./gradlew dependencies --write-locks` after changing a manifest"},

	{Name: "changelog-relevant", Fast: []Command{
		{Label: "changelog: operator-visible change", Reminder: "This branch touches a migration, backend/config/config.go, or .env.example. Add an '## Upgrade notes' block (and '## Breaking changes' if it applies) to the PR body, or 'no-changelog: <reason>'. See docs/changelog-policy.md."},
	}},

	{Name: "repro", Note: "the byte-reproducibility double build is CI-only (reproducibility.yml)"},
	{Name: "infra", Note: "the container image build and deploy smoke test are CI-only (docker-build-check.yml, deploy-smoke.yml)"},
	{Name: "chaos", Note: "fault-injection tests are CI-only and nightly (chaos-tests.yml)"},
	{Name: "differential", Note: "the reference-implementation differential suite is CI-only (differential-e2e.yml)"},
	{Name: "carddav", Note: "the real-server CardDAV suite is CI-only and nightly (carddav-e2e.yml)"},
	{Name: "realservers", Note: "the real-server integration contract suite is CI-only and nightly (integration-real-servers.yml)"},
	{Name: "reference-clients", Note: "the third-party reference-client suite is CI-only and nightly (reference-clients-e2e.yml)"},
	{Name: "workflows", Note: "a .github change re-runs every CI suite; workflow linters (actionlint, zizmor) and the suite matrix are CI-only"},
}

// GovernanceSteps run for every invocation, matching unit-tests.yml's
// docs-citations job. They are the unconditional class of check in CI, so a
// docs-only branch still gets a meaningful local gate.
var GovernanceSteps = []Command{
	{Label: "docs: citecheck", Dir: "backend", Argv: []string{"go", "run", "./cmd/citecheck"}},
	{Label: "docs: ASVS/MASVS census", Dir: "backend", Argv: []string{"go", "test", "./cmd/citecheck", "-run", "^TestVerificationReportClaimsMatchCensus$", "-count=1"}},
	{Label: "docs: depexceptions", Dir: "backend", Argv: []string{"go", "run", "./cmd/depexceptions"}},
	{Label: "docs: deprecations", Dir: "backend", Argv: []string{"go", "run", "./cmd/deprecations"}},
	{Label: "docs: docscheck", Dir: "backend", Argv: []string{"go", "run", "./cmd/docscheck"}},
	{Label: "docs: contextcheck", Dir: "backend", Argv: []string{"go", "run", "./cmd/contextcheck"}},
	{Label: "docs: releasegatecheck", Dir: "backend", Argv: []string{"go", "run", "./cmd/releasegatecheck"}},
	{Label: "docs: governancecheck", Dir: "backend", Argv: []string{"go", "run", "./cmd/governancecheck"}},
	{Label: "docs: codecovcheck", Dir: "backend", Argv: []string{"go", "run", "./cmd/codecovcheck"}},
	{Label: "docs: pragmacheck", Dir: "backend", Argv: []string{"go", "run", "./cmd/pragmacheck"}},
	{Label: "docs: nightlyalertcheck", Dir: "backend", Argv: []string{"go", "run", "./cmd/nightlyalertcheck"}},
	{Label: "docs: mainalertcheck", Dir: "backend", Argv: []string{"go", "run", "./cmd/mainalertcheck"}},
	{Label: "docs: androidskipcheck", Dir: "backend", Argv: []string{"go", "run", "./cmd/androidskipcheck", "-static"}},
}

// BaselineHints are the commands that update a coverage baseline when tests
// were added or changed. They are printed, never run: the baseline diff is a
// reviewed change (docs/development/coverage.md).
var BaselineHints = map[string]string{
	"backend":  "cd backend && make gen-coverage-baseline",
	"frontend": "cd frontend && yarn coverage:ratchet:update",
	"android":  "cd backend && go run ./cmd/androidcoverageratchet -update",
}

// Lookup returns the area table entry for a filter name.
func Lookup(name string) (Area, bool) {
	for _, a := range Areas {
		if a.Name == name {
			return a, true
		}
	}
	return Area{}, false
}

// AreaNames returns every area name in table order.
func AreaNames() []string {
	out := make([]string, 0, len(Areas))
	for _, a := range Areas {
		out = append(out, a.Name)
	}
	return out
}
