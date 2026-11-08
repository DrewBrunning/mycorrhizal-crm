package services

import (
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"
)

// Entity dedupe suggestions and the "create entities from my contacts" helper
// (issue #2299, ADR 0043 decisions 8 and 9). Everything that reads is read-only
// and idempotent; nothing here runs in the background or from an import.
//
// Normalization is exactly ADR 0043 decision 8: name_key (NFC, Unicode
// lower-casing, whitespace collapsed, edge punctuation trimmed) for the exact
// tier; a closed list of legal suffixes dropped for comparison only (never
// written back) for the normalized tier; no diacritic folding (ADR 0016).

// entityLegalSuffixes is the closed legal-suffix list of the suggestion tier.
var entityLegalSuffixes = map[string]bool{
	"inc": true, "llc": true, "ltd": true, "gmbh": true, "ag": true,
	"corp": true, "co": true, "sa": true, "bv": true,
}

// comparisonTokens lower-cases (NFC), turns every non-letter/non-digit into a
// separator and returns the remaining tokens.
func comparisonTokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(norm.NFC.String(s)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsMark(r)
	})
}

// OrganisationComparisonKey is the normalized-tier key of an organisation
// name: tokens joined by single spaces with trailing legal suffixes dropped
// ("Acme, Inc." and "ACME Corp" both give "acme"). At least one token is kept.
func OrganisationComparisonKey(name string) string {
	toks := comparisonTokens(name)
	for len(toks) > 1 && entityLegalSuffixes[toks[len(toks)-1]] {
		toks = toks[:len(toks)-1]
	}
	return strings.Join(toks, " ")
}

// addressKeyOrder is the component order of the place key (ADR 0043 decision 8).
var addressKeyOrder = []string{"number", "name", "locality", "postcode", "country"}

// AddressComponentsKey is the normalized joined components of an address in
// Address order, punctuation and whitespace collapsed ("12 High St." equals
// "12 High St"). With none of the keyed components it falls back to the
// address's free-form label; an empty key means "cannot be matched".
func AddressComponentsKey(a contactmodel.Address) string {
	var toks []string
	for _, kind := range addressKeyOrder {
		for _, comp := range a.Components {
			if comp.Kind == kind {
				toks = append(toks, comparisonTokens(comp.Value)...)
			}
		}
	}
	if len(toks) == 0 {
		toks = comparisonTokens(a.Full)
	}
	return strings.Join(toks, " ")
}

// EntityAddressLabel derives a display name for a place created from an
// address: the full label, else the components joined, at most 200 characters.
func EntityAddressLabel(a contactmodel.Address) string {
	label := strings.TrimSpace(a.Full)
	if label == "" {
		parts := make([]string, 0, len(a.Components))
		for _, comp := range a.Components {
			if v := strings.TrimSpace(comp.Value); v != "" {
				parts = append(parts, v)
			}
		}
		label = strings.Join(parts, ", ")
	}
	if label == "" {
		label = "Address"
	}
	for utf8.RuneCountInString(label) > models.EntityNameMaxLength {
		_, size := utf8.DecodeLastRuneInString(label)
		label = label[:len(label)-size]
	}
	return label
}

// ---------------------------------------------------------------------------
// Cursor: suggestions are sorted by an opaque unique key; the cursor is the
// last key returned, so a page never repeats or skips an item while the data
// is unchanged (keyset paging).

func encodeSuggestionCursor(key string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(key))
}

// ErrBadSuggestionCursor is returned for an undecodable cursor.
var ErrBadSuggestionCursor = errors.New("invalid suggestions cursor")

func decodeSuggestionCursor(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", ErrBadSuggestionCursor
	}
	return string(b), nil
}

// ---------------------------------------------------------------------------
// Pair suggestions

func reasonRank(r string) int {
	switch r {
	case models.EntitySuggestionReasonExactName:
		return 0
	case models.EntitySuggestionReasonNormalName:
		return 1
	}
	return 2
}

// SuggestEntityPairs returns candidate duplicate pairs of live entities of
// userID (of kind when non-empty), minus dismissed pairs, sorted by a stable
// key and paged after cursor. It writes nothing.
func SuggestEntityPairs(db *gorm.DB, userID uint, kind, cursor string, limit int) ([]models.EntitySuggestionPair, string, error) {
	after, err := decodeSuggestionCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	q := db.Where("user_id = ?", userID)
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	var ents []models.Entity
	if err := q.Order("id").Find(&ents).Error; err != nil {
		return nil, "", err
	}
	var dismissed []models.DismissedEntityPair
	if err := db.Where("user_id = ?", userID).Find(&dismissed).Error; err != nil {
		return nil, "", err
	}
	hidden := make(map[[2]string]bool, len(dismissed))
	for _, d := range dismissed {
		hidden[[2]string{d.IDLow, d.IDHigh}] = true
	}

	byID := make(map[string]*models.Entity, len(ents))
	buckets := map[string][]string{} // tier-prefixed key -> entity ids
	tierOf := map[string]string{}    // bucket key -> reason
	add := func(bucket, reason, id string) {
		buckets[bucket] = append(buckets[bucket], id)
		tierOf[bucket] = reason
	}
	for i := range ents {
		e := &ents[i]
		byID[e.ID] = e
		if e.NameKey != "" {
			k := "n|" + e.Kind + "|" + e.NameKey
			if e.Kind == models.EntityKindPet {
				if core, derr := e.DecodeCore(); derr == nil {
					if pc, ok := core.(*models.PetCore); ok {
						k += "|" + strings.Join(comparisonTokens(pc.Species), " ")
					}
				}
			}
			add(k, models.EntitySuggestionReasonExactName, e.ID)
		}
		switch e.Kind {
		case models.EntityKindOrganisation:
			if k := OrganisationComparisonKey(e.Name); k != "" {
				add("o|"+k, models.EntitySuggestionReasonNormalName, e.ID)
			}
		case models.EntityKindPlace:
			if core, derr := e.DecodeCore(); derr == nil {
				if pc, ok := core.(*models.PlaceCore); ok {
					if k := AddressComponentsKey(pc.Address); k != "" {
						add("a|"+k, models.EntitySuggestionReasonAddressParts, e.ID)
					}
				}
			}
		}
	}

	reasons := map[[2]string]string{}
	for bucket, ids := range buckets {
		if len(ids) < 2 {
			continue
		}
		sort.Strings(ids)
		reason := tierOf[bucket]
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				pair := [2]string{ids[i], ids[j]}
				if hidden[pair] || ids[i] == ids[j] {
					continue
				}
				if cur, ok := reasons[pair]; !ok || reasonRank(reason) < reasonRank(cur) {
					reasons[pair] = reason
				}
			}
		}
	}

	type keyed struct {
		key  string
		pair models.EntitySuggestionPair
	}
	all := make([]keyed, 0, len(reasons))
	for pair, reason := range reasons {
		a, b := byID[pair[0]], byID[pair[1]]
		all = append(all, keyed{
			key: fmt.Sprintf("%s\x00%s\x00%s", a.NameKey, pair[0], pair[1]),
			pair: models.EntitySuggestionPair{
				Reason: reason,
				A:      models.EntitySuggestionEntity{ID: a.ID, Kind: a.Kind, Name: a.Name},
				B:      models.EntitySuggestionEntity{ID: b.ID, Kind: b.Kind, Name: b.Name},
			},
		})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].key < all[j].key })

	out := make([]models.EntitySuggestionPair, 0, limit)
	next, lastKey := "", ""
	for _, k := range all {
		if after != "" && k.key <= after {
			continue
		}
		if len(out) == limit {
			next = encodeSuggestionCursor(lastKey)
			break
		}
		out = append(out, k.pair)
		lastKey = k.key
	}
	return out, next, nil
}

// ---------------------------------------------------------------------------
// Contact-value groups (the "create entities from my contacts" helper)

// valueRef is one contact's own ORG or ADR value.
type valueRef struct {
	contactID uint
	vcardUID  string
	firstname string
	lastname  string
	name      string // display spelling
	key       string
	sens      string
	org       *contactmodel.Organization
	addr      *contactmodel.Address
}

type valueGroup struct {
	key    string
	values []valueRef
}

func sensRank(s string) int {
	switch s {
	case models.RelationshipSensitivitySecret:
		return 2
	case models.RelationshipSensitivityPrivate:
		return 1
	}
	return 0
}

// loadValueGroups reads the user's live contacts' persisted ORG (or ADR)
// values and groups the ones that compare equal, minus values already matched
// by a live link of the same contact to a live entity with the same key (so a
// re-run proposes only what is still unmatched). Read-only.
func loadValueGroups(db *gorm.DB, userID uint, kind string) ([]valueGroup, map[string]bool, error) {
	// Keys already covered, per contact: "vcardUID|key".
	var links []models.ContactEntityLink
	if err := db.Where("user_id = ? AND kind = ?", userID, kind).Find(&links).Error; err != nil {
		return nil, nil, err
	}
	hasPrimary := map[string]bool{}
	matched := map[string]bool{}
	if len(links) > 0 {
		ids := make([]string, 0, len(links))
		for _, l := range links {
			ids = append(ids, l.EntityID)
			if l.IsPrimary {
				hasPrimary[l.ContactVCardUID] = true
			}
		}
		var ents []models.Entity
		if err := db.Where("user_id = ? AND id IN ?", userID, ids).Find(&ents).Error; err != nil {
			return nil, nil, err
		}
		keyOf := map[string]string{}
		for i := range ents {
			keyOf[ents[i].ID] = entityValueKey(&ents[i])
		}
		for _, l := range links {
			if k := keyOf[l.EntityID]; k != "" {
				matched[l.ContactVCardUID+"|"+k] = true
			}
		}
	}

	var contacts []models.Contact
	if err := db.Select("id", "user_id", "vcard_uid", "firstname", "lastname", "card").
		Where("user_id = ?", userID).Order("id").Find(&contacts).Error; err != nil {
		return nil, nil, err
	}
	groups := map[string]*valueGroup{}
	var order []string
	push := func(v valueRef) {
		if v.key == "" || matched[v.vcardUID+"|"+v.key] {
			return
		}
		g := groups[v.key]
		if g == nil {
			g = &valueGroup{key: v.key}
			groups[v.key] = g
			order = append(order, v.key)
		}
		for _, have := range g.values {
			if have.contactID == v.contactID {
				return // one value per contact per group
			}
		}
		g.values = append(g.values, v)
	}
	for i := range contacts {
		c := &contacts[i]
		base := valueRef{contactID: c.ID, vcardUID: c.VCardUID, firstname: c.Firstname, lastname: c.Lastname}
		switch kind {
		case models.EntityKindOrganisation:
			for j := range c.Card.Organizations {
				o := c.Card.Organizations[j]
				if strings.TrimSpace(o.Name) == "" {
					continue
				}
				v := base
				v.name, v.key, v.org = strings.TrimSpace(o.Name), "org:"+OrganisationComparisonKey(o.Name), &o
				v.sens = models.RelationshipSensitivityNormal
				push(v)
			}
		case models.EntityKindPlace:
			for j := range c.Card.Addresses {
				a := c.Card.Addresses[j]
				k := AddressComponentsKey(a)
				if k == "" {
					continue
				}
				v := base
				v.name, v.key, v.addr = EntityAddressLabel(a), "adr:"+k, &a
				v.sens = orNormal(a.Sensitivity)
				push(v)
			}
		}
	}
	sort.Strings(order)
	out := make([]valueGroup, 0, len(order))
	for _, k := range order {
		out = append(out, *groups[k])
	}
	return out, hasPrimary, nil
}

func orNormal(s string) string {
	if s == "" {
		return models.RelationshipSensitivityNormal
	}
	return s
}

// entityValueKey is the key an existing entity answers to when matching a
// contact's own value (the same function the groups use).
func entityValueKey(e *models.Entity) string {
	switch e.Kind {
	case models.EntityKindOrganisation:
		return "org:" + OrganisationComparisonKey(e.Name)
	case models.EntityKindPlace:
		core, err := e.DecodeCore()
		if err != nil { // # pragma: no cover — a stored core is validated JSON on every write
			return ""
		}
		if pc, ok := core.(*models.PlaceCore); ok {
			if k := AddressComponentsKey(pc.Address); k != "" {
				return "adr:" + k
			}
		}
	}
	return ""
}

func (g valueGroup) variants() []string {
	count := map[string]int{}
	for _, v := range g.values {
		count[v.name]++
	}
	names := make([]string, 0, len(count))
	for n := range count {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if count[names[i]] != count[names[j]] {
			return count[names[i]] > count[names[j]]
		}
		return names[i] < names[j]
	})
	return names
}

func (g valueGroup) reason(kind string) string {
	if kind == models.EntityKindPlace {
		return models.EntitySuggestionReasonAddressParts
	}
	first := models.EntityNameKey(g.values[0].name)
	for _, v := range g.values[1:] {
		if models.EntityNameKey(v.name) != first {
			return models.EntitySuggestionReasonNormalName
		}
	}
	return models.EntitySuggestionReasonExactName
}

func (g valueGroup) response(kind string, hasPrimary map[string]bool) models.EntityValueGroup {
	variants := g.variants()
	contacts := make([]models.EntityValueGroupContact, 0, len(g.values))
	for _, v := range g.values {
		contacts = append(contacts, models.EntityValueGroupContact{
			ContactID: v.contactID, Firstname: v.firstname, Lastname: v.lastname,
			ValueName: v.name, HasPrimary: hasPrimary[v.vcardUID],
		})
	}
	return models.EntityValueGroup{Key: g.key, Kind: kind, Name: variants[0], Reason: g.reason(kind), Variants: variants, Contacts: contacts}
}

// SuggestEntityValueGroups proposes groups of the user's own ORG (kind
// organisation) or ADR (kind place) values for the create-from-contacts
// helper. Read-only and idempotent; groups are sorted by key and paged.
func SuggestEntityValueGroups(db *gorm.DB, userID uint, kind, cursor string, limit int) ([]models.EntityValueGroup, string, error) {
	after, err := decodeSuggestionCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	groups, hasPrimary, err := loadValueGroups(db, userID, kind)
	if err != nil {
		return nil, "", err
	}
	out := make([]models.EntityValueGroup, 0, limit)
	next := ""
	for _, g := range groups {
		if after != "" && g.key <= after {
			continue
		}
		if len(out) == limit {
			next = encodeSuggestionCursor(out[len(out)-1].Key)
			break
		}
		out = append(out, g.response(kind, hasPrimary))
	}
	return out, next, nil
}

// ---------------------------------------------------------------------------
// Dismissal

// ErrDismissSameEntity is returned when both sides of a dismissal are one id.
var ErrDismissSameEntity = errors.New("a pair needs two different entities")

// DismissEntityPair hides the pair (a, b) from the suggestions for good. Both
// must be live entities of userID (gorm.ErrRecordNotFound otherwise) of the
// same kind. Re-dismissing is a no-op.
func DismissEntityPair(db *gorm.DB, userID uint, a, b string) (low, high string, err error) {
	if a == b {
		return "", "", ErrDismissSameEntity
	}
	low, high = a, b
	if high < low {
		low, high = high, low
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var ents []models.Entity
		if err := tx.Select("id", "kind").Where("user_id = ? AND id IN ?", userID, []string{a, b}).Find(&ents).Error; err != nil {
			return err
		}
		if len(ents) != 2 {
			return gorm.ErrRecordNotFound
		}
		if ents[0].Kind != ents[1].Kind {
			return ErrEntityMergeKindMismatch
		}
		var have int64
		if err := tx.Model(&models.DismissedEntityPair{}).Where("user_id = ? AND id_low = ? AND id_high = ?", userID, low, high).Count(&have).Error; err != nil {
			return err
		}
		if have > 0 {
			return nil
		}
		return tx.Create(&models.DismissedEntityPair{UserID: userID, IDLow: low, IDHigh: high}).Error
	})
	return low, high, err
}

// ---------------------------------------------------------------------------
// Create entities from contacts

// EntityFromContactsCreated is one entity the helper created plus its links.
type EntityFromContactsCreated struct {
	Key    string
	Entity models.Entity
	Links  []models.ContactEntityLink
}

// EntityFromContactsOutcome is the helper's result.
type EntityFromContactsOutcome struct {
	Created []EntityFromContactsCreated
	Skipped []models.EntityFromContactsSkip
}

// CreateEntitiesFromContacts creates, for each confirmed group, one entity and
// one link per confirmed contact, in one transaction. The groups are
// recomputed server-side from the contacts' persisted values: a key that is no
// longer proposed (already matched, or never existed) and a contact that is
// not in the group (including another user's id) are reported as skipped,
// never trusted. A link is primary only when requested and the contact has no
// other live primary of the kind. Contact rows are only read: no value is
// edited or removed.
func CreateEntitiesFromContacts(db *gorm.DB, userID uint, kind string, reqs []models.EntityFromContactsGroup) (*EntityFromContactsOutcome, error) {
	out := &EntityFromContactsOutcome{Created: []EntityFromContactsCreated{}, Skipped: []models.EntityFromContactsSkip{}}
	err := db.Transaction(func(tx *gorm.DB) error {
		groups, _, err := loadValueGroups(tx, userID, kind)
		if err != nil {
			return err
		}
		byKey := make(map[string]valueGroup, len(groups))
		for _, g := range groups {
			byKey[g.key] = g
		}
		done := map[string]bool{}
		for _, req := range reqs {
			g, ok := byKey[req.Key]
			if !ok || done[req.Key] {
				out.Skipped = append(out.Skipped, models.EntityFromContactsSkip{Key: req.Key, Reason: "group_not_found"})
				continue
			}
			done[req.Key] = true
			var chosen []valueRef
			seen := map[uint]bool{}
			for _, id := range req.ContactIDs {
				if seen[id] {
					continue
				}
				seen[id] = true
				var found *valueRef
				for i := range g.values {
					if g.values[i].contactID == id {
						found = &g.values[i]
						break
					}
				}
				if found == nil {
					out.Skipped = append(out.Skipped, models.EntityFromContactsSkip{Key: req.Key, ContactID: id, Reason: "contact_not_in_group"})
					continue
				}
				chosen = append(chosen, *found)
			}
			if len(chosen) == 0 {
				continue
			}
			name := strings.TrimSpace(req.Name)
			if name == "" {
				name = g.variants()[0]
			}
			if utf8.RuneCountInString(name) > models.EntityNameMaxLength {
				out.Skipped = append(out.Skipped, models.EntityFromContactsSkip{Key: req.Key, Reason: "name_invalid"})
				continue
			}
			rep := chosen[0]
			for _, v := range chosen {
				if v.name == name {
					rep = v
					break
				}
			}
			ent := models.Entity{UserID: userID, Kind: kind, Name: name, Sensitivity: models.RelationshipSensitivityNormal}
			switch kind {
			case models.EntityKindOrganisation:
				org := *rep.org
				org.ID = ""
				if err := ent.SetCore(&models.OrganisationCore{Organization: org}); err != nil {
					out.Skipped = append(out.Skipped, models.EntityFromContactsSkip{Key: req.Key, Reason: "core_invalid"})
					continue
				}
			case models.EntityKindPlace:
				addr := *rep.addr
				addr.ID, addr.Sensitivity = "", "" // the entity column is the single source of sensitivity
				for _, v := range chosen {
					if sensRank(v.sens) > sensRank(ent.Sensitivity) {
						ent.Sensitivity = v.sens
					}
				}
				if err := ent.SetCore(&models.PlaceCore{Address: addr}); err != nil {
					out.Skipped = append(out.Skipped, models.EntityFromContactsSkip{Key: req.Key, Reason: "core_invalid"})
					continue
				}
			}
			if err := tx.Create(&ent).Error; err != nil {
				return err
			}
			created := EntityFromContactsCreated{Key: req.Key, Entity: ent, Links: []models.ContactEntityLink{}}
			for _, v := range chosen {
				primary := false
				if req.SetPrimary {
					var n int64
					if err := tx.Model(&models.ContactEntityLink{}).
						Where("user_id = ? AND contact_vcard_uid = ? AND kind = ? AND is_primary = ?", userID, v.vcardUID, kind, true).Count(&n).Error; err != nil {
						return err
					}
					primary = n == 0
				}
				link := models.ContactEntityLink{
					UserID: userID, ContactVCardUID: v.vcardUID, EntityID: ent.ID, Kind: kind, Role: req.Role,
					IsPrimary: primary, Sensitivity: models.RelationshipSensitivityNormal,
				}
				if err := CreateContactEntityLink(tx, &link); err != nil {
					return err
				}
				created.Links = append(created.Links, link)
			}
			out.Created = append(out.Created, created)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
