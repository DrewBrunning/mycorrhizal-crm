package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
			name:  "a bare path to an existing file passes",
			doc:   header + "| 1.1.1 | Fine | satisfied | `backend/thing.go` |\n",
			extra: map[string]string{"backend/thing.go": "package thing\n"},
		},
		{
			name: "citation to a file that does not exist fails",
			doc:  header + "| 1.1.1 | Gone | satisfied | `backend/missing.go#Thing` |\n",
			fail: true,
			want: "does not resolve to any file",
		},
		{
			name:  "a line-number citation is rejected with the anchor form named",
			doc:   header + "| 1.1.1 | Stale | satisfied | `backend/thing.go:2` |\n",
			extra: map[string]string{"backend/thing.go": "package thing\n\nfunc Thing() {}\n"},
			fail:  true,
			want:  "line-number citation `backend/thing.go:2` is not allowed",
		},
		{
			name:  "a line-range citation is rejected too",
			doc:   header + "| 1.1.1 | Stale | satisfied | `backend/thing.go:1-3` |\n",
			extra: map[string]string{"backend/thing.go": "package thing\n\nfunc Thing() {}\n"},
			fail:  true,
			want:  "is not allowed",
		},
		{
			name:  "a comma-separated line list is rejected too",
			doc:   header + "| 1.1.1 | Stale | satisfied | `backend/thing.go:1,3-4` |\n",
			extra: map[string]string{"backend/thing.go": "package thing\n\nfunc Thing() {}\n"},
			fail:  true,
			want:  "is not allowed",
		},
		{
			name:  "a Go function anchor resolves",
			doc:   header + "| 1.1.1 | Fine | satisfied | `backend/thing.go#Thing` |\n",
			extra: map[string]string{"backend/thing.go": "package thing\n\nfunc Thing() {}\n"},
		},
		{
			name:  "a Go anchor survives edits above the declaration",
			doc:   header + "| 1.1.1 | Fine | satisfied | `backend/thing.go#Thing` |\n",
			extra: map[string]string{"backend/thing.go": "package thing\n\n// a\n// b\n// c\nvar filler = 1\n\nfunc Thing() {}\n"},
		},
		{
			name:  "a Go anchor for a removed declaration fails",
			doc:   header + "| 1.1.1 | Gone | satisfied | `backend/thing.go#Thing` |\n",
			extra: map[string]string{"backend/thing.go": "package thing\n\nfunc Other() {}\n"},
			fail:  true,
			want:  "anchor `Thing` not found in backend/thing.go",
		},
		{
			name: "Go type, method, pointer-receiver method, field, var and const anchors resolve",
			doc: header +
				"| 1.1.1 | T | satisfied | `backend/thing.go#Server` |\n" +
				"| 1.1.2 | M | satisfied | `backend/thing.go#Server.Handle` |\n" +
				"| 1.1.3 | P | satisfied | `backend/thing.go#Server.Close` |\n" +
				"| 1.1.4 | F | satisfied | `backend/thing.go#Server.Addr` |\n" +
				"| 1.1.5 | V | satisfied | `backend/thing.go#Limit` |\n" +
				"| 1.1.6 | C | satisfied | `backend/thing.go#Mode` |\n" +
				"| 1.1.7 | G | satisfied | `backend/thing.go#Cache.Get` |\n" +
				"| 1.1.8 | I | satisfied | `backend/thing.go#Store.Save` |\n",
			extra: map[string]string{"backend/thing.go": `package thing

type Server struct{ Addr string }

func (Server) Handle() {}

func (s *Server) Close() {}

type Cache[K comparable, V any] struct{}

func (c *Cache[K, V]) Get() {}

type Store interface{ Save() }

var Limit = 1

const Mode = "x"
`},
		},
		{
			name:  "a single-type-parameter generic receiver and a trailing-dot anchor",
			doc:   header + "| 1.1.1 | G | satisfied | `backend/thing.go#Box.Get` |\n| 1.1.2 | Bad | satisfied | `android/Screen.kt#Screen.` |\n",
			extra: map[string]string{"backend/thing.go": "package thing\n\ntype Box[T any] struct{}\n\nfunc (b *Box[T]) Get() {}\n", "android/Screen.kt": "class Screen\n"},
			fail:  true,
			want:  "anchor `Screen.` not found",
		},
		{
			name:  "a method anchor naming the wrong type fails",
			doc:   header + "| 1.1.1 | Wrong | satisfied | `backend/thing.go#Client.Handle` |\n",
			extra: map[string]string{"backend/thing.go": "package thing\n\ntype Server struct{}\n\nfunc (Server) Handle() {}\n"},
			fail:  true,
			want:  "anchor `Client.Handle` not found",
		},
		{
			name:  "a Go file that does not parse is a finding, not a crash",
			doc:   header + "| 1.1.1 | Broken | satisfied | `backend/thing.go#Thing` |\n",
			extra: map[string]string{"backend/thing.go": "package thing\nfunc ( {\n"},
			fail:  true,
			want:  "cannot check anchor",
		},
		{
			name:  "a quoted literal anchor resolves in a Go file",
			doc:   header + "| 1.1.1 | Fine | satisfied | `backend/thing.go#\"protected := \"` |\n",
			extra: map[string]string{"backend/thing.go": "package thing\n\nfunc f() { protected := 1 }\n"},
		},
		{
			name:  "a quoted literal that is gone fails",
			doc:   header + "| 1.1.1 | Gone | satisfied | `backend/thing.go#\"protected := \"` |\n",
			extra: map[string]string{"backend/thing.go": "package thing\n"},
			fail:  true,
			want:  "not found",
		},
		{
			name:  "a workflow step name anchor resolves",
			doc:   header + "| 1.1.1 | Fine | satisfied | `.github/workflows/ci.yml#Run Trivy scanner` |\n",
			extra: map[string]string{".github/workflows/ci.yml": "jobs:\n  scan:\n    steps:\n      - name: Run Trivy scanner\n        uses: x@v1\n"},
		},
		{
			name:  "a quoted workflow step name resolves",
			doc:   header + "| 1.1.1 | Fine | satisfied | `.github/workflows/ci.yml#Run Trivy scanner` |\n",
			extra: map[string]string{".github/workflows/ci.yml": "jobs:\n  scan:\n    steps:\n      - name: \"Run Trivy scanner\"\n"},
		},
		{
			name:  "a workflow job key anchor resolves",
			doc:   header + "| 1.1.1 | Fine | satisfied | `.github/workflows/ci.yml#apk-provenance` |\n",
			extra: map[string]string{".github/workflows/ci.yml": "jobs:\n  apk-provenance:\n    runs-on: x\n"},
		},
		{
			name:  "a workflow step that was renamed fails",
			doc:   header + "| 1.1.1 | Gone | satisfied | `.github/workflows/ci.yml#Run Trivy scanner` |\n",
			extra: map[string]string{".github/workflows/ci.yml": "jobs:\n  scan:\n    steps:\n      - name: Run Grype scanner\n"},
			fail:  true,
			want:  "anchor `Run Trivy scanner` not found",
		},
		{
			name:  "a workflow anchor must be a whole step name, not a substring of one",
			doc:   header + "| 1.1.1 | Partial | satisfied | `.github/workflows/ci.yml#Run Trivy` |\n",
			extra: map[string]string{".github/workflows/ci.yml": "steps:\n  - name: Run Trivy scanner\n"},
			fail:  true,
			want:  "anchor `Run Trivy` not found",
		},
		{
			name: "Kotlin and TypeScript declaration anchors resolve",
			doc: header +
				"| 1.1.1 | K | satisfied | `android/Screen.kt#Screen` |\n" +
				"| 1.1.2 | F | satisfied | `android/Screen.kt#secureWindow` |\n" +
				"| 1.1.3 | X | satisfied | `android/Screen.kt#Screen.render` |\n" +
				"| 1.1.4 | T | satisfied | `frontend/src/api.ts#fetchUser` |\n" +
				"| 1.1.5 | E | satisfied | `android/Screen.kt#clearWhenLoggedOut` |\n",
			extra: map[string]string{
				"android/Screen.kt":   "class Screen {\n    fun render() {}\n}\n\nprivate fun secureWindow() {}\n\ninternal suspend fun <T : Any> MutableStateFlow<T?>.clearWhenLoggedOut() {}\n",
				"frontend/src/api.ts": "export async function fetchUser() {}\n",
			},
		},
		{
			name:  "a Kotlin declaration that was removed fails",
			doc:   header + "| 1.1.1 | Gone | satisfied | `android/Screen.kt#secureWindow` |\n",
			extra: map[string]string{"android/Screen.kt": "class Screen\n"},
			fail:  true,
			want:  "anchor `secureWindow` not found",
		},
		{
			name:  "a Kotlin member anchor whose owner is absent fails",
			doc:   header + "| 1.1.1 | Wrong | satisfied | `android/Screen.kt#Other.render` |\n",
			extra: map[string]string{"android/Screen.kt": "fun render() {}\n"},
			fail:  true,
			want:  "anchor `Other.render` not found",
		},
		{
			name:  "an unquoted anchor in a config file is a literal substring",
			doc:   header + "| 1.1.1 | Fine | satisfied | `docker/nginx.conf#X-Frame-Options` |\n",
			extra: map[string]string{"docker/nginx.conf": "add_header X-Frame-Options DENY;\n"},
		},
		{
			name:  "a literal missing from a config file fails",
			doc:   header + "| 1.1.1 | Gone | satisfied | `docker/nginx.conf#X-Frame-Options` |\n",
			extra: map[string]string{"docker/nginx.conf": "server {}\n"},
			fail:  true,
			want:  "anchor `X-Frame-Options` not found",
		},
		{
			name:  "backend-relative citation resolves via the prefix list",
			doc:   header + "| 1.1.1 | Fine | satisfied | `config/config.go#Load` |\n",
			extra: map[string]string{"backend/config/config.go": "package config\n\nfunc Load() {}\n"},
		},
		{
			name:  "bare basename resolves",
			doc:   header + "| 1.1.1 | Fine | satisfied | `mailer.go#Send` |\n",
			extra: map[string]string{"backend/services/mailer.go": "package services\n\nfunc Send() {}\n"},
		},
		{
			name: "an ambiguous basename passes when one candidate has the anchor",
			doc:  header + "| 1.1.1 | Fine | satisfied | `auth.go#Middleware` |\n",
			extra: map[string]string{
				"backend/carddav/auth.go":    "package carddav\n",
				"backend/middleware/auth.go": "package middleware\n\nfunc Middleware() {}\n",
			},
		},
		{
			name: "an ambiguous basename fails when no candidate has the anchor",
			doc:  header + "| 1.1.1 | Gone | satisfied | `auth.go#Middleware` |\n",
			extra: map[string]string{
				"backend/carddav/auth.go":    "package carddav\n",
				"backend/middleware/auth.go": "package middleware\n",
			},
			fail: true,
			want: "anchor `Middleware` not found",
		},
		{
			name: "an elided Android path must match on a segment boundary",
			doc:  header + "| 1.1.1 | Drifted | satisfied | `feature/settings/.../SettingsScreen.kt#Settings` |\n",
			extra: map[string]string{
				// Only the near-miss sibling declares the anchor; the
				// segment-boundary rule must keep it from standing in for
				// SettingsScreen.kt, which does not.
				"android/feature/settings/src/ImmichSettingsScreen.kt": "class Settings\n",
				"android/feature/settings/src/SettingsScreen.kt":       "class Other\n",
			},
			fail: true,
			want: "anchor `Settings` not found in android/feature/settings/src/SettingsScreen.kt",
		},
		{
			name: "an allowlisted non-file is not a finding",
			doc:  header + "| 1.1.1 | Firebase | satisfied | `google-services.json` and `app/build.gradle.kts#\"plugins {\"` |\n",
			extra: map[string]string{
				"android/app/build.gradle.kts": "plugins {}\n",
			},
		},
		{
			name:  "a status outside the legend fails",
			doc:   header + "| 1.1.1 | Typo | done | `backend/thing.go` |\n",
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
			doc:   header + "| 1.5.1 | Sensitivity | satisfied | `normal\\|private\\|secret` — `backend/thing.go` |\n",
			extra: map[string]string{"backend/thing.go": "a\n"},
		},
		{
			name:  "rows outside a control table are not checked",
			doc:   header + "| 1.1.1 | Fine | satisfied | `backend/thing.go` |\n\n| Operation | Bound | Where | Test |\n|---|---|---|---|\n| Import | 500 | `backend/thing.go` | none |\n",
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

func mustGetwd(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return cwd
}

func TestReadRepoFileRefusesEscapingPaths(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "docs", "security")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inside, "ok.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A secret one directory above the "repository", the thing the bounds
	// check exists to keep unreadable.
	outside := filepath.Join(filepath.Dir(root), "citecheck-outside-probe")
	if err := os.WriteFile(outside, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outside)

	if got, err := readRepoFile(root, "docs/security/ok.md"); err != nil || string(got) != "hi\n" {
		t.Fatalf("expected to read the in-repo file, got %q / %v", got, err)
	}

	for _, rel := range []string{
		"../" + filepath.Base(outside),
		"docs/../../" + filepath.Base(outside),
		"..",
		filepath.Join(filepath.Dir(root), filepath.Base(outside)), // absolute
	} {
		if _, err := readRepoFile(root, rel); err == nil {
			t.Errorf("readRepoFile(%q) succeeded; it must refuse paths outside the root", rel)
		}
	}
}

// --- crypto-surface gate (issue #612) ---------------------------------------

// cryptoFixture runs checkCryptoSurface over a fixture repo and returns the
// exit code and output. extra keys are written as repo-relative files.
func cryptoFixture(t *testing.T, extra map[string]string) (int, string) {
	t.Helper()
	root := fixtureRoot(t, header, extra)
	idx, err := buildIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	code, err := checkCryptoSurface(&out, root, idx, []string{"docs/security/asvs-l2.md"})
	if err != nil {
		t.Fatal(err)
	}
	return code, out.String()
}

// cryptoRow is the minimal V6 surface row the fixtures cite.
const cryptoRow = "| 6.2.5 | No weak modes/hashes | satisfied | AES-256-GCM (`backend/services/credential_crypto.go`); the whole surface |\n"

const cryptoImporter = "package services\n\nimport \"crypto/aes\"\n\nfunc init() { _ = aes.BlockSize }\n"

func TestCryptoSurface_NewImporterWithNoV6RowFails(t *testing.T) {
	code, out := cryptoFixture(t, map[string]string{
		"backend/services/scratch.go": cryptoImporter,
	})
	if code == 0 {
		t.Fatalf("expected a new crypto importer with no V6 row to fail, got a clean pass:\n%s", out)
	}
	if !strings.Contains(out, "backend/services/scratch.go imports a crypto library") {
		t.Fatalf("expected the importer to be named, got:\n%s", out)
	}
}

func TestCryptoSurface_V6CitedImporterPasses(t *testing.T) {
	code, out := cryptoFixture(t, map[string]string{
		"backend/services/credential_crypto.go": cryptoImporter,
		"docs/security/asvs-l2.md":              header + cryptoRow,
	})
	if code != 0 {
		t.Fatalf("expected a V6-cited crypto importer to pass, got:\n%s", out)
	}
}

func TestCryptoSurface_IgnoredImporterPasses(t *testing.T) {
	code, out := cryptoFixture(t, map[string]string{
		"backend/services/scratch.go": cryptoImporter,
		cryptoSurfaceIgnoreFile:       "backend/services/scratch.go  # transport TLS only\n",
		"docs/security/asvs-l2.md":    header,
	})
	if code != 0 {
		t.Fatalf("expected a justified-ignore importer to pass, got:\n%s", out)
	}
}

func TestCryptoSurface_V6CitedFileNoLongerImportsCryptoFails(t *testing.T) {
	code, out := cryptoFixture(t, map[string]string{
		"backend/services/credential_crypto.go": "package services\n\nfunc init() {}\n",
		"docs/security/asvs-l2.md":              header + cryptoRow,
	})
	if code == 0 {
		t.Fatalf("expected a V6-cited file that stopped importing crypto to fail as stale, got a clean pass:\n%s", out)
	}
	if !strings.Contains(out, "no longer imports crypto — stale row") {
		t.Fatalf("expected the stale-row message, got:\n%s", out)
	}
}

func TestCryptoSurface_DeadIgnoreEntryFails(t *testing.T) {
	code, out := cryptoFixture(t, map[string]string{
		"backend/services/gone.go": "package services\n\nfunc init() {}\n",
		cryptoSurfaceIgnoreFile:    "backend/services/gone.go  # was crypto/rand\n",
		"docs/security/asvs-l2.md": header,
	})
	if code == 0 {
		t.Fatalf("expected a dead ignore entry to fail, got a clean pass:\n%s", out)
	}
	if !strings.Contains(out, "dead suppression") {
		t.Fatalf("expected the dead-suppression message, got:\n%s", out)
	}
}

func TestHasCryptoImport_IgnoresMentionsOutsideImportBlock(t *testing.T) {
	body := []byte("package services\n\n// crypto/rand is mentioned in a comment\nfunc init() { _ = \"crypto/rand\" }\n")
	if hasCryptoImport(body) {
		t.Fatalf("a comment/string mention of crypto/rand must not count as an import")
	}
	body = []byte("package services\n\nimport (\n\t\"crypto/rand\"\n)\n")
	if !hasCryptoImport(body) {
		t.Fatalf("an import-block crypto/rand must count")
	}
	body = []byte("package services\n\nimport \"github.com/golang-jwt/jwt/v4\"\n")
	if !hasCryptoImport(body) {
		t.Fatalf("a JWT/signing library import must count (issue #612 includes JWT libraries)")
	}
}

func TestCryptoSurface_SkipsTestFiles(t *testing.T) {
	// A _test.go file importing crypto is not a call site the surface rows
	// claim — tests exercise the primitives, they don't choose them.
	code, out := cryptoFixture(t, map[string]string{
		"backend/services/credential_crypto_test.go": "package services\n\nimport \"crypto/aes\"\n",
	})
	if code != 0 {
		t.Fatalf("a test-only crypto importer must not fail the surface gate:\n%s", out)
	}
}

// --- verification-report consistency (issue #939) ---------------------------

// countStatuses totals parsed control rows by status.
func countStatuses(rows []controlRow) map[string]int {
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.status]++
	}
	return counts
}

// readReportFile reads a repo-relative file as a string, failing the test if it
// cannot be read.
func readReportFile(t *testing.T, root, rel string) string {
	t.Helper()
	body, err := readRepoFile(root, rel)
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return string(body)
}

// declaredCount extracts capture group `group` from the first match of `re` in
// body, failing the test when the pattern is absent — a reworded claim must not
// silently stop being gated.
func declaredCount(t *testing.T, body, re string, group int, where string) int {
	t.Helper()
	m := regexp.MustCompile(re).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("%s: count pattern %q not found — the claim it guards was reworded without updating the gate", where, re)
	}
	n, err := strconv.Atoi(m[group])
	if err != nil {
		t.Fatalf("%s: %q is not an integer: %v", where, m[group], err)
	}
	return n
}

// TestVerificationReportClaimsMatchCensus is issue #939's two-way gate: every
// place a level claim or an exception count is stated must equal the count
// parsed from the checklists themselves. It fails when a control flips status
// without the docs being updated, and when a doc is edited to a count the
// checklists do not support — the drift that accumulated between Pass #1 and
// Pass #2 (#932), where the report's own header/claim/census said 26/191/186
// while §7 and a fresh citecheck said 23/194. The report's §7 count and the
// level claim in both checklists and in CLAUDE.md are all compared to the one
// parsed TOTAL, so the census can no longer "cannot drift" by assertion.
func TestVerificationReportClaimsMatchCensus(t *testing.T) {
	root, err := findRepoRoot(mustGetwd(t))
	if err != nil {
		t.Fatal(err)
	}

	asvs := countStatuses(parseControlRows(strings.Split(readReportFile(t, root, "docs/security/asvs-l2.md"), "\n")))
	masvs := countStatuses(parseControlRows(strings.Split(readReportFile(t, root, "docs/security/masvs-l1.md"), "\n")))
	report := readReportFile(t, root, "docs/security/asvs-l2-verification-report.md")
	asvsHeader := readReportFile(t, root, "docs/security/asvs-l2.md")
	masvsHeader := readReportFile(t, root, "docs/security/masvs-l1.md")
	claude := readReportFile(t, root, "CLAUDE.md")

	checks := []struct {
		where string
		body  string
		re    string
		group int
		want  int
		what  string
	}{
		{"asvs-l2.md Level claimed", asvsHeader, `ASVS L2 with (\d+) documented exceptions`, 1, asvs["partial"], "ASVS partial"},
		{"masvs-l1.md Level claimed", masvsHeader, `MASVS-L1 with (\d+) documented exception`, 1, masvs["partial"], "MASVS partial"},
		{"report claim", report, `ASVS Level 2, with (\d+) documented exceptions`, 1, asvs["partial"], "ASVS partial"},
		{"report §7 heading", report, "The (\\d+) ASVS `partial` rows", 1, asvs["partial"], "ASVS partial"},
		{"report claim satisfied count", report, `(\d+) of \d+ ASVS control rows are ` + "`satisfied`", 1, asvs["satisfied"], "ASVS satisfied"},
		{"report claim partial count", report, `\*\*(\d+) are ` + "`partial`" + `\*\*`, 1, asvs["partial"], "ASVS partial"},
		{"report census ASVS total satisfied", report, `\*\*ASVS total\*\* \| \*\*(\d+)\*\* \|`, 1, asvs["satisfied"], "ASVS satisfied"},
		{"report census ASVS total partial", report, `\*\*ASVS total\*\* \| \*\*\d+\*\* \| \*\*(\d+)\*\* \|`, 1, asvs["partial"], "ASVS partial"},
		{"report census ASVS total not-applicable", report, `\*\*ASVS total\*\* \| \*\*\d+\*\* \| \*\*\d+\*\* \| \*\*(\d+)\*\* \|`, 1, asvs["not-applicable"], "ASVS not-applicable"},
		{"report census MASVS total partial", report, `\*\*MASVS total\*\* \| \*\*\d+\*\* \| \*\*(\d+)\*\* \|`, 1, masvs["partial"], "MASVS partial"},
		{"CLAUDE.md level claim", claude, `ASVS L2 with (\d+) documented exceptions`, 1, asvs["partial"], "ASVS partial"},
	}
	for _, c := range checks {
		if got := declaredCount(t, c.body, c.re, c.group, c.where); got != c.want {
			t.Errorf("%s states %s = %d, but the checklists parse to %d", c.where, c.what, got, c.want)
		}
	}
}
