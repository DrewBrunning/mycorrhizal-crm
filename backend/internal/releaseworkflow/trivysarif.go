package releaseworkflow

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// Issue #1702: aquasecurity/trivy-action's entrypoint.sh unsets TRIVY_SEVERITY
// in SARIF mode unless `limit-severities-for-sarif: 'true'` is set, so a step
// with `format: sarif` + `severity:` + `exit-code: 1` silently gates on a
// finding of ANY severity (the v1.4.2 publish was blocked by one MEDIUM). The
// failure is quiet -- the step still runs and still looks scoped -- so
// CheckTrivySARIFSeverity pins it for every workflow.

type trivyWorkflow struct {
	Jobs map[string]struct {
		Steps []struct {
			Name string         `yaml:"name"`
			Uses string         `yaml:"uses"`
			With map[string]any `yaml:"with"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// CheckTrivySARIFSeverity returns a violation per trivy-action step in the
// given workflow text that sets format sarif and a severity but not
// limit-severities-for-sarif: true.
func CheckTrivySARIFSeverity(file, text string) []string {
	var w trivyWorkflow
	if err := yaml.Unmarshal([]byte(text), &w); err != nil {
		return []string{fmt.Sprintf("%s: unparseable workflow: %v", file, err)}
	}
	var out []string
	for jobID, job := range w.Jobs {
		for _, st := range job.Steps {
			if !strings.HasPrefix(st.Uses, "aquasecurity/trivy-action@") {
				continue
			}
			if strings.ToLower(fmt.Sprint(st.With["format"])) != "sarif" {
				continue
			}
			if st.With["severity"] == nil {
				continue
			}
			if strings.ToLower(fmt.Sprint(st.With["limit-severities-for-sarif"])) != "true" {
				out = append(out, fmt.Sprintf("%s: job %q step %q: trivy-action format sarif with severity needs limit-severities-for-sarif: 'true' (#1702)", file, jobID, st.Name))
			}
		}
	}
	sort.Strings(out)
	return out
}

// CheckAllTrivySARIFSeverity applies CheckTrivySARIFSeverity to every
// workflow under dir and reports how many files use trivy-action.
func CheckAllTrivySARIFSeverity(dir string) ([]string, int, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		return nil, 0, err
	}
	var out []string
	scanned := 0
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // G304: f comes from Glob over the caller-supplied workflows dir
		if err != nil {
			return nil, 0, err
		}
		if strings.Contains(string(b), "aquasecurity/trivy-action@") {
			scanned++
		}
		out = append(out, CheckTrivySARIFSeverity(filepath.Base(f), string(b))...)
	}
	return out, scanned, nil
}
