# Controls

The harness derives the stripped-context control from the same task and base
ref at run time; there is deliberately no committed second copy of the prompts
or specs.

For the control worktree it removes `.agents/skills/`, the `.claude/skills`
alias, every nested `AGENTS.md`/`CLAUDE.md`, and the root `## Project map`
routing section (including the skills pointer), then runs the same seed patch,
agent command, and suite commands. The artifact records the exact stripped
paths and the context-vs-control delta.

Keep control prompts identical to the context prompt: changing only the context
files is what makes the delta attributable.
