package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_UsageErrors(t *testing.T) {
	var out, errb bytes.Buffer
	assert.Equal(t, 2, run(context.Background(), []string{"-nope"}, &out, &errb))
	assert.Equal(t, 2, run(context.Background(), []string{"-fault", "bogus"}, &out, &errb))
	assert.Equal(t, 2, run(context.Background(), []string{"-target", "http://127.0.0.1:1", "-metrics-token", ""}, &out, &errb))
	assert.Contains(t, errb.String(), "metrics-token")
	// Unreachable target: a harness error, not a finding.
	assert.Equal(t, 2, run(context.Background(), []string{"-target", "http://127.0.0.1:1", "-metrics-token", "x", "-duration", "1s"}, &out, &errb))
}

func TestRun_WriteBudgetsDoc(t *testing.T) {
	var out, errb bytes.Buffer
	dir := t.TempDir()
	p := filepath.Join(dir, "d.md")
	require.NoError(t, os.WriteFile(p, []byte("a\n<!-- soak:budgets:begin -->\nOLD\n<!-- soak:budgets:end -->\nb\n"), 0o600))
	assert.Equal(t, 0, run(context.Background(), []string{"-write-budgets-doc", p}, &out, &errb))
	got, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.NotContains(t, string(got), "OLD")
	assert.Contains(t, string(got), "`goroutines`")

	assert.Equal(t, 2, run(context.Background(), []string{"-write-budgets-doc", filepath.Join(dir, "missing.md")}, &out, &errb))
	bad := filepath.Join(dir, "bad.md")
	require.NoError(t, os.WriteFile(bad, []byte("no markers"), 0o600))
	assert.Equal(t, 2, run(context.Background(), []string{"-write-budgets-doc", bad}, &out, &errb))
	ro := filepath.Join(dir, "ro.md")
	require.NoError(t, os.WriteFile(ro, []byte("<!-- soak:budgets:begin -->\n<!-- soak:budgets:end -->"), 0o400))
	if os.Geteuid() != 0 {
		assert.Equal(t, 2, run(context.Background(), []string{"-write-budgets-doc", ro}, &out, &errb))
	}
}

func TestRun_HealthyPassesFaultyFailsAndReportsAreWritten(t *testing.T) {
	if testing.Short() {
		t.Skip("boots a real server")
	}
	dir := t.TempDir()
	md := filepath.Join(dir, "summary.md")
	js := filepath.Join(dir, "report.json")
	base := []string{"-duration", "12s", "-sample", "1s", "-users", "3", "-rate", "25", "-min-ops", "50", "-md", md, "-json", js}

	var out, errb bytes.Buffer
	require.Equal(t, 0, run(context.Background(), base, &out, &errb), out.String()+errb.String())
	assert.Contains(t, out.String(), "Soak run: PASS")
	assert.Contains(t, out.String(), "t=", "progress lines are printed unless -quiet")
	mdb, err := os.ReadFile(md)
	require.NoError(t, err)
	assert.Contains(t, string(mdb), "Soak run: PASS")
	jsb, err := os.ReadFile(js)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(jsb), `"series"`))

	out.Reset()
	errb.Reset()
	assert.Equal(t, 1, run(context.Background(), append([]string{"-quiet", "-fault", "goroutines,fds"}, base...), &out, &errb))
	assert.Contains(t, errb.String(), "FAILED")

	// Unwritable report paths are harness errors.
	assert.Equal(t, 2, run(context.Background(), []string{"-duration", "6s", "-sample", "1s", "-quiet", "-md", filepath.Join(dir, "no", "dir.md")}, &out, &errb))
	assert.Equal(t, 2, run(context.Background(), []string{"-duration", "6s", "-sample", "1s", "-quiet", "-json", filepath.Join(dir, "no", "dir.json")}, &out, &errb))
}
