// Package faults provides in-process failure injection for deterministic
// chaos testing (issue #434).
//
// # The split-harness rule (write it down)
//
// A fault that can be expressed as an error returned across an existing
// interface is injected **in-process**, via this package, in the normal test
// suite. A fault that requires the process or the filesystem to actually
// misbehave — SIGKILL mid-migration, disk exhaustion — belongs in the
// **external-fault CI job** (.github/workflows/chaos-tests.yml). The failure
// mode of a split harness is people guessing, so this is a hard rule, not a
// suggestion: if you can return an error from a seam, inject it in-process.
//
// # How a fault is armed
//
// Faults are inert unless armed; an unarmed Hook is a single read-lock map
// lookup that returns nil. Arming happens two ways:
//
//   - Programmatically, from the test suite: ArmError / ArmPause, paired with
//     Disarm / Reset in t.Cleanup.
//
//   - Environment-driven, for the external-fault job: the MYCORRHIZAL_FAULTS
//     variable (parsed once at package init) arms faults in a subprocess the
//     test cannot reach from inside. Format:
//
//     MYCORRHIZAL_FAULTS=fault1,fault2:err:message,fault3:pause:5s
//
//     A bare name arms an error fault with a standard message; `:err:<text>`
//     arms one with custom text; `:pause:<duration>` arms a pause fault that
//     blocks for the duration (the deterministic SIGKILL window the external
//     job needs), logging a marker the harness greps for.
//
// # How a seam is placed
//
// Production code checks a fault at a real boundary — the migration driver's
// per-statement Run, an import confirm's transaction, an integration client's
// request path — with a single Hook call, and returns the armed error
// unchanged. The caller's existing error path (and its defined outcome) is
// exactly what the injection test asserts, which is what makes the hand-verify
// step from CLAUDE.md meaningful: remove the recovery branch and the injection
// test fails.
//
// Every fault name is documented in docs/development/fault-injection.md, which
// is the table the v0.8.0 adversarial audit (issue #500) reviews.
package faults

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"mycorrhizal/logger"
)

// envVar names the environment variable the external-fault CI job uses to arm
// faults in a subprocess. Parsed once at init; see the package doc for syntax.
const envVar = "MYCORRHIZAL_FAULTS"

// DefaultErrorText is the message a bare-name fault entry (or an ArmError with
// a nil error) carries.
const DefaultErrorText = "injected fault"

// ErrInjected is the error every armed error fault returns (possibly wrapped
// by the arm site). errors.Is against it is the portable way to assert "the
// fault fired" without knowing the arm site's own error vocabulary.
type ErrInjected struct {
	// Name is the fault's registered name.
	Name string
	// Message is the text from the `:err:<text>` entry, or DefaultErrorText.
	Message string
}

func (e *ErrInjected) Error() string {
	return fmt.Sprintf("%s: %s", e.Message, e.Name)
}

// Is makes errors.Is(err, &ErrInjected{Name: n}) match any ErrInjected with the
// same Name regardless of Message — the portable "the fault named n fired"
// assertion. Matching on Name only is deliberate: a site's own wrap of the
// fault must not change whether errors.Is sees it.
func (e *ErrInjected) Is(target error) bool {
	t, ok := target.(*ErrInjected)
	if !ok {
		return false
	}
	return t.Name == e.Name
}

type kind int

const (
	kindError kind = iota
	kindPause
)

// fault is one armed fault. Only the fields relevant to its kind are set.
type fault struct {
	name string
	kind kind
	err  error
	dur  time.Duration
}

var (
	mu   sync.RWMutex
	armd = make(map[string]fault)
)

// Hook checks whether the named fault is armed and, if so, fires it: an error
// fault returns its error, a pause fault blocks for its configured duration
// (logging a marker first) and then returns nil. An unarmed fault returns nil.
// Callers MUST check the returned error and route it through their normal
// error path — that path is what the injection test asserts.
func Hook(name string) error {
	mu.RLock()
	f, armed := armd[name]
	mu.RUnlock()
	if !armed {
		return nil
	}
	if f.kind == kindPause {
		// Warn, not Debug: the external-fault job greps this marker to know
		// the process has reached the seam and is pausable (then SIGKILLs it).
		logger.Warn().Str("fault", name).Str("for", f.dur.String()).Msg("injected fault pause")
		time.Sleep(f.dur)
		return nil
	}
	logger.Debug().Str("fault", name).Msg("injected fault fired")
	return f.err
}

// Enabled reports whether the named fault is currently armed.
func Enabled(name string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, ok := armd[name]
	return ok
}

// ArmError arms name as an error fault returning err. A nil err arms the
// default ErrInjected. Hook(name) returns it until Disarm/Reset.
func ArmError(name string, err error) {
	if err == nil {
		err = &ErrInjected{Name: name, Message: DefaultErrorText}
	}
	mu.Lock()
	defer mu.Unlock()
	armd[name] = fault{name: name, kind: kindError, err: err}
}

// ArmPause arms name as a pause fault that blocks for d at Hook(name), logging
// a marker first. This is the seam the external-fault job interrupts with
// SIGKILL. A non-positive duration arms nothing, so a misconfigured harness
// cannot silently hang a job.
func ArmPause(name string, d time.Duration) {
	if d <= 0 {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	armd[name] = fault{name: name, kind: kindPause, dur: d}
}

// Disarm removes the named fault.
func Disarm(name string) {
	mu.Lock()
	defer mu.Unlock()
	delete(armd, name)
}

// Reset disarms every fault, programmatic and env-armed alike. Tests call it in
// t.Cleanup so an armed fault cannot leak into an unrelated test.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	armd = make(map[string]fault)
}

// List returns the names of all armed faults, sorted, for diagnostics and the
// fault catalog's drift check.
func List() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(armd))
	for name := range armd {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// SetFromEnv parses MYCORRHIZAL_FAULTS and arms every entry it names, on top
// of whatever is already armed (a later entry wins for a repeated name). It is
// called once at package init; it is exported so tests can exercise the parser
// and the external job's exact arming path.
func SetFromEnv() error {
	raw := os.Getenv(envVar)
	if raw == "" {
		return nil
	}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, spec, err := parseEntry(entry)
		if err != nil {
			return fmt.Errorf("MYCORRHIZAL_FAULTS: %w", err)
		}
		switch spec.kind {
		case kindError:
			ArmError(name, spec.err)
		case kindPause:
			ArmPause(name, spec.dur)
		}
	}
	return nil
}

// spec is the parsed form of one MYCORRHIZAL_FAULTS entry.
type spec struct {
	kind kind
	err  error
	dur  time.Duration
}

// parseEntry parses one `name` / `name:err:message` / `name:pause:duration`
// entry. The message may itself contain colons; the duration may not.
func parseEntry(entry string) (string, spec, error) {
	head, tail, hasTail := strings.Cut(entry, ":")
	head = strings.TrimSpace(head)
	if head == "" {
		return "", spec{}, fmt.Errorf("fault entry %q has no name", entry)
	}
	if !hasTail {
		return head, spec{kind: kindError, err: &ErrInjected{Name: head, Message: DefaultErrorText}}, nil
	}
	kindToken, rest, _ := strings.Cut(tail, ":")
	switch strings.TrimSpace(kindToken) {
	case "err":
		return head, spec{kind: kindError, err: &ErrInjected{Name: head, Message: strings.TrimSpace(rest)}}, nil
	case "pause":
		d, err := time.ParseDuration(strings.TrimSpace(rest))
		if err != nil {
			return "", spec{}, fmt.Errorf("fault entry %q: bad pause duration %q: %w", entry, rest, err)
		}
		return head, spec{kind: kindPause, dur: d}, nil
	default:
		return "", spec{}, fmt.Errorf("fault entry %q: unknown kind %q (want err or pause)", entry, kindToken)
	}
}

func init() {
	if err := SetFromEnv(); err != nil {
		// A malformed MYCORRHIZAL_FAULTS must not fail a boot in production —
		// the variable is a test/ops harness input. Log a warning instead; the
		// external job's own timeout is what surfaces a fault that never fired.
		// Structurally untestable in-process: init() runs before any test can
		// set the env var, and SetFromEnv's own error path is covered directly.
		logger.Warn().Err(err).Msg("ignoring malformed MYCORRHIZAL_FAULTS") // # pragma: no cover
	}
}
