// Package logtest captures the backend's structured log during a Go test so a
// test can assert the server did not quietly misbehave (issue #1474).
//
// "An error was logged and swallowed" (audit: failed to persist, failed to
// load pre-update state, ...) is invisible to ordinary assertions: the HTTP
// response is a 200 and the only evidence is a stderr line. Capture swaps the
// global logger.Logger for an in-memory JSON sink; AssertNoWarnings fails the
// test on any warn-or-above line not explicitly allowed; Guard wires both up
// with an automatic end-of-test assertion for happy-path tests.
//
// The swap is process-global (like the ad-hoc captureTestLogger helpers it
// generalises), so it is not safe with t.Parallel in the same package.
package logtest

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"mycorrhizal/logger"
)

// Record is one captured log line.
type Record struct {
	Level   string
	Message string
	Raw     string
}

// Recorder is the in-memory sink installed by Capture.
type Recorder struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write implements io.Writer; guarded because handlers log from goroutines.
func (r *Recorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Write(p)
}

// String returns everything captured so far.
func (r *Recorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.String()
}

// Records parses every captured line.
func (r *Recorder) Records() []Record {
	var out []Record
	for _, line := range strings.Split(r.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m struct {
			Level   string `json:"level"`
			Message string `json:"message"`
		}
		if json.Unmarshal([]byte(line), &m) != nil {
			continue
		}
		out = append(out, Record{Level: m.Level, Message: m.Message, Raw: line})
	}
	return out
}

func severe(level string) bool {
	switch level {
	case "warn", "error", "fatal", "panic":
		return true
	}
	return false
}

// Warnings returns the captured warn-or-above records.
func (r *Recorder) Warnings() []Record {
	var out []Record
	for _, rec := range r.Records() {
		if severe(rec.Level) {
			out = append(out, rec)
		}
	}
	return out
}

// Capture installs the recorder as the global logger for the test's lifetime
// and restores the previous logger and level on cleanup.
func Capture(t testing.TB) *Recorder {
	t.Helper()
	rec := &Recorder{}
	oldLogger := logger.Logger
	oldLevel := zerolog.GlobalLevel()
	logger.Logger = zerolog.New(rec)
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() {
		logger.Logger = oldLogger
		zerolog.SetGlobalLevel(oldLevel)
	})
	return rec
}

// AssertNoWarnings fails the test for every captured warn-or-above line whose
// message is not exactly one of allow.
func (r *Recorder) AssertNoWarnings(t testing.TB, allow ...string) {
	t.Helper()
	allowed := make(map[string]bool, len(allow))
	for _, a := range allow {
		allowed[a] = true
	}
	for _, rec := range r.Warnings() {
		if !allowed[rec.Message] {
			t.Errorf("unexpected %s log line (issue #1474): %s", rec.Level, rec.Raw)
		}
	}
}

var (
	allowMu  sync.Mutex
	allowSet = map[string]bool{} // test names (and so their subtests) opted out
)

// Guard captures the log and, when the test finishes without having already
// failed, asserts no warn-or-above line was emitted. Use it from shared
// happy-path helpers; a test that legitimately exercises an error path calls
// AllowWarnings(t, reason) instead.
func Guard(t testing.TB) *Recorder {
	t.Helper()
	rec := Capture(t)
	t.Cleanup(func() {
		if t.Failed() || allowed(t.Name()) {
			return
		}
		rec.AssertNoWarnings(t)
	})
	return rec
}

// allowed reports whether name, or any parent test of it, opted out.
func allowed(name string) bool {
	allowMu.Lock()
	defer allowMu.Unlock()
	for {
		if allowSet[name] {
			return true
		}
		i := strings.LastIndexByte(name, '/')
		if i < 0 {
			return false
		}
		name = name[:i]
	}
}

// AllowWarnings opts t (and its subtests) out of Guard's end-of-test
// assertion, for a test that deliberately drives a path that logs a
// warn/error line. The reason is mandatory so the opt-out is reviewable (it is
// recorded via t.Log). It may be called before or after Guard registers.
func AllowWarnings(t testing.TB, reason string) {
	t.Helper()
	if strings.TrimSpace(reason) == "" {
		t.Fatal("logtest: AllowWarnings requires a reason")
		return
	}
	t.Logf("logtest: warn/error log guard disabled: %s", reason)
	name := t.Name()
	allowMu.Lock()
	allowSet[name] = true
	allowMu.Unlock()
	// Deliberately never removed: Guard's own cleanup runs after any cleanup
	// registered here (LIFO) and must still see the opt-out. Test names are
	// unique per run, so the entry cannot leak onto another test.
}
