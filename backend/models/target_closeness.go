package models

// Target closeness tiers (issue #1796, ADR 0023 amendment 2026-10-16). The
// tokens are the four tier_definitions keys in
// internal/scoring/testdata/weights.json; scoring's config validate() rejects
// a tunables file whose keys differ from TargetClosenessTiers(). The DB CHECK
// (migration 000076), the API oneof below, backend/openapi.yaml and the
// frontend/Android mirrors must stay in sync by hand (frontend trap #4).
const (
	TargetClosenessImmediate = "immediate"
	TargetClosenessClose     = "close"
	TargetClosenessCasual    = "casual"
	TargetClosenessDistant   = "distant"
)

// TargetClosenessTiers returns the valid tiers, closest first.
func TargetClosenessTiers() []string {
	return []string{TargetClosenessImmediate, TargetClosenessClose, TargetClosenessCasual, TargetClosenessDistant}
}

// ContactTargetClosenessInput is the body of PUT /contacts/:id/target-closeness.
type ContactTargetClosenessInput struct {
	Tier string `json:"tier" validate:"required,oneof=immediate close casual distant"`
}

// ContactTargetClosenessResponse is the 200 body of PUT and DELETE
// /contacts/:id/target-closeness. No field carries omitempty: null is always
// present. After a DELETE the last three are null.
type ContactTargetClosenessResponse struct {
	ContactID       uint    `json:"contact_id"`
	TargetCloseness *string `json:"target_closeness"`
	// DefaultIntervalDays is the tier's default_interval_days.
	DefaultIntervalDays *int `json:"default_interval_days"`
	// CadencePolicyOffer is non-null only when a tier was set and the contact has
	// no live cadence policy; accepting it is a plain POST /cadence-policies.
	CadencePolicyOffer *CadencePolicyInput `json:"cadence_policy_offer"`
}
