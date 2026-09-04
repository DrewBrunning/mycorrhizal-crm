package perfbench

import (
	"fmt"
	"os"

	"mycorrhizal/internal/largedata"

	"gorm.io/gorm"
)

// Suite is the full output of a benchmark run: per-profile results, the
// derived cross-scale growth findings, and the profile order (smallest first).
type Suite struct {
	ResultsByProfile map[string][]Result
	Findings         []GrowthFinding
	ProfileOrder     []string
}

// RunAll measures every registered operation against each profile in order.
//
// workRoot is the parent directory for each profile's scratch dir; "" uses the
// system temp dir and removes each profile's dir after that profile finishes.
// A non-empty workRoot (a test's t.TempDir()) is left for the caller to clean.
// openMigrated is nil for a real migration run (database.InitDB).
func RunAll(profiles []largedata.Profile, workRoot string, openMigrated func(path string) (*gorm.DB, error)) (Suite, error) {
	s := Suite{ResultsByProfile: map[string][]Result{}}
	var all []Result
	for _, p := range profiles {
		dir, err := os.MkdirTemp(workRoot, "perfbench-"+p.Name+"-")
		if err != nil { // # pragma: no cover — MkdirTemp under a writable root
			return Suite{}, err
		}
		res, err := RunProfile(EnvOptions{Profile: p, WorkDir: dir, OpenMigrated: openMigrated})
		if workRoot == "" { // # pragma: no cover — only the cmd passes an empty root; tests pass t.TempDir()
			_ = os.RemoveAll(dir)
		}
		if err != nil { // # pragma: no cover — RunProfile's failure modes are all pragma'd there
			return Suite{}, fmt.Errorf("perfbench: profile %q: %w", p.Name, err)
		}
		s.ResultsByProfile[p.Name] = res
		s.ProfileOrder = append(s.ProfileOrder, p.Name)
		all = append(all, res...)
	}
	s.Findings = AnalyzeGrowth(all)
	return s, nil
}

// isAtScale reports whether p is the `large` profile or larger — the profiles
// where a SkipAtScale operation is omitted (see Operation.SkipAtScale). Keyed
// on the contact target, not the name, so a future xlarge/stress profile is
// covered without another edit.
func isAtScale(p largedata.Profile) bool {
	return p.Contacts >= largedata.Large.Contacts
}

// operationsForProfile is Registry() minus the SkipAtScale operations when p is
// the `large` profile or larger (see Operation.SkipAtScale). Every profile
// runs the full set otherwise.
func operationsForProfile(p largedata.Profile) []Operation {
	all := Registry()
	if !isAtScale(p) {
		return all
	}
	kept := make([]Operation, 0, len(all))
	for _, op := range all {
		if op.SkipAtScale {
			continue
		}
		kept = append(kept, op)
	}
	return kept
}

// Baseline builds the committed baseline for this suite.
func (s Suite) Baseline() Baseline {
	return BuildBaseline(s.ResultsByProfile, s.ProfileOrder)
}

// Report renders the human-facing markdown report for this suite.
func (s Suite) Report() string {
	return RenderReport(s.ResultsByProfile, s.ProfileOrder, s.Findings)
}
