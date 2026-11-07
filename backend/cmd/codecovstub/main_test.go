package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filters = "backend: ['backend/**']\nfrontend: ['frontend/**']\nandroid: ['android/**']\nopenapi: ['backend/openapi.yaml']\nworkflows: ['.github/**']\n"

func writeFilters(t *testing.T, body string) string {
	p := filepath.Join(t.TempDir(), "filters.yaml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

func TestRunNoUpload(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{writeFilters(t, filters)}, strings.NewReader("CHANGELOG.md\ndocs/a.md\n\n"), &out, &errOut)
	assert.Equal(t, 0, code)
	assert.Equal(t, "no-upload\n", out.String())
}

func TestRunUpload(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{writeFilters(t, filters)}, strings.NewReader("docs/a.md\nbackend/main.go\n"), &out, &errOut)
	assert.Equal(t, 0, code)
	assert.Equal(t, "upload\n", out.String())
}

func TestRunErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	assert.Equal(t, 2, run(nil, strings.NewReader(""), &out, &errOut), "missing filters arg")
	assert.Equal(t, 2, run([]string{"/nonexistent/filters.yaml"}, strings.NewReader(""), &out, &errOut), "unreadable file")
	assert.Equal(t, 2, run([]string{writeFilters(t, "backend: ['x']\n")}, strings.NewReader("a\n"), &out, &errOut), "filters missing upload suites")
	assert.Empty(t, out.String(), "nothing may print a verdict on error")
}

func TestRunOverlongLineIsAnError(t *testing.T) {
	var out, errOut bytes.Buffer
	// A line past the scanner's 1 MiB cap must fail closed, not truncate the
	// file list (which could hide a code file from the verdict).
	huge := strings.Repeat("a", 2*1024*1024) + "\n"
	assert.Equal(t, 2, run([]string{writeFilters(t, filters)}, strings.NewReader(huge), &out, &errOut))
	assert.Empty(t, out.String())
}
