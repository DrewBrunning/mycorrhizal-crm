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
