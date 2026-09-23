// Package scoring computes the relationship health score (issue #383): a
// pure, deterministic function from a contact's raw interaction/relationship
// facts to a 0-100 score and a moss/chanterelle/russula band. It has no
// database dependency and no wall-clock dependency of its own (callers pass
// "now" and every raw fact) so it is directly unit-testable with seeded
// fixtures — the DB-facing gathering of those raw facts lives in
// services/contact_score_service.go, not here.
//
// See docs/adrs/0023-relationship-health-score.md for why these five facets,
// these weights, and these degradation rules were chosen.
package scoring

import "math"

// Band names, chosen to match the app's existing fungus-palette visual
// language (moss=success, chanterelle=warning, russula=error in
// frontend/src/theme.ts and android's MycorrhizalColors) rather than
// inventing generic healthy/warning/critical tokens.
const (
	BandMoss        = "moss"
	BandChanterelle = "chanterelle"
	BandRussula     = "russula"
)

// RawInputs are the already-gathered, already-sensitivity-filtered raw facts
// for one contact. Building this from the database (bulk, N+1-safe) is
// services.ComputeAllContactScores's job; Compute itself never queries
// anything.
type RawInputs struct {
	// HasCadencePolicy/CadenceIntervalDays: the contact's real CadencePolicy
	// interval, when the user created one. When false, Recency/Frequency
	// fall back to the closeness-tier default interval derived inside
	// Compute (see resolveIntervalDays).
	HasCadencePolicy    bool
	CadenceIntervalDays int

	// HasQualifyingInteraction/DaysSinceLastInteraction: from the same
	// Activity.Qualifying()/CadencePolicy.Qualifies gate the cadence engine
	// uses (services/cadence_service.go) — never reimplemented here.
	HasQualifyingInteraction bool
	DaysSinceLastInteraction int

	// QualifyingInteractionsInWindow is the count of qualifying interactions
	// in the trailing Config.FrequencyWindowDays-day window.
	QualifyingInteractionsInWindow int

	// DirectRelationTypes are every confirmed, non-secret relationship-edge
	// type found directly between the user's self-contact and this contact,
	// in either direction. May include affinity types (gets_along_with,
	// conflicts_with) — Compute ignores any token with no entry in
	// Config.RelationCloseness, which is exactly how affinity types are
	// excluded (they are deliberately never given a tier — see ADR-0023).
	DirectRelationTypes []string

	// ReachableHops is the shortest hop count of a structural-only path
	// (every step's relation type present in Config.RelationCloseness) from
	// the self-contact to this contact, when one exists and no direct edge
	// was found. nil when unreachable, when reachability could not be
	// established (e.g. no self-contact set), or when a direct edge already
	// answered Closeness.
	ReachableHops *int

	// HasPendingReachOut: an un-dismissed ReachOutSuggestion exists for this
	// contact (org/title/address change the user hasn't followed up on).
	HasPendingReachOut bool

	// DaysSinceUpdated: days since Contact.UpdatedAt.
	DaysSinceUpdated int
}

// FacetResult is one weighted facet's contribution, always included in
// Result so the score is explainable (issue #383 requirement 1): never just
// a number, always a per-facet breakdown a UI can render directly.
type FacetResult struct {
	// Value is the facet's own 0-100 sub-score, before weighting.
	Value float64 `json:"value"`
	// Weight is the facet's share of the total (0-100, all five sum to 100).
	Weight float64 `json:"weight"`
	// Reason is a short, plain-language explanation of Value — e.g. "Last
	// interacted 42 days ago (target every 30 days)" or "No self-contact set
	// — closeness unknown".
	Reason string `json:"reason"`
}

// Result is the full, explainable score for one contact.
type Result struct {
	Score int    `json:"score"`
	Band  string `json:"band"`

	Recency     FacetResult `json:"recency"`
	Frequency   FacetResult `json:"frequency"`
	Closeness   FacetResult `json:"closeness"`
	ReachOut    FacetResult `json:"reach_out"`
	LastUpdated FacetResult `json:"last_updated"`
}

// Compute derives Result from raw, already-gathered facts. Pure: same inputs
// always produce the same output, no I/O, no clock read.
func Compute(raw RawInputs, cfg Config) Result {
	intervalDays, closeness := resolveClosenessAndInterval(raw, cfg)
	recency := computeRecency(raw, cfg, intervalDays)
	frequency := computeFrequency(raw, cfg, intervalDays)
	reachOut := computeReachOut(raw, cfg)
	lastUpdated := computeLastUpdated(raw, cfg)

	weighted := recency.Value*cfg.WeightRecency +
		frequency.Value*cfg.WeightFrequency +
		closeness.Value*cfg.WeightCloseness +
		reachOut.Value*cfg.WeightReachOut +
		lastUpdated.Value*cfg.WeightLastUpdated
	score := int(math.Round(weighted / 100))

	return Result{
		Score:       score,
		Band:        band(float64(score), cfg),
		Recency:     recency,
		Frequency:   frequency,
		Closeness:   closeness,
		ReachOut:    reachOut,
		LastUpdated: lastUpdated,
	}
}

// band maps a 0-100 score to its fungus-palette band using the tunable
// thresholds. moss >= MossMin; chanterelle in [ChanterelleMin, MossMin);
// russula below ChanterelleMin. Validate guarantees ChanterelleMin < MossMin.
func band(score float64, cfg Config) string {
	switch {
	case score >= cfg.MossMin:
		return BandMoss
	case score >= cfg.ChanterelleMin:
		return BandChanterelle
	default:
		return BandRussula
	}
}

// clamp constrains v to [lo, hi].
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
