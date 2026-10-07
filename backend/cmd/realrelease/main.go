// Command realrelease is the issue #1489 harness: it proves an upgrade against
// data written by the REAL old release, not a transplant.
//
//	realrelease seed    --base-url URL --out DIR
//	    Drive a RUNNING published release (docker image vX.Y.Z) through its
//	    public API into a representative state, enrol 2FA, mint an API token,
//	    read everything back, and write DIR/credentials.json and
//	    DIR/pre-snapshot.json. Synthetic data only.
//
//	realrelease capture --base-url URL --creds FILE --out FILE
//	    Log in as the seeded account on a running instance and write a fresh
//	    read-back (used by the rollback leg, where the OLD release is started
//	    again over the restored pre-migration backup).
//
//	realrelease verify  --db FILE --creds FILE --pre FILE [--out FILE]
//	    Upgrade the data directory the old release left behind by booting the
//	    CURRENT server over it, then log in, read back, compare against the
//	    pre-upgrade snapshot, and run the doctor/audit-chain checks. Exit 1 on
//	    any difference or finding.
//
//	realrelease compare --pre FILE --post FILE
//	    Compare two snapshots (exit 1 on any difference).
//
// Never point any subcommand at a real instance: seed registers an account
// with a published throwaway password and writes synthetic records.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"mycorrhizal/internal/realrelease"
)

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr)) // # pragma: no cover — os.Exit terminates; tests call runCLI directly
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: realrelease seed|capture|verify|compare|print-secret [flags]")
		return 2
	}
	var err error
	switch args[0] {
	case "seed":
		err = runSeed(args[1:], stdout)
	case "capture":
		err = runCapture(args[1:], stdout)
	case "verify":
		err = runVerify(args[1:], stdout)
	case "compare":
		err = runCompare(args[1:], stdout)
	case "print-secret":
		fmt.Fprintln(stdout, realrelease.SeedJWTSecret)
	default:
		fmt.Fprintf(stderr, "realrelease: unknown subcommand %q\n", args[0])
		return 2
	}
	var d *differences
	switch {
	case err == nil:
		return 0
	case errors.As(err, &d):
		fmt.Fprintln(stderr, d.Error())
		return 1
	default:
		fmt.Fprintln(stderr, "realrelease:", err)
		return 2
	}
}

// differences is the "ran fine, found problems" outcome (exit 1), as opposed
// to an operational failure (exit 2).
type differences struct{ lines []string }

func (d *differences) Error() string {
	out := fmt.Sprintf("FAIL: %d finding(s)\n", len(d.lines))
	for _, l := range d.lines {
		out += "  - " + l + "\n"
	}
	return out
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path) // #nosec G304 -- operator-supplied harness path
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func runSeed(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	baseURL := fs.String("base-url", "", "base URL of the running OLD release (e.g. http://127.0.0.1:7390)")
	out := fs.String("out", "", "directory to write credentials.json and pre-snapshot.json into")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *baseURL == "" || *out == "" {
		return errors.New("seed needs --base-url and --out")
	}
	if err := os.MkdirAll(*out, 0o750); err != nil {
		return err
	}
	ctx := context.Background()
	c := realrelease.NewClient(*baseURL, nil)
	creds, err := realrelease.Seed(ctx, c)
	if err != nil {
		return err
	}
	if err := realrelease.EnableTwoFactor(ctx, c, creds); err != nil {
		return err
	}
	snap, err := realrelease.Capture(ctx, c)
	if err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(*out, "credentials.json"), creds); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(*out, "pre-snapshot.json"), snap); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "seeded %s: %d contacts read back\n", *baseURL, listLen(snap["contacts"]))
	return nil
}

func listLen(v any) int {
	l, _ := v.([]any)
	return len(l)
}

func runCapture(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("capture", flag.ContinueOnError)
	baseURL := fs.String("base-url", "", "base URL of the running instance")
	credsPath := fs.String("creds", "", "credentials.json written by seed")
	out := fs.String("out", "", "snapshot file to write")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *baseURL == "" || *credsPath == "" || *out == "" {
		return errors.New("capture needs --base-url, --creds and --out")
	}
	var creds realrelease.Credentials
	if err := readJSON(*credsPath, &creds); err != nil {
		return err
	}
	ctx := context.Background()
	c := realrelease.NewClient(*baseURL, nil)
	if err := realrelease.Login(ctx, c, &creds); err != nil {
		return err
	}
	snap, err := realrelease.Capture(ctx, c)
	if err != nil {
		return err
	}
	if err := writeJSON(*out, snap); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "captured %s: %d contacts\n", *baseURL, listLen(snap["contacts"]))
	return nil
}

func runVerify(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	db := fs.String("db", "", "SQLite file the old release wrote")
	photos := fs.String("photos", "", "PROFILE_PHOTO_DIR of the old install (default: beside the database)")
	attach := fs.String("attachments", "", "ATTACHMENTS_DIR of the old install (default: beside the database)")
	credsPath := fs.String("creds", "", "credentials.json written by seed")
	prePath := fs.String("pre", "", "pre-snapshot.json written by seed")
	out := fs.String("out", "", "optional file to write the post-upgrade snapshot to")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *db == "" || *credsPath == "" || *prePath == "" {
		return errors.New("verify needs --db, --creds and --pre")
	}
	var creds realrelease.Credentials
	if err := readJSON(*credsPath, &creds); err != nil {
		return err
	}
	var pre realrelease.Snapshot
	if err := readJSON(*prePath, &pre); err != nil {
		return err
	}
	res, err := realrelease.Verify(context.Background(), realrelease.VerifyOptions{
		DBPath: *db, PhotoDir: *photos, AttachDir: *attach, Creds: &creds, Pre: pre,
		Logf: func(f string, a ...any) { fmt.Fprintf(stdout, f+"\n", a...) },
	})
	if err != nil {
		return err
	}
	if *out != "" {
		if err := writeJSON(*out, res.Post); err != nil {
			return err
		}
	}
	if !res.OK() {
		return &differences{lines: append(append([]string{}, res.Diffs...), res.Problems...)}
	}
	fmt.Fprintln(stdout, "OK: upgrade preserved everything the old release's API reported")
	return nil
}

func runCompare(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	prePath := fs.String("pre", "", "baseline snapshot")
	postPath := fs.String("post", "", "snapshot that must still say everything the baseline did")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *prePath == "" || *postPath == "" {
		return errors.New("compare needs --pre and --post")
	}
	var pre, post realrelease.Snapshot
	if err := readJSON(*prePath, &pre); err != nil {
		return err
	}
	if err := readJSON(*postPath, &post); err != nil {
		return err
	}
	if diffs := realrelease.Compare(pre, post); len(diffs) > 0 {
		return &differences{lines: diffs}
	}
	fmt.Fprintln(stdout, "OK: snapshots agree")
	return nil
}
