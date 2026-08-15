package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"mycorrhizal/contactmodel"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// memberClass is the suggestion engine's classification of a household
// member — see classifyMember. Confirmed with the user during WP-83
// planning: derived from Role + Contact.CRM.Kind only, never from
// Birthday/age (birthdays are frequently unknown, especially for the thin
// entities WP-81 promotes name-only relationships into).
type memberClass int

const (
	classAdult memberClass = iota
	classChild
	classPet
)

// classifyMember decides what role a household member plays in the
// suggestion rules below. Pet/animal is authoritative from Contact.CRM.Kind
// (WP-82 built that field for exactly this). Among humans, only an explicit
// "child" Role means child — every other Role value (including future ones
// this switch doesn't know about) defaults to adult, so the engine never
// blocks on an unrecognized or missing Role.
func classifyMember(role string, contact models.Contact) memberClass {
	if contact.CRM.Kind == "animal" {
		return classPet
	}
	if role == models.HouseholdRoleChild {
		return classChild
	}
	return classAdult
}

type classifiedMember struct {
	vcardUID string
	class    memberClass
}

// GenerateHouseholdSuggestions is the mechanism from docs/adrs/0001-neutral-hub-and-spoke-contact-model.md: re-scans a household's CURRENT membership
// and idempotently ensures a suggested RelationshipEdge exists for every
// applicable pair, rather than diffing what changed since a prior call —
// simpler and safe to call repeatedly (e.g. after every membership add).
//
// Every generated edge has Status: suggested, Source: household-inferred —
// §91.4 is explicit that a household's membership is never treated as a
// hard fact on its own, no matter how confidently the type implies a
// relationship. Confirming or rejecting a suggestion is a user action in a
// review surface this WP does not build (P-later, per the roadmap).
//
// Returns the edges newly created by this call (not ones that already
// existed and were skipped).
func GenerateHouseholdSuggestions(db *gorm.DB, household models.Household) ([]models.RelationshipEdge, error) {
	var members []models.HouseholdMember
	if err := db.Where("household_id = ?", household.ID).Find(&members).Error; err != nil {
		return nil, fmt.Errorf("loading members for household id=%s: %w", household.ID, err)
	}
	if len(members) < 2 {
		return []models.RelationshipEdge{}, nil
	}

	classified := make([]classifiedMember, 0, len(members))
	for _, m := range members {
		var contact models.Contact
		if err := db.Where("vcard_uid = ? AND user_id = ?", m.MemberVCardUID, household.UserID).First(&contact).Error; err != nil {
			return nil, fmt.Errorf("loading contact vcard_uid=%s for household id=%s: %w", m.MemberVCardUID, household.ID, err)
		}
		classified = append(classified, classifiedMember{vcardUID: m.MemberVCardUID, class: classifyMember(m.Role, contact)})
	}

	created := []models.RelationshipEdge{}
	suggest := func(sourceID, targetID, edgeType string, confidence float64) error {
		edge, err := suggestEdgeIfNew(db, household.UserID, sourceID, targetID, edgeType, confidence)
		if err != nil {
			return err
		}
		if edge != nil {
			created = append(created, *edge)
		}
		return nil
	}

	switch household.Type {
	case models.HouseholdTypeFamilyUnit:
		// §91.4: adult<->adult spouse_of; adult->child parent_of; every
		// HUMAN (adult or child, not just adult) -> pet owned_by.
		const familyConfidence = 0.8
		for i := 0; i < len(classified); i++ {
			for j := i + 1; j < len(classified); j++ {
				a, b := classified[i], classified[j]
				switch {
				case a.class == classAdult && b.class == classAdult:
					if err := suggest(a.vcardUID, b.vcardUID, "spouse_of", familyConfidence); err != nil {
						return created, err
					}
				case a.class == classAdult && b.class == classChild:
					if err := suggest(a.vcardUID, b.vcardUID, "parent_of", familyConfidence); err != nil {
						return created, err
					}
				case b.class == classAdult && a.class == classChild:
					if err := suggest(b.vcardUID, a.vcardUID, "parent_of", familyConfidence); err != nil {
						return created, err
					}
				case a.class != classPet && b.class == classPet:
					// "A owned_by B" reads "A is owned by B" — the pet is
					// the source, matching the type registry's own
					// convention (models/relationship_type_registry.go).
					if err := suggest(b.vcardUID, a.vcardUID, "owned_by", familyConfidence); err != nil {
						return created, err
					}
				case b.class != classPet && a.class == classPet:
					if err := suggest(a.vcardUID, b.vcardUID, "owned_by", familyConfidence); err != nil {
						return created, err
					}
				// child<->child and pet<->pet: no rule in §91.4; skipped.
				default:
				}
			}
		}

	case models.HouseholdTypeRoommates:
		// §91.4: member<->member roommate_of only — explicitly never
		// parent/owner/spouse, regardless of role or kind.
		const roommateConfidence = 0.4
		for i := 0; i < len(classified); i++ {
			for j := i + 1; j < len(classified); j++ {
				if err := suggest(classified[i].vcardUID, classified[j].vcardUID, "roommate_of", roommateConfidence); err != nil {
					return created, err
				}
			}
		}

	default:
		// "other", and any type value this switch doesn't recognize: no
		// structural inference (§91.4's own table says exactly this for
		// "other" — unrecognized values get the same treatment, not an
		// error, matching how the rest of this WP degrades on open enums).
	}

	return created, nil
}

// suggestEdgeIfNew creates a suggested RelationshipEdge for (sourceID,
// targetID, edgeType) unless an edge for that relationship already exists —
// checked in EITHER storage direction, so a relationship already recorded
// as (target, source, InverseRelationType(edgeType)) is recognized as the
// same fact and not duplicated. Covers both the symmetric case (a type's
// inverse is itself) and the directional case (the reciprocal token)
// uniformly. Matches regardless of the existing edge's status — a
// `confirmed` edge is never re-suggested over, any more than a `suggested`
// one is duplicated.
//
// Returns the created edge, or nil if one already existed.
func suggestEdgeIfNew(db *gorm.DB, userID uint, sourceID, targetID, edgeType string, confidence float64) (*models.RelationshipEdge, error) {
	inverse := models.InverseRelationType(edgeType)

	var count int64
	err := db.Model(&models.RelationshipEdge{}).Where(
		"(source_id = ? AND target_id = ? AND type = ? AND user_id = ?) OR (source_id = ? AND target_id = ? AND type = ? AND user_id = ?)",
		sourceID, targetID, edgeType, userID,
		targetID, sourceID, inverse, userID,
	).Count(&count).Error
	if err != nil {
		return nil, fmt.Errorf("checking for an existing %s edge %s->%s: %w", edgeType, sourceID, targetID, err)
	}
	if count > 0 {
		return nil, nil
	}

	edge := models.RelationshipEdge{
		UserID:      userID,
		SourceID:    sourceID,
		TargetID:    targetID,
		Type:        edgeType,
		Directional: !models.IsSymmetricRelationType(edgeType),
		Source:      models.RelationshipSourceHouseholdInferred,
		Confidence:  confidence,
		Status:      models.RelationshipStatusSuggested,
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	if err := db.Create(&edge).Error; err != nil {
		return nil, fmt.Errorf("creating suggested %s edge %s->%s: %w", edgeType, sourceID, targetID, err)
	}
	return &edge, nil
}

// ---------------------------------------------------------------------------
// T40 — address-based household suggestions
// (T40)
//
// The T1 engine above only proposes RelationshipEdges *within* an existing
// household. This half scans contacts who share a normalized address but
// aren't co-members of any household yet, and surfaces each such group as a
// suggestion to create one. Detection is on-demand (an explicit trigger, like
// the T1 engine), propose-then-approve only — nothing is auto-created.
// ---------------------------------------------------------------------------

// normalizeAddressPart is T40's per-component normalization: lowercase, drop
// anything that isn't a letter/digit/space, collapse internal whitespace.
// Two addresses differing only in casing, trailing punctuation, or spacing
// ("123 Main St.", "123 main st") normalize to the same part.
func normalizeAddressPart(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// AddressNormalizedKey is T40's canonical identity for a structured address:
// the normalized street/city/region/postal/country parts joined by "|",
// omitting empty parts. The ticket's documented comparison scope (street +
// city/region + postal) plus country as a disambiguator. The sub-street parts
// the flat model gained in T79 (PO box / apartment / floor) are deliberately
// NOT part of the key: two addresses sharing a street but differing in
// apartment are still the same building, and a household suggestion is about
// shared residence — keeping them out preserves T40's recall instead of
// narrowing it. Address Type (home/work) is likewise deliberately NOT part of
// the key — a "work" address matching someone's "home" address is still the
// same physical residence.
func AddressNormalizedKey(a models.ContactAddress) string {
	parts := []string{
		normalizeAddressPart(a.Street),
		normalizeAddressPart(a.City),
		normalizeAddressPart(a.Region),
		normalizeAddressPart(a.Postal),
		normalizeAddressPart(a.Country),
	}
	nonEmpty := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, "|")
}

// AddressSuggestion is one T40 suggestion: a group of 2+ contacts sharing a
// normalized address, with the stable hash pair that identifies it for
// accept/dismiss and the shared address to display and to copy onto the
// created household.
type AddressSuggestion struct {
	// AddressHash is SHA-256 hex of the normalized address key; MemberHash is
	// SHA-256 hex of the sorted member VCardUIDs joined. Together they are
	// the dismissal-table identity (models.DismissedHouseholdSuggestion).
	AddressHash     string               `json:"address_hash"`
	MemberHash      string               `json:"member_hash"`
	MemberVCardUIDs []string             `json:"member_vcard_uids"`
	Address         contactmodel.Address `json:"address"`
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// dedupeContacts removes duplicate contacts from a slice by VCardUID, keeping
// first occurrence order — a contact listing the same address twice in its
// Addresses array would otherwise appear twice in one group.
func dedupeContacts(contacts []models.Contact) []models.Contact {
	seen := map[string]bool{}
	out := make([]models.Contact, 0, len(contacts))
	for _, c := range contacts {
		if seen[c.VCardUID] {
			continue
		}
		seen[c.VCardUID] = true
		out = append(out, c)
	}
	return out
}

// groupAlreadyCoMembers reports whether ANY pair of the group is already a
// co-member of a household. The ticket's rule — contacts "who are not already
// co-members of any existing Household" — is applied group-wide: a single
// pre-existing pair vetoes the whole group, because the group's point is to
// be one new co-residence and partially-covered groups have no clean
// interpretation.
func groupAlreadyCoMembers(db *gorm.DB, userID uint, uids []string) (bool, error) {
	var memberships []models.HouseholdMember
	if err := db.Where("user_id = ? AND member_vcard_uid IN ?", userID, uids).Find(&memberships).Error; err != nil {
		return false, fmt.Errorf("checking existing household co-membership: %w", err)
	}
	perHousehold := map[string][]string{}
	for _, m := range memberships {
		perHousehold[m.HouseholdID] = append(perHousehold[m.HouseholdID], m.MemberVCardUID)
	}
	for _, householdUIDs := range perHousehold {
		if len(householdUIDs) >= 2 {
			return true, nil
		}
	}
	return false, nil
}

// groupIsDismissed reports whether the (address_hash, member_hash) group has
// been dismissed by this user.
func groupIsDismissed(db *gorm.DB, userID uint, addressHash, memberHash string) (bool, error) {
	var count int64
	err := db.Model(&models.DismissedHouseholdSuggestion{}).
		Where("user_id = ? AND address_hash = ? AND member_hash = ?", userID, addressHash, memberHash).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("checking dismissed household suggestions: %w", err)
	}
	return count > 0, nil
}

// GenerateAddressHouseholdSuggestions is T40's detection pass: group the
// user's own non-archived contacts by normalized address and return every
// group of 2+ that is not already a co-resident household and not dismissed.
// Read-only — nothing is persisted here. Deterministic ordering (by
// address_hash) so a client's list and tests stay stable across runs.
func GenerateAddressHouseholdSuggestions(db *gorm.DB, userID uint) ([]AddressSuggestion, error) {
	var contacts []models.Contact
	if err := db.Where("user_id = ? AND archived = ?", userID, false).Find(&contacts).Error; err != nil {
		return nil, fmt.Errorf("loading contacts for address suggestions: %w", err)
	}

	groups := map[string][]models.Contact{}
	for _, c := range contacts {
		for _, addr := range c.Addresses {
			key := AddressNormalizedKey(addr)
			if key == "" {
				continue
			}
			groups[key] = append(groups[key], c)
		}
	}

	suggestions := []AddressSuggestion{}
	for key, members := range groups {
		members = dedupeContacts(members)
		if len(members) < 2 {
			continue
		}
		// Sort by VCardUID so the shared address below is deterministic
		// (always the lexicographically-first member's address), keeping a
		// given group stable across scans regardless of DB row order.
		sort.Slice(members, func(i, j int) bool { return members[i].VCardUID < members[j].VCardUID })

		uids := make([]string, 0, len(members))
		for _, m := range members {
			uids = append(uids, m.VCardUID)
		}
		sort.Strings(uids)

		addressHash := sha256Hex(key)
		memberHash := sha256Hex(strings.Join(uids, ","))

		coMember, err := groupAlreadyCoMembers(db, userID, uids)
		if err != nil {
			return nil, err
		}
		if coMember {
			continue
		}

		dismissed, err := groupIsDismissed(db, userID, addressHash, memberHash)
		if err != nil {
			return nil, err
		}
		if dismissed {
			continue
		}

		// The shared address for display/creation: the first member's
		// address that normalized to this key.
		var sharedAddress models.ContactAddress
		for _, m := range members {
			for _, addr := range m.Addresses {
				if AddressNormalizedKey(addr) == key {
					sharedAddress = addr
					break
				}
			}
			if sharedAddress.Street != "" || sharedAddress.City != "" || sharedAddress.Region != "" || sharedAddress.Postal != "" || sharedAddress.Country != "" {
				break
			}
		}

		suggestions = append(suggestions, AddressSuggestion{
			AddressHash:     addressHash,
			MemberHash:      memberHash,
			MemberVCardUIDs: uids,
			Address:         models.AddressFromContactAddress(sharedAddress),
		})
	}

	sort.Slice(suggestions, func(i, j int) bool { return suggestions[i].AddressHash < suggestions[j].AddressHash })
	return suggestions, nil
}

// AcceptAddressHouseholdSuggestion creates the Household (+ member rows) for
// a suggested group (T40's accept action). The members are re-validated
// server-side rather than trusting client-supplied hashes or addresses:
// they must be the user's own contacts, must still share a normalized
// address, must not already be co-members of a household, and must not be a
// dismissed group. The shared address is copied onto the household; Name and
// Type default when not supplied (Type validated when it is). Roles default
// to "adult" for every member.
func AcceptAddressHouseholdSuggestion(db *gorm.DB, userID uint, memberVCardUIDs []string, name, householdType string) (*models.Household, error) {
	if len(memberVCardUIDs) < 2 {
		return nil, apperrors.ErrInvalidInput("member_vcard_uids", "at least two contacts are required to form a household")
	}
	if householdType != "" && !validHouseholdType(householdType) {
		return nil, apperrors.ErrInvalidInput("type", "type must be one of family_unit, roommates, other")
	}

	uids := make([]string, len(memberVCardUIDs))
	copy(uids, memberVCardUIDs)
	sort.Strings(uids)
	deduped := uids[:0]
	for i, uid := range uids {
		if i == 0 || uid != uids[i-1] {
			deduped = append(deduped, uid)
		}
	}
	uids = deduped
	if len(uids) < 2 {
		return nil, apperrors.ErrInvalidInput("member_vcard_uids", "at least two distinct contacts are required to form a household")
	}

	var contacts []models.Contact
	if err := db.Where("user_id = ? AND vcard_uid IN ?", userID, uids).Find(&contacts).Error; err != nil {
		return nil, fmt.Errorf("loading suggested household members: %w", err)
	}
	if len(contacts) != len(uids) {
		return nil, apperrors.ErrNotFound("Contact")
	}
	byUID := map[string]models.Contact{}
	for _, c := range contacts {
		byUID[c.VCardUID] = c
	}

	// Recompute the shared address from the members' actual data: every
	// member must have at least one address normalizing to a common key.
	orderedContacts := make([]models.Contact, 0, len(uids))
	for _, uid := range uids {
		orderedContacts = append(orderedContacts, byUID[uid])
	}
	sharedKey, sharedAddress := sharedAddressKey(orderedContacts)
	if sharedKey == "" {
		return nil, apperrors.ErrConflict("The suggested members no longer share an address — re-run the suggestion scan")
	}

	addressHash := sha256Hex(sharedKey)
	memberHash := sha256Hex(strings.Join(uids, ","))

	coMember, err := groupAlreadyCoMembers(db, userID, uids)
	if err != nil {
		return nil, err
	}
	if coMember {
		return nil, apperrors.ErrConflict("These contacts are already in a household together")
	}

	dismissed, err := groupIsDismissed(db, userID, addressHash, memberHash)
	if err != nil {
		return nil, err
	}
	if dismissed {
		return nil, apperrors.ErrConflict("This suggestion was dismissed — re-run the scan to re-offer it, or create the household manually")
	}

	if householdType == "" {
		householdType = models.HouseholdTypeFamilyUnit
	}
	if name == "" {
		names := make([]string, 0, len(uids))
		for _, uid := range uids {
			c := byUID[uid]
			names = append(names, firstnames(c))
		}
		name = strings.Join(names, " & ")
	}

	household := models.Household{
		UserID: userID,
		Name:   name,
		Type:   householdType,
	}
	addr := models.AddressFromContactAddress(sharedAddress)
	household.Address = &addr

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&household).Error; err != nil {
			return err
		}
		for _, uid := range uids {
			member := models.HouseholdMember{
				HouseholdID:    household.ID,
				UserID:         userID,
				MemberVCardUID: uid,
				Role:           models.HouseholdRoleAdult,
			}
			if err := tx.Create(&member).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("creating household from shared address: %w", err)
	}

	return &household, nil
}

func validHouseholdType(t string) bool {
	switch t {
	case models.HouseholdTypeFamilyUnit, models.HouseholdTypeRoommates, models.HouseholdTypeOther:
		return true
	}
	return false
}

// sharedAddressKey returns the normalized address key shared by every given
// contact (plus a representative ContactAddress carrying it), or "" when no
// single key appears in all of them. Contacts may hold multiple addresses;
// any key present in every contact's address list is acceptable. Used by the
// accept and dismiss paths to re-derive a suggestion's shared address
// server-side from the members' real data rather than trusting a client.
func sharedAddressKey(contacts []models.Contact) (string, models.ContactAddress) {
	if len(contacts) == 0 {
		return "", models.ContactAddress{}
	}
	keys := make([][]string, len(contacts))
	for i, c := range contacts {
		seen := map[string]bool{}
		for _, addr := range c.Addresses {
			key := AddressNormalizedKey(addr)
			if key != "" && !seen[key] {
				keys[i] = append(keys[i], key)
				seen[key] = true
			}
		}
	}
	for _, candidate := range keys[0] {
		inAll := true
		for i := 1; i < len(keys); i++ {
			found := false
			for _, mk := range keys[i] {
				if mk == candidate {
					found = true
					break
				}
			}
			if !found {
				inAll = false
				break
			}
		}
		if inAll {
			for _, addr := range contacts[0].Addresses {
				if AddressNormalizedKey(addr) == candidate {
					return candidate, addr
				}
			}
		}
	}
	return "", models.ContactAddress{}
}

func firstnames(c models.Contact) string {
	if c.Firstname != "" {
		return c.Firstname
	}
	if c.FN != "" {
		return c.FN
	}
	return c.Nickname
}

// DismissAddressHouseholdSuggestion records a permanent rejection for a
// suggested group (T40's dismiss action): inserts the (address_hash,
// member_hash) pair computed from the member contacts. Recomputes
// server-side so a client cannot dismiss an arbitrary hash. Re-dismissing an
// already-dismissed group is a checked ErrAlreadyExists (409), not a sniffed
// constraint error — the natural-key unique index remains the DB-level safety
// net beneath that check.
func DismissAddressHouseholdSuggestion(db *gorm.DB, userID uint, memberVCardUIDs []string) error {
	uids := make([]string, len(memberVCardUIDs))
	copy(uids, memberVCardUIDs)
	sort.Strings(uids)
	deduped := uids[:0]
	for i, uid := range uids {
		if i == 0 || uid != uids[i-1] {
			deduped = append(deduped, uid)
		}
	}
	uids = deduped
	if len(uids) < 2 {
		return apperrors.ErrInvalidInput("member_vcard_uids", "at least two distinct contacts are required")
	}

	var contacts []models.Contact
	if err := db.Where("user_id = ? AND vcard_uid IN ?", userID, uids).Find(&contacts).Error; err != nil {
		return fmt.Errorf("loading dismissed suggestion members: %w", err)
	}
	if len(contacts) != len(uids) {
		return apperrors.ErrNotFound("Contact")
	}

	// Recover the shared address key from the members' actual addresses.
	sharedKey, _ := sharedAddressKey(contacts)
	if sharedKey == "" {
		return apperrors.ErrInvalidInput("member_vcard_uids", "these contacts do not share an address")
	}

	addressHash := sha256Hex(sharedKey)
	memberHash := sha256Hex(strings.Join(uids, ","))

	already, err := groupIsDismissed(db, userID, addressHash, memberHash)
	if err != nil {
		return err
	}
	if already {
		return apperrors.ErrAlreadyExists("Household suggestion dismissal")
	}

	suggestion := models.DismissedHouseholdSuggestion{
		UserID:      userID,
		AddressHash: addressHash,
		MemberHash:  memberHash,
	}
	if err := db.Create(&suggestion).Error; err != nil {
		return fmt.Errorf("recording dismissed household suggestion: %w", err)
	}
	return nil
}
