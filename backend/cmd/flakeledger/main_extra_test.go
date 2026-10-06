package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClientDefaultBase(t *testing.T) {
	c, err := client(env(map[string]string{"GITHUB_REPOSITORY": "o/r", "GITHUB_TOKEN": "t"}))
	if err != nil || c.Base != "https://api.github.com" || c.Repo != "o/r" || c.Token != "t" {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestCollectClosedWriteFailureAndReportWarnings(t *testing.T) {
	var created []map[string]any
	srv := fakeAPI(t, nil, &created)
	e := map[string]string{"GITHUB_REPOSITORY": "o/r", "FLAKELEDGER_API_BASE": srv.URL}
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "closed.json"), 0o750) // a directory where the file belongs
	if code, _ := call(t, e, "collect", "-data", dir, "-workflows", "unit-tests.yml"); code != 2 {
		t.Error("closed.json write failure must exit 2")
	}

	d2 := t.TempDir()
	_ = os.WriteFile(filepath.Join(d2, "runs.json"), []byte(`[{"run_id":5,"url":"u","created_at":"`+fixedNow.Format(time.RFC3339)+`"}]`), 0o600)
	_ = os.MkdirAll(filepath.Join(d2, "5", "flake-go-x"), 0o750)
	_ = os.WriteFile(filepath.Join(d2, "5", "flake-go-x", "junit.xml"), []byte("<testcase name="), 0o600)
	code, out := call(t, e, "report", "-data", d2, "-md", filepath.Join(d2, "l.md"), "-candidates", filepath.Join(d2, "c.json"))
	if code != 0 || !strings.Contains(out, "warning: run 5") {
		t.Fatalf("%d %s", code, out)
	}
}
