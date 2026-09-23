package services

import (
	"fmt"
	"time"

	"mycorrhizal/internal/scoring"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// ComputeAllContactScores computes the relationship health score (issue
// #383, ADR-0023) for every non-archived contact belonging to userID, in a
// fixed, small number of bulk queries — never one query per contact,
// mirroring fetchLastInteractionDates's (cadence_service.go) batch shape.
// Both GetGraph and GetGraphConnections (graph_controller.go) decorate their
// responses from this single call, so Closeness is always relative to the
// same self-contact anchor regardless of which endpoint's own traversal
// triggered the read.
func ComputeAllContactScores(db *gorm.DB, userID uint, now time.Time) (map[uint]scoring.Result, error) {
	var contacts []models.Contact
	if err := db.Select("id", "vcard_uid", "updated_at").
		Where("user_id = ? AND archived = ?", userID, false).
		Find(&contacts).Error; err != nil {
		return nil, fmt.Errorf("loading contacts: %w", err)
	}
	return computeContactScores(db, userID, now, contacts)
}

// ComputeContactScore computes the score for exactly one contact — the
// GET /contacts/:id/score breakdown endpoint's entry point. It reuses
// computeContactScores rather than a second facet-assembly implementation,
// but (unlike ComputeAllContactScores) does not filter by archived status:
// GetContactBriefing and GetContactByID don't exclude an archived contact
// either, and an archived contact silently getting a misleading
// all-defaults score (a map-miss on the bulk path, not a real computation)
// would be a worse bug than the extra bulk-style queries for a
// single-contact read.
func ComputeContactScore(db *gorm.DB, userID uint, contact *models.Contact, now time.Time) (scoring.Result, error) {
	results, err := computeContactScores(db, userID, now, []models.Contact{*contact})
	if err != nil {
		return scoring.Result{}, err
	}
	return results[contact.ID], nil
}

// computeContactScores is the shared core: given an already-loaded slice of
// contacts (whichever the two exported entry points want), gather every raw
// facet in bulk and call scoring.Compute once per contact.
func computeContactScores(db *gorm.DB, userID uint, now time.Time, contacts []models.Contact) (map[uint]scoring.Result, error) {
	results := make(map[uint]scoring.Result, len(contacts))
	if len(contacts) == 0 {
		return results, nil
	}

	cfg, err := scoring.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("loading scoring config: %w", err)
	}

	uids := make([]string, len(contacts))
	for i, c := range contacts {
		uids[i] = c.VCardUID
	}

	realPolicies, err := cadencePoliciesByEntityID(db, userID, uids)
	if err != nil {
		return nil, fmt.Errorf("loading cadence policies: %w", err)
	}

	lastInteraction, err := lastQualifyingInteractionByEntityID(db, userID, uids, realPolicies)
	if err != nil {
		return nil, fmt.Errorf("loading last qualifying interactions: %w", err)
	}

	since := now.AddDate(0, 0, -cfg.FrequencyWindowDays)
	frequencyCounts, err := qualifyingInteractionCountsByEntityID(db, userID, uids, realPolicies, since)
	if err != nil {
		return nil, fmt.Errorf("loading interaction frequency: %w", err)
	}

	selfContactUID, err := selfContactVCardUID(db, userID)
	if err != nil {
		return nil, fmt.Errorf("loading self-contact: %w", err)
	}

	var directTypes map[string][]string
	var reachableHops map[string]int
	if selfContactUID != "" {
		directTypes, err = directRelationTypesByEntityID(db, userID, selfContactUID, uids)
		if err != nil {
			return nil, fmt.Errorf("loading direct relationship edges: %w", err)
		}
		reachableHops, err = structuralHopsByEntityID(db, userID, selfContactUID, cfg)
		if err != nil {
			return nil, fmt.Errorf("loading graph reachability: %w", err)
		}
	}

	pendingReachOut, err := pendingReachOutEntityIDs(db, userID, uids)
	if err != nil {
		return nil, fmt.Errorf("loading pending reach-out suggestions: %w", err)
	}

	for _, c := range contacts {
		raw := scoring.RawInputs{
			DaysSinceUpdated: daysBetween(c.UpdatedAt, now),
		}

		if policy, ok := realPolicies[c.VCardUID]; ok {
			raw.HasCadencePolicy = true
			raw.CadenceIntervalDays = policy.TargetIntervalDays
		}

		if last, ok := lastInteraction[c.VCardUID]; ok {
			raw.HasQualifyingInteraction = true
			raw.DaysSinceLastInteraction = daysBetween(*last, now)
		}

		raw.QualifyingInteractionsInWindow = frequencyCounts[c.VCardUID]
		raw.DirectRelationTypes = directTypes[c.VCardUID]
		if hops, ok := reachableHops[c.VCardUID]; ok {
			h := hops
			raw.ReachableHops = &h
		}
		raw.HasPendingReachOut = pendingReachOut[c.VCardUID]

		results[c.ID] = scoring.Compute(raw, cfg)
	}

	return results, nil
}

// daysBetween returns whole days from t to now (never negative — a future
// UpdatedAt/interaction, which cannot happen from real data, would otherwise
// produce a negative day count that scoring.Compute would still clamp
// safely, but there is no legitimate case for it here).
func daysBetween(t, now time.Time) int {
	d := int(now.Sub(t).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}

// cadencePoliciesByEntityID bulk-loads every real CadencePolicy for the
// given contacts (most contacts will have none — CadencePolicy is opt-in),
// keyed by EntityID (Contact.VCardUID).
func cadencePoliciesByEntityID(db *gorm.DB, userID uint, uids []string) (map[string]models.CadencePolicy, error) {
	var policies []models.CadencePolicy
	if err := db.Where("user_id = ? AND entity_id IN ?", userID, uids).Find(&policies).Error; err != nil {
		return nil, err
	}
	result := make(map[string]models.CadencePolicy, len(policies))
	for _, p := range policies {
		result[p.EntityID] = p
	}
	return result, nil
}

// lastQualifyingInteractionByEntityID finds, for every contact, the most
// recent Activity that counts as qualifying — through the contact's real
// CadencePolicy.Qualifies when one exists, or a synthetic zero-value policy
// (which Qualifies() correctly reduces to plain Activity.Qualifying() for,
// since an empty QualifyingTypes list means "every default-qualifying type
// counts") when it doesn't. Never reimplements the qualifying gate — this is
// the same two-gate check cadence_service.go's lastQualifyingInteraction
// uses, just batched by contact instead of by a single policy.
func lastQualifyingInteractionByEntityID(db *gorm.DB, userID uint, uids []string, realPolicies map[string]models.CadencePolicy) (map[string]*time.Time, error) {
	var rows []activityDateRow
	if err := db.Table("activities").
		Select("activities.date, activities.type, c.vcard_uid").
		Joins("JOIN activity_contacts ac ON ac.activity_id = activities.id").
		Joins("JOIN contacts c ON c.id = ac.contact_id").
		Where("c.vcard_uid IN ? AND c.user_id = ? AND activities.user_id = ? AND activities.deleted_at IS NULL", uids, userID, userID).
		Order("activities.date DESC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[string]*time.Time, len(uids))
	for _, uid := range uids {
		policy, ok := realPolicies[uid]
		if !ok {
			policy = models.CadencePolicy{EntityID: uid}
		}
		for i := range rows {
			if rows[i].VCardUID != uid {
				continue
			}
			candidate := models.Activity{Type: rows[i].Type, Date: rows[i].Date}
			if policy.Qualifies(&candidate) {
				t := rows[i].Date
				result[uid] = &t
				break
			}
		}
	}
	return result, nil
}

// qualifyingInteractionCountsByEntityID counts, per contact, how many
// qualifying activities occurred on or after since — the Frequency facet's
// raw input.
func qualifyingInteractionCountsByEntityID(db *gorm.DB, userID uint, uids []string, realPolicies map[string]models.CadencePolicy, since time.Time) (map[string]int, error) {
	var rows []activityDateRow
	if err := db.Table("activities").
		Select("activities.date, activities.type, c.vcard_uid").
		Joins("JOIN activity_contacts ac ON ac.activity_id = activities.id").
		Joins("JOIN contacts c ON c.id = ac.contact_id").
		Where("c.vcard_uid IN ? AND c.user_id = ? AND activities.user_id = ? AND activities.deleted_at IS NULL AND activities.date >= ?",
			uids, userID, userID, since).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	counts := make(map[string]int, len(uids))
	for i := range rows {
		policy, ok := realPolicies[rows[i].VCardUID]
		if !ok {
			policy = models.CadencePolicy{EntityID: rows[i].VCardUID}
		}
		candidate := models.Activity{Type: rows[i].Type, Date: rows[i].Date}
		if policy.Qualifies(&candidate) {
			counts[rows[i].VCardUID]++
		}
	}
	return counts, nil
}

// selfContactVCardUID returns the caller's User.SelfContactVCardUID, or ""
// when unset (T90 — every account gets one lazily via EnsureSelfContact, but
// an admin-created or never-logged-in-since account may still have none).
func selfContactVCardUID(db *gorm.DB, userID uint) (string, error) {
	var user models.User
	if err := db.Select("self_contact_vcard_uid").Where("id = ?", userID).First(&user).Error; err != nil {
		return "", err
	}
	if user.SelfContactVCardUID == nil {
		return "", nil
	}
	return *user.SelfContactVCardUID, nil
}

// directRelationTypesByEntityID bulk-loads every confirmed, non-secret
// relationship-edge type directly connecting selfUID to each of uids, in
// either direction — the Closeness facet's direct-edge raw input. A contact
// may have more than one type (e.g. friend_of AND conflicts_with); Compute
// itself picks the highest tier deterministically, ignoring any type with no
// tier assignment (see ADR-0023).
func directRelationTypesByEntityID(db *gorm.DB, userID uint, selfUID string, uids []string) (map[string][]string, error) {
	type edgeRow struct {
		SourceID string
		TargetID string
		Type     string
	}
	var rows []edgeRow
	if err := db.Table("relationship_edges").
		Select("source_id, target_id, type").
		Where("user_id = ? AND status = ? AND sensitivity != ? AND ((source_id = ? AND target_id IN ?) OR (target_id = ? AND source_id IN ?))",
			userID, models.RelationshipStatusConfirmed, models.RelationshipSensitivitySecret,
			selfUID, uids, selfUID, uids).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[string][]string, len(rows))
	for _, r := range rows {
		other := r.TargetID
		if other == selfUID {
			other = r.SourceID
		}
		result[other] = append(result[other], r.Type)
	}
	return result, nil
}

// structuralHopsByEntityID finds the shortest structural-only path length
// from selfUID to each reachable contact, via one TraverseGraph call. A
// chain only counts when every one of its steps is a relation type present
// in cfg.RelationCloseness — a chain reachable only through an affinity edge
// (gets_along_with/conflicts_with) is deliberately excluded (ADR-0023), so
// it falls through to the "unknown" closeness default exactly like an
// unreachable contact, rather than reading as hop-close.
func structuralHopsByEntityID(db *gorm.DB, userID uint, selfUID string, cfg scoring.Config) (map[string]int, error) {
	chains, err := TraverseGraph(db, userID, selfUID, maxTraversalDepth, "")
	if err != nil {
		return nil, err
	}

	result := make(map[string]int, len(chains))
	for _, chain := range chains {
		if !isStructuralChain(chain, cfg) {
			continue
		}
		if existing, ok := result[chain.TargetVCardUID]; !ok || chain.Depth < existing {
			result[chain.TargetVCardUID] = chain.Depth
		}
	}
	return result, nil
}

// isStructuralChain reports whether every step of chain is a relation type
// with a closeness tier — see structuralHopsByEntityID's doc comment.
func isStructuralChain(chain models.GraphChain, cfg scoring.Config) bool {
	for _, step := range chain.Steps {
		if _, ok := cfg.RelationCloseness[step.Relation]; !ok {
			return false
		}
	}
	return true
}

// pendingReachOutEntityIDs returns the set of contacts with at least one
// un-dismissed ReachOutSuggestion — the ReachOut facet's raw input.
func pendingReachOutEntityIDs(db *gorm.DB, userID uint, uids []string) (map[string]bool, error) {
	var matched []string
	if err := db.Model(&models.ReachOutSuggestion{}).
		Where("user_id = ? AND contact_vcard_uid IN ? AND status = ?", userID, uids, models.ReachOutStatusPending).
		Distinct().Pluck("contact_vcard_uid", &matched).Error; err != nil {
		return nil, err
	}
	result := make(map[string]bool, len(matched))
	for _, uid := range matched {
		result[uid] = true
	}
	return result, nil
}
