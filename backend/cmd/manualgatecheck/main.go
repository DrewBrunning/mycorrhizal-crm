// Command manualgatecheck enforces the manual-gate attestation ledger
// (issue #1486, .github/manual-gates.json): the release obligations only a
// human can discharge, such as running LocalOnlyModeE2eTest on a real arm64
// device. The logic lives in internal/manualgates; this is its workflow-facing
// front.
//
//	manualgatecheck validate -ledger F
//	    Structural check of the ledger (every attestation must prove the test
//	    ran: tests>=1, skipped=0, failures=0, the required ABI, a JUnit digest).
//	manualgatecheck commits  -ledger F
//	    Print the commits of each gate's latest attestation, one per line --
//	    the input of .github/scripts/manual-gate-facts.sh.
//	manualgatecheck check    -ledger F -attest S -kind final|rc -facts F
//	                         [-now RFC3339] [-actor A] [-out F]
//	    Enforce release.yml's `attest_manual_gates` input against the ledger.
//	    On success appends `manual_gates=<json>` to -out (the $GITHUB_OUTPUT
//	    file); the JSON is what release-readiness.json records.
//	manualgatecheck status   -ledger F -kind final|rc -facts F [-now RFC3339]
//	    Local convenience: treat every applicable gate as attested and say which
//	    would pass -- "is my attestation still good for the next release?".
//
// Exit 0: satisfied. Exit 1: at least one finding. Exit 2: usage / unreadable input.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mycorrhizal/internal/manualgates"
)

// osExit is os.Exit through a seam so tests can drive main() itself.
var osExit = os.Exit

// now is the clock seam.
var now = time.Now

type (
	readFileFn  func(string) ([]byte, error)
	writeFileFn func(string, []byte, os.FileMode) error
)

func main() {
	osExit(run(os.Args[1:], os.Stdout, os.Stderr, os.ReadFile, appendFile))
}

// appendFile appends to the file (GITHUB_OUTPUT is append-only by contract).
func appendFile(name string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, perm) // #nosec G304 G703 -- the operator/workflow-supplied $GITHUB_OUTPUT path, never request input
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func run(args []string, stdout, stderr io.Writer, read readFileFn, write writeFileFn) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: manualgatecheck <validate|commits|check|status> [flags]")
		return 2
	}
	switch args[0] {
	case "validate", "commits", "check", "status":
	default:
		fmt.Fprintf(stderr, "manualgatecheck: unknown subcommand %q\n", args[0])
		return 2
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	ledgerPath := fs.String("ledger", ".github/manual-gates.json", "the manual-gates ledger")
	attest := fs.String("attest", "", "the attest_manual_gates dispatch input")
	kind := fs.String("kind", "", "release kind: final or rc")
	factsPath := fs.String("facts", "", "git facts JSON from manual-gate-facts.sh")
	nowFlag := fs.String("now", "", "override the clock (RFC3339); for tests")
	actor := fs.String("actor", "", "the dispatching GitHub actor")
	root := fs.String("root", ".", "repository root that retained evidence paths resolve against")
	out := fs.String("out", "", "file to append the manual_gates=<json> output line to")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	raw, err := read(*ledgerPath)
	if err != nil {
		fmt.Fprintln(stderr, "manualgatecheck: read ledger:", err)
		return 2
	}
	ledger, findings := manualgates.ParseLedger(raw)
	if len(findings) == 0 {
		findings = manualgates.VerifyEvidence(ledger, func(p string) ([]byte, bool) {
			b, rerr := read(filepath.Join(*root, p))
			return b, rerr == nil
		})
	}
	if len(findings) > 0 {
		return report(stdout, findings)
	}

	switch args[0] {
	case "validate":
		fmt.Fprintf(stdout, "manual-gates ledger OK: %d gate(s) (%s)\n", len(ledger.Gates), *ledgerPath)
		return 0
	case "commits":
		for _, c := range ledger.Commits() {
			fmt.Fprintln(stdout, c)
		}
		return 0
	}

	if *kind != manualgates.KindFinal && *kind != manualgates.KindRC {
		fmt.Fprintf(stderr, "manualgatecheck: -kind must be %q or %q\n", manualgates.KindFinal, manualgates.KindRC)
		return 2
	}
	clock := now()
	if *nowFlag != "" {
		t, perr := time.Parse(time.RFC3339, *nowFlag)
		if perr != nil {
			fmt.Fprintln(stderr, "manualgatecheck: -now:", perr)
			return 2
		}
		clock = t
	}
	var facts manualgates.Facts
	if *factsPath != "" {
		fb, ferr := read(*factsPath)
		if ferr != nil {
			fmt.Fprintln(stderr, "manualgatecheck: read facts:", ferr)
			return 2
		}
		if jerr := json.Unmarshal(fb, &facts); jerr != nil {
			fmt.Fprintln(stderr, "manualgatecheck: facts do not parse:", jerr)
			return 2
		}
	}

	if args[0] == "status" {
		in := map[string]manualgates.Input{}
		for _, g := range ledger.Gates {
			if g.Applies(*kind) {
				in[g.ID] = manualgates.Input{}
			}
		}
		records, fnd := manualgates.Check(ledger, *kind, in, clock, facts, "")
		for _, r := range records {
			fmt.Fprintf(stdout, "%s: %s\n", r.ID, r.LedgerState)
		}
		return report(stdout, fnd)
	}

	in, fnd := manualgates.ParseAttest(*attest)
	records, more := manualgates.Check(ledger, *kind, in, clock, facts, *actor)
	if fnd = append(fnd, more...); len(fnd) > 0 {
		return report(stdout, fnd)
	}
	js, _ := json.Marshal(records) // Record holds only marshalable fields
	for _, r := range records {
		fmt.Fprintf(stdout, "manual gate `%s`: %s (%s)\n", r.ID, r.Mode, summary(r))
	}
	if *out != "" {
		if werr := write(*out, []byte("manual_gates="+string(js)+"\n"), 0o644); werr != nil {
			fmt.Fprintln(stderr, "manualgatecheck: write output:", werr)
			return 2
		}
	}
	return 0
}

func summary(r manualgates.Record) string {
	if r.Mode == manualgates.ModeSkipped {
		return "reason: " + r.Reason + "; ledger: " + r.LedgerState
	}
	return "ledger: " + r.LedgerState
}

// report prints findings as workflow error annotations. It returns 1 when
// there are any, else 0.
func report(w io.Writer, findings []string) int {
	if len(findings) == 0 {
		return 0
	}
	for _, f := range findings {
		fmt.Fprintf(w, "::error::%s\n", strings.ReplaceAll(f, "\n", " "))
	}
	return 1
}
