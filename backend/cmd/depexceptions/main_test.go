package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(dateLayout, s)
	if err != nil {
		t.Fatalf("mustParse(%q): %v", s, err)
	}
	return tm
}

// TestRealLedger is the gate itself, run as a unit test: the real committed
// docs/security/dependency-exceptions.ignore must parse cleanly and have
// nothing expired, so a code move or a forgotten renewal fails the backend
// suite too, not only CI's dedicated step.
func TestRealLedger(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	var out strings.Builder
	code, err := check(&out, root, time.Now().UTC())
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if code != 0 {
		t.Fatalf("depexceptions found problems in the real ledger:\n%s", out.String())
	}
	if !strings.Contains(out.String(), exceptionsFile) {
		t.Fatalf("expected the summary to name %s, got:\n%s", exceptionsFile, out.String())
	}
}

// fixtureRoot builds a throwaway repository containing backend/go.mod (the
// findRepoRoot anchor) and, unless ledgerBody is nil, the ledger file.
func fixtureRoot(t *testing.T, ledgerBody *string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "backend", "go.mod"), []byte("module mycorrhizal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ledgerBody != nil {
		dir := filepath.Join(root, filepath.Dir(exceptionsFile))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, exceptionsFile), []byte(*ledgerBody), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func body(s string) *string { return &s }

func TestCheck(t *testing.T) {
	now := mustParse(t, "2026-09-04")

	tests := []struct {
		name    string
		ledger  *string // nil means the file is absent
		now     time.Time
		wantErr bool
		wantMsg string
	}{
		{
			name:    "missing file is zero exceptions, not a failure",
			ledger:  nil,
			now:     now,
			wantMsg: "does not exist",
		},
		{
			name:    "header-only file is zero exceptions",
			ledger:  body("# just comments\n\n# and blank lines\n"),
			now:     now,
			wantMsg: "0 recorded exception(s), none expired",
		},
		{
			name: "valid, unexpired entry passes",
			ledger: body(
				"GHSA-aaaa-bbbb-cccc | npm | example-pkg | 2026-08-01 | 2026-10-01 | drew | waiting on upstream fix\n"),
			now:     now,
			wantMsg: "1 recorded exception(s), none expired",
		},
		{
			name:    "wrong field count fails",
			ledger:  body("GHSA-aaaa-bbbb-cccc | npm | example-pkg\n"),
			now:     now,
			wantErr: true,
			wantMsg: "expected 7 pipe-delimited fields",
		},
		{
			name: "every field invalid at once still reports each problem",
			ledger: body(
				" |  |  | not-a-date | also-not-a-date |  | \n"),
			now:     now,
			wantErr: true,
			wantMsg: "advisory is empty",
		},
		{
			name: "invalid ecosystem fails",
			ledger: body(
				"GHSA-aaaa-bbbb-cccc | rust | example-pkg | 2026-08-01 | 2026-10-01 | drew | reason\n"),
			now:     now,
			wantErr: true,
			wantMsg: `ecosystem "rust" is not one of`,
		},
		{
			name: "expires before opened fails",
			ledger: body(
				"GHSA-aaaa-bbbb-cccc | go | example-pkg | 2026-08-15 | 2026-08-01 | drew | reason\n"),
			now:     now,
			wantErr: true,
			wantMsg: "is before opened",
		},
		{
			name: "window over 90 days fails",
			ledger: body(
				"GHSA-aaaa-bbbb-cccc | go | example-pkg | 2026-08-01 | 2026-12-01 | drew | reason\n"),
			now:     now,
			wantErr: true,
			wantMsg: "more than 90 days after opened",
		},
		{
			name: "expired entry fails",
			ledger: body(
				"GHSA-aaaa-bbbb-cccc | docker | example-pkg | 2026-06-01 | 2026-07-01 | drew | reason\n"),
			now:     now,
			wantErr: true,
			wantMsg: "expired on 2026-07-01",
		},
		{
			name: "entry expiring exactly today is not yet expired",
			ledger: body(
				"GHSA-aaaa-bbbb-cccc | github-actions | example-pkg | 2026-08-01 | 2026-09-04 | drew | reason\n"),
			now:     now,
			wantMsg: "1 recorded exception(s), none expired",
		},
		{
			name: "multiple problems are reported in line order",
			ledger: body(
				"GHSA-aaaa-bbbb-cccc | rust | example-pkg | 2026-08-01 | 2026-08-02 | drew | reason\n" +
					"GHSA-bbbb-cccc-dddd | npm | example-pkg | 2026-08-01 | 2026-08-02 | drew\n"),
			now:     now,
			wantErr: true,
			wantMsg: "2 problem(s)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := fixtureRoot(t, tt.ledger)
			var out strings.Builder
			code, err := check(&out, root, tt.now)
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			wantCode := 0
			if tt.wantErr {
				wantCode = 1
			}
			if code != wantCode {
				t.Fatalf("code = %d, want %d; output:\n%s", code, wantCode, out.String())
			}
			if !strings.Contains(out.String(), tt.wantMsg) {
				t.Fatalf("output does not contain %q:\n%s", tt.wantMsg, out.String())
			}
		})
	}
}

// TestCheck_ReadError exercises the non-IsNotExist error path: a directory
// where the ledger file should be makes os.ReadFile fail with something
// other than "not exist".
func TestCheck_ReadError(t *testing.T) {
	root := fixtureRoot(t, nil)
	dir := filepath.Join(root, exceptionsFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if _, err := check(&out, root, time.Now().UTC()); err == nil {
		t.Fatal("expected an error reading a directory as the ledger file")
	}
}

func TestFindRepoRoot(t *testing.T) {
	if _, err := findRepoRoot(); err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
}

func TestFindRepoRoot_NotFound(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := findRepoRoot(); err == nil {
		t.Fatal("expected an error outside any repository")
	}
}

func TestRun(t *testing.T) {
	t.Run("clean repo exits 0", func(t *testing.T) {
		root := fixtureRoot(t, body("# empty\n"))
		t.Chdir(root)
		var out strings.Builder
		if code := run(&out, time.Now().UTC()); code != 0 {
			t.Fatalf("code = %d, want 0; output:\n%s", code, out.String())
		}
	})

	t.Run("problems exit 1", func(t *testing.T) {
		root := fixtureRoot(t, body("GHSA-x | npm | pkg | 2026-08-01 | 2026-08-02\n")) // too few fields
		t.Chdir(root)
		var out strings.Builder
		if code := run(&out, time.Now().UTC()); code != 1 {
			t.Fatalf("code = %d, want 1; output:\n%s", code, out.String())
		}
	})

	t.Run("no repository root exits 2", func(t *testing.T) {
		t.Chdir(t.TempDir())
		var out strings.Builder
		if code := run(&out, time.Now().UTC()); code != 2 {
			t.Fatalf("code = %d, want 2; output:\n%s", code, out.String())
		}
		if !strings.Contains(out.String(), "depexceptions:") {
			t.Fatalf("expected the error to be prefixed, got:\n%s", out.String())
		}
	})

	t.Run("check error exits 2", func(t *testing.T) {
		root := fixtureRoot(t, nil)
		if err := os.MkdirAll(filepath.Join(root, exceptionsFile), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Chdir(root)
		var out strings.Builder
		if code := run(&out, time.Now().UTC()); code != 2 {
			t.Fatalf("code = %d, want 2; output:\n%s", code, out.String())
		}
	})
}
