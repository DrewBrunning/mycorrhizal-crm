package services

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"mycorrhizal/contactmodel"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Address suggestions from relationships and household membership (inverse of
// T40's "suggest households from shared address"): the relationship/household
// already exists, and we want to propose the *address* the other party's
// record carries, since addresses drift out of sync (a spouse/roommate linked
// later, a new household member, etc.).
//
// Read-only generation + explicit apply: nothing is written at generate time,
// and an apply re-derives the address server-side from the current graph so a
// client can neither stuff an arbitrary address onto a contact nor apply a
// suggestion that no longer holds.
// ---------------------------------------------------------------------------

// addressSuggestionRelationshipTypes is the closed set of confirmed edge types
// whose existence implies — but doesn't guarantee — a shared address
// (docs: parent/child, spouse, roommate). Only status: confirmed edges are
// fact; a suggested (household-inferred) edge must never drive an address
// suggestion.
var addressSuggestionRelationshipTypes = []string{"parent_of", "child_of", "spouse_of", "roommate_of"}

// addressSuggestionSource values stored on ContactAddressSuggestion.SourceKind.
const (
	addressSuggestionSourceRelationship = "relationship"
	addressSuggestionSourceHousehold    = "household"
)

// ContactAddressSuggestion is one address proposal: contact C (the recipient)
// should consider the address the Source carries (a related contact or a
// household), because C and the Source are connected in a way that implies a
// shared residence. AddressKey is the normalized-address identity (the same
// key T40's grouping uses) so a client can dedupe and an apply can name which
// address it means.
type ContactAddressSuggestion struct {
	ContactVCardUID string `json:"contact_vcard_uid"`
	ContactName     string `json:"contact_name"`
	// SourceKind: "relationship" | "household"; SourceID is the other
	// contact's VCardUID or the household's ID.
	SourceKind   string               `json:"source_kind"`
	SourceID     string               `json:"source_id"`
	SourceName   string               `json:"source_name"`
	RelationType string               `json:"relation_type,omitempty"` // relationship sources only, from the recipient's perspective
	Address      contactmodel.Address `json:"address"`
	AddressKey   string               `json:"address_key"`
}

// displayContactName renders a contact's best display name for the suggestion
// surfaces (full name, then FN, then nickname — the same fallback chain the
// household name builder uses).
func displayContactName(c models.Contact) string {
	if n := strings.TrimSpace(c.Firstname + " " + c.Lastname); n != "" {
		return n
	}
	if c.FN != "" {
		return c.FN
	}
	if c.Nickname != "" {
		return c.Nickname
	}
	return c.VCardUID
}

// neutralAddressKey computes the normalized-identity key for a neutral
// contactmodel.Address by projecting its components back onto the flat
// ContactAddress shape and running the same AddressNormalizedKey T40 uses for
// flat addresses (street + city/region + postal + country; sub-street parts
// and type deliberately excluded, matching T40's shared-residence scope).
func neutralAddressKey(a contactmodel.Address) string {
	return AddressNormalizedKey(contactAddressFromNeutralProjection(a))
}

// GenerateContactAddressSuggestions scans the user's relationship edges and
// households and returns every address proposal that is not already present
// on the recipient contact. Read-only — nothing is persisted here. Deterministic
// ordering (by contact, then address key, then source) so a client's list and
// tests stay stable across runs.
//
// Recipients are the user's non-archived contacts (an archived contact is out
// of rotation); a source contact that is archived is likewise not used, and a
// confirmed edge or household membership referencing a missing contact is
// skipped. Sensitivity: confirmed edges with sensitivity != secret participate
// (mirroring the graph engine's rule — a secret edge must not leak into a
// derived suggestion); a private edge may seed a normal-sensitivity
// suggestion, the same minor tradeoff T104 accepts.
func GenerateContactAddressSuggestions(db *gorm.DB, userID uint) ([]ContactAddressSuggestion, error) {
	var contacts []models.Contact
	if err := db.Where("user_id = ? AND archived = ?", userID, false).Find(&contacts).Error; err != nil {
		return nil, fmt.Errorf("loading contacts for address suggestions: %w", err)
	}
	byUID := make(map[string]models.Contact, len(contacts))
	existingKeys := make(map[string]map[string]bool, len(contacts))
	for _, c := range contacts {
		byUID[c.VCardUID] = c
		keys := map[string]bool{}
		for _, a := range c.Addresses {
			if k := AddressNormalizedKey(a); k != "" {
				keys[k] = true
			}
		}
		existingKeys[c.VCardUID] = keys
	}

	candidates := []ContactAddressSuggestion{}

	// Relationship sources: each confirmed, non-secret edge of the four types
	// proposes each party's addresses to the other.
	var edges []models.RelationshipEdge
	if err := db.Where("user_id = ? AND status = ? AND sensitivity != ? AND type IN ?",
		userID, models.RelationshipStatusConfirmed, models.RelationshipSensitivitySecret, addressSuggestionRelationshipTypes).
		Find(&edges).Error; err != nil {
		return nil, fmt.Errorf("loading confirmed edges for address suggestions: %w", err)
	}
	for _, e := range edges {
		src, srcOK := byUID[e.SourceID]
		dst, dstOK := byUID[e.TargetID]
		if !srcOK || !dstOK {
			continue
		}
		// Propose the target's addresses to the source; the stored type
		// already reads "what the source is to the target".
		for _, a := range dst.Addresses {
			candidates = append(candidates, ContactAddressSuggestion{
				ContactVCardUID: src.VCardUID,
				ContactName:     displayContactName(src),
				SourceKind:      addressSuggestionSourceRelationship,
				SourceID:        dst.VCardUID,
				SourceName:      displayContactName(dst),
				RelationType:    e.Type,
				Address:         models.AddressFromContactAddress(a),
				AddressKey:      AddressNormalizedKey(a),
			})
		}
		// And the source's addresses to the target — the relation token
		// inverted, because it must read "what the target is to the source".
		for _, a := range src.Addresses {
			candidates = append(candidates, ContactAddressSuggestion{
				ContactVCardUID: dst.VCardUID,
				ContactName:     displayContactName(dst),
				SourceKind:      addressSuggestionSourceRelationship,
				SourceID:        src.VCardUID,
				SourceName:      displayContactName(src),
				RelationType:    models.InverseRelationType(e.Type),
				Address:         models.AddressFromContactAddress(a),
				AddressKey:      AddressNormalizedKey(a),
			})
		}
	}

	// Household sources: each household with an address proposes it to every
	// non-archived member. The stronger implication — same household should
	// mean same address.
	var households []models.Household
	if err := db.Where("user_id = ?", userID).Find(&households).Error; err != nil {
		return nil, fmt.Errorf("loading households for address suggestions: %w", err)
	}
	for _, h := range households {
		if h.Address == nil {
			continue
		}
		key := neutralAddressKey(*h.Address)
		if key == "" {
			continue
		}
		var members []models.HouseholdMember
		if err := db.Where("household_id = ? AND user_id = ?", h.ID, userID).Find(&members).Error; err != nil {
			return nil, fmt.Errorf("loading members for household id=%s: %w", h.ID, err)
		}
		for _, m := range members {
			member, ok := byUID[m.MemberVCardUID]
			if !ok {
				continue
			}
			candidates = append(candidates, ContactAddressSuggestion{
				ContactVCardUID: member.VCardUID,
				ContactName:     displayContactName(member),
				SourceKind:      addressSuggestionSourceHousehold,
				SourceID:        h.ID,
				SourceName:      h.Name,
				Address:         *h.Address,
				AddressKey:      key,
			})
		}
	}

	// Deterministic ordering, then dedupe by (contact, address key) keeping
	// the first (deterministic) source. A recipient that already carries the
	// key — including one added since the scan started — is dropped here.
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ContactVCardUID != candidates[j].ContactVCardUID {
			return candidates[i].ContactVCardUID < candidates[j].ContactVCardUID
		}
		if candidates[i].AddressKey != candidates[j].AddressKey {
			return candidates[i].AddressKey < candidates[j].AddressKey
		}
		if candidates[i].SourceKind != candidates[j].SourceKind {
			return candidates[i].SourceKind < candidates[j].SourceKind
		}
		return candidates[i].SourceID < candidates[j].SourceID
	})

	seen := map[string]bool{}
	deduped := make([]ContactAddressSuggestion, 0, len(candidates))
	for _, s := range candidates {
		if existingKeys[s.ContactVCardUID][s.AddressKey] {
			continue
		}
		id := s.ContactVCardUID + "|" + s.AddressKey
		if seen[id] {
			continue
		}
		seen[id] = true
		deduped = append(deduped, s)
	}
	return deduped, nil
}

// ApplyContactAddressSuggestion adds the address a suggestion points at to the
// contact's Addresses, re-deriving everything server-side from the current
// graph rather than trusting the client:
//
//   - the contact must exist and belong to the user;
//   - the source must still imply a shared address (a confirmed, non-secret
//     edge of the four types to the source contact, or current household
//     membership);
//   - the source must still carry an address whose normalized key equals the
//     claimed AddressKey.
//
// The derived address is appended (never duplicates a present key) and the
// contact is saved through the T75 merge path, so Card-only data and the
// other addresses survive. Returns the saved contact.
//
// Client-visible rejections are *apperrors.AppError (404 unknown contact or
// source, 400 invalid key, 409 no longer valid); real failures are wrapped
// errors.
func ApplyContactAddressSuggestion(db *gorm.DB, userID uint, input *models.ApplyContactAddressSuggestionInput) (*models.Contact, error) {
	var contact models.Contact
	if err := db.Where("vcard_uid = ? AND user_id = ?", input.ContactVCardUID, userID).First(&contact).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrNotFound("Contact").WithDetails("contact_vcard_uid", input.ContactVCardUID)
		}
		return nil, fmt.Errorf("loading contact for address suggestion: %w", err)
	}

	address, aerr := suggestedAddressFor(db, userID, input)
	if aerr != nil {
		return nil, aerr
	}

	key := neutralAddressKey(address)
	for _, a := range contact.Addresses {
		if AddressNormalizedKey(a) == key {
			return nil, apperrors.ErrConflict("The contact already has this address")
		}
	}

	contact.Addresses = append(contact.Addresses, contactAddressFromNeutralProjection(address))
	if err := db.Save(&contact).Error; err != nil {
		return nil, fmt.Errorf("saving contact after address suggestion: %w", err)
	}
	return &contact, nil
}

// suggestedAddressFor recomputes the address a suggestion means from the
// current graph, per ApplyContactAddressSuggestion's re-derivation contract.
func suggestedAddressFor(db *gorm.DB, userID uint, input *models.ApplyContactAddressSuggestionInput) (contactmodel.Address, *apperrors.AppError) {
	key := input.AddressKey
	if key == "" {
		return contactmodel.Address{}, apperrors.ErrInvalidInput("address_key", "address_key is required")
	}

	switch input.SourceKind {
	case addressSuggestionSourceRelationship:
		// The source must be another contact reachable via a confirmed,
		// non-secret edge of one of the four types — either storage
		// direction. The same implication holds either way, so match on the
		// endpoint pair without fixing a direction.
		var edge models.RelationshipEdge
		err := db.Where("user_id = ? AND status = ? AND sensitivity != ? AND type IN ? AND ((source_id = ? AND target_id = ?) OR (source_id = ? AND target_id = ?))",
			userID, models.RelationshipStatusConfirmed, models.RelationshipSensitivitySecret, addressSuggestionRelationshipTypes,
			input.ContactVCardUID, input.SourceID, input.SourceID, input.ContactVCardUID,
		).First(&edge).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return contactmodel.Address{}, apperrors.ErrConflict("The suggested relationship no longer exists — re-run the address suggestion scan")
		}
		if err != nil {
			return contactmodel.Address{}, apperrors.ErrDatabase("Failed to verify the suggested relationship").WithError(err)
		}
		var source models.Contact
		if err := db.Where("vcard_uid = ? AND user_id = ?", input.SourceID, userID).First(&source).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return contactmodel.Address{}, apperrors.ErrNotFound("Contact").WithDetails("source_id", input.SourceID)
			}
			return contactmodel.Address{}, apperrors.ErrDatabase("Failed to load the source contact").WithError(err)
		}
		for _, a := range source.Addresses {
			if AddressNormalizedKey(a) == key {
				return models.AddressFromContactAddress(a), nil
			}
		}
		return contactmodel.Address{}, apperrors.ErrConflict("The source contact no longer has that address — re-run the address suggestion scan")

	case addressSuggestionSourceHousehold:
		var household models.Household
		if err := db.Where("id = ? AND user_id = ?", input.SourceID, userID).First(&household).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return contactmodel.Address{}, apperrors.ErrNotFound("Household").WithDetails("source_id", input.SourceID)
			}
			return contactmodel.Address{}, apperrors.ErrDatabase("Failed to load the household").WithError(err)
		}
		var membershipCount int64
		if err := db.Model(&models.HouseholdMember{}).Where("household_id = ? AND user_id = ? AND member_vcard_uid = ?", household.ID, userID, input.ContactVCardUID).Count(&membershipCount).Error; err != nil {
			return contactmodel.Address{}, apperrors.ErrDatabase("Failed to verify household membership").WithError(err)
		}
		if membershipCount == 0 {
			return contactmodel.Address{}, apperrors.ErrConflict("The contact is no longer a member of this household")
		}
		if household.Address == nil || neutralAddressKey(*household.Address) != key {
			return contactmodel.Address{}, apperrors.ErrConflict("The household no longer has that address — re-run the address suggestion scan")
		}
		return *household.Address, nil

	default:
		return contactmodel.Address{}, apperrors.ErrInvalidInput("source_kind", "source_kind must be one of relationship, household")
	}
}

// contactAddressFromNeutralProjection projects a neutral address onto the flat
// ContactAddress shape so it can be appended to Contact.Addresses. Mirrors the
// model's own contactAddressFromNeutral (models/contact_record_reverse.go) --
// including its T91 Contexts->Type translation so the applied address keeps
// its home/work type -- duplicated here rather than exported there because the
// models package's variant is deliberately unexported and this service must
// not reach into model internals.
func contactAddressFromNeutralProjection(a contactmodel.Address) models.ContactAddress {
	var out models.ContactAddress
	for _, comp := range a.Components {
		switch comp.Kind {
		case "name":
			out.Street = comp.Value
		case "postOfficeBox":
			out.POBox = comp.Value
		case "apartment":
			out.Apartment = comp.Value
		case "floor":
			out.Floor = comp.Value
		case "locality":
			out.City = comp.Value
		case "region":
			out.Region = comp.Value
		case "postcode":
			out.Postal = comp.Value
		case "country":
			out.Country = comp.Value
		}
	}
	// T91: the neutral private/work Contexts vocabulary translates back to the
	// flat model's home/work tokens; an unrecognized context is user data to
	// preserve, not a value to blank (same fall-through as the models' copy).
	if len(a.Contexts) > 0 {
		if tok, ok := contactmodel.ContextToTypeToken[a.Contexts[0]]; ok {
			out.Type = tok
		} else {
			out.Type = a.Contexts[0]
		}
	}
	return out
}
