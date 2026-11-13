package models

import (
	"encoding/json"
	"reflect"

	"mycorrhizal/contactmodel"

	"gorm.io/gorm"
)

// Entity projection -> contact revision (issue #2565, ADR 0043 decision 3,
// amended 2026-11-13).
//
// The projected ORG/ADR lives in no contact row, so a write to a link or an
// entity changes the card a CardDAV client would download without touching the
// contact. A sync client keys on the contact's ETag (CardDAV) and updated_at
// (the ?since= feed), so every writer that can change a projection brackets its
// write with an EntityProjectionSnapshot:
//
//	snap, err := models.SnapshotEntityProjections(tx, userID, uids) // before
//	... the write ...
//	_, err = snap.BumpChanged(tx)                                    // after
//
// Detection is by value, not by guessing which fields matter: a digest (the canonical JSON) of what
// the contact would project (the include-sensitive superset, the widest view any
// surface can show) is taken before and after, and only a contact whose digest
// moved is bumped. A rename of a pet, an edit of a link's role, a notes-only
// edit of an organisation, or a non-primary link therefore bump nothing.
//
// Cost is set-based: two statements per chunk of contacts for each digest and
// one UPDATE per chunk to bump, whatever the number of contacts. The bump runs
// in the writer's transaction.

// EntityProjectionSnapshot is the "before" half of a projection change check.
type EntityProjectionSnapshot struct {
	userID uint
	before map[string]string // vCard UID -> digest ("" = projects nothing)
	// own caches each contact's own ORG/ADR entries: none of the writers this
	// brackets edits them, and the digest needs them to apply the precedence
	// rule (a projected entry value-equal to an own one is not emitted).
	own map[string]ownCardValues
}

// SnapshotEntityProjections records what each of uids (vCard UIDs of userID's
// contacts) projects now.
func SnapshotEntityProjections(db *gorm.DB, userID uint, uids []string) (*EntityProjectionSnapshot, error) {
	s := &EntityProjectionSnapshot{userID: userID, own: map[string]ownCardValues{}}
	d, err := projectionDigests(db, userID, uids, s.own)
	if err != nil {
		return nil, err
	}
	s.before = d
	return s, nil
}

// BumpChanged re-reads the projection of every snapshotted contact and bumps
// (revision, ETag, updated_at) those whose projection differs. It returns the
// number bumped. Soft-deleted contacts are skipped (their tombstone stamp is
// the feed's record of them).
func (s *EntityProjectionSnapshot) BumpChanged(tx *gorm.DB) (int, error) {
	if len(s.before) == 0 {
		return 0, nil
	}
	uids := make([]string, 0, len(s.before))
	for uid := range s.before {
		uids = append(uids, uid)
	}
	after, err := projectionDigests(tx, s.userID, uids, s.own)
	if err != nil {
		return 0, err
	}
	var changed []string
	for _, uid := range uids {
		if s.before[uid] != after[uid] {
			changed = append(changed, uid)
		}
	}
	return len(changed), bumpContactRevisions(tx, s.userID, changed)
}

// ProjectingContactUIDs returns the vCard UIDs of every contact holding a link
// (live or tombstoned: a restore revives one) of kind organisation or place to
// any of entityIDs. It is a superset of the contacts whose projection a write to
// those entities can change; the snapshot digests narrow it.
func ProjectingContactUIDs(db *gorm.DB, userID uint, entityIDs ...string) ([]string, error) {
	var uids []string
	err := db.Unscoped().Model(&ContactEntityLink{}).
		Where("user_id = ? AND entity_id IN ? AND kind IN ?", userID, entityIDs, []string{EntityKindOrganisation, EntityKindPlace}).
		Distinct().Pluck("contact_vcard_uid", &uids).Error
	return uids, err
}

// ownCardValues is the ORG and ADR entries a contact carries itself.
type ownCardValues struct {
	orgs  []contactmodel.Organization
	addrs []contactmodel.Address
}

// projectionDigests maps each uid to a digest (the canonical JSON) of the ORG/ADR entries its card
// would carry on both views a surface can serve: the owner's full view and the
// normal-sensitivity view that sync, shares and exports use. A sensitivity
// change can alter only the second (an organisation's entry vanishes from sync
// but its full-view value is the same), and still changes the card a sync client
// holds. The entries are merged into the contact's own with the production
// precedence rule, so a link whose entity merely repeats a value the contact
// already stores (link-from-existing, adopt) changes no card and bumps nothing.
// Every uid is present; "" means the contact projects nothing on either view.
// own caches the contacts' own entries across calls.
func projectionDigests(db *gorm.DB, userID uint, uids []string, own map[string]ownCardValues) (map[string]string, error) {
	out := make(map[string]string, len(uids))
	for _, uid := range uids {
		out[uid] = ""
	}
	if len(uids) == 0 {
		return out, nil
	}
	full, err := loadEntityProjectionsFor(db, userID, uids, true)
	if err != nil {
		return nil, err
	}
	normal, err := loadEntityProjectionsFor(db, userID, uids, false)
	if err != nil {
		return nil, err
	}
	var need []string
	for uid := range full { // the normal view is a subset of the full one
		if _, ok := own[uid]; !ok {
			need = append(need, uid)
		}
	}
	if err := loadOwnCardValues(db, userID, need, own); err != nil {
		return nil, err
	}
	// merged is what the view adds to the contact's own entries; nil when the
	// precedence rule drops every projected entry (or there are none).
	merged := func(uid string, projs []projectedEntity) any {
		card := contactmodel.Card{Organizations: own[uid].orgs, Addresses: own[uid].addrs}
		appendProjections(&card, projs)
		if len(card.Organizations) == len(own[uid].orgs) && len(card.Addresses) == len(own[uid].addrs) {
			return nil
		}
		return [2]any{card.Organizations[len(own[uid].orgs):], card.Addresses[len(own[uid].addrs):]}
	}
	for uid, projs := range full {
		fullAdds, normalAdds := merged(uid, projs), merged(uid, normal[uid])
		if fullAdds == nil && normalAdds == nil {
			continue
		}
		raw, err := json.Marshal([2]any{fullAdds, normalAdds})
		if err != nil { // # pragma: no cover — the entries are plain structs of strings and slices
			return nil, err
		}
		out[uid] = string(raw)
	}
	return out, nil
}

// loadOwnCardValues loads the contacts' own ORG/ADR entries into dst, the way a
// read derives them (the stored Card, or the legacy flat columns for a contact
// that has none). A uid with no live contact is recorded as empty.
func loadOwnCardValues(db *gorm.DB, userID uint, uids []string, dst map[string]ownCardValues) error {
	for start := 0; start < len(uids); start += entityProjectionLookupChunk {
		chunk := uids[start:min(start+entityProjectionLookupChunk, len(uids))]
		var contacts []Contact
		if err := db.Where("user_id = ? AND vcard_uid IN ?", userID, chunk).Find(&contacts).Error; err != nil {
			return err
		}
		for i := range contacts {
			c := &contacts[i]
			v := ownCardValues{orgs: c.Card.Organizations, addrs: c.Card.Addresses}
			if reflect.DeepEqual(c.Card, contactmodel.Card{}) {
				v = ownCardValues{orgs: buildOrganizations(c), addrs: buildAddresses(c)}
			}
			dst[c.VCardUID] = v
		}
		for _, uid := range chunk {
			if _, ok := dst[uid]; !ok {
				dst[uid] = ownCardValues{}
			}
		}
	}
	return nil
}

// bumpContactRevisions advances revision, ETag and updated_at of userID's live
// contacts with the given vCard UIDs, in chunks. It is the set-based twin of
// the Contact AfterSave hook's bumpRevisionCAS: the same "e-<id>-<revision>"
// ETag (ADR 0006), the same deleted_at guard, but no per-row compare-and-swap
// and no audit event, because no stored contact field changed. A client
// holding the old ETag gets 412 on its next If-Match write (ADR 0008), which
// is correct: the card it edited is no longer the card the server would serve.
func bumpContactRevisions(tx *gorm.DB, userID uint, uids []string) error {
	now := tx.NowFunc()
	for start := 0; start < len(uids); start += entityProjectionLookupChunk {
		chunk := uids[start:min(start+entityProjectionLookupChunk, len(uids))]
		if err := tx.Exec(`UPDATE contacts
			SET revision = revision + 1, etag = 'e-' || id || '-' || (revision + 1), updated_at = ?
			WHERE user_id = ? AND vcard_uid IN ? AND deleted_at IS NULL`, now, userID, chunk).Error; err != nil {
			return err
		}
	}
	return nil
}
