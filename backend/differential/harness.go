package differential

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// Reference availability + invocation
// ---------------------------------------------------------------------------

// pyRef is the Python-vobject reference CLI. It is resolved at test time;
// see NewPyRef.
type pyRef struct {
	python string // resolved python3 executable
}

// NewPyRef probes for a usable vobject reference: a `python3` on PATH whose
// vobject import works (the pinned version is documented in
// docs/development/testing.md). A zero pyRef means "not available" — tests
// t.Skip with the reason rather than fail, so `go test ./...` stays green on
// machines without the reference, while CI installs it and the leg runs for
// real (the availability probe itself is pinned by a dedicated test).
func NewPyRef() (pyRef, string) {
	python, err := exec.LookPath("python3")
	if err != nil {
		return pyRef{}, "vobject reference unavailable: python3 not on PATH (install it or run the differential CI leg)"
	}
	probe := exec.Command(python, "-c", "import vobject") // #nosec G204 -- `python` is the path exec.LookPath resolved for the system python3 binary, never request/input-controlled; this is the reference-availability probe
	if err := probe.Run(); err != nil {
		return pyRef{}, "vobject reference unavailable: `python3 -c \"import vobject\"` failed (pip install vobject==0.9.9, see docs/development/testing.md)"
	}
	return pyRef{python: python}, ""
}

// scriptPath resolves the checked-in reference CLI relative to this package.
func scriptPath() string {
	return filepath.Join("reference", "vobject", "vcard_ref.py")
}

// toFormat runs neutral Record JSON -> vCard bytes through the reference.
func (r pyRef) toFormat(rec []byte, version string) ([]byte, error) {
	args := []string{scriptPath(), "--to-format", version}
	return r.run(args, rec)
}

// toNeutral runs vCard bytes -> neutral Record JSON through the reference.
func (r pyRef) toNeutral(raw []byte) ([]byte, error) {
	return r.run([]string{scriptPath(), "--to-neutral"}, raw)
}

func (r pyRef) run(args []string, stdin []byte) ([]byte, error) {
	// #nosec G204 -- args is always the fixed checked-in script path plus a
	// fixed subcommand (see toFormat/toNeutral), never request-controlled
	cmd := exec.Command(r.python, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("vobject reference %s failed: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return out, nil
}

// calcardRef is the Rust calcard reference CLI. It is resolved by
// NewCalcardRef (env override -> prebuilt binary); a zero calcardRef means
// "not available" and the JSContact leg skips.
type calcardRef struct {
	argv []string // base argv (executable + fixed flags); the subcommand is appended
}

// calcardEnvVar names the env var that overrides the calcard command. CI's
// scheduled differential job sets it to a docker-run invocation or a built
// binary path; local developers can set it to their own built binary.
const calcardEnvVar = "MYCORRHIZAL_CALCARD_CMD"

// NewCalcardRef resolves the calcard reference command. Resolution order:
//
//  1. $MYCORRHIZAL_CALCARD_CMD (a command line, split on spaces — CI sets it
//     to e.g. "docker run --rm -i <image>" or a binary path);
//  2. a prebuilt binary at reference/calcard/target/release/calcard-ref
//     (from `cargo build --release --locked` in that directory).
//
// The reference is deliberately NOT auto-built: building Rust from a Go test
// is surprising and slow; the scheduled CI job builds it explicitly.
func NewCalcardRef() (calcardRef, string) {
	if raw := os.Getenv(calcardEnvVar); raw != "" {
		return calcardRef{argv: strings.Fields(raw)}, ""
	}
	for _, dir := range []string{
		filepath.Join("reference", "calcard", "target", "release", "calcard-ref"),
		filepath.Join("reference", "calcard", "target", "debug", "calcard-ref"),
	} {
		if _, err := os.Stat(dir); err == nil {
			return calcardRef{argv: []string{dir}}, ""
		}
	}
	return calcardRef{}, fmt.Sprintf("calcard reference unavailable: set %s or build reference/calcard with `cargo build --release --locked`", calcardEnvVar)
}

// run executes the reference with subcommand + args, feeding stdin.
func (r calcardRef) run(subcommand string, stdin []byte) ([]byte, error) {
	argv := append(append([]string(nil), r.argv...), subcommand)
	// #nosec G204 -- argv is the operator-set $MYCORRHIZAL_CALCARD_CMD (CI's
	// built binary / docker invocation) or a fixed prebuilt-binary path, plus a
	// fixed subcommand token — the same operator-controlled posture as
	// cmd/schemagate's report path, never request-controlled
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = bytes.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("calcard reference %s failed: %w: %s", subcommand, err, stderr.String())
	}
	return out, nil
}
