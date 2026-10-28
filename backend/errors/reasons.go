package errors

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"mycorrhizal/logger"
)

// ReasonCode is a stable, machine-readable error reason (ADR 0038, issue
// #2095). Clients localize from it; `message` stays English.
type ReasonCode string

type reasonSpec struct {
	// Template is the English rendering; placeholders are {name}.
	Template string
	// Params is the declared param order. Android positional args (%N$s) are
	// numbered in this order.
	Params []string
}

// reasonAreas is the closed set of first segments a reason code may use. A
// new area is added here in the same PR as its first code.
var reasonAreas = []string{
	"common", "auth", "account", "admin", "import", "meerkat", "monica",
	"bundle", "csv", "vcard", "integration", "immich", "seafile", "paperless",
	"geopulse", "webdav", "calendar", "feed", "notification", "contact",
	"household", "relationship", "occasion", "photo", "attachment", "validation",
}

// Reason constants. Each must be a key of reasonRegistry (and vice versa);
// TestEveryReasonConstantRegistered enforces both directions.
const (
	// ReasonCommonTestOnly exists for tests only and is removed in #2100.
	ReasonCommonTestOnly ReasonCode = "common_test_only"
)

var reasonRegistry = map[ReasonCode]reasonSpec{
	ReasonCommonTestOnly: {Template: "Test {n}", Params: []string{"n"}},
}

var reasonPlaceholder = regexp.MustCompile(`\{([^{}]*)\}`)

// WithReason attaches a reason code and params and sets Message and
// Details["reason"] to the rendered English template. If the code is unknown
// or the params do not match the registered names exactly, the error is left
// untouched (clients fall back to English) and an info line is logged; a
// request path never panics over this.
func (e *AppError) WithReason(code ReasonCode, params map[string]any) *AppError {
	spec, ok := reasonRegistry[code]
	if !ok || !paramsMatch(spec.Params, params) {
		logger.Logger.Info().
			Str(logger.FieldEvent, "reason_params_mismatch").
			Str(logger.FieldReasonCode, string(code)).
			Msg("reason params do not match registry; sending error without reason")
		return e
	}
	rendered := reasonPlaceholder.ReplaceAllStringFunc(spec.Template, func(m string) string {
		return fmt.Sprint(params[m[1:len(m)-1]])
	})
	e.ReasonCode = string(code)
	e.ReasonParams = params
	e.Message = rendered
	e.WithDetails("reason", rendered)
	return e
}

func paramsMatch(names []string, params map[string]any) bool {
	if len(names) != len(params) {
		return false
	}
	for _, n := range names {
		if _, ok := params[n]; !ok {
			return false
		}
	}
	return true
}

// ReasonCodes returns every registered code, sorted. Client drift tests
// consume it.
func ReasonCodes() []ReasonCode {
	out := make([]ReasonCode, 0, len(reasonRegistry))
	for c := range reasonRegistry {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return strings.Compare(string(out[i]), string(out[j])) < 0 })
	return out
}

// ReasonSpecFor returns the English template and declared param order.
func ReasonSpecFor(code ReasonCode) (template string, params []string, ok bool) {
	spec, ok := reasonRegistry[code]
	if !ok {
		return "", nil, false
	}
	return spec.Template, append([]string(nil), spec.Params...), true
}
