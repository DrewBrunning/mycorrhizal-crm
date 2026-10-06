// Command logguard fails a CI job when the server logged an unexpected
// warning or error (issue #1474).
//
// A passing E2E run can hide a misbehaving server: issue #1471 shipped 698
// FK-failing audit WARN lines inside a green run and was only found by a
// person reading the log. logguard turns that into a mechanical gate. It reads
// the backend's structured (zerolog JSON) log from stdin -- typically
// `docker compose logs --no-log-prefix mycorrhizal` -- and fails if any line
// at level warn or above carries a `message` that is not in the committed,
// reason-bearing allowlist (allowlist.json), or if an allowlisted message
// occurs more often than its max_count.
//
// Non-JSON lines (nginx, supervisord, gin's text access log) are ignored; a
// line is parsed from its first '{' so a leftover compose "service | " prefix
// does not hide it.
//
// Usage: go run ./cmd/logguard [-allowlist path] < logs
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// entry is one allowlist row. Reason is mandatory: an allowlist entry with no
// recorded reason is a silent weakening of the gate (CLAUDE.md "Gates are not
// silently weakened").
type entry struct {
	Message  string `json:"message"`
	MaxCount int    `json:"max_count"`
	Reason   string `json:"reason"`
}

// levelRank orders zerolog level names; anything unknown ranks 0 (ignored).
var levelRank = map[string]int{
	"trace": -1, "debug": 0, "info": 1, "warn": 2, "error": 3, "fatal": 4, "panic": 5,
}

const warnRank = 2

func main() {
	path := flag.String("allowlist", "cmd/logguard/allowlist.json", "path to the allowlist JSON")
	flag.Parse()
	if err := run(os.Stdin, os.Stdout, *path); err != nil {
		fmt.Fprintln(os.Stderr, "logguard:", err)
		os.Exit(1)
	}
}

func run(in io.Reader, out io.Writer, allowlistPath string) error {
	raw, err := os.ReadFile(allowlistPath) // #nosec G304 -- operator/CI-supplied path
	if err != nil {
		return fmt.Errorf("read allowlist: %w", err)
	}
	allow, err := parseAllowlist(raw)
	if err != nil {
		return fmt.Errorf("allowlist %s: %w", allowlistPath, err)
	}
	counts, total, err := scan(in)
	if err != nil {
		return fmt.Errorf("read log: %w", err)
	}
	failures := evaluate(counts, allow)
	if len(failures) > 0 {
		return fmt.Errorf("%d unexpected warn/error log finding(s) (of %d warn+ lines):\n  %s\n"+
			"fix the cause, or add a reason-bearing entry to %s",
			len(failures), total, strings.Join(failures, "\n  "), allowlistPath)
	}
	fmt.Fprintf(out, "OK: %d warn+ log line(s), all allowlisted within max_count.\n", total)
	return nil
}

func parseAllowlist(raw []byte) (map[string]entry, error) {
	var rows []entry
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rows); err != nil {
		return nil, err
	}
	m := make(map[string]entry, len(rows))
	for i, r := range rows {
		switch {
		case r.Message == "":
			return nil, fmt.Errorf("entry %d: empty message", i)
		case strings.TrimSpace(r.Reason) == "":
			return nil, fmt.Errorf("entry %q: reason is required", r.Message)
		case r.MaxCount < 1:
			return nil, fmt.Errorf("entry %q: max_count must be >= 1", r.Message)
		}
		if _, dup := m[r.Message]; dup {
			return nil, fmt.Errorf("entry %q: duplicate message", r.Message)
		}
		m[r.Message] = r
	}
	return m, nil
}

// scan counts warn+ lines by message.
func scan(in io.Reader) (map[string]int, int, error) {
	counts := map[string]int{}
	total := 0
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		level, msg, ok := parseLine(sc.Text())
		if !ok || levelRank[level] < warnRank {
			continue
		}
		counts[msg]++
		total++
	}
	return counts, total, sc.Err()
}

// parseLine extracts level and message from a zerolog JSON line. It takes the
// FIRST "level"/"message" key: the startup "Logger initialized" line carries a
// second, config-valued `level` field that must not shadow the severity.
func parseLine(line string) (level, msg string, ok bool) {
	i := strings.IndexByte(line, '{')
	if i < 0 {
		return "", "", false
	}
	dec := json.NewDecoder(strings.NewReader(line[i:]))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return "", "", false
	}
	var haveLevel, haveMsg bool
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return "", "", false
		}
		key, _ := kt.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return "", "", false
		}
		var s string
		switch {
		case key == "level" && !haveLevel:
			if json.Unmarshal(v, &s) == nil {
				level, haveLevel = s, true
			}
		case key == "message" && !haveMsg:
			if json.Unmarshal(v, &s) == nil {
				msg, haveMsg = s, true
			}
		}
	}
	return level, msg, haveLevel
}

func evaluate(counts map[string]int, allow map[string]entry) []string {
	msgs := make([]string, 0, len(counts))
	for m := range counts {
		msgs = append(msgs, m)
	}
	sort.Strings(msgs)
	var failures []string
	for _, m := range msgs {
		n := counts[m]
		e, ok := allow[m]
		switch {
		case !ok:
			failures = append(failures, fmt.Sprintf("%dx unlisted: %q", n, m))
		case n > e.MaxCount:
			failures = append(failures, fmt.Sprintf("%dx %q exceeds max_count %d (%s)", n, m, e.MaxCount, e.Reason))
		}
	}
	return failures
}
