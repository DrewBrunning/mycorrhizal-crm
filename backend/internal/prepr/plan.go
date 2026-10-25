package prepr

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// Mode is the depth of the pre-PR run.
type Mode string

const (
	// ModeFast is the lint/build/drift subset: the same class as the
	// pre-commit hook, run over the areas the branch touches.
	ModeFast Mode = "fast"
	// ModeFull adds the area suites, coverage ratchets, and flake stress.
	ModeFull Mode = "full"
)

// ParseMode validates a --fast/--full selection.
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(s) {
	case "fast":
		return ModeFast, nil
	case "full":
		return ModeFull, nil
	default:
		return "", fmt.Errorf("unknown mode %q (want fast or full)", s)
	}
}

// Step is one planned action, tagged with the area that selected it.
type Step struct {
	Area    string
	Command Command
}

// Display is the exact command a failure summary names, or the reminder text.
func (s Step) Display() string {
	if s.Command.Reminder != "" {
		return s.Command.Reminder
	}
	if s.Command.Shell != "" {
		return s.Command.Shell
	}
	return strings.Join(s.Command.Argv, " ")
}

// Skip records a selected area with no local command, and why.
type Skip struct {
	Area   string
	Reason string
}

// Plan is the deterministic result of a changed-path selection.
type Plan struct {
	Mode      Mode
	Base      string
	Changed   []string
	Matched   []string
	Selected  []string
	Steps     []Step
	Skipped   []Skip
	Notes     []string
	Hints     []string
	Unmatched []string
}

// Build maps a changed-path list through the filters and produces the steps
// for the requested depth. It is pure: no filesystem or process access.
func Build(f *Filters, changed []string, mode Mode, base string) *Plan {
	matched := f.Select(changed)
	selectedSet := map[string]bool{}
	for _, name := range matched {
		selectedSet[name] = true
		expandImplies(selectedSet, name)
	}

	selected := make([]string, 0, len(selectedSet))
	for name := range selectedSet {
		selected = append(selected, name)
	}
	sort.Strings(selected)

	p := &Plan{Mode: mode, Base: base, Changed: changed, Matched: matched, Selected: selected}

	for _, name := range selected {
		area, ok := Lookup(name)
		if !ok {
			p.Skipped = append(p.Skipped, Skip{Area: name, Reason: "no local command is defined for this filter"})
			continue
		}
		if len(area.Implies) > 0 {
			p.Notes = append(p.Notes, fmt.Sprintf("%s implies %s", name, strings.Join(area.Implies, ", ")))
			continue
		}
		commands := area.Fast
		if mode == ModeFull {
			commands = append(append([]Command{}, commands...), area.Full...)
		}
		if len(commands) == 0 {
			reason := area.Note
			if reason == "" {
				reason = "no local command is defined for this filter"
			}
			p.Skipped = append(p.Skipped, Skip{Area: name, Reason: reason})
			continue
		}
		for _, cmd := range commands {
			p.Steps = append(p.Steps, Step{Area: name, Command: cmd})
		}
	}

	if mode == ModeFull {
		if selectedSet["backend"] && base != "" {
			p.Steps = append(p.Steps, Step{Area: "backend", Command: Command{
				Label: "backend: changed-line coverage warning (non-blocking)",
				Dir:   "backend",
				Argv:  []string{"go", "run", "./cmd/patchcov", "-profile", "coverage-cross.out", "-base", base, "-target", "95"},
				Warn:  true,
			}})
		}
		p.Steps = append(p.Steps, stressSteps(changed)...)
		p.Hints = baselineHints(changed, selectedSet)
	}

	for _, cmd := range GovernanceSteps {
		p.Steps = append(p.Steps, Step{Area: "docs", Command: cmd})
	}

	for _, name := range f.AreaNames() {
		if !selectedSet[name] {
			p.Unmatched = append(p.Unmatched, name)
		}
	}
	sort.Strings(p.Unmatched)

	return p
}

func expandImplies(set map[string]bool, name string) {
	area, ok := Lookup(name)
	if !ok {
		return
	}
	for _, implied := range area.Implies {
		if set[implied] {
			continue
		}
		set[implied] = true
		expandImplies(set, implied)
	}
}

func baselineHints(changed []string, selected map[string]bool) []string {
	want := map[string]bool{}
	for _, file := range changed {
		switch {
		case isGoTest(file):
			want["backend"] = true
		case isVitestTest(file), isPlaywrightSpec(file):
			want["frontend"] = true
		case isAndroidUnitTest(file), isAndroidInstrumentedTest(file):
			want["android"] = true
		}
	}
	var out []string
	for _, area := range []string{"backend", "frontend", "android"} {
		if want[area] && selected[area] {
			out = append(out, BaselineHints[area])
		}
	}
	return out
}

func stressSteps(changed []string) []Step {
	var steps []Step

	for _, pkg := range goStressPackages(changed) {
		dir := "backend"
		steps = append(steps, Step{Area: "backend", Command: Command{
			Label: "stress: go test " + pkg,
			Dir:   dir,
			Argv:  []string{"go", "test", "-race", "-count=10", "-shuffle=on", pkg},
		}})
	}

	for _, file := range sortedUnique(changed) {
		if isVitestTest(file) {
			rel := strings.TrimPrefix(file, "frontend/")
			steps = append(steps, Step{Area: "frontend", Command: Command{
				Label:  "stress: vitest " + rel,
				Dir:    "frontend",
				Argv:   []string{"./node_modules/.bin/vitest", "run", rel, "--sequence.shuffle"},
				Env:    []string{"CI=true"},
				Repeat: 10,
			}})
		}
		if isPlaywrightSpec(file) {
			rel := strings.TrimPrefix(file, "frontend/")
			steps = append(steps, Step{Area: "frontend", Command: Command{
				Label: "stress: playwright " + rel,
				Dir:   "frontend",
				Argv:  []string{"./node_modules/.bin/playwright", "test", rel, "--repeat-each=5"},
			}})
		}
		if isAndroidUnitTest(file) {
			module, class := androidTestTarget(file)
			if module == "" || class == "" {
				continue
			}
			steps = append(steps, Step{Area: "android", Command: Command{
				Label:  "stress: gradle " + class,
				Dir:    "android",
				Argv:   []string{"./gradlew", "--warning-mode=fail", module + ":testDebugUnitTest", "--tests", class, "--rerun-tasks"},
				Repeat: 5,
			}})
		}
		if isAndroidInstrumentedTest(file) {
			_, class := androidTestTarget(file)
			steps = append(steps, Step{Area: "android", Command: Command{
				Label:    "stress: Android instrumented test " + file,
				Reminder: fmt.Sprintf("Android instrumented test %s changed; repeat %s on the emulator (connectedAndroidTest is CI-only).", file, class),
			}})
		}
	}

	return steps
}

func goStressPackages(changed []string) []string {
	set := map[string]bool{}
	for _, file := range changed {
		if !isGoTest(file) {
			continue
		}
		dir := path.Dir(strings.TrimPrefix(file, "backend/"))
		pkg := "./" + dir
		if dir == "." {
			pkg = "./"
		}
		set[pkg] = true
	}
	out := make([]string, 0, len(set))
	for pkg := range set {
		out = append(out, pkg)
	}
	sort.Strings(out)
	return out
}

func sortedUnique(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func isGoTest(file string) bool {
	return strings.HasPrefix(file, "backend/") && strings.HasSuffix(file, "_test.go")
}

func isVitestTest(file string) bool {
	if !strings.HasPrefix(file, "frontend/") {
		return false
	}
	for _, suffix := range []string{".test.ts", ".test.tsx", ".test.js", ".test.jsx", ".test.mjs"} {
		if strings.HasSuffix(file, suffix) {
			return true
		}
	}
	return false
}

func isPlaywrightSpec(file string) bool {
	return strings.HasPrefix(file, "frontend/e2e/") && strings.HasSuffix(file, ".spec.ts")
}

func isAndroidUnitTest(file string) bool {
	return strings.HasPrefix(file, "android/") && strings.Contains(file, "/src/test/") && strings.HasSuffix(file, "Test.kt")
}

func isAndroidInstrumentedTest(file string) bool {
	return strings.HasPrefix(file, "android/") && strings.Contains(file, "/src/androidTest/") && strings.HasSuffix(file, "Test.kt")
}

// androidTestTarget derives the Gradle module task prefix (":feature:contacts")
// and the fully-qualified class name from an Android test file path.
func androidTestTarget(file string) (module, class string) {
	trimmed := strings.TrimPrefix(file, "android/")
	src := strings.Index(trimmed, "/src/")
	if src < 0 {
		return "", ""
	}
	module = ":" + strings.ReplaceAll(trimmed[:src], "/", ":")
	if module == ":" {
		return "", ""
	}

	var rest string
	switch {
	case strings.Contains(trimmed, "/kotlin/"):
		rest = trimmed[strings.Index(trimmed, "/kotlin/")+len("/kotlin/"):]
	case strings.Contains(trimmed, "/java/"):
		rest = trimmed[strings.Index(trimmed, "/java/")+len("/java/"):]
	default:
		return module, ""
	}
	class = strings.TrimSuffix(strings.ReplaceAll(rest, "/", "."), path.Ext(rest))
	return module, class
}
