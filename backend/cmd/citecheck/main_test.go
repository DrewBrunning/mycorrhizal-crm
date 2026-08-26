package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRealSecurityDocs is the gate itself, run as a unit test: every citation in
// the real docs/security/*.md must resolve against the real tree. It is here as
// well as in CI so a code move that orphans a citation fails the backend suite
// too, not only the docs job.
func TestRealSecurityDocs(t *testing.T) {
	var out strings.Builder
	code, err := run(&out)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Fatalf("citecheck found citation problems in the real security docs:\n%s", out.String())
	}
	// A pass over zero citations would also exit 0, so assert the docs were
	// actually read.
	if !strings.Contains(out.String(), "docs/security/asvs-l2.md:") {
		t.Fatalf("expected a per-doc summary line for asvs-l2.md, got:\n%s", out.String())
	}
}

// fixtureRoot writes a throwaway repository whose docs/security/asvs-l2.md is
// doc, plus any extra files, and returns its root.
func fixtureRoot(t *testing.T, doc string, extra map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{"docs/security/asvs-l2.md": doc}
	for k, v := range extra {
		files[k] = v
	}
	for rel, body := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// checkFixture runs the check over a fixture root and returns exit code + output.
func checkFixture(t *testing.T, root string) (int, string) {
	t.Helper()
	idx, err := buildIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	code, err := check(&out, root, idx, []string{"docs/security/asvs-l2.md"})
	if err != nil {
		t.Fatal(err)
	}
	return code, out.String()
}

const header = "| ID | Requirement (abbrev.) | Status | Evidence |\n|---|---|---|---|\n"

func TestCheck(t *testing.T) {
	tests := []struct {
		name  string
		doc   string
		extra map[string]string
		fail  bool
		want  string
	}{
		{
			name:  "resolving citation with an in-range line passes",
			doc:   header + "| 1.1.1 | Fine | satisfied | `backend/thing.go:2` |\n",
			extra: map[string]string{"backend/thing.go": "a\nb\nc\n"},
		},
		{
			name: "citation to a file that does not exist fails",
			doc:  header + "| 1.1.1 | Gone | satisfied | `backend/missing.go:2` |\n",
			fail: true,
			want: "does not resolve to any file",
		},
		{
			name:  "line past the end of the file fails",
			doc:   header + "| 1.1.1 | Drifted | satisfied | `backend/thing.go:9` |\n",
			extra: map[string]string{"backend/thing.go": "a\nb\n"},
			fail:  true,
			want:  "names line 9 but",
		},
		{
			name:  "line range past the end of the file fails",
			doc:   header + "| 1.1.1 | Drifted | satisfied | `backend/thing.go:1-9` |\n",
			extra: map[string]string{"backend/thing.go": "a\nb\n"},
			fail:  true,
			want:  "names line 9 but",
		},
		{
			name:  "a file with no trailing newline still counts its last line",
			doc:   header + "| 1.1.1 | Fine | satisfied | `backend/thing.go:3` |\n",
			extra: map[string]string{"backend/thing.go": "a\nb\nc"},
		},
		{
			name:  "backend-relative citation resolves via the prefix list",
			doc:   header + "| 1.1.1 | Fine | satisfied | `config/config.go:1` |\n",
			extra: map[string]string{"backend/config/config.go": "package config\n"},
		},
		{
			name:  "bare basename resolves",
			doc:   header + "| 1.1.1 | Fine | satisfied | `mailer.go:1` |\n",
			extra: map[string]string{"backend/services/mailer.go": "package services\n"},
		},
		{
			name: "an ambiguous basename passes when one candidate has the line",
			doc:  header + "| 1.1.1 | Fine | satisfied | `auth.go:5` |\n",
			extra: map[string]string{
				"backend/carddav/auth.go":    "1\n",
				"backend/middleware/auth.go": "1\n2\n3\n4\n5\n",
			},
		},
		{
			name: "an ambiguous basename fails when no candidate has the line",
			doc:  header + "| 1.1.1 | Drifted | satisfied | `auth.go:5` |\n",
			extra: map[string]string{
				"backend/carddav/auth.go":    "1\n",
				"backend/middleware/auth.go": "1\n2\n",
			},
			fail: true,
			want: "names line 5 but",
		},
		{
			name: "an elided Android path must match on a segment boundary",
			doc:  header + "| 1.1.1 | Drifted | satisfied | `feature/settings/.../SettingsScreen.kt:9` |\n",
			extra: map[string]string{
				// The near-miss sibling is long enough to contain the cited
				// line; only the segment-boundary rule keeps it from matching.
				"android/feature/settings/src/ImmichSettingsScreen.kt": "1\n2\n3\n4\n5\n6\n7\n8\n9\n",
				"android/feature/settings/src/SettingsScreen.kt":       "1\n2\n",
			},
			fail: true,
			want: "names line 9 but",
		},
		{
			name: "an allowlisted non-file is not a finding",
			doc:  header + "| 1.1.1 | Firebase | satisfied | `google-services.json` and `app/build.gradle.kts:1` |\n",
			extra: map[string]string{
				"android/app/build.gradle.kts": "plugins {}\n",
			},
		},
		{
			name:  "a status outside the legend fails",
			doc:   header + "| 1.1.1 | Typo | done | `backend/thing.go:1` |\n",
			extra: map[string]string{"backend/thing.go": "a\n"},
			fail:  true,
			want:  `has status "done"`,
		},
		{
			name: "an empty evidence cell fails",
			doc:  header + "| 1.1.1 | Bare | partial |  |\n",
			fail: true,
			want: "empty evidence cell",
		},
		{
			name: "satisfied without a citation fails",
			doc:  header + "| 1.1.1 | Asserted | satisfied | It is simply fine, trust us |\n",
			fail: true,
			want: "satisfied but cites nothing",
		},
		{
			name: "partial without a citation is allowed",
			doc:  header + "| 1.1.1 | Known gap | partial | No mechanism exists yet; tracked as a gap |\n",
		},
		{
			name: "a cited Go test that does not exist fails",
			doc:  header + "| 1.1.1 | Pinned | satisfied | `TestNoSuchThingAnywhere` |\n",
			fail: true,
			want: "test identifier `TestNoSuchThingAnywhere` does not exist",
		},
		{
			name:  "a cited Go test that exists passes",
			doc:   header + "| 1.1.1 | Pinned | satisfied | `TestRealOne` |\n",
			extra: map[string]string{"backend/x_test.go": "func TestRealOne(t *testing.T) {}\n"},
		},
		{
			name:  "a test-family citation matches by prefix",
			doc:   header + "| 1.1.1 | Pinned | satisfied | `TestFamily_*` |\n",
			extra: map[string]string{"backend/x_test.go": "func TestFamily_Success(t *testing.T) {}\n"},
		},
		{
			name: "a test-family citation matching nothing fails",
			doc:  header + "| 1.1.1 | Pinned | satisfied | `TestFamily_*` |\n",
			fail: true,
			want: "test-family citation `TestFamily_*` matches no identifier",
		},
		{
			name: "a Kotlin Class.method citation is checked on both halves",
			doc:  header + "| 1.1.1 | Pinned | satisfied | `ScreenTest.windowIsFlaggedSecure` |\n",
			extra: map[string]string{
				"android/ScreenTest.kt": "class ScreenTest { fun windowIsFlaggedSecure() {} }\n",
			},
		},
		{
			name: "a Kotlin method that no longer exists fails",
			doc:  header + "| 1.1.1 | Pinned | satisfied | `ScreenTest.windowIsFlaggedSecure` |\n",
			extra: map[string]string{
				"android/ScreenTest.kt": "class ScreenTest { fun somethingElse() {} }\n",
			},
			fail: true,
			want: "`windowIsFlaggedSecure` does not exist",
		},
		{
			name:  "an escaped pipe stays inside its evidence cell",
			doc:   header + "| 1.5.1 | Sensitivity | satisfied | `normal\\|private\\|secret` — `backend/thing.go:1` |\n",
			extra: map[string]string{"backend/thing.go": "a\n"},
		},
		{
			name:  "rows outside a control table are not checked",
			doc:   header + "| 1.1.1 | Fine | satisfied | `backend/thing.go:1` |\n\n| Operation | Bound | Where | Test |\n|---|---|---|---|\n| Import | 500 | `backend/thing.go:1` | none |\n",
			extra: map[string]string{"backend/thing.go": "a\n"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, out := checkFixture(t, fixtureRoot(t, tc.doc, tc.extra))
			if tc.fail && code == 0 {
				t.Fatalf("expected a finding, got a clean pass:\n%s", out)
			}
			if !tc.fail && code != 0 {
				t.Fatalf("expected a clean pass, got findings:\n%s", out)
			}
			if tc.want != "" && !strings.Contains(out, tc.want) {
				t.Fatalf("expected output to mention %q, got:\n%s", tc.want, out)
			}
		})
	}
}

func TestSplitRow(t *testing.T) {
	got := splitRow(`| 1.5.1 | Sensitivity | satisfied | ` + "`normal\\|private\\|secret`" + ` here |`)
	if len(got) != 4 {
		t.Fatalf("expected 4 cells (escaped pipes are not separators), got %d: %q", len(got), got)
	}
	if got[3] != "`normal\\|private\\|secret` here" {
		t.Fatalf("escaped pipes were not restored: %q", got[3])
	}
}

func TestParseControlRowsSkipsNonControlTables(t *testing.T) {
	doc := header +
		"| 1.1.1 | A | satisfied | `x.go:1` |\n" +
		"\n" +
		"| Operation | Bound | Where | Test |\n|---|---|---|---|\n" +
		"| Import | 500 | `x.go:1` | none |\n"
	rows := parseControlRows(strings.Split(doc, "\n"))
	if len(rows) != 1 {
		t.Fatalf("expected only the control row, got %d: %+v", len(rows), rows)
	}
	if rows[0].id != "1.1.1" || rows[0].status != "satisfied" {
		t.Fatalf("unexpected row: %+v", rows[0])
	}
}

func TestCensusLinesCountsPerChapter(t *testing.T) {
	doc := "## V1 — Architecture\n" + header +
		"| 1.1.1 | A | satisfied | `x.go:1` |\n" +
		"| 1.1.2 | B | partial | gap |\n" +
		"## V2 — Authentication\n" + header +
		"| 2.1.1 | C | not-applicable | n/a |\n"
	got := strings.Join(censusLines(parseControlRows(strings.Split(doc, "\n"))), "\n")
	for _, want := range []string{"V1 — Architecture", "satisfied 1, partial 1", "V2 — Authentication", "not-applicable 1", "TOTAL"} {
		if !strings.Contains(got, want) {
			t.Fatalf("census missing %q:\n%s", want, got)
		}
	}
}

func TestFindRepoRootWalksUp(t *testing.T) {
	root := fixtureRoot(t, header, nil)
	deep := filepath.Join(root, "backend", "cmd", "citecheck")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := findRepoRoot(deep)
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	if got != root {
		t.Fatalf("expected %s, got %s", root, got)
	}
	if _, err := findRepoRoot(t.TempDir()); err == nil {
		t.Fatal("expected an error when no repository root is above the start directory")
	}
}

func TestDriftFlagsAMovedTargetAndNotAMatchingOne(t *testing.T) {
	// The drift heuristic is advisory, but it is the check that actually found
	// the 74 moved line ranges in the 2026-08-26 pass, so both directions are
	// pinned: prose that still matches the cited lines stays quiet, prose that
	// no longer matches is surfaced.
	drift := func(doc, src string) string {
		t.Helper()
		root := fixtureRoot(t, doc, map[string]string{"backend/main.go": src})
		idx, err := buildIndex(root)
		if err != nil {
			t.Fatal(err)
		}
		var out strings.Builder
		if err := driftReport(&out, root, idx, []string{"docs/security/asvs-l2.md"}); err != nil {
			t.Fatalf("driftReport: %v", err)
		}
		return out.String()
	}

	moved := drift(
		header+"| 1.1.4 | Boundaries | satisfied | CORS boundary `backend/main.go:2-3` |\n",
		"corsConfig := cors.Config{}\n// scheduler\ns.Every(24).Hours()\n",
	)
	if !strings.Contains(moved, "backend/main.go:2-3") {
		t.Fatalf("expected the moved citation to be surfaced, got:\n%s", moved)
	}

	matching := drift(
		header+"| 1.1.4 | Boundaries | satisfied | CORS boundary `backend/main.go:1` |\n",
		"corsConfig := cors.Config{}\n// scheduler\n",
	)
	if strings.Contains(matching, "1 for review") {
		t.Fatalf("expected a matching citation to stay quiet, got:\n%s", matching)
	}
}
