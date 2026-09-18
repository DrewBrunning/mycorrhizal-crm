// Package adversarialdelta is the code side of the per-release adversarial
// delta obligation (issue #953).
//
// docs/security/adversarial-deltas.md is a dated ledger, one row per final
// release, recording what new security-relevant surface that release added and
// what covers it. The milestone-level adversarial passes (#500, #502) are
// one-time events; a release between them used to add a client, an integration,
// an authentication path or a persistence target with no required delta review
// at all. This package makes the review a release gate.
//
// It is deliberately coarse and over-inclusive about what counts as a
// security-relevant surface: a false positive costs one reviewed ledger row,
// while a false negative reproduces the gap this exists to close. The
// fine-grained classes each already have a mechanical owner (the authorization
// matrix, the outbound-client classification, the cascade-coverage test and the
// session-minting-route gate — see asvs-l2-verification-report.md §9); this
// gate is the per-release backstop for a *new class* none of those anticipated.
package adversarialdelta

import (
	"fmt"
	"sort"
	"strings"
)

// LedgerFile is the per-release delta ledger, repo-root-relative.
const LedgerFile = "docs/security/adversarial-deltas.md"

// Surface classes the gate recognises. They are the tokens a ledger row must
// name when a release's diff touched that class.
const (
	SurfaceRoute          = "route"
	SurfacePersistence    = "persistence"
	SurfaceOutboundClient = "outbound-client"
	SurfaceAuthPath       = "auth-path"
)

// Verdict is the outcome of evaluating one release against the ledger.
type Verdict struct {
	// Surfaces are the security-relevant surface classes the release diff
	// touched, sorted.
	Surfaces []string
	// RowFound reports whether the ledger has a row for the release.
	RowFound bool
	// Missing lists surface classes the release touched but the ledger row for
	// it does not name.
	Missing []string
}

// OK reports whether the release satisfies the obligation: either it touched no
// recognised surface class, or its ledger row names every class it did touch.
func (v Verdict) OK() bool {
	return len(v.Surfaces) == 0 || (v.RowFound && len(v.Missing) == 0)
}

// Evaluate inspects the changed paths of a release and its ledger and returns
// the verdict. A ledger that does not parse yields the same verdict as an empty
// one — the release is missing its row — never a crash.
func Evaluate(ledger []byte, changedPaths []string, release string) Verdict {
	surfaces := DetectSurfaces(changedPaths)
	v := Verdict{Surfaces: surfaces}
	if len(surfaces) == 0 {
		return v
	}
	row, ok := parseRows(ledger)[release]
	if !ok {
		return v
	}
	v.RowFound = true
	for _, class := range surfaces {
		if !strings.Contains(row, class) {
			v.Missing = append(v.Missing, class)
		}
	}
	return v
}

// DetectSurfaces classifies changed repository-relative paths into surface
// classes. It is intentionally over-inclusive.
func DetectSurfaces(changedPaths []string) []string {
	seen := map[string]bool{}
	for _, p := range changedPaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if class := classify(p); class != "" {
			seen[class] = true
		}
	}
	out := make([]string, 0, len(seen))
	for class := range seen {
		out = append(out, class)
	}
	sort.Strings(out)
	return out
}

// classify maps one path to a surface class, or "" when it is not recognised.
func classify(path string) string {
	switch {
	case strings.HasPrefix(path, "backend/routes/"):
		return SurfaceRoute
	case strings.HasPrefix(path, "backend/database/migrations/"):
		return SurfacePersistence
	case strings.HasPrefix(path, "backend/integrations/"):
		return SurfaceOutboundClient
	case strings.HasPrefix(path, "backend/services/") && strings.HasSuffix(path, "_client.go"):
		return SurfaceOutboundClient
	case isAuthPath(path):
		return SurfaceAuthPath
	default:
		return ""
	}
}

// authPaths are the files that mint sessions or enforce an authentication
// precondition. Kept as an explicit list so the tripwire's coverage is
// reviewable and a file rename is a deliberate change here.
var authPaths = map[string]bool{
	"backend/middleware/auth.go":                     true,
	"backend/middleware/client_version.go":           true,
	"backend/middleware/login_lockout.go":            true,
	"backend/controllers/user_controller.go":         true,
	"backend/controllers/two_factor_controller.go":   true,
	"backend/controllers/oidc_controller.go":         true,
	"backend/controllers/api_token_controller.go":    true,
	"backend/controllers/device_grant_controller.go": true,
	"backend/services/session_service.go":            true,
}

func isAuthPath(path string) bool {
	if authPaths[path] {
		return true
	}
	// A new device-grant / native-OIDC controller or a new 2FA service should
	// not slip through on a filename the list above did not anticipate.
	return strings.HasPrefix(path, "backend/controllers/device_grant") ||
		strings.HasPrefix(path, "backend/services/two_factor")
}

// parseRows reads the ledger's Markdown table into release -> row cell text.
// The first data cell is the release; every cell's text is retained so a row
// can name its surface classes anywhere the reviewer found natural.
func parseRows(ledger []byte) map[string]string {
	rows := map[string]string{}
	for _, line := range strings.Split(string(ledger), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells := splitRow(trimmed)
		if len(cells) < 2 {
			continue
		}
		release := strings.TrimSpace(cells[0])
		if release == "" || strings.EqualFold(release, "release") || isSeparator(release) {
			continue
		}
		rows[release] = strings.Join(cells, " ")
	}
	return rows
}

// splitRow splits a `| a | b |` row into its trimmed cells.
func splitRow(line string) []string {
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// isSeparator reports whether a cell is a Markdown alignment separator, e.g.
// `---` or `:---:`.
func isSeparator(cell string) bool {
	for _, r := range cell {
		if r != '-' && r != ':' && r != ' ' {
			return false
		}
	}
	return strings.ContainsRune(cell, '-')
}

// FormatMissing names what the ledger lacks, for an actionable error message.
func FormatMissing(v Verdict, release string) string {
	if len(v.Surfaces) == 0 {
		return fmt.Sprintf("release %s touched no recognised security-relevant surface class", release)
	}
	if !v.RowFound {
		return fmt.Sprintf("release %s added security-relevant surface (%s); %s has no row for %s",
			release, strings.Join(v.Surfaces, ", "), LedgerFile, release)
	}
	return fmt.Sprintf("release %s added security-relevant surface (%s); its row in %s does not name %s",
		release, strings.Join(v.Surfaces, ", "), LedgerFile, strings.Join(v.Missing, ", "))
}
