// Package mutationscope defines the Go mutation-testing scope for the
// safety-critical backend paths that issue #915 found uncovered: migration/
// upgrade, backup/restore, delete cascade, import/export, and data-integrity
// invariants. Mutation testing (gremlins) proves the test suite would notice
// a planted bug in these paths, which line coverage cannot — a line can be
// executed by an assertion-free test and still read 100% covered.
//
// atrest, database, vcard3, vcard4 and jscontact are already isolated
// packages, so their scopes mutate the whole package. Delete cascade
// (controllers) and import (services) live inside much larger packages, so
// their scopes name specific files and every other source file in the
// package is excluded. gremlins' --exclude-files patterns are Go regexps
// (RE2), which has no negative lookahead to express "everything except
// these files" directly — so the exclude list is generated (cmd/
// genmutationscope) from TargetFiles rather than hand-maintained, and the
// generated-artifact drift test in this package fails if a file is added to
// or removed from controllers/services without regenerating it. Each
// scope's --threshold-efficacy/--threshold-mcover floor is generated into
// the same config from Efficacy/MutantCoverage below, so gremlins itself
// exits non-zero on a regression (go-mutation.yml's per-scope job, not
// report-only).
package mutationscope

// Scope is one gremlins mutation-testing target.
type Scope struct {
	// Name identifies the scope: the generated config's basename (under
	// backend/.gremlins/) and the go-mutation.yml matrix leg.
	Name string
	// PackageDir is the package path relative to backend/, e.g. "atrest".
	PackageDir string
	// TargetFiles, when non-empty, restricts mutation to these basenames
	// within PackageDir — every other non-test *.go file in the package is
	// excluded. Empty means the whole package is mutated.
	TargetFiles []string
	// Efficacy is the --threshold-efficacy floor: the minimum percent of
	// covered mutants that must be KILLED (Killed / (Killed+Lived)).
	Efficacy float64
	// MutantCoverage is the --threshold-mcover floor: the minimum percent
	// of mutants that are covered by some test at all.
	MutantCoverage float64
	// Reason records the baseline run this threshold ratchets from and why
	// it sits below the measured number (TestScopesHaveReasons enforces
	// non-empty; a threshold change is a deliberate edit that rewrites it).
	Reason string
}

// Scopes is the complete, hand-authored mutation-testing scope. Adding a
// scope here is the only step needed to add a matrix leg — regenerate the
// exclude-files config (`go run ./cmd/genmutationscope`) and add the same
// Name to go-mutation.yml's matrix (TestWorkflowMatrixMatchesScopes enforces
// they stay in sync).
var Scopes = []Scope{
	{
		Name:           "atrest",
		PackageDir:     "atrest",
		Efficacy:       90,
		MutantCoverage: 80,
		Reason: "baseline 2026-09-15 (workers=4, timeout-coefficient=30, " +
			"fresh test cache — see CLAUDE.md's gremlins-timeout trap): " +
			"76 killed / 2 lived / 12 not covered / 0 timed out (efficacy " +
			"97.4%, mcover 86.7%); floor set below the measured value to " +
			"absorb a weaker CI runner, not because the package is " +
			"expected to regress",
	},
	{
		Name:           "database",
		PackageDir:     "database",
		Efficacy:       85,
		MutantCoverage: 85,
		Reason: "baseline 2026-09-15 (workers=4, timeout-coefficient=30): " +
			"253 killed / 0 lived / 3 not covered / 0 timed out (efficacy " +
			"100.0%, mcover 98.8%); floor set below measured to absorb " +
			"CI-runner timeout variance on this package's real-sqlite tests " +
			"(17m45s locally, all covered mutants killed)",
	},
	{
		Name:           "controllers-delete-cascade",
		PackageDir:     "controllers",
		TargetFiles:    []string{"contact_controller.go", "admin_user_controller.go", "user_delete_cascade.go"},
		Efficacy:       90,
		MutantCoverage: 90,
		Reason: "baseline 2026-09-15 (workers=4, timeout-coefficient=30, " +
			"fresh test cache — see CLAUDE.md's gremlins-timeout trap) on " +
			"DeleteContact/DeleteUser's two canonical cascade-checklist " +
			"files (CLAUDE.md backend trap 6): 229 killed / 0 lived / 5 not " +
			"covered / 0 timed out (efficacy 100.0%, mcover 97.9%); floor " +
			"set below measured to absorb CI-runner timeout variance " +
			"(6m26s locally, all covered mutants killed). Issue #972 " +
			"extracted the shared cascade body itself into " +
			"user_delete_cascade.go (called from admin_user_controller." +
			"go's DeleteUser, now also from user_controller.go's " +
			"DeleteOwnAccount) and added that file here so the checklist " +
			"this scope targets tracks where the mechanics actually " +
			"live; a pure extraction with no behavior change, so the " +
			"floor is carried forward rather than re-measured",
	},
	{
		Name:       "services-import",
		PackageDir: "services",
		TargetFiles: []string{
			"import_service.go",
			"import_source.go",
			"import_groupings_service.go",
			"import_custom_field_mapping.go",
			"meerkat_import.go",
			"meerkat_import_session.go",
			"monica_import.go",
			"monica_import_session.go",
			"source_import_session.go",
		},
		Efficacy:       85,
		MutantCoverage: 80,
		Reason: "baseline 2026-09-15 (workers=4, timeout-coefficient=30) on " +
			"the import-source ingestion files (meerkat/monica/generic " +
			"source import + field-mapping + grouping): 727 killed / 0 " +
			"lived / 30 not covered / 0 timed out (efficacy 100.0%, mcover " +
			"96.0%); floor set below measured to absorb CI-runner timeout " +
			"variance on this large, slow-compiling file set (31m41s " +
			"locally, all covered mutants killed)",
	},
	{
		Name:           "vcard3",
		PackageDir:     "vcard3",
		Efficacy:       70,
		MutantCoverage: 60,
		Reason: "baseline 2026-09-15 (workers=4, timeout-coefficient=30, " +
			"fresh test cache — see CLAUDE.md's gremlins-timeout trap) on " +
			"the vCard 3 exporter: 152 killed / 43 lived / 42 not covered / " +
			"1 timed out (efficacy 78.0%, mcover 82.3%); floor set below " +
			"measured to absorb CI-runner timeout variance",
	},
	{
		Name:           "vcard4",
		PackageDir:     "vcard4",
		Efficacy:       70,
		MutantCoverage: 60,
		Reason: "baseline 2026-09-15 (workers=4, timeout-coefficient=30, " +
			"fresh test cache — see CLAUDE.md's gremlins-timeout trap) on " +
			"the vCard 4 exporter: 152 killed / 59 lived / 30 not covered / " +
			"3 timed out (efficacy 72.0%, mcover 87.6%); floor set below " +
			"measured to absorb CI-runner timeout variance",
	},
	{
		Name:           "jscontact",
		PackageDir:     "jscontact",
		Efficacy:       70,
		MutantCoverage: 60,
		Reason: "baseline 2026-09-15 (workers=4, timeout-coefficient=30, " +
			"fresh test cache — see CLAUDE.md's gremlins-timeout trap) on " +
			"the JSContact exporter: 161 killed / 33 lived / 2 not covered / " +
			"0 timed out (efficacy 83.0%, mcover 99.0%); floor set below " +
			"measured to absorb CI-runner timeout variance",
	},
}
