// Command gennotices regenerates THIRD_PARTY_NOTICES.md from the dependencies
// that actually ship in the container image (issue #1880).
//
// Run from the backend directory:
//
//	go run ./cmd/gennotices
//	go run ./cmd/gennotices -check
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"mycorrhizal/internal/notices"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type collector func(context.Context, string) ([]notices.Dependency, error)

func run(args []string, stdout, stderr io.Writer) int {
	return runWith(args, stdout, stderr, notices.CollectGo, notices.CollectNPM)
}

func runWith(args []string, stdout, stderr io.Writer, collectGo, collectNPM collector) int {
	flags := flag.NewFlagSet("gennotices", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "..", "repository root")
	outPath := flags.String("out", "", "output file (default <root>/THIRD_PARTY_NOTICES.md)")
	check := flags.Bool("check", false, "fail if the committed notice file is stale instead of writing it")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	ctx := context.Background()
	goDeps, err := collectGo(ctx, filepath.Join(*root, "backend"))
	if err != nil {
		fmt.Fprintln(stderr, "gennotices:", err)
		return 1
	}
	npmDeps, err := collectNPM(ctx, *root)
	if err != nil {
		fmt.Fprintln(stderr, "gennotices:", err)
		return 1
	}
	deps := append(goDeps, npmDeps...)
	rendered := notices.Render(deps)

	if *outPath == "" {
		*outPath = filepath.Join(*root, "THIRD_PARTY_NOTICES.md")
	}
	if *check {
		existing, err := os.ReadFile(*outPath)
		if err != nil {
			fmt.Fprintln(stderr, "gennotices:", err)
			return 1
		}
		if string(existing) != rendered {
			fmt.Fprintf(stderr, "%s is stale: a dependency changed without regenerating the notice file.\n", *outPath)
			fmt.Fprintln(stderr, "Run `cd backend && go run ./cmd/gennotices`, review the diff, and commit it.")
			return 1
		}
		return 0
	}
	if err := os.WriteFile(*outPath, []byte(rendered), 0o644); err != nil { // #nosec G306 -- generated notice is a world-readable repository artifact
		fmt.Fprintln(stderr, "gennotices:", err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s (%d dependencies)\n", *outPath, len(deps))
	return 0
}
