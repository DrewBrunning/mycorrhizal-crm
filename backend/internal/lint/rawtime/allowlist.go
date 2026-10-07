package rawtime

// Entry is one file's tolerated raw-time budget.
type Entry struct {
	// Count is the exact number of raw time.Now/Since/Until uses the file may
	// contain (inline `rawtime:allow` sites are not counted).
	Count int
	// Reason says why the file is not on the injected clock yet — what area
	// owns migrating it. Mandatory (TestAllowlistIsExact).
	Reason string
}

// Allowlist is the file-level escape hatch, keyed "<package dir>/<file>"
// (e.g. "services/foo.go"). It is EMPTY: issue #1494 migrated every
// time-dependent site in controllers, services and middleware to the injected
// clock, and the sites that must stay raw (elapsed-duration measurements, an
// OS net.Conn deadline, a PRNG seed) carry an inline
// `// rawtime:allow <reason>` at the call itself, where the reason is read in
// review next to the code.
//
// Add an entry here only for a bulk of sites that genuinely cannot be
// migrated in the same change (the way a new area lands behind a follow-up
// ticket). TestAllowlistIsExact keeps each entry exact — it fails when a
// listed file has more OR fewer raw uses than Count, or no Reason — so the
// list can only shrink.
var Allowlist = map[string]Entry{}
