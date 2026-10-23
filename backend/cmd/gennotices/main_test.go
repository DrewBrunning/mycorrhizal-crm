package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/internal/notices"
)

func fakeDeps(_ context.Context, _ string) ([]notices.Dependency, error) {
	return []notices.Dependency{{
		Ecosystem: "go",
		Name:      "example.org/lib",
		Version:   "v1.0.0",
		License:   "MIT",
		Source:    "https://pkg.go.dev/example.org/lib@v1.0.0",
		Text:      "Permission is hereby granted, free of charge",
	}}, nil
}

func TestRunWritesAndChecksTheNotice(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "THIRD_PARTY_NOTICES.md")
	var stdout, stderr bytes.Buffer
	if code := runWith([]string{"-root", root, "-out", out}, &stdout, &stderr, fakeDeps, fakeDeps); code != 0 {
		t.Fatalf("write returned %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "wrote") || !strings.Contains(stdout.String(), "2 dependencies") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "example.org/lib") {
		t.Fatalf("notice is missing the dependency:\n%s", body)
	}

	stdout.Reset()
	stderr.Reset()
	if code := runWith([]string{"-root", root, "-out", out, "-check"}, &stdout, &stderr, fakeDeps, fakeDeps); code != 0 {
		t.Fatalf("check on a current file returned %d: %s", code, stderr.String())
	}
}

func TestRunCheckFailsOnAStaleNotice(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "THIRD_PARTY_NOTICES.md")
	if err := os.WriteFile(out, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := runWith([]string{"-root", root, "-out", out, "-check"}, &stdout, &stderr, fakeDeps, fakeDeps)
	if code != 1 {
		t.Fatalf("check on a stale file returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "is stale") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestRunCheckFailsWhenTheNoticeIsMissing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runWith([]string{"-root", t.TempDir(), "-check"}, &stdout, &stderr, fakeDeps, fakeDeps); code != 1 {
		t.Fatalf("check without a notice file returned %d, want 1", code)
	}
}

func TestRunReportsCollectorFailures(t *testing.T) {
	fail := func(context.Context, string) ([]notices.Dependency, error) {
		return nil, errors.New("no dependency tree")
	}
	var stdout, stderr bytes.Buffer
	if code := runWith([]string{"-root", t.TempDir()}, &stdout, &stderr, fail, fakeDeps); code != 1 {
		t.Fatalf("collector failure returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no dependency tree") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := runWith([]string{"-root", t.TempDir()}, &stdout, &stderr, fakeDeps, fail); code != 1 {
		t.Fatalf("npm collector failure returned %d, want 1", code)
	}
}

func TestRunRejectsBadFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-not-a-flag"}, &stdout, &stderr); code != 2 {
		t.Fatalf("bad flags returned %d, want 2", code)
	}
}

func TestRunDefaultsTheOutputPath(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := runWith([]string{"-root", root}, &stdout, &stderr, fakeDeps, fakeDeps); code != 0 {
		t.Fatalf("default output returned %d: %s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "THIRD_PARTY_NOTICES.md")); err != nil {
		t.Fatalf("default output was not written: %v", err)
	}
}

func TestRunReportsAWriteFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runWith([]string{"-root", t.TempDir(), "-out", filepath.Join(t.TempDir(), "missing", "notice.md")}, &stdout, &stderr, fakeDeps, fakeDeps)
	if code != 1 {
		t.Fatalf("write failure returned %d, want 1", code)
	}
}
