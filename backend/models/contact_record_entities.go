package models

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/logger"

	"gorm.io/gorm"
)

// Entity -> neutral Card projection (issue #2298, ADR 0043 decisions 3 and 4).
//
// projectPrimaryEntities sits beside projectRelationshipEdges / projectTags /
// projectCustomFields in RecordForContactFiltered, so REST detail, CardDAV and
// WebDAV (RecordForContactForSync), VCF/JSContact export, contact shares and
// MCP get_contact all see the same projected ORG/ADR through one path. It is
// read-time synthesis: the result is never written back to Contact.Card. The
// inbound half (SuppressEntityProjectionEcho + the ent- ID drop inside
// ApplyRecordToContact) stops a client that PUTs a downloaded card back from
// writing the projection into the stored Card.

// EntityProjectionIDPrefix is the reserved element-ID prefix of a projected
// entry: `ent-<linkId>` (vCard 4.0 PROP-ID, JSContact map key). Inbound
// ApplyRecordToContact drops any ORG/ADR entry carrying it.
const EntityProjectionIDPrefix = contactmodel.EntityProjectionIDPrefix

// maxPref is the highest vCard PREF value (RFC 6350 §5.3).
const maxPref = 100

// IsEntityProjectionID reports whether id is a reserved projection element ID.
func IsEntityProjectionID(id string) bool {
	return strings.HasPrefix(id, EntityProjectionIDPrefix)
}

// entityProjectionToday is the owner's calendar day used to decide whether a
// link is current (ADR 0043 decision 3, ADR 0015). RecordForContactFiltered
// has no request context, so the day is a process-level seam: the default is
// the wall-clock day in UTC and the server installs the owner's reminder zone
// at start-up (SetEntityProjectionToday). Tests pin it.
var entityProjectionToday atomic.Value // func() *contactmodel.PartialDate

// SetEntityProjectionToday installs the owner-day provider and returns a
// function restoring the previous one.
func SetEntityProjectionToday(fn func() *contactmodel.PartialDate) (restore func()) {
	prev := entityProjectionToday.Load()
	entityProjectionToday.Store(fn)
	return func() {
		if prev == nil {
			entityProjectionToday.Store((func() *contactmodel.PartialDate)(nil))
			return
		}
		entityProjectionToday.Store(prev)
	}
}

// InstallEntityProjectionClock wires the owner-day provider from a clock and
// the owner's zone (the reminder timezone, ADR 0015) and returns the restore.
func InstallEntityProjectionClock(now func() time.Time, loc *time.Location) (restore func()) {
	return SetEntityProjectionToday(func() *contactmodel.PartialDate {
		return PartialDateOf(now().In(loc))
	})
}

// PartialDateOf returns t's calendar day as a full PartialDate.
func PartialDateOf(t time.Time) *contactmodel.PartialDate {
	y, m, d := t.Year(), int(t.Month()), t.Day()
	return &contactmodel.PartialDate{Year: &y, Month: &m, Day: &d}
}

func projectionToday() *contactmodel.PartialDate {
	if fn, ok := entityProjectionToday.Load().(func() *contactmodel.PartialDate); ok && fn != nil {
		return fn()
	}
	return PartialDateOf(time.Now().UTC()) // models layer has no request clock; see entityProjectionToday
}

// linkIsCurrent: start not in the future and end not in the past; endpoints
// not comparable at shared precision count as current (ADR 0015 rule 3).
func linkIsCurrent(p *contactmodel.TemporalRange, today *contactmodel.PartialDate) bool {
	if p == nil {
		return true
	}
	if cmp, ok := contactmodel.ComparePartialDates(p.Start, today); ok && cmp > 0 {
		return false
	}
	if cmp, ok := contactmodel.ComparePartialDates(today, p.End); ok && cmp > 0 {
		return false
	}
	return true
}

// projectedEntity is one synthesized ORG or ADR entry (exactly one of Org/Addr).
type projectedEntity struct {
	LinkID string
	Org    *contactmodel.Organization
	Addr   *contactmodel.Address
}

// currentEntityProjections loads the contact's current primary links and builds
// the entries they project. Filtering is in the query: with includeSensitive
// false only links AND entities at normal sensitivity are selected. Pets and
// non-primary links never project. A lookup failure degrades to no projection
// (best-effort, like the sibling projections).
func currentEntityProjections(db *gorm.DB, userID uint, vcardUID string, includeSensitive bool) []projectedEntity {
	out, err := loadEntityProjections(db, userID, vcardUID, includeSensitive)
	if err != nil {
		logger.Warn().Err(err).Str("vcard_uid", vcardUID).Msg("projectPrimaryEntities: failed to load primary entity links")
		return nil
	}
	return out
}

// loadEntityProjections is currentEntityProjections with the lookup error
// surfaced, for the write path (echo suppression), which must fail closed.
func loadEntityProjections(db *gorm.DB, userID uint, vcardUID string, includeSensitive bool) ([]projectedEntity, error) {
	if db == nil || vcardUID == "" {
		return nil, nil
	}
	m, err := loadEntityProjectionsFor(db, userID, []string{vcardUID}, includeSensitive)
	return m[vcardUID], err
}

// entityProjectionLookupChunk bounds the IN-list of one lookup statement, well
// under SQLite's bound-variable limit.
const entityProjectionLookupChunk = 400

// loadEntityProjectionsFor is the batch form of loadEntityProjections: one links
// query and one entities query per chunk of contacts, not per contact. The
// result maps vCard UID to that contact's entries in (kind, link id) order;
// contacts with no projection are absent. It is the single place that decides
// what a contact projects, shared by the read path and by the write-side
// change detection (EntityProjectionSnapshot).
func loadEntityProjectionsFor(db *gorm.DB, userID uint, uids []string, includeSensitive bool) (map[string][]projectedEntity, error) {
	out := map[string][]projectedEntity{}
	today := projectionToday()
	for start := 0; start < len(uids); start += entityProjectionLookupChunk {
		chunk := uids[start:min(start+entityProjectionLookupChunk, len(uids))]
		q := db.Where("user_id = ? AND contact_vcard_uid IN ? AND is_primary = ? AND kind IN ?",
			userID, chunk, true, []string{EntityKindOrganisation, EntityKindPlace})
		if !includeSensitive {
			q = q.Where("sensitivity = ?", RelationshipSensitivityNormal)
		}
		var links []ContactEntityLink
		if err := q.Order("contact_vcard_uid, kind, id").Find(&links).Error; err != nil {
			return nil, err
		}
		if len(links) == 0 {
			continue
		}
		byID, err := loadProjectedEntities(db, userID, links, includeSensitive)
		if err != nil {
			return nil, err
		}
		for _, l := range links {
			e := byID[l.EntityID]
			if e == nil || e.Kind != l.Kind || !linkIsCurrent(l.Period, today) {
				continue
			}
			if p, ok := projectEntityLink(e, &l); ok {
				out[l.ContactVCardUID] = append(out[l.ContactVCardUID], p)
			}
		}
	}
	return out, nil
}

// loadProjectedEntities loads the live entities the links point at (at normal
// sensitivity only unless includeSensitive), keyed by id.
func loadProjectedEntities(db *gorm.DB, userID uint, links []ContactEntityLink, includeSensitive bool) (map[string]*Entity, error) {
	seen := make(map[string]bool, len(links))
	ids := make([]string, 0, len(links))
	for _, l := range links {
		if !seen[l.EntityID] {
			seen[l.EntityID] = true
			ids = append(ids, l.EntityID)
		}
	}
	byID := make(map[string]*Entity, len(ids))
	for start := 0; start < len(ids); start += entityProjectionLookupChunk {
		eq := db.Where("user_id = ? AND id IN ?", userID, ids[start:min(start+entityProjectionLookupChunk, len(ids))])
		if !includeSensitive {
			eq = eq.Where("sensitivity = ?", RelationshipSensitivityNormal)
		}
		var entities []Entity
		if err := eq.Find(&entities).Error; err != nil {
			return nil, err
		}
		for i := range entities {
			byID[entities[i].ID] = &entities[i]
		}
	}
	return byID, nil
}

// projectEntityLink builds the ORG/ADR entry link l projects from its entity e, or
// false when it projects nothing (an empty address, an undecodable core).
func projectEntityLink(e *Entity, l *ContactEntityLink) (projectedEntity, bool) {
	core, err := e.DecodeCore()
	if err != nil { // # pragma: no cover — core is validated JSON on every write
		return projectedEntity{}, false
	}
	effective := maxSensitivity(e.Sensitivity, l.Sensitivity)
	switch v := core.(type) {
	case *OrganisationCore:
		org := v.Organization
		if strings.TrimSpace(org.Name) == "" {
			org.Name = e.Name
		}
		org.ID = EntityProjectionIDPrefix + l.ID
		return projectedEntity{LinkID: l.ID, Org: &org}, true
	case *PlaceCore:
		addr := v.Address
		if len(addr.Components) == 0 && strings.TrimSpace(addr.Full) == "" {
			return projectedEntity{}, false
		}
		addr.ID = EntityProjectionIDPrefix + l.ID
		if effective != RelationshipSensitivityNormal {
			addr.Sensitivity = effective
		}
		return projectedEntity{LinkID: l.ID, Addr: &addr}, true
	}
	return projectedEntity{}, false
}

var sensitivityRank = map[string]int{"": 0, "normal": 0, "private": 1, "secret": 2}

func maxSensitivity(a, b string) string {
	if sensitivityRank[b] > sensitivityRank[a] {
		a = b
	}
	if a == "" {
		return RelationshipSensitivityNormal
	}
	return a
}

// projectPrimaryEntities appends the current primary entity links' ORG/ADR to
// COPIES of card.Organizations / card.Addresses. Own entries lead and are never
// altered; a projected entry value-equal to an own entry (after ADR 0016 NFC
// normalization) is dropped; a projected address gets a pref greater than any
// own address's. It never mutates the backing arrays and never writes back to
// the stored Card.
func projectPrimaryEntities(db *gorm.DB, userID uint, vcardUID string, card *contactmodel.Card, includeSensitive bool) {
	appendProjections(card, currentEntityProjections(db, userID, vcardUID, includeSensitive))
}

// appendProjections is projectPrimaryEntities' merge step: projs appended to
// COPIES of card's Organizations/Addresses with the precedence rules above.
func appendProjections(card *contactmodel.Card, projs []projectedEntity) {
	if len(projs) == 0 {
		return
	}
	orgs := append([]contactmodel.Organization(nil), card.Organizations...)
	addrs := append([]contactmodel.Address(nil), card.Addresses...)
	for _, p := range projs {
		switch {
		case p.Org != nil:
			if containsOrg(orgs, *p.Org) {
				continue
			}
			orgs = append(orgs, *p.Org)
		case p.Addr != nil:
			if containsAddr(addrs, *p.Addr) {
				continue
			}
			a := *p.Addr
			pref := 1
			for _, own := range addrs {
				if own.Pref != nil && *own.Pref >= pref {
					pref = *own.Pref + 1
				}
			}
			if pref > maxPref {
				pref = maxPref
			}
			a.Pref = &pref
			addrs = append(addrs, a)
		}
	}
	card.Organizations, card.Addresses = orgs, addrs
}

// normalizedOrg / normalizedAddr render an entry in its ADR 0016 canonical form
// (NFC via contactmodel.NormalizeRecord, the form storage uses) with the
// synthetic fields (ID, projected pref, sensitivity stamp) removed, for
// value-equality.
func normalizedOrg(o contactmodel.Organization) string {
	o.ID = ""
	r := contactmodel.NormalizeRecord(&contactmodel.Record{Card: contactmodel.Card{Organizations: []contactmodel.Organization{o}}})
	b, _ := json.Marshal(r.Card.Organizations)
	return string(b)
}

func normalizedAddr(a contactmodel.Address) string {
	a.ID, a.Pref, a.Sensitivity = "", nil, ""
	r := contactmodel.NormalizeRecord(&contactmodel.Record{Card: contactmodel.Card{Addresses: []contactmodel.Address{a}}})
	b, _ := json.Marshal(r.Card.Addresses)
	return string(b)
}

func containsOrg(list []contactmodel.Organization, o contactmodel.Organization) bool {
	want := normalizedOrg(o)
	for _, own := range list {
		if normalizedOrg(own) == want {
			return true
		}
	}
	return false
}

func containsAddr(list []contactmodel.Address, a contactmodel.Address) bool {
	want := normalizedAddr(a)
	for _, own := range list {
		if normalizedAddr(own) == want {
			return true
		}
	}
	return false
}

// StripProjectedEntityIDs removes every ORG/ADR entry whose element ID carries
// the reserved ent- prefix (a projection echo from a well-behaved client), and
// the title association / period that referenced one. It returns r unchanged
// when there is nothing to drop; otherwise a copy of r with new slices.
func StripProjectedEntityIDs(r *contactmodel.Record) *contactmodel.Record {
	if r == nil {
		return r
	}
	has := false
	for _, o := range r.Card.Organizations {
		has = has || IsEntityProjectionID(o.ID)
	}
	for _, a := range r.Card.Addresses {
		has = has || IsEntityProjectionID(a.ID)
	}
	if !has {
		return r
	}
	out := *r
	dropped := map[string]bool{}
	out.Card.Organizations = nil
	for _, o := range r.Card.Organizations {
		if IsEntityProjectionID(o.ID) {
			dropped[o.ID] = true
			continue
		}
		out.Card.Organizations = append(out.Card.Organizations, o)
	}
	out.Card.Addresses = nil
	for _, a := range r.Card.Addresses {
		if IsEntityProjectionID(a.ID) {
			dropped[a.ID] = true
			continue
		}
		out.Card.Addresses = append(out.Card.Addresses, a)
	}
	if len(r.Card.Titles) > 0 {
		out.Card.Titles = append([]contactmodel.Title(nil), r.Card.Titles...)
		for i := range out.Card.Titles {
			if dropped[out.Card.Titles[i].OrganizationID] {
				out.Card.Titles[i].OrganizationID = ""
			}
		}
	}
	if len(r.Envelope.Periods) > 0 {
		out.Envelope.Periods = nil
		for _, p := range r.Envelope.Periods {
			if IsEntityProjectionID(p.EntryID) {
				continue
			}
			out.Envelope.Periods = append(out.Envelope.Periods, p)
		}
	}
	return &out
}

// SuppressEntityProjectionEcho is the value-based half of echo suppression, for
// clients that strip element IDs (vCard 3.0, many native address books): an
// incoming ORG/ADR entry that is value-equal to the contact's CURRENT projection
// and not value-equal to any value the contact already stores is the echo of
// the projection and is dropped. A user who edits a projected value in such a
// client therefore creates an ordinary contact value (no loss). existing is the
// persisted row the write applies to (nil for a new contact: nothing to echo).
// Call it before ApplyRecordToContact on every inbound path that has a db and an
// existing contact (CardDAV PUT, REST update). It mutates only rec.Card's
// ORG/ADR slice headers (copies) and is a no-op when nothing is projected.
//
// It returns the lookup error and the caller MUST abort the write on one: unlike
// the read projection (best-effort), failing open here would let an id-stripped
// echo of the projection be stored as a contact value.
func SuppressEntityProjectionEcho(db *gorm.DB, existing *Contact, rec *contactmodel.Record) error {
	if db == nil || existing == nil || rec == nil || existing.VCardUID == "" {
		return nil
	}
	projs, err := loadEntityProjections(db, existing.UserID, existing.VCardUID, false)
	if err != nil {
		return err
	}
	if len(projs) == 0 {
		return nil
	}
	for _, p := range projs {
		switch {
		case p.Org != nil:
			want := normalizedOrg(*p.Org)
			if containsOrg(existing.Card.Organizations, *p.Org) {
				continue // the contact owns this value too: keep it
			}
			kept := make([]contactmodel.Organization, 0, len(rec.Card.Organizations))
			for _, o := range rec.Card.Organizations {
				if normalizedOrg(o) == want {
					continue
				}
				kept = append(kept, o)
			}
			rec.Card.Organizations = kept
		case p.Addr != nil:
			want := normalizedAddr(*p.Addr)
			if containsAddr(existing.Card.Addresses, *p.Addr) {
				continue
			}
			kept := make([]contactmodel.Address, 0, len(rec.Card.Addresses))
			for _, a := range rec.Card.Addresses {
				if normalizedAddr(a) == want {
					continue
				}
				kept = append(kept, a)
			}
			rec.Card.Addresses = kept
		}
	}
	return nil
}

// DetachProjectedEntityIDs returns a copy of r in which every reserved ent-
// element ID is blanked (the adapters then mint ordinary synthetic IDs). It is
// for the cross-user share surfaces: the recipient imports the projected ORG/ADR
// as an ordinary value (an ent- ID would be dropped on import as an echo), and
// the sender's link ID has no business reaching another account.
func DetachProjectedEntityIDs(r *contactmodel.Record) *contactmodel.Record {
	if r == nil {
		return r
	}
	has := false
	for _, o := range r.Card.Organizations {
		has = has || IsEntityProjectionID(o.ID)
	}
	for _, a := range r.Card.Addresses {
		has = has || IsEntityProjectionID(a.ID)
	}
	if !has {
		return r
	}
	out := *r
	out.Card.Organizations = append([]contactmodel.Organization(nil), r.Card.Organizations...)
	for i := range out.Card.Organizations {
		if IsEntityProjectionID(out.Card.Organizations[i].ID) {
			out.Card.Organizations[i].ID = ""
		}
	}
	out.Card.Addresses = append([]contactmodel.Address(nil), r.Card.Addresses...)
	for i := range out.Card.Addresses {
		if IsEntityProjectionID(out.Card.Addresses[i].ID) {
			out.Card.Addresses[i].ID = ""
		}
	}
	return &out
}
