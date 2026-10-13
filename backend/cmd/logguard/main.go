// Command logguard fails a CI job when the server logged an unexpected
// warning or error (issue #1474).
//
// A passing E2E run can hide a misbehaving server: issue #1471 shipped 698
// FK-failing audit WARN lines inside a green run and was only found by a
// person reading the log. logguard turns that into a mechanical gate. It reads
// the backend's structured (zerolog JSON) log from stdin -- typically
// `docker compose logs --no-log-prefix mycorrhizal` -- and fails if any line
// at level warn or above is not matched by the committed, reason-bearing
// allowlist (allowlist.json), or if an allowlist entry matches more lines than
// its max_count.
//
// An entry matches on `message` and, optionally, on other JSON fields of the
// line ("fields": RE2 full-match regexps, e.g. path/status), so a generic
// message such as "HTTP request" is only excused on the routes that
// legitimately produce it.
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
	"regexp"
	"sort"
	"strings"
)

// entry is one allowlist row. Reason is mandatory: an allowlist entry with no
// recorded reason is a silent weakening of the gate (CLAUDE.md "Gates are not
// silently weakened").
//
// Fields optionally scopes the entry to log lines whose other JSON fields match
// (e.g. {"path": "...", "status": "503"}). Each value is an RE2 regular
// expression that must match the WHOLE stringified field value (the parser
// anchors it; numbers compare as their decimal text). A line carrying the
// entry's message but a different -- or missing -- field value does not match
// the entry and is therefore unexpected. An entry with no Fields matches on
// message alone. When several entries match a line the first one in file
// order takes it.
type entry struct {
	Message  string            `json:"message"`
	Fields   map[string]string `json:"fields,omitempty"`
	MaxCount int               `json:"max_count"`
	Reason   string            `json:"reason"`

	matchers map[string]*regexp.Regexp
}

// matches reports whether a record's message and fields satisfy the entry.
func (e *entry) matches(r record) bool {
	if r.msg != e.Message {
		return false
	}
	for k, re := range e.matchers {
		v, ok := r.fields[k]
		if !ok || !re.MatchString(v) {
			return false
		}
	}
	return true
}

// key identifies an entry for duplicate detection and reporting.
func (e *entry) key() string {
	if len(e.Fields) == 0 {
		return fmt.Sprintf("%q", e.Message)
	}
	ks := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	parts := make([]string, len(ks))
	for i, k := range ks {
		parts[i] = k + "=" + e.Fields[k]
	}
	return fmt.Sprintf("%q{%s}", e.Message, strings.Join(parts, ", "))
}

// record is one parsed log line.
type record struct {
	level, msg string
	fields     map[string]string
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
	recs, err := scan(in)
	if err != nil {
		return fmt.Errorf("read log: %w", err)
	}
	failures := evaluate(recs, allow)
	if len(failures) > 0 {
		return fmt.Errorf("%d unexpected warn/error log finding(s) (of %d warn+ lines):\n  %s\n"+
			"fix the cause, or add a reason-bearing entry to %s",
			len(failures), len(recs), strings.Join(failures, "\n  "), allowlistPath)
	}
	fmt.Fprintf(out, "OK: %d warn+ log line(s), all allowlisted within max_count.\n", len(recs))
	return nil
}

func parseAllowlist(raw []byte) ([]*entry, error) {
	var rows []*entry
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rows); err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(rows))
	for i, r := range rows {
		if r == nil {
			return nil, fmt.Errorf("entry %d: null entry", i)
		}
		switch {
		case r.Message == "":
			return nil, fmt.Errorf("entry %d: empty message", i)
		case strings.TrimSpace(r.Reason) == "":
			return nil, fmt.Errorf("entry %q: reason is required", r.Message)
		case r.MaxCount < 1:
			return nil, fmt.Errorf("entry %q: max_count must be >= 1", r.Message)
		}
		r.matchers = make(map[string]*regexp.Regexp, len(r.Fields))
		for k, pat := range r.Fields {
			if strings.TrimSpace(k) == "" {
				return nil, fmt.Errorf("entry %q: empty field name", r.Message)
			}
			if k == "message" || k == "level" {
				return nil, fmt.Errorf("entry %q: field %q is not scopable (matched by the entry itself)", r.Message, k)
			}
			if pat == "" {
				return nil, fmt.Errorf("entry %q: field %q has an empty matcher", r.Message, k)
			}
			re, err := regexp.Compile("^(?:" + pat + ")$")
			if err != nil {
				return nil, fmt.Errorf("entry %q: field %q: bad regexp: %w", r.Message, k, err)
			}
			r.matchers[k] = re
		}
		if seen[r.key()] {
			return nil, fmt.Errorf("entry %s: duplicate", r.key())
		}
		seen[r.key()] = true
	}
	return rows, nil
}

// scan collects every warn+ record.
func scan(in io.Reader) ([]record, error) {
	var recs []record
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		r, ok := parseRecord(sc.Text())
		if !ok || levelRank[r.level] < warnRank {
			continue
		}
		recs = append(recs, r)
	}
	return recs, sc.Err()
}

// parseLine extracts level and message from a zerolog JSON line.
func parseLine(line string) (level, msg string, ok bool) {
	r, ok := parseRecord(line)
	return r.level, r.msg, ok
}

// parseRecord extracts level, message and the stringified remaining fields
// from a zerolog JSON line. It takes the FIRST occurrence of each key: the
// startup "Logger initialized" line carries a second, config-valued `level`
// field that must not shadow the severity. ok is false when there is no level.
func parseRecord(line string) (record, bool) {
	r := record{fields: map[string]string{}}
	i := strings.IndexByte(line, '{')
	if i < 0 {
		return record{}, false
	}
	dec := json.NewDecoder(strings.NewReader(line[i:]))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return record{}, false
	}
	var haveLevel, haveMsg bool
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return record{}, false
		}
		key, _ := kt.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return record{}, false
		}
		var s string
		isStr := json.Unmarshal(v, &s) == nil
		switch {
		case key == "level" && !haveLevel:
			if isStr {
				r.level, haveLevel = s, true
			}
		case key == "message" && !haveMsg:
			if isStr {
				r.msg, haveMsg = s, true
			}
		case key == "level" || key == "message":
			// later duplicate: ignored
		default:
			if _, dup := r.fields[key]; dup {
				continue
			}
			if isStr {
				r.fields[key] = s
			} else {
				r.fields[key] = strings.TrimSpace(string(v))
			}
		}
	}
	return r, haveLevel
}

// evaluate matches every record to the first allowlist entry it satisfies,
// then checks each entry's count against max_count. A record no entry matches
// is reported with its method/path/status (when it has them) so a scoped
// message that surfaced on the wrong route is visible.
func evaluate(recs []record, allow []*entry) []string {
	used := make([]int, len(allow))
	unlisted := map[string]int{}
	for _, r := range recs {
		matched := false
		for i, e := range allow {
			if e.matches(r) {
				used[i]++
				matched = true
				break
			}
		}
		if !matched {
			desc := fmt.Sprintf("%q", r.msg)
			var extra []string
			for _, k := range []string{"method", "path", "status"} {
				if v, ok := r.fields[k]; ok {
					extra = append(extra, k+"="+v)
				}
			}
			if len(extra) > 0 {
				desc += " (" + strings.Join(extra, " ") + ")"
			}
			unlisted[desc]++
		}
	}
	descs := make([]string, 0, len(unlisted))
	for d := range unlisted {
		descs = append(descs, d)
	}
	sort.Strings(descs)
	var failures []string
	for _, d := range descs {
		failures = append(failures, fmt.Sprintf("%dx unlisted: %s", unlisted[d], d))
	}
	for i, e := range allow {
		if used[i] > e.MaxCount {
			failures = append(failures, fmt.Sprintf("%dx %s exceeds max_count %d (%s)", used[i], e.key(), e.MaxCount, e.Reason))
		}
	}
	return failures
}
