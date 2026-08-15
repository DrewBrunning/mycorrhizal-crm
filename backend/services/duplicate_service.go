package services

import (
	"sort"

	"mycorrhizal/models"

	"gorm.io/gorm"
)

// tierRow is one (contact id, grouping key) hit from a duplicate tier query.
// Key/Key2 together are the SQL grouping value a contact shares with at least
// one other contact. Most tiers use only Key; the name tier keeps the
// lowercased firstname and lastname SEPARATE (Key, Key2) so no
// separator-in-a-name can ever collapse two different name tuples onto the
// same grouping key ("A|B"+"C" must not group with "A"+"B|C").
type tierRow struct {
	ID   uint
	Key  string
	Key2 string
}

// groupKey is the collision-free in-memory grouping key derived from a
// tierRow: [2]string{Key, Key2} is a comparable Go struct, so the name tier's
// tuple stays a tuple all the way into the pair builder.
type groupKey [2]string

// pairKey is the unordered identity of a contact pair, normalized so (a,b)
// and (b,a) map to the same key.
type pairKey struct {
	A uint
	B uint
}

// pairAccumulator collects pairs across the three tiers. Reasons is a set so
// a pair matching on email AND phone records both.
type pairAccumulator struct {
	reasons map[string]bool
}

// duplicateReasons is the canonical ordering of the reason tokens on the
// wire, matching the tier order (email, name, phone).
var duplicateReasons = []string{
	string(models.DuplicateReasonEmail),
	string(models.DuplicateReasonName),
	string(models.DuplicateReasonPhone),
}

// FindDuplicatePairs scans a user's contacts for duplicate candidates
// (T93, T93).
//
// It answers the set-wide question "which pairs of my contacts look like the
// same person?" — the deliberate counterpart of DetectDuplicate (import_service
// .go), which answers the per-row question "does this one incoming row match
// anything?" and stays as-is for the import path. The two share only their key
// functions (PhoneKey, the email/name normalizers).
//
// Detection is per-tier SQL grouping over columns Contact.BeforeSave already
// maintains — NOT n² DetectDuplicate calls (whose phone tier is itself an O(n)
// in-memory scan, so the naive approach is O(n²) with a full table load inside
// the loop):
//
//   - email: GROUP BY LOWER(email) over non-empty values.
//   - name:  GROUP BY LOWER(firstname), LOWER(lastname), both non-empty.
//     sort_name (T73) is deliberately NOT used as the key: it is
//     COALESCE-guarded and falls back to firstname alone, so it is not a
//     faithful firstname+lastname join. Contacts with no contact info at all
//     are excluded from this tier specifically — pets and relationship stubs
//     ("Mum", unnamed pets) cluster hard on exact-name matching and are the
//     ticket's named false-positive source.
//   - phone: group on models.PhoneKey, read out of the phones_normalized
//     column (T69) by tokenizing its space-joined digit/PhoneKey tokens in SQL.
//     phones_normalized covers every Phones[] entry rather than just the flat
//     primary, so this tier finds shared non-primary numbers DetectDuplicate
//     misses — the report's "checking for duplicate phone numbers" clause.
//
// The bounded-query guarantee: exactly one query per tier plus one for the
// involved contacts' summaries and one for the dismissal set — a constant
// independent of contact count. Pinned by a test counting the queries.
//
// Archived contacts are included (ContactSummary carries Archived so the
// review surface can flag them); soft-deleted ones are excluded everywhere.
// Ownership is scoped by user_id in every query. Dismissed pairs
// (models.DismissedDuplicatePair) are filtered out.
func FindDuplicatePairs(db *gorm.DB, userID uint) ([]models.DuplicatePair, error) {
	// A window-function COUNT(*) OVER (PARTITION BY key) gets "only rows whose
	// key occurs more than once" in one pass per tier, instead of an
	// IN-subquery that repeats the whole table scan. Soft-deleted rows are
	// excluded explicitly: db.Raw bypasses GORM's default soft-delete scope.
	var emailRows []tierRow
	if err := db.Raw(`SELECT id, key FROM (
			SELECT id, LOWER(email) AS key, '' AS key2,
			       COUNT(*) OVER (PARTITION BY LOWER(email)) AS cnt
			FROM contacts
			WHERE user_id = ? AND deleted_at IS NULL AND email != ''
		) WHERE cnt > 1`, userID).Scan(&emailRows).Error; err != nil {
		return nil, err
	}

	var nameRows []tierRow
	if err := db.Raw(`SELECT id, key, key2 FROM (
			SELECT id, LOWER(firstname) AS key, LOWER(lastname) AS key2,
			       COUNT(*) OVER (PARTITION BY LOWER(firstname), LOWER(lastname)) AS cnt
			FROM contacts
			WHERE user_id = ? AND deleted_at IS NULL AND firstname != '' AND lastname != ''
			  AND (email != '' OR phone != '' OR phones_normalized != '')
		) WHERE cnt > 1`, userID).Scan(&nameRows).Error; err != nil {
		return nil, err
	}

	// Phone tier: tokenize phones_normalized (space-joined full-digit strings
	// and PhoneKeys, see FlattenPhones in models/contact.go) via a json_each
	// split, keep tokens of at least 7 digits (PhoneKey returns "" below 7),
	// then keep only tokens shared by more than one contact.
	//
	// T113: the outer SELECT is DISTINCT on (id, token) so a single contact can
	// never emit the same token twice. FlattenPhones legitimately emits a
	// duplicate token when a contact has two numbers that reduce to the same
	// PhoneKey (e.g. "+1 800 555 1234" next to "800-555-1234"), and without
	// DISTINCT the scan would pair that contact with ITSELF — which then made
	// the web review surface offer a same-person merge that failed with
	// "merge_id must differ from keep_id".
	var phoneRows []tierRow
	if err := db.Raw(`WITH split AS (
			SELECT contacts.id, value AS token
			FROM contacts, json_each('["' || replace(phones_normalized, ' ', '","') || '"]')
			WHERE user_id = ? AND deleted_at IS NULL AND phones_normalized != ''
			  AND json_valid('["' || replace(phones_normalized, ' ', '","') || '"]')
			  AND length(value) >= 7
		)
		SELECT DISTINCT split.id, token AS key, '' AS key2 FROM split
		WHERE token IN (SELECT token FROM split GROUP BY token HAVING COUNT(*) > 1)`, userID).Scan(&phoneRows).Error; err != nil {
		return nil, err
	}

	// Merge the tier hits into an unordered pair set with the full reason set
	// per pair.
	pairs := map[pairKey]*pairAccumulator{}
	involved := map[uint]bool{}
	addTier := func(rows []tierRow, reason string) {
		groups := map[groupKey][]uint{}
		for _, r := range rows {
			gk := groupKey{r.Key, r.Key2}
			groups[gk] = append(groups[gk], r.ID)
		}
		for _, group := range groups {
			for i := 0; i < len(group); i++ {
				for j := i + 1; j < len(group); j++ {
					a, b := group[i], group[j]
					// T113: defense in depth for any tier that could hand us a
					// self-row (a contact twice in its own group) -- the phone
					// tier is DISTINCT-guarded at the SQL, but a future tier
					// must not silently pair a contact with itself either.
					if a == b {
						continue
					}
					if a > b {
						a, b = b, a
					}
					k := pairKey{A: a, B: b}
					acc, ok := pairs[k]
					if !ok {
						acc = &pairAccumulator{reasons: map[string]bool{}}
						pairs[k] = acc
					}
					acc.reasons[reason] = true
					involved[a] = true
					involved[b] = true
				}
			}
		}
	}
	addTier(emailRows, string(models.DuplicateReasonEmail))
	addTier(nameRows, string(models.DuplicateReasonName))
	addTier(phoneRows, string(models.DuplicateReasonPhone))

	if len(pairs) == 0 {
		return []models.DuplicatePair{}, nil
	}

	// Load the ContactSummaries for every involved contact in one query.
	idList := make([]uint, 0, len(involved))
	for id := range involved {
		idList = append(idList, id)
	}
	var contacts []models.Contact
	if err := db.Select(models.ContactSummaryColumns).Where("user_id = ? AND id IN ?", userID, idList).Find(&contacts).Error; err != nil {
		return nil, err
	}
	summaryByID := make(map[uint]models.ContactSummary, len(contacts))
	for i := range contacts {
		summaryByID[contacts[i].ID] = models.NewContactSummary(&contacts[i])
	}

	// Load the dismissal set once and key it on the ordered uid pair.
	var dismissals []models.DismissedDuplicatePair
	if err := db.Where("user_id = ?", userID).Find(&dismissals).Error; err != nil {
		return nil, err
	}
	dismissed := make(map[[2]string]bool, len(dismissals))
	for _, d := range dismissals {
		dismissed[[2]string{d.UIDLow, d.UIDHigh}] = true
	}

	result := make([]models.DuplicatePair, 0, len(pairs))
	for k, acc := range pairs {
		aSum, aOK := summaryByID[k.A]
		bSum, bOK := summaryByID[k.B]
		if !aOK || !bOK {
			// One side vanished between detection and summary load (e.g. a
			// concurrent delete) — skip rather than emit a broken pair.
			continue
		}
		aUID, bUID := aSum.UID, bSum.UID
		if aUID > bUID {
			aUID, bUID = bUID, aUID
		}
		if dismissed[[2]string{aUID, bUID}] {
			continue
		}
		reasons := make([]string, 0, len(acc.reasons))
		for _, r := range duplicateReasons {
			if acc.reasons[r] {
				reasons = append(reasons, r)
			}
		}
		result = append(result, models.DuplicatePair{
			A:          aSum,
			B:          bSum,
			Reasons:    reasons,
			Confidence: duplicateConfidence(acc.reasons),
		})
	}

	// Strongest first; a stable (a.id, b.id) tiebreak keeps offset pagination
	// deterministic across requests.
	sort.Slice(result, func(i, j int) bool {
		if result[i].Confidence != result[j].Confidence {
			return result[i].Confidence > result[j].Confidence
		}
		if result[i].A.ID != result[j].A.ID {
			return result[i].A.ID < result[j].A.ID
		}
		return result[i].B.ID < result[j].B.ID
	})

	return result, nil
}

// duplicateConfidence maps the set of matched tiers to a 0-1 heuristic.
// More independent tiers → far more likely a real duplicate; name alone is
// the false-positive tier and scores lowest.
func duplicateConfidence(reasons map[string]bool) float64 {
	_, hasEmail := reasons[string(models.DuplicateReasonEmail)]
	_, hasName := reasons[string(models.DuplicateReasonName)]
	_, hasPhone := reasons[string(models.DuplicateReasonPhone)]
	switch {
	case hasEmail && hasPhone && hasName:
		return 0.98
	case hasEmail && hasPhone:
		return 0.95
	case hasName && (hasEmail || hasPhone):
		return 0.9
	case hasPhone:
		return 0.75
	case hasEmail:
		return 0.7
	default:
		// name only — the false-positive tier
		return 0.5
	}
}
