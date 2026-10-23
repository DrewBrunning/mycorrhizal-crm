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

	// Ambient/AmbientReason (issue #1733, ADR 0023 amendment): the contact
	// counts as in regular contact without logged interactions, either because
	// the user lives with them (AmbientReasonHousehold, derived from the shared
	// household) or marked them (AmbientReasonFlag). When Ambient is true,
	// Recency and Frequency are both 100 regardless of the interaction inputs
	// above; Closeness, ReachOut and LastUpdated are unchanged.
	Ambient       bool
	AmbientReason string

	// TargetTier is the contact's target closeness (issue #1796, ADR 0023
	// amendment): "" = no target. A token with no entry in
	// Config.TargetTierIntervalDays is treated as no target.
	TargetTier string
}

// Ambient reasons carried on RawInputs.AmbientReason and Result.AmbientReason.
// Frontend/Android mirror these tokens (frontend trap #4): keep in sync.
const (
	AmbientReasonHousehold = "household"
	AmbientReasonFlag      = "flag"
)

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
	// ReasonCode identifies Reason's sentence so clients can render it in the
	// user's language (v1.5.0 review: the English Reason was shown verbatim in
	// every locale). One of the Reason* codes below; ReasonParams carries the
	// numbers/tokens the sentence interpolates. Mirrored by hand in the web and
	// Android clients (frontend trap #4): keep in sync.
	ReasonCode   string       `json:"reason_code"`
	ReasonParams ReasonParams `json:"reason_params"`
}

// ReasonParams are the values a reason sentence interpolates. Only the fields
// the ReasonCode uses are set; the rest are omitted from the JSON.
type ReasonParams struct {
	Days         *int     `json:"days,omitempty"`
	IntervalDays *int     `json:"interval_days,omitempty"`
	Hops         *int     `json:"hops,omitempty"`
	Count        *int     `json:"count,omitempty"`
	WindowDays   *int     `json:"window_days,omitempty"`
	Expected     *float64 `json:"expected,omitempty"`
	Score        *int     `json:"score,omitempty"`
	Relation     string   `json:"relation,omitempty"`
	Tier         string   `json:"tier,omitempty"`
}

// Reason codes on FacetResult.ReasonCode and TargetResult.ReasonCode.
// Mirrored in openapi.yaml and both clients (frontend trap #4).
const (
	ReasonClosenessDirect  = "closeness_direct"      // relation
	ReasonClosenessHops    = "closeness_hops"        // hops
	ReasonClosenessUnknown = "closeness_unknown"     //
	ReasonRecencyNone      = "recency_none"          //
	ReasonRecencyDays      = "recency_days"          // days, interval_days
	ReasonFrequencyCount   = "frequency_count"       // count, window_days, expected
	ReasonReachOutPending  = "reach_out_pending"     //
	ReasonReachOutNone     = "reach_out_none"        //
	ReasonLastUpdatedDays  = "last_updated_days"     // days
	ReasonAmbientHousehold = "ambient_household"     //
	ReasonAmbientFlag      = "ambient_flag"          //
	ReasonTargetPolicy     = "target_cadence_policy" // score, interval_days
	ReasonTargetTier       = "target_tier"           // score, interval_days, tier
	ReasonTargetAmbient    = "target_ambient"        //
)

// ReasonCodes lists every reason code, for drift tests against the clients.
func ReasonCodes() []string {
	return []string{
		ReasonClosenessDirect, ReasonClosenessHops, ReasonClosenessUnknown,
		ReasonRecencyNone, ReasonRecencyDays, ReasonFrequencyCount,
		ReasonReachOutPending, ReasonReachOutNone, ReasonLastUpdatedDays,
		ReasonAmbientHousehold, ReasonAmbientFlag,
		ReasonTargetPolicy, ReasonTargetTier, ReasonTargetAmbient,
	}
}

func intPtr(v int) *int { return &v }

// Result is the full, explainable score for one contact.
type Result struct {
	Score int    `json:"score"`
	Band  string `json:"band"`

	Recency     FacetResult `json:"recency"`
	Frequency   FacetResult `json:"frequency"`
	Closeness   FacetResult `json:"closeness"`
	ReachOut    FacetResult `json:"reach_out"`
	LastUpdated FacetResult `json:"last_updated"`

	// Ambient is true when Recency and Frequency were overridden to 100 because
	// the contact is ambient (issue #1733); AmbientReason says why
	// (AmbientReasonHousehold or AmbientReasonFlag), empty when not ambient.
	Ambient       bool   `json:"ambient"`
	AmbientReason string `json:"ambient_reason,omitempty"`

	// Target is nil when the contact has no (recognized) target closeness.
	Target *TargetResult `json:"target"`
}

// Target statuses on TargetResult.Status. Mirrored in openapi.yaml and the
// clients (frontend trap #4).
const (
	TargetStatusOnTrack  = "on_track"
	TargetStatusDrifting = "drifting"
	TargetStatusBehind   = "behind"
)

// Interval sources on TargetResult.IntervalSource.
const (
	IntervalSourceCadencePolicy = "cadence_policy"
	IntervalSourceTargetTier    = "target_tier"
)

// TargetResult is how a contact is doing against its target closeness.
type TargetResult struct {
	Tier           string  `json:"tier"`
	Status         string  `json:"status"`
	Value          float64 `json:"value"`
	IntervalDays   int     `json:"interval_days"`
	IntervalSource string  `json:"interval_source"`
	Reason         string  `json:"reason"`
	// ReasonCode/ReasonParams: see FacetResult.
	ReasonCode   string       `json:"reason_code"`
	ReasonParams ReasonParams `json:"reason_params"`
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

	result := Result{
		Score:       score,
		Band:        band(float64(score), cfg),
		Recency:     recency,
		Frequency:   frequency,
		Closeness:   closeness,
		ReachOut:    reachOut,
		LastUpdated: lastUpdated,
	}
	if raw.Ambient {
		result.Ambient = true
		result.AmbientReason = raw.AmbientReason
	}
	result.Target = computeTarget(raw, cfg, intervalDays, recency, frequency)
	return result
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
