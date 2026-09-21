package config

import (
	"fmt"
	"reflect"
	"strings"
)

// RegisterEntry is one row of the generated configuration register: what a
// single Config (or nested OIDCConfig) field is, how it is set, and what
// values it accepts. Generated from the `cfgreg` struct tag on the
// corresponding field — see Register().
type RegisterEntry struct {
	// Field is the Go field path, e.g. "Port" or "OIDC.ProviderURL".
	Field string
	// EnvVar is the environment variable that sets this field. Empty when
	// Derived is true.
	EnvVar string
	// Type is a short type label: string, int, bool, duration, stringlist,
	// or enum.
	Type string
	// Default is the value used when EnvVar is unset, as shown to an
	// operator (not necessarily a Go literal).
	Default string
	// Required is true when LoadConfig booting with this field empty fails
	// Validate().
	Required bool
	// Restart is true when changing this value takes effect only on the
	// next process restart (Config is loaded once at boot and never
	// re-read — true for every field today).
	Restart bool
	// Range describes the accepted values or bounds in prose, e.g.
	// "1..65535" or "debug|info|warn|error". Empty when unconstrained.
	Range string
	// Desc is a one-line, human-readable description of what the value
	// controls.
	Desc string
	// Derived is true for a field that is computed from other fields
	// (e.g. UseResend) or from FrontendURL (e.g. OIDC.RedirectURL) rather
	// than read directly from its own environment variable.
	Derived bool
}

// cfgTagName is the struct tag key carrying register metadata.
const cfgTagName = "cfgreg"

// parseCfgTag parses a `cfgreg:"..."` tag value into a RegisterEntry. Pairs
// are `;`-separated `key=value`; `desc` must be the last pair (its value
// runs to the end of the tag), since a description may itself contain `=`.
// Returns an error naming what was missing or malformed so the completeness
// test can fail with an actionable message.
func parseCfgTag(fieldPath, tag string) (RegisterEntry, error) {
	entry := RegisterEntry{Field: fieldPath}
	if strings.TrimSpace(tag) == "" {
		return entry, fmt.Errorf("field %s: missing `cfgreg` struct tag", fieldPath)
	}

	parts := strings.SplitN(tag, "desc=", 2)
	pairsPart := parts[0]
	if len(parts) == 2 {
		entry.Desc = strings.TrimSuffix(parts[1], ";")
	}
	pairsPart = strings.TrimSuffix(pairsPart, ";")

	for _, pair := range strings.Split(pairsPart, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			return entry, fmt.Errorf("field %s: malformed cfgreg pair %q", fieldPath, pair)
		}
		key, val := kv[0], kv[1]
		switch key {
		case "env":
			entry.EnvVar = val
		case "type":
			entry.Type = val
		case "default":
			entry.Default = val
		case "required":
			entry.Required = val == "true"
		case "restart":
			entry.Restart = val == "true"
		case "range", "enum":
			entry.Range = val
		case "derived":
			entry.Derived = val == "true"
		default:
			return entry, fmt.Errorf("field %s: unknown cfgreg key %q", fieldPath, key)
		}
	}

	if entry.Desc == "" {
		return entry, fmt.Errorf("field %s: cfgreg tag has no desc", fieldPath)
	}
	if entry.Derived {
		if entry.EnvVar != "" {
			return entry, fmt.Errorf("field %s: derived=true field must not set env", fieldPath)
		}
	} else if entry.EnvVar == "" {
		return entry, fmt.Errorf("field %s: cfgreg tag has neither env nor derived=true", fieldPath)
	} else if entry.Type == "" {
		return entry, fmt.Errorf("field %s: cfgreg tag has env but no type", fieldPath)
	}

	return entry, nil
}

// isRegisterStruct reports whether t is a struct type that Register should
// recurse into (a nested config block) rather than requiring its own
// cfgreg tag — everything under config.Config except types with their own
// direct env-var representation (time.Duration, []string).
func isRegisterStruct(t reflect.Type) bool {
	if t.Kind() != reflect.Struct {
		return false
	}
	return t.PkgPath() == reflect.TypeOf(Config{}).PkgPath()
}

// collectRegisterEntries walks t's exported fields, parsing each one's
// cfgreg tag (or recursing into a nested config struct), and appends
// results (or errors) to the given slices.
func collectRegisterEntries(t reflect.Type, prefix string, entries *[]RegisterEntry, errs *[]error) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" { // unexported
			continue
		}
		path := f.Name
		if prefix != "" {
			path = prefix + "." + f.Name
		}
		if isRegisterStruct(f.Type) {
			collectRegisterEntries(f.Type, path, entries, errs)
			continue
		}
		tag, ok := f.Tag.Lookup(cfgTagName)
		if !ok {
			*errs = append(*errs, fmt.Errorf("field %s: missing `cfgreg` struct tag", path))
			continue
		}
		entry, err := parseCfgTag(path, tag)
		if err != nil {
			*errs = append(*errs, err)
			continue
		}
		*entries = append(*entries, entry)
	}
}

// Register returns one RegisterEntry per exported Config (and nested
// OIDCConfig) field, in struct declaration order. It panics if any field's
// cfgreg tag is missing or malformed — Register is meant to be called by
// generators and tests, both of which want a hard failure pointing at the
// broken field rather than a silently incomplete register.
func Register() []RegisterEntry {
	var entries []RegisterEntry
	var errs []error
	collectRegisterEntries(reflect.TypeOf(Config{}), "", &entries, &errs)
	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		panic("config.Register: " + strings.Join(msgs, "; "))
	}
	return entries
}

// RegisterErrors is like Register but returns parse errors instead of
// panicking, for the completeness test to report individually.
func RegisterErrors() []error {
	var entries []RegisterEntry
	var errs []error
	collectRegisterEntries(reflect.TypeOf(Config{}), "", &entries, &errs)
	return errs
}

// OutOfConfigVar is one environment variable that changes runtime behavior
// but is read outside config.LoadConfig (issue #936) — either because it
// must be resolved before Config exists (migration bootstrap) or because it
// is dev/test-only tooling. Every such variable this project ships gets a
// row here so the register stays complete even though config.Config is not
// the whole configuration surface.
type OutOfConfigVar struct {
	EnvVar     string
	Type       string
	Default    string
	Required   bool
	Restart    bool
	Range      string
	Desc       string
	WhyOutside string
}

// OutOfConfigVars is the hand-authored register of environment variables
// that affect the running server but cannot (or deliberately do not) live
// on config.Config. MYCORRHIZAL_FAULTS and MYCORRHIZAL_DIFFERENTIAL_* are
// deliberately excluded — dev/test-only fault-injection knobs, not operator
// configuration.
var OutOfConfigVars = []OutOfConfigVar{
	{
		EnvVar:     "MYCORRHIZAL_ALLOW_SUB_FLOOR_MIGRATION",
		Type:       "bool",
		Default:    "false",
		Required:   false,
		Restart:    true,
		Range:      "true to enable; any other value (including unset) leaves it off",
		Desc:       "One-time bridge allowing a pre-v1.0.0 database to migrate (issue #529)",
		WhyOutside: "read by database.InitDB during migration bootstrap, before config.LoadConfig runs — see docs/upgrade-compatibility.md",
	},
	{
		EnvVar:     "MYCORRHIZAL_PRE_MIGRATION_BACKUP_DIR",
		Type:       "string",
		Default:    "a 'pre-migration' sibling directory of SQLITE_DB_PATH",
		Required:   false,
		Restart:    true,
		Range:      "directory path",
		Desc:       "Where the automatic pre-migration backup snapshot is written (issue #530)",
		WhyOutside: "read by database.preMigrationBackupDir during migration bootstrap, before config.LoadConfig runs",
	},
}

// RenderRegister builds the markdown configuration reference: the
// config.Config surface (from Register()) followed by the out-of-Config
// variables (from OutOfConfigVars). This is the sole source for the
// committed docs/configuration-reference.md — see cmd/genconfigreference.
func RenderRegister() string {
	entries := Register() // declaration order, which groups related settings the way config.go does

	var b strings.Builder
	b.WriteString("# Configuration reference\n\n")
	b.WriteString("Generated by `cd backend && go run ./cmd/genconfigreference` from the `cfgreg` struct " +
		"tags on `config.Config` (issue #933, #501 action 1-2/7) — never hand-edited. The completeness test " +
		"`config.TestConfigRegisterCoversEveryField` fails CI if a `Config` field is added without a tag, and " +
		"the drift test `config.TestConfigurationReferenceDocUpToDate` fails until this file is regenerated " +
		"after a tag changes.\n\n")

	b.WriteString("## config.Config surface\n\n")
	b.WriteString("| Variable | Type | Default | Required | Restart required | Range / enum | Description |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")
	for _, e := range entries {
		envCell := e.EnvVar
		if e.Derived {
			envCell = fmt.Sprintf("*(derived: %s)*", e.Field)
		}
		b.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s | %s | %s |\n",
			envCell, orDash(e.Type), orDash(mdEscape(e.Default)), yesNo(e.Required), yesNo(e.Restart),
			orDash(mdEscape(e.Range)), mdEscape(e.Desc)))
	}

	b.WriteString("\n## Variables outside config.Config\n\n")
	b.WriteString("Read directly from the environment outside `config.LoadConfig` (issue #936) — see each " +
		"row's \"Why outside Config\" for why it cannot simply move onto `Config`.\n\n")
	b.WriteString("| Variable | Type | Default | Required | Restart required | Range / enum | Description | Why outside Config |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|\n")
	for _, v := range OutOfConfigVars {
		b.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s | %s | %s | %s |\n",
			v.EnvVar, orDash(v.Type), orDash(mdEscape(v.Default)), yesNo(v.Required), yesNo(v.Restart),
			orDash(mdEscape(v.Range)), mdEscape(v.Desc), mdEscape(v.WhyOutside)))
	}

	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// mdEscape neutralizes markdown table syntax (`|` breaks a table cell) in
// generated prose.
func mdEscape(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
