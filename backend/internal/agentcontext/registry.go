package agentcontext

// Agent context files carry two budgets (issue #1955):
//
//   - TargetLines (100) is guidance. A file between TargetLines and MaxLines
//     passes and emits no warning; the target exists so a file is not padded,
//     never so load-bearing context is cut.
//   - MaxLines (200) is the hard maximum. Only exceeding it fails the check.
const (
	targetLines = 100
	maxLines    = 200
)

// Skill directories (issue #1956). The canonical tree is .agents/skills;
// .claude/skills is an alias and never a second source of truth.
const (
	SkillsRoot       = ".agents/skills"
	ClaudeSkillsRoot = ".claude/skills"
	ClaudeSkillsLink = "../.agents/skills"
)

// AgentFile is one tracked agent-context file. AliasOf is non-empty only for a
// CLAUDE.md alias, whose path must be a symlink to (or a one-line @ import of)
// that colocated AGENTS.md. Alias entries carry no line budget.
type AgentFile struct {
	Path        string
	AliasOf     string
	TargetLines int
	MaxLines    int
}

// Registry is the expected agent-context file set, paired with the budgets the
// validator enforces. It is the single source of truth: an AGENTS.md on disk
// that is missing here fails contextcheck, and so does an entry with no file.
//
// AGENTS.md is the content; CLAUDE.md is only an alias so Claude Code and
// other AGENTS.md-aware tools read the same text. Never put guidance in an
// alias.
func Registry() []AgentFile {
	agent := func(path string) AgentFile {
		return AgentFile{Path: path, TargetLines: targetLines, MaxLines: maxLines}
	}
	alias := func(path, target string) AgentFile {
		return AgentFile{Path: path, AliasOf: target}
	}
	return []AgentFile{
		agent("AGENTS.md"),
		alias("CLAUDE.md", "AGENTS.md"),
		agent("backend/AGENTS.md"),
		alias("backend/CLAUDE.md", "backend/AGENTS.md"),
		agent("frontend/AGENTS.md"),
		alias("frontend/CLAUDE.md", "frontend/AGENTS.md"),
		agent("android/AGENTS.md"),
		alias("android/CLAUDE.md", "android/AGENTS.md"),
		agent("docs/AGENTS.md"),
		alias("docs/CLAUDE.md", "docs/AGENTS.md"),
		agent(".github/AGENTS.md"),
		alias(".github/CLAUDE.md", ".github/AGENTS.md"),
		agent("backend/database/AGENTS.md"),
		alias("backend/database/CLAUDE.md", "backend/database/AGENTS.md"),
	}
}

// Skill is one parsed skill. Name comes from the directory (the schema says the
// two must agree); References are the files actually present under the skill's
// references/ directory, repo-relative.
type Skill struct {
	Name       string
	File       string
	References []string
}

// Suite is one row of the "what verifies this change" registry (issue #1956).
// Skills, CI and the eval harness read the same rows, so a DoD command and the
// job that runs it cannot drift apart silently.
type Suite struct {
	Name      string
	Commands  []string
	CIJobs    []string
	CICommand string
}

// Suites is the Tier 1 registry. Tier 2 suites are added in #1958.
func Suites() []Suite {
	return []Suite{
		{
			Name: "database-migration",
			Commands: []string{
				"cd backend && go test ./database/... ./internal/dbtest/...",
				"cd backend && go run ./cmd/genschema",
				"cd backend && go test ./internal/schemafixture/... ./cmd/genschema/...",
			},
			CIJobs:    []string{"upgrade-matrix", "schemafixture-suite"},
			CICommand: "go test ./internal/schemafixture/",
		},
		{
			Name: "contact-data-fidelity",
			Commands: []string{
				"cd backend && go test ./correspondence/... ./vcard3/... ./vcard4/... ./jscontact/... ./internal/rfctest/...",
				"cd backend && go run ./cmd/gencompatmatrix",
				"cd frontend && npx tsc --noEmit",
			},
			CIJobs:    []string{"backend-tests", "frontend"},
			CICommand: "npx tsc --noEmit",
		},
		{
			Name: "relationship-semantics-change",
			Commands: []string{
				"cd backend && go test ./models/... ./services/... ./controllers/...",
				"cd frontend && npx vitest run src/utils/graphBudget.test.ts",
			},
			CIJobs:    []string{"backend-tests"},
			CICommand: "go test",
		},
		{
			Name: "api-contract-change",
			Commands: []string{
				"cd backend && go run ./cmd/gencontract",
				"cd backend && go run ./cmd/gentsapi",
				"cd backend && go run ./cmd/genapibaseline",
				"cd backend && go test ./internal/apibaseline/... ./internal/contractfixtures/... ./internal/tsapi/...",
				"cd frontend && npx tsc --noEmit",
				"cd android && ./gradlew :core:network:testDebugUnitTest --tests '*ContractFixtureTest*'",
			},
			CIJobs:    []string{"frontend"},
			CICommand: "npx tsc --noEmit",
		},
		{
			Name: "security-sensitive-change",
			Commands: []string{
				"cd backend && go test ./middleware/... ./routes/...",
				"cd backend && go run ./cmd/citecheck",
				"cd backend && go run ./cmd/docscheck",
			},
			CIJobs:    []string{"docs-citations"},
			CICommand: "go run ./cmd/citecheck",
		},
		{
			Name: "release-readiness",
			Commands: []string{
				"cd backend && go run ./cmd/releasegatecheck",
				"cd backend && go run ./cmd/manualgatecheck validate -ledger ../.github/manual-gates.json -root ..",
				"cd backend && go run ./cmd/asvsstale",
				"cd backend && go run ./cmd/genschema",
			},
			CIJobs:    []string{"docs-citations", "release"},
			CICommand: "go run ./cmd/releasegatecheck",
		},
		{
			Name: "localization-change",
			Commands: []string{
				"cd frontend && npx vitest run src/i18n/locales.test.ts",
				"cd frontend && npx tsc --noEmit",
			},
			CIJobs:    []string{"frontend"},
			CICommand: "npx tsc --noEmit",
		},
		{
			Name: "dependency-update-review",
			Commands: []string{
				"cd backend && go run ./cmd/depexceptions",
				"cd backend && make gen-notices",
			},
			CIJobs:    []string{"docs-citations"},
			CICommand: "go run ./cmd/depexceptions",
		},
		{
			Name: "documentation-maintenance",
			Commands: []string{
				"cd backend && go run ./cmd/docscheck",
				"cd backend && go run ./cmd/citecheck",
			},
			CIJobs:    []string{"docs-citations"},
			CICommand: "go run ./cmd/docscheck",
		},
		{
			Name: "performance-investigation",
			Commands: []string{
				"cd backend && go test ./internal/perfbench/ -run '^TestCoreOperationBenchmarks$' -count=1",
			},
			CIJobs:    []string{"backend-checks"},
			CICommand: "go test -run '^$' -bench .",
		},
		{
			Name: "add-quality-gate",
			Commands: []string{
				"cd backend && go run ./cmd/governancecheck",
				"cd backend && go run ./cmd/releasegatecheck",
				"cd backend && go run ./cmd/nightlyalertcheck",
				"cd backend && go run ./cmd/mainalertcheck",
			},
			CIJobs:    []string{"docs-citations"},
			CICommand: "go run ./cmd/governancecheck",
		},
	}
}

// Fact is one stable, load-bearing value whose canonical home is code or
// configuration, not prose. Source names a reader in facts.go; Pattern is the
// regex that finds the value(s) in the named context Files. Every non-empty
// capture group must be one of the values the reader returns, and every value
// the reader returns must be captured at least once.
type Fact struct {
	Name    string
	Source  string
	Files   []string
	Pattern string
}

// Facts is the fact-parity registry. It deliberately anchors only values that
// change rarely: the upgrade floor, the locale set, the e2e port and the
// required gate contexts. Counts and other churny numbers stay out.
func Facts() []Fact {
	return []Fact{
		{
			Name:   "upgrade-floor",
			Source: "upgrade-floor",
			Files: []string{
				"backend/database/AGENTS.md",
				".agents/skills/database-migration/SKILL.md",
			},
			Pattern: "from `([^`]+)` \\(migration `([0-9]+)`\\)",
		},
		{
			Name:    "locale-set",
			Source:  "locale-set",
			Files:   []string{"frontend/AGENTS.md", ".agents/skills/localization-change/SKILL.md"},
			Pattern: "(?i)locale files[^\\n]*\\(([^)]*)\\)",
		},
		{
			Name:   "e2e-port",
			Source: "e2e-port",
			Files: []string{
				"frontend/AGENTS.md",
				".agents/skills/contact-data-fidelity/SKILL.md",
			},
			Pattern: "localhost:([0-9]+)",
		},
		{
			Name:   "required-gate-jobs",
			Source: "required-gate-jobs",
			Files: []string{
				".agents/skills/add-quality-gate/SKILL.md",
				".agents/skills/add-quality-gate/references/gate-checklist.md",
			},
			Pattern: "(?m)^\\s*[-*]\\s+`([^`]+)`\\s*$",
		},
	}
}
