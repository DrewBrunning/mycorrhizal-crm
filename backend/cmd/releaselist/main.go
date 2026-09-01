// Command releaselist prints the supported-release upgrade set
// (internal/schemafixture.SupportedReleases) as a JSON array shaped for a
// GitHub Actions matrix `include:` list.
//
// It is the single source of truth for migration-tests.yml's upgrade-matrix,
// which used to carry a hand-maintained release list that drifted out of sync
// with the registry (v0.6.4 and v0.6.5 were in SupportedReleases but missing
// from the matrix). The matrix job now derives its legs from this command, so a
// release added by the release workflow is covered automatically.
//
// Each element is {"release": "<tag>", "entry": "<human label>"}. The
// upgrade-floor release (schemafixture.FloorTag) is also the longest supported
// skip — floor -> current through the whole migration chain — so it gets a
// distinct label, matching the label the hand-written matrix used.
//
// Exit status 0 means the JSON was written to stdout; 2 means it could not be
// marshalled or written.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"mycorrhizal/internal/schemafixture"
)

func main() {
	os.Exit(run(os.Stdout)) // # pragma: no cover — os.Exit terminates the process; tests exercise run() directly
}

// matrixEntry is one GitHub Actions `strategy.matrix.include` element: the
// release tag the leg tests, and the display name it runs under.
type matrixEntry struct {
	Release string `json:"release"`
	Entry   string `json:"entry"`
}

// run writes the matrix JSON to w and returns the process exit code.
func run(w io.Writer) int {
	entries := make([]matrixEntry, 0, len(schemafixture.SupportedReleases))
	for _, r := range schemafixture.SupportedReleases {
		label := r.Tag + " → current"
		if r.Tag == schemafixture.FloorTag {
			label = r.Tag + " → current (longest supported skip)"
		}
		entries = append(entries, matrixEntry{Release: r.Tag, Entry: label})
	}

	out, err := json.Marshal(entries)
	if err != nil {
		// # pragma: no cover — a slice of string-only structs always marshals.
		fmt.Fprintln(os.Stderr, "releaselist:", err)
		return 2
	}
	if _, err := fmt.Fprintln(w, string(out)); err != nil {
		// # pragma: no cover — os.Stdout does not fail this write in practice;
		// the io.Writer seam exists for the tests, which pass a bytes.Buffer.
		fmt.Fprintln(os.Stderr, "releaselist:", err)
		return 2
	}
	return 0
}
