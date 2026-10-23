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
	if err := db.Select("id", "vcard_uid", "updated_at", "target_closeness").
		Where("user_id = ? AND archived = ?", userID, false).
		Find(&contacts).Error; err != nil {
		return nil, fmt.Errorf("loading contacts: %w", err)
	}
	return computeContactScores(db, userID, now, contacts)
}

// ComputeContactScores computes the score for exactly the given, already
// loaded contacts (each needs ID, VCardUID and UpdatedAt). GET /graph's
// ego-network mode uses it to score only the contacts it returns instead of
// the whole account (issue #1791). The caller owns the archived filter.
func ComputeContactScores(db *gorm.DB, userID uint, now time.Time, contacts []models.Contact) (map[uint]scoring.Result, error) {
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

	in, err := gatherScoreInputs(db, userID, now, contacts)
	if err != nil {
		return nil, err
	}
	for _, c := range contacts {
		results[c.ID] = scoring.Compute(in.raw[c.ID], in.cfg)
	}
	return results, nil
}

// scoreInputs is everything scoring.Compute needs for a batch of contacts,
// gathered in a fixed number of bulk queries. The reach-out job's target pass
// (detectTargetBehindSuggestions) reuses it alongside the score service.
type scoreInputs struct {
	cfg             scoring.Config
	raw             map[uint]scoring.RawInputs
	lastInteraction map[string]*time.Time
	hasPolicy       map[string]bool
}

// gatherScoreInputs loads every raw facet for the given contacts (each needs
// ID, VCardUID, UpdatedAt and TargetCloseness) in bulk. Never one query per
// contact.
func gatherScoreInputs(db *gorm.DB, userID uint, now time.Time, contacts []models.Contact) (scoreInputs, error) {
	cfg, err := scoring.LoadConfig()
	if err != nil { // # pragma: no cover — the embedded tunables file is validated by the scoring package tests, so it cannot fail to load
		return scoreInputs{}, fmt.Errorf("loading scoring config: %w", err)
	}

	uids := make([]string, len(contacts))
	for i, c := range contacts {
		uids[i] = c.VCardUID
	}

	realPolicies, err := cadencePoliciesByEntityID(db, userID, uids)
	if err != nil {
		return scoreInputs{}, fmt.Errorf("loading cadence policies: %w", err)
	}

	lastInteraction, err := lastQualifyingInteractionByEntityID(db, userID, uids, realPolicies)
	if err != nil {
		return scoreInputs{}, fmt.Errorf("loading last qualifying interactions: %w", err)
	}

	// The window is counted in calendar days: interaction dates are calendar
	// days (ADR 0015 category 2), and "today" is now's wall-clock date.
	sinceDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -cfg.FrequencyWindowDays)
	frequencyCounts, err := qualifyingInteractionCountsByEntityID(db, userID, uids, realPolicies, sinceDay)
	if err != nil {
		return scoreInputs{}, fmt.Errorf("loading interaction frequency: %w", err)
	}

	selfContactUID, err := selfContactVCardUID(db, userID)
	if err != nil {
		return scoreInputs{}, fmt.Errorf("loading self-contact: %w", err)
	}

	var directTypes map[string][]string
	var reachableHops map[string]int
	if selfContactUID != "" {
		directTypes, err = directRelationTypesByEntityID(db, userID, selfContactUID, uids)
		if err != nil {
			return scoreInputs{}, fmt.Errorf("loading direct relationship edges: %w", err)
		}
		reachableHops, err = structuralHopsByEntityID(db, userID, selfContactUID, cfg)
		if err != nil {
			return scoreInputs{}, fmt.Errorf("loading graph reachability: %w", err)
		}
	}

	ambient, err := ambientByEntityID(db, userID, uids, now)
	if err != nil {
		return scoreInputs{}, fmt.Errorf("loading ambient contacts: %w", err)
	}

	pendingReachOut, err := pendingReachOutEntityIDs(db, userID, uids)
	if err != nil {
		return scoreInputs{}, fmt.Errorf("loading pending reach-out suggestions: %w", err)
	}

	rawByID := make(map[uint]scoring.RawInputs, len(contacts))
	hasPolicy := make(map[string]bool, len(realPolicies))
	for uid := range realPolicies {
		hasPolicy[uid] = true
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
			raw.DaysSinceLastInteraction = interactionDaysSince(*last, now)
		}

		raw.QualifyingInteractionsInWindow = frequencyCounts[c.VCardUID]
		raw.DirectRelationTypes = directTypes[c.VCardUID]
		if hops, ok := reachableHops[c.VCardUID]; ok {
			h := hops
			raw.ReachableHops = &h
		}
		raw.HasPendingReachOut = pendingReachOut[c.VCardUID]
		if reason, ok := ambient[c.VCardUID]; ok {
			raw.Ambient = true
			raw.AmbientReason = reason
		}

		if c.TargetCloseness != nil {
			raw.TargetTier = *c.TargetCloseness
		}

		rawByID[c.ID] = raw
	}

	return scoreInputs{cfg: cfg, raw: rawByID, lastInteraction: lastInteraction, hasPolicy: hasPolicy}, nil
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

// interactionDaysSince counts whole calendar days from an interaction's
// date-only value to "today" in now's zone (never negative). An activity's
// date is a calendar day stored at midnight UTC (ADR 0015 category 2), so this
// compares UTC date components with now's local date -- elapsed hours would
// turn a same-day interaction into "1 day ago" once the wall clock has run
// past 24 hours from midnight UTC west of UTC (issue #1823).
func interactionDaysSince(last, now time.Time) int {
	d := calendarDaysBetween(interactionCalendarDay(last), now)
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
	if err := activityDateRowsQuery(db, userID, uids).
		Order("a.date DESC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	// Issue #1736: given gifts are interactions of type "gift" (one bulk query).
	gifts, err := givenGiftDateRows(db, userID, uids, nil)
	if err != nil {
		return nil, err
	}
	rows = mergeRowsDateDesc(rows, gifts)

	// Group once by contact (rows are date DESC): O(rows + contacts) rather
	// than rescanning every row per contact (issue #1791, the 15k x rows
	// nested loop behind contact_score_service's SLOW SQL / CPU burn).
	byUID := groupActivityRowsByUID(rows)

	result := make(map[string]*time.Time, len(uids))
	for _, uid := range uids {
		policy, ok := realPolicies[uid]
		if !ok {
			policy = models.CadencePolicy{EntityID: uid}
		}
		if t := firstQualifyingDate(byUID[uid], &policy); t != nil {
			result[uid] = t
		}
	}
	return result, nil
}

// qualifyingInteractionCountsByEntityID counts, per contact, how many
// qualifying interactions fall on or after the calendar day sinceDay (a UTC
// midnight) -- the Frequency facet's raw input.
//
// The day cut is applied in Go on each row's calendar day
// (interactionCalendarDay), not in SQL: the date columns hold RFC 3339 text in
// whatever offset the writer sent, and SQLite compares that text, so a bound
// `a.date >= ?` silently mis-orders values with different offsets (v1.5.0
// review). SQL only prefilters with frequencyWindowSlack, which is wider than
// any UTC offset can shift the date digits.
func qualifyingInteractionCountsByEntityID(db *gorm.DB, userID uint, uids []string, realPolicies map[string]models.CadencePolicy, sinceDay time.Time) (map[string]int, error) {
	prefilter := sinceDay.UTC().Add(-frequencyWindowSlack)
	var rows []activityDateRow
	if err := activityDateRowsQuery(db, userID, uids).
		Where("a.date >= ?", prefilter).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	// Issue #1736: given gifts in the window count like activities.
	gifts, err := givenGiftDateRows(db, userID, uids, &prefilter)
	if err != nil {
		return nil, err
	}
	rows = append(rows, gifts...)

	counts := make(map[string]int, len(uids))
	for i := range rows {
		if interactionCalendarDay(rows[i].Date).Before(sinceDay) {
			continue
		}
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

// frequencyWindowSlack widens the SQL prefilter of the frequency window: a
// stored offset moves an instant's text by at most one calendar day, so two
// days always include every row whose calendar day is inside the window.
const frequencyWindowSlack = 48 * time.Hour

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
// from selfUID to each reachable contact. A step only counts when its display
// relation is a relation type present in cfg.RelationCloseness -- a contact
// reachable only through an affinity edge (gets_along_with/conflicts_with) is
// deliberately excluded (ADR-0023), so it falls through to the "unknown"
// closeness default exactly like an unreachable contact, rather than reading
// as hop-close.
//
// Issue #1791: this used to call TraverseGraph at maxTraversalDepth and
// filter the resulting chains. That CTE enumerates every simple PATH to depth
// 5 (exponential in a dense graph -- the cause of the 8-9 minute GET /graph
// at the `large` scale) only for this function to keep the shortest depth per
// target. A shortest-path question is a BFS: one edge query, then a linear
// in-memory walk. Same step semantics as TraverseGraph (traversalStepRelation)
// and the same maxTraversalDepth cap, so a contact is hop-reachable exactly
// when a fully structural simple path of <= maxTraversalDepth steps exists.
func structuralHopsByEntityID(db *gorm.DB, userID uint, selfUID string, cfg scoring.Config) (map[string]int, error) {
	type edgeRow struct {
		SourceID string
		TargetID string
		Type     string
	}
	var rows []edgeRow
	if err := db.Table("relationship_edges").
		Select("source_id, target_id, type").
		Where("user_id = ? AND status = ? AND sensitivity != ?",
			userID, models.RelationshipStatusConfirmed, models.RelationshipSensitivitySecret).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	adjacency := make(map[string][]string, len(rows))
	for _, r := range rows {
		// Forward hop (source -> target): the target is the INVERSE of the
		// stored type relative to the source. Backward hop: the stored type.
		if _, ok := cfg.RelationCloseness[traversalStepRelation(r.Type, true)]; ok {
			adjacency[r.SourceID] = append(adjacency[r.SourceID], r.TargetID)
		}
		if _, ok := cfg.RelationCloseness[traversalStepRelation(r.Type, false)]; ok {
			adjacency[r.TargetID] = append(adjacency[r.TargetID], r.SourceID)
		}
	}

	result := make(map[string]int)
	visited := map[string]bool{selfUID: true}
	frontier := []string{selfUID}
	for depth := 1; depth <= maxTraversalDepth && len(frontier) > 0; depth++ {
		if err := statementContextErr(db); err != nil {
			return nil, err
		}
		var next []string
		for _, uid := range frontier {
			for _, n := range adjacency[uid] {
				if visited[n] {
					continue
				}
				visited[n] = true
				result[n] = depth
				next = append(next, n)
			}
		}
		frontier = next
	}
	return result, nil
}

// statementContextErr returns the request context's error (cancelled or
// deadline exceeded) when db carries a context, so a long in-memory loop can
// stop promptly after the client disconnects (issue #1791). The SQL layer
// already honours the context; this covers the Go-side work between queries.
func statementContextErr(db *gorm.DB) error {
	if db == nil || db.Statement == nil || db.Statement.Context == nil {
		return nil
	}
	return db.Statement.Context.Err()
}

// pendingReachOutEntityIDs returns the set of contacts with at least one
// un-dismissed org/title/address ReachOutSuggestion — the ReachOut facet's raw
// input. target_behind rows are excluded (issue #1796, ADR 0023 "No feedback
// loop"): being behind target must not itself lower the score.
func pendingReachOutEntityIDs(db *gorm.DB, userID uint, uids []string) (map[string]bool, error) {
	var matched []string
	if err := db.Model(&models.ReachOutSuggestion{}).
		Where("user_id = ? AND contact_vcard_uid IN ? AND status = ? AND kind != ?", userID, uids, models.ReachOutStatusPending, models.ReachOutKindTargetBehind).
		Distinct().Pluck("contact_vcard_uid", &matched).Error; err != nil {
		return nil, err
	}
	result := make(map[string]bool, len(matched))
	for _, uid := range matched {
		result[uid] = true
	}
	return result, nil
}
