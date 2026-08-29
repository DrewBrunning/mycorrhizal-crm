package jscontact

import (
	"strings"

	"mycorrhizal/contactmodel"
)

// hasDiagWarn reports whether a diagnostic list carries a warn for the given
// concept (the ADR-0002 degradation tests' shared check).
func hasDiagWarn(diags []contactmodel.Diagnostic, concept string) bool {
	for _, d := range diags {
		if d.Concept == concept && strings.EqualFold(d.Severity, "warn") {
			return true
		}
	}
	return false
}
