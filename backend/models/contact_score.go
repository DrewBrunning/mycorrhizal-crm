package models

// ContactScoreResponse is the API response for GET /contacts/:id/score
// (issue #383, ADR-0023) — the full explainable breakdown behind a
// contact's relationship health score, mirroring ContactBriefing's shape
// (one struct per facet, degrading independently) rather than leaking
// internal/scoring's Result type directly onto the wire (same reasoning as
// BriefingCadence wrapping services.CadenceHealth).
type ContactScoreResponse struct {
	ContactID uint `json:"contact_id"`
	// Score is 0-100; Band is one of moss/chanterelle/russula.
	Score int    `json:"score"`
	Band  string `json:"band"`

	Recency     ContactScoreFacet `json:"recency"`
	Frequency   ContactScoreFacet `json:"frequency"`
	Closeness   ContactScoreFacet `json:"closeness"`
	ReachOut    ContactScoreFacet `json:"reach_out"`
	LastUpdated ContactScoreFacet `json:"last_updated"`

	// Ambient is always present: true when the contact counts as in regular
	// contact without logged activity (issue #1733), in which case Recency and
	// Frequency are both 100. AmbientReason is "household" (lives with the
	// user's self-contact) or "flag" (the contact's own ambient flag), and is
	// omitted when Ambient is false.
	Ambient       bool   `json:"ambient"`
	AmbientReason string `json:"ambient_reason,omitempty"`

	// Target is always present (issue #1796): null when the contact has no target
	// closeness. It reports the target's status against the same Recency and
	// Frequency facets; it does not feed back into Score or Band.
	Target *ContactScoreTarget `json:"target"`
}

// ContactScoreTarget is the target-closeness status of a contact (ADR 0023
// amendment 2026-10-16). Status is on_track|drifting|behind; IntervalSource is
// cadence_policy|target_tier.
type ContactScoreTarget struct {
	Tier           string  `json:"tier"`
	Status         string  `json:"status"`
	Value          float64 `json:"value"`
	IntervalDays   int     `json:"interval_days"`
	IntervalSource string  `json:"interval_source"`
	Reason         string  `json:"reason"`
}

// ContactScoreFacet is one weighted facet's contribution and plain-language
// explanation — the score is never a black box (issue #383 requirement 1).
type ContactScoreFacet struct {
	// Value is the facet's own 0-100 sub-score, before weighting.
	Value float64 `json:"value"`
	// Weight is the facet's share of the total (0-100; all five sum to 100).
	Weight float64 `json:"weight"`
	// Reason is a short, plain-language explanation of Value.
	Reason string `json:"reason"`
}
