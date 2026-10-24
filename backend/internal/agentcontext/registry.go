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
