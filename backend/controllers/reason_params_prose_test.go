package controllers

import (
	"testing"

	"mycorrhizal/internal/lint/reasonprose"
)

// requireNoReasonProse runs the reason-param prose guard over a recorded
// failure response (issue #2920, item 2). A reason param is interpolated into
// a localized template, so a value that is itself an English sentence leaves
// non-English clients reading English (the #2912 failure).
func requireNoReasonProse(t *testing.T, code string, params map[string]any) {
	t.Helper()
	if code == "" || len(params) == 0 {
		return
	}
	for _, p := range reasonprose.Check(code, params) {
		t.Errorf("reason_params[%q] for %s is free prose: %q (%s)", p.Key, p.Code, p.Value, p.Why)
	}
}
