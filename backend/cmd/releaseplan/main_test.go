package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const composer = `jobs:
  unit-tests:
    uses: ./.github/workflows/unit-tests.yml
  e2e-tests:
    uses: ./.github/workflows/e2e-tests.yml
  deploy-smoke:
    uses: ./.github/workflows/deploy-smoke.yml
  build-candidate:
    runs-on: ubuntu-latest
  results:
    runs-on: ubuntu-latest
`

const (
	sha1 = "1111111111111111111111111111111111111111"
	sha2 = "2222222222222222222222222222222222222222"
)

func ledgerJSON(sha string, conclusions map[string]string) string {
	var rows []map[string]string
	for g, c := range conclusions {
		rows = append(rows, map[string]string{"sha": sha, "release_tag": "v1.0.0", "gate": g, "conclusion": c, "run_id": "100"})
	}
	b, _ := json.Marshal(rows)
	return string(b)
}

// fs is an in-memory filesystem standing in for os.ReadFile / os.WriteFile.
type fs struct {
	files map[string]string
	wrote map[string]string
}

func newFS(files map[string]string) *fs { return &fs{files: files, wrote: map[string]string{}} }

func (f *fs) read(name string) ([]byte, error) {
	if v, ok := f.files[name]; ok {
		return []byte(v), nil
	}
	return nil, os.ErrNotExist
}

func (f *fs) write(name string, b []byte, _ os.FileMode) error {
	if strings.HasPrefix(name, "/unwritable") {
		return errors.New("read-only")
	}
	f.wrote[name] = string(b)
	return nil
}

func exec(f *fs, args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	rc := run(args, &out, &errb, f.read, f.write)
	return rc, out.String(), errb.String()
}

func kv(out string) map[string]string {
	m := map[string]string{}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		k, v, _ := strings.Cut(l, "=")
		m[k] = v
	}
	return m
}

func TestRun_Dispatch(t *testing.T) {
	f := newFS(nil)
	rc, _, errb := exec(f)
	assert.Equal(t, 2, rc)
	assert.Contains(t, errb, "usage")
	rc, _, errb = exec(f, "bogus")
	assert.Equal(t, 2, rc)
	assert.Contains(t, errb, `unknown subcommand "bogus"`)
}

func TestLedger(t *testing.T) {
	files := map[string]string{
		"c.yml":  composer,
		"r.json": `{"unit-tests":"success","e2e-tests":"failure","deploy-smoke":"success","build-candidate":"success"}`,
	}
	base := []string{"ledger", "-composer", "c.yml", "-results", "r.json", "-sha", sha1, "-tag", "v1.0.0", "-run-id", "7"}

	t.Run("stdout", func(t *testing.T) {
		rc, out, _ := exec(newFS(files), base...)
		require.Equal(t, 0, rc)
		var l []map[string]string
		require.NoError(t, json.Unmarshal([]byte(out), &l))
		require.Len(t, l, 3)
		assert.Equal(t, "deploy-smoke", l[0]["gate"], "sorted by gate id")
		assert.Equal(t, "failure", l[1]["conclusion"])
	})

	t.Run("file out", func(t *testing.T) {
		f := newFS(files)
		rc, out, _ := exec(f, append(base, "-out", "ledger.json")...)
		require.Equal(t, 0, rc)
		assert.Empty(t, out)
		assert.Contains(t, f.wrote["ledger.json"], `"gate": "unit-tests"`)
	})

	t.Run("write failure", func(t *testing.T) {
		rc, _, errb := exec(newFS(files), append(base, "-out", "/unwritable/x")...)
		assert.Equal(t, 1, rc)
		assert.Contains(t, errb, "read-only")
	})

	t.Run("carried successes ride through skip", func(t *testing.T) {
		f := newFS(map[string]string{"c.yml": composer,
			"r.json": `{"unit-tests":"skipped","e2e-tests":"success","deploy-smoke":"success"}`})
		carried := ledgerJSON(sha1, map[string]string{"unit-tests": "success"})
		rc, out, _ := exec(f, append(base, "-skip", "unit-tests", "-carried", carried)...)
		require.Equal(t, 0, rc)
		assert.Contains(t, out, `"carried_from_run": "100"`)
	})

	t.Run("errors", func(t *testing.T) {
		cases := []struct {
			name  string
			files map[string]string
			extra []string
			want  string
		}{
			{"composer missing", map[string]string{"r.json": files["r.json"]}, nil, "file does not exist"},
			{"results missing", map[string]string{"c.yml": composer}, nil, "file does not exist"},
			{"results malformed", map[string]string{"c.yml": composer, "r.json": "{"}, nil, "results do not parse"},
			{"carried malformed", files, []string{"-carried", "{"}, "does not parse"},
			{"missing sha", files, []string{"-sha", ""}, "required"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rc, _, errb := exec(newFS(tc.files), append(append([]string{}, base...), tc.extra...)...)
				assert.Equal(t, 1, rc)
				assert.Contains(t, errb, tc.want)
			})
		}
	})

	t.Run("bad flag", func(t *testing.T) {
		rc, _, _ := exec(newFS(files), "ledger", "-nope")
		assert.Equal(t, 2, rc)
	})
}

func TestRerun(t *testing.T) {
	prior := ledgerJSON(sha1, map[string]string{"unit-tests": "success", "e2e-tests": "failure", "deploy-smoke": "success"})
	f := func() *fs { return newFS(map[string]string{"c.yml": composer, "l.json": prior}) }
	base := []string{"rerun", "-composer", "c.yml", "-ledger", "l.json", "-sha", sha1, "-tag", "v1.0.0"}

	t.Run("named gate", func(t *testing.T) {
		rc, out, errb := exec(f(), append(base, "-rerun", "e2e-tests")...)
		require.Equal(t, 0, rc)
		m := kv(out)
		assert.Equal(t, "e2e-tests", m["rerun_gates"])
		assert.Equal(t, "deploy-smoke,unit-tests", m["skip_gates"])
		assert.Contains(t, m["carried"], `"gate":"unit-tests"`)
		assert.NotContains(t, errb, "also re-running")
	})

	t.Run("auto-added gate is reported", func(t *testing.T) {
		rc, out, errb := exec(f(), append(base, "-rerun", "unit-tests")...)
		require.Equal(t, 0, rc)
		assert.Equal(t, "e2e-tests,unit-tests", kv(out)["rerun_gates"])
		assert.Contains(t, errb, "also re-running e2e-tests")
	})

	t.Run("failed keyword", func(t *testing.T) {
		rc, out, _ := exec(f(), append(base, "-rerun", "failed")...)
		require.Equal(t, 0, rc)
		assert.Equal(t, "e2e-tests", kv(out)["rerun_gates"])
	})

	t.Run("errors", func(t *testing.T) {
		cases := []struct {
			name  string
			files map[string]string
			extra []string
			want  string
		}{
			{"unknown gate", f().files, []string{"-rerun", "nope"}, "unknown gate"},
			{"no composer", map[string]string{"l.json": prior}, []string{"-rerun", "e2e-tests"}, "file does not exist"},
			{"no ledger", map[string]string{"c.yml": composer}, []string{"-rerun", "e2e-tests"}, "prior ledger"},
			{"bad ledger", map[string]string{"c.yml": composer, "l.json": "{"}, []string{"-rerun", "e2e-tests"}, "does not parse"},
			{"empty request", f().files, nil, "rerun_gates is empty"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rc, _, errb := exec(newFS(tc.files), append(append([]string{}, base...), tc.extra...)...)
				assert.Equal(t, 1, rc)
				assert.Contains(t, errb, tc.want)
			})
		}
	})

	t.Run("bad flag", func(t *testing.T) {
		rc, _, _ := exec(f(), "rerun", "-nope")
		assert.Equal(t, 2, rc)
	})
}

func TestReuse(t *testing.T) {
	green := ledgerJSON(sha1, map[string]string{"unit-tests": "success", "e2e-tests": "success", "deploy-smoke": "success"})
	fixture := "backend/database/testdata/schemas/v1.0.0.sql\nbackend/internal/schemafixture/releases.go\n"
	base := []string{"reuse", "-composer", "c.yml", "-ledger", "l.json", "-tag", "v1.0.0", "-tag-sha", sha2, "-changed-files", "d.txt"}
	files := func(l, d string) *fs {
		m := map[string]string{"c.yml": composer}
		if l != "" {
			m["l.json"] = l
		}
		if d != "-" {
			m["d.txt"] = d
		}
		return newFS(m)
	}

	t.Run("fixture-only diff reuses", func(t *testing.T) {
		rc, out, errb := exec(files(green, fixture), base...)
		require.Equal(t, 0, rc)
		m := kv(out)
		assert.Equal(t, "true", m["reuse"])
		assert.Equal(t, "e2e-tests,unit-tests", m["skip_gates"])
		var d map[string]any
		require.NoError(t, json.Unmarshal([]byte(m["decision"]), &d))
		assert.Equal(t, sha1, d["validated_sha"])
		assert.Equal(t, []any{"deploy-smoke"}, d["retest_gates"])
		assert.Contains(t, errb, "reuse=true")
	})

	t.Run("a source change does not", func(t *testing.T) {
		rc, out, _ := exec(files(green, fixture+"backend/main.go\n"), base...)
		require.Equal(t, 0, rc, "reuse never fails the caller")
		m := kv(out)
		assert.Equal(t, "false", m["reuse"])
		assert.Empty(t, m["skip_gates"])
		assert.Contains(t, m["reason"], "backend/main.go")
	})

	t.Run("blank lines in the diff list are ignored", func(t *testing.T) {
		rc, out, _ := exec(files(green, "\n"+fixture+"\n\n"), base...)
		require.Equal(t, 0, rc)
		assert.Equal(t, "true", kv(out)["reuse"])
	})

	t.Run("missing ledger falls back, exit 0", func(t *testing.T) {
		rc, out, _ := exec(files("", fixture), base...)
		require.Equal(t, 0, rc)
		m := kv(out)
		assert.Equal(t, "false", m["reuse"])
		assert.Contains(t, m["reason"], "could not be read")
		assert.Equal(t, "[]", m["carried"])
	})

	t.Run("malformed ledger falls back, exit 0", func(t *testing.T) {
		rc, out, _ := exec(files("{", fixture), base...)
		require.Equal(t, 0, rc)
		assert.Equal(t, "false", kv(out)["reuse"])
		assert.Contains(t, kv(out)["reason"], "does not parse")
	})

	t.Run("a ledger but no readable diff does not reuse", func(t *testing.T) {
		rc, out, _ := exec(files(green, "-"), base...)
		require.Equal(t, 0, rc)
		m := kv(out)
		assert.Equal(t, "false", m["reuse"])
		assert.Contains(t, m["reason"], "changed-file list could not be read")
		assert.Empty(t, m["skip_gates"])
	})

	t.Run("an unreadable composer is a hard error, not a silent fallback", func(t *testing.T) {
		rc, _, errb := exec(newFS(map[string]string{}), base...)
		assert.Equal(t, 1, rc)
		assert.Contains(t, errb, "file does not exist")
	})

	t.Run("bad flag", func(t *testing.T) {
		rc, _, _ := exec(files(green, fixture), "reuse", "-nope")
		assert.Equal(t, 2, rc)
	})
}

func TestMain_PropagatesExitCode(t *testing.T) {
	orig, origArgs := osExit, os.Args
	t.Cleanup(func() { osExit, os.Args = orig, origArgs })
	got := -1
	osExit = func(c int) { got = c }
	os.Args = []string{"releaseplan"}
	main()
	assert.Equal(t, 2, got)
}
