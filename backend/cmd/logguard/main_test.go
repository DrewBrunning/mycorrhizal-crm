package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAllow(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "allow.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const warnAudit = `{"level":"warn","message":"audit: failed to persist","time":"x"}`

func TestParseLine(t *testing.T) {
	cases := []struct {
		name, in, level, msg string
		ok                   bool
	}{
		{"plain", warnAudit, "warn", "audit: failed to persist", true},
		{"compose prefix", `mycorrhizal  | ` + warnAudit, "warn", "audit: failed to persist", true},
		{"first level wins", `{"level":"info","level":"warn","message":"Logger initialized"}`, "info", "Logger initialized", true},
		{"text line", `[GIN] 200 | GET /x`, "", "", false},
		{"not an object", `{ "a"`, "", "", false},
		{"array", `[1] {`, "", "", false},
		{"bad key", `{"level":"warn", 5}`, "", "", false},
		{"bad value", `{"level":"warn","message":`, "", "", false},
		{"no level", `{"message":"m"}`, "", "m", false},
		{"non-string level", `{"level":3,"message":"m"}`, "", "m", false},
	}
	for _, c := range cases {
		lv, m, ok := parseLine(c.in)
		if lv != c.level || m != c.msg || ok != c.ok {
			t.Errorf("%s: got (%q,%q,%v) want (%q,%q,%v)", c.name, lv, m, ok, c.level, c.msg, c.ok)
		}
	}
}

func TestParseAllowlist(t *testing.T) {
	bad := map[string]string{
		"not json":      `nope`,
		"empty message": `[{"message":"","max_count":1,"reason":"r"}]`,
		"no reason":     `[{"message":"m","max_count":1,"reason":" "}]`,
		"zero count":    `[{"message":"m","max_count":0,"reason":"r"}]`,
		"duplicate":     `[{"message":"m","max_count":1,"reason":"r"},{"message":"m","max_count":2,"reason":"r"}]`,
		"unknown field": `[{"message":"m","max_count":1,"reason":"r","extra":1}]`,
	}
	for name, body := range bad {
		if _, err := parseAllowlist([]byte(body)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	m, err := parseAllowlist([]byte(`[{"message":"m","max_count":2,"reason":"r"}]`))
	if err != nil || len(m) != 1 || m[0].MaxCount != 2 {
		t.Fatalf("good allowlist rejected: %v %v", m, err)
	}
}

func TestRun(t *testing.T) {
	logs := strings.Join([]string{
		`{"level":"info","message":"started"}`,
		`{"level":"debug","message":"noise"}`,
		`[GIN] text`,
		warnAudit, warnAudit,
	}, "\n")

	t.Run("unlisted fails", func(t *testing.T) {
		err := run(strings.NewReader(logs), &bytes.Buffer{}, writeAllow(t, `[]`))
		if err == nil || !strings.Contains(err.Error(), "2x unlisted") || !strings.Contains(err.Error(), "audit: failed to persist") {
			t.Fatalf("want unlisted failure with count+message, got %v", err)
		}
	})
	t.Run("over max_count fails", func(t *testing.T) {
		err := run(strings.NewReader(logs), &bytes.Buffer{},
			writeAllow(t, `[{"message":"audit: failed to persist","max_count":1,"reason":"r"}]`))
		if err == nil || !strings.Contains(err.Error(), "exceeds max_count 1") {
			t.Fatalf("want max_count failure, got %v", err)
		}
	})
	t.Run("within max_count passes", func(t *testing.T) {
		var out bytes.Buffer
		err := run(strings.NewReader(logs), &out,
			writeAllow(t, `[{"message":"audit: failed to persist","max_count":2,"reason":"r"}]`))
		if err != nil || !strings.Contains(out.String(), "OK: 2 warn+") {
			t.Fatalf("want pass, got %v %q", err, out.String())
		}
	})
	t.Run("clean log passes", func(t *testing.T) {
		if err := run(strings.NewReader(`{"level":"info","message":"x"}`), &bytes.Buffer{}, writeAllow(t, `[]`)); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("missing allowlist", func(t *testing.T) {
		if err := run(strings.NewReader(""), &bytes.Buffer{}, filepath.Join(t.TempDir(), "nope.json")); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("invalid allowlist", func(t *testing.T) {
		if err := run(strings.NewReader(""), &bytes.Buffer{}, writeAllow(t, `[{"message":"m"}]`)); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("oversize line", func(t *testing.T) {
		huge := strings.Repeat("a", 17<<20)
		if err := run(strings.NewReader(huge), &bytes.Buffer{}, writeAllow(t, `[]`)); err == nil {
			t.Fatal("want read error")
		}
	})
}

func TestParseAllowlistFieldMatchers(t *testing.T) {
	bad := map[string]string{
		"empty matcher":   `[{"message":"m","fields":{"path":""},"max_count":1,"reason":"r"}]`,
		"empty name":      `[{"message":"m","fields":{" ":"x"},"max_count":1,"reason":"r"}]`,
		"bad regexp":      `[{"message":"m","fields":{"path":"("},"max_count":1,"reason":"r"}]`,
		"non-string":      `[{"message":"m","fields":{"status":503},"max_count":1,"reason":"r"}]`,
		"message field":   `[{"message":"m","fields":{"message":"m"},"max_count":1,"reason":"r"}]`,
		"level field":     `[{"message":"m","fields":{"level":"warn"},"max_count":1,"reason":"r"}]`,
		"null entry":      `[null]`,
		"duplicate field": `[{"message":"m","fields":{"a":"1"},"max_count":1,"reason":"r"},{"message":"m","fields":{"a":"1"},"max_count":2,"reason":"r"}]`,
	}
	for name, body := range bad {
		if _, err := parseAllowlist([]byte(body)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	// Same message with different matchers (or one unscoped) is not a duplicate.
	ok := `[{"message":"m","fields":{"a":"1"},"max_count":1,"reason":"r"},{"message":"m","fields":{"a":"2"},"max_count":1,"reason":"r"},{"message":"m","max_count":1,"reason":"r"}]`
	if _, err := parseAllowlist([]byte(ok)); err != nil {
		t.Fatalf("distinct scopes rejected: %v", err)
	}
}

func TestRunFieldScopedEntries(t *testing.T) {
	line := func(path string, status int) string {
		return fmt.Sprintf(`{"level":"error","method":"GET","path":%q,"status":%d,"message":"HTTP request"}`, path, status)
	}
	allow := `[{"message":"HTTP request","fields":{"path":"/api/v1/immich/people","status":"503"},"max_count":2,"reason":"r"}]`
	cases := []struct {
		name, logs, wantErr string
	}{
		{"matching lines pass", line("/api/v1/immich/people", 503) + "\n" + line("/api/v1/immich/people", 503), ""},
		{"over the scoped count fails", strings.Repeat(line("/api/v1/immich/people", 503)+"\n", 3), `3x "HTTP request"{`},
		{"same message other path fails", line("/api/v1/contacts", 503), `unlisted: "HTTP request" (method=GET path=/api/v1/contacts status=503)`},
		{"same message other status fails", line("/api/v1/immich/people", 500), "status=500"},
		{"path is fully anchored", line("/api/v1/immich/people/extra", 503), "unlisted"},
		{"missing field fails", `{"level":"error","status":503,"message":"HTTP request"}`, "unlisted"},
	}
	for _, c := range cases {
		err := run(strings.NewReader(c.logs), &bytes.Buffer{}, writeAllow(t, allow))
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected failure %v", c.name, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%s: want %q, got %v", c.name, c.wantErr, err)
		}
	}
}

func TestFirstMatchingEntryWins(t *testing.T) {
	allow := `[{"message":"m","fields":{"path":"/a"},"max_count":1,"reason":"r"},{"message":"m","max_count":1,"reason":"r"}]`
	logs := `{"level":"warn","path":"/a","message":"m"}` + "\n" + `{"level":"warn","path":"/b","message":"m"}`
	if err := run(strings.NewReader(logs), &bytes.Buffer{}, writeAllow(t, allow)); err != nil {
		t.Fatal(err)
	}
	// A second /a overflows the scoped entry rather than spilling into the
	// unscoped catch-all's budget.
	logs += "\n" + `{"level":"warn","path":"/a","message":"m"}`
	if err := run(strings.NewReader(logs), &bytes.Buffer{}, writeAllow(t, allow)); err == nil {
		t.Fatal("want overflow of the scoped entry")
	}
}

func TestCommittedAllowlistValid(t *testing.T) {
	raw, err := os.ReadFile("allowlist.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseAllowlist(raw); err != nil {
		t.Fatal(err)
	}
}
