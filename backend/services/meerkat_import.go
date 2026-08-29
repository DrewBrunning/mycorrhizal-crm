package services

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"mycorrhizal/contactmodel"
	"mycorrhizal/meerkat"
	"mycorrhizal/models"
)

// This file maps a Meerkat CRM database (meerkat.Snapshot, read from the
// source SQLite file) onto the shared ImportSourcePlan (issues #351/#353).
//
// The mapping is direct-DB, per ADR-0007: Meerkat is the hard fork's own
// upstream, and a Meerkat deployment's SQLite file is the only carrier of its
// relationship, circle, and preference structure (an export drops it by
// construction). Contacts land as neutral contactmodel.Record values so the
// engine persists them through ApplyRecordToContact (CLAUDE.md backend trap
// #2); graph entities reference plan contacts by SourceRef and the engine
// links them by local VCardUID.
//
// Loss discipline: anything this mapping cannot carry is reported on the
// plan's report with its record, field, and category (the #442 shape over
// ADR-0002's tiers) — never dropped silently. Known, deliberate losses are
// photos (the image bytes live in the source server's filesystem, not the
// database) and relationships whose target is not a contact row.

// MeerkatSourceUser returns the source user_id whose data an import of snap
// will include, or nil when the database has no user scoping at all (every
// row is imported). requested, when non-nil, overrides the default: the
// snapshot's first user. A multi-user Meerkat deployment's other users' data
// is never silently mixed into one local user — the default is the first
// source user, and a caller importing a different one passes it explicitly.
func MeerkatSourceUser(snap *meerkat.Snapshot, requested *int64) *int64 {
	if requested != nil {
		return requested
	}
	if snap == nil {
		return nil
	}
	return snap.SourceUserID
}

// MapMeerkatSnapshot turns a read Meerkat database into an import plan. The
// returned plan's Report is populated with every mapping loss/decision.
func MapMeerkatSnapshot(snap *meerkat.Snapshot, requestedSourceUser *int64) *ImportSourcePlan {
	plan := &ImportSourcePlan{System: "meerkat"}
	if snap == nil {
		return plan
	}
	filter := MeerkatSourceUser(snap, requestedSourceUser)

	belongsToUser := func(userID *int64) bool {
		if filter == nil {
			return true // pre-scoping database: every row belongs to the importing user
		}
		return userID != nil && *userID == *filter
	}

	// Contact index for graph linking, restricted to the selected user.
	contactByID := map[int64]bool{}
	for _, c := range snap.Contacts {
		if belongsToUser(c.UserID) {
			contactByID[c.ID] = true
		}
	}
	refOf := func(id int64) SourceRef {
		return SourceRef{System: "meerkat", ExternalID: contactRef(id)}
	}

	for _, c := range snap.Contacts {
		if !belongsToUser(c.UserID) {
			continue
		}
		if c.DeletedAt != nil {
			plan.Report.appendIssue(ImportIssue{
				Record:   refOf(c.ID).String(),
				Field:    "contact",
				Category: ImportIssueCategorySkipped,
				Message:  "soft-deleted in the source database",
			})
			continue
		}
		plan.Contacts = append(plan.Contacts, mapMeerkatContact(c, refOf(c.ID), plan))
		mapMeerkatCustomFields(plan, c, refOf(c.ID))
	}

	// Relationships: the legacy flat table. A row {contact_id: X, name:
	// "John", type: "Father"} is stored on X's page and renders as "Father:
	// John" — the TYPE describes the *named person's* role relative to the
	// owner. Our edge stores the source's role relative to the target, so the
	// edge is named-person → owner with the matched type verbatim ("Father" →
	// parent_of: John is X's parent). Meerkat stores one direction only, but
	// real data may contain both halves of a reciprocal pair (added from each
	// contact's page); the reciprocal halves are collapsed the same way as
	// Monica's, since the local graph derives the inverse itself.
	var candidates []meerkatEdgeCandidate
	for _, r := range snap.Relationships {
		if !belongsToUser(r.UserID) {
			continue
		}
		if r.DeletedAt != nil {
			plan.Report.appendIssue(ImportIssue{
				Record:   SourceRef{System: "meerkat", ExternalID: relationshipRef(r.ID)}.String(),
				Field:    "relationship",
				Category: ImportIssueCategorySkipped,
				Message:  "soft-deleted in the source database",
			})
			continue
		}
		if r.ContactID == nil || !contactByID[*r.ContactID] {
			continue
		}
		if r.RelatedContact == nil || !contactByID[*r.RelatedContact] {
			label := ""
			if r.Name != nil {
				label = *r.Name
			}
			plan.Report.appendIssue(ImportIssue{
				Record:   SourceRef{System: "meerkat", ExternalID: relationshipRef(r.ID)}.String(),
				Field:    "relationship.target",
				Category: ImportIssueCategoryUnsupported,
				Message:  "target person (" + label + ") is not a contact row in the source database",
			})
			continue
		}
		relType := ""
		if r.Type != nil {
			relType = strings.TrimSpace(*r.Type)
		}
		matched, _ := models.MatchLegacyRelationType(relType)
		if matched == "" {
			matched = "related_to"
		}
		// source = the named person (related_contact_id), target = the owner.
		candidates = append(candidates, meerkatEdgeCandidate{edgeID: r.ID, source: *r.RelatedContact, target: *r.ContactID, typ: matched})
	}
	for _, c := range collapseReciprocalCandidates(candidates) {
		plan.Relationships = append(plan.Relationships, MappedRelationship{
			Ref:         SourceRef{System: "meerkat", ExternalID: relationshipRef(c.edgeID)},
			Source:      refOf(c.source),
			Target:      refOf(c.target),
			Type:        c.typ,
			Directional: !models.IsSymmetricRelationType(c.typ),
		})
	}
	for _, n := range snap.Notes {
		if !belongsToUser(n.UserID) || n.ContactID == nil {
			continue
		}
		if n.DeletedAt != nil {
			plan.Report.appendIssue(ImportIssue{
				Record:   SourceRef{System: "meerkat", ExternalID: noteRef(n.ID)}.String(),
				Field:    "note",
				Category: ImportIssueCategorySkipped,
				Message:  "soft-deleted in the source database",
			})
			continue
		}
		if !contactByID[*n.ContactID] {
			continue
		}
		content := ""
		if n.Content != nil {
			content = *n.Content
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		date := ""
		if n.Date != nil {
			date = normalizeSourceDate(*n.Date)
		}
		plan.Notes = append(plan.Notes, MappedNote{
			Ref:     SourceRef{System: "meerkat", ExternalID: noteRef(n.ID)},
			Contact: refOf(*n.ContactID),
			Content: content,
			Date:    date,
		})
	}

	attendeeByActivity := map[int64][]int64{}
	for _, ac := range snap.ActivityContacts {
		if contactByID[ac.ContactID] {
			attendeeByActivity[ac.ActivityID] = append(attendeeByActivity[ac.ActivityID], ac.ContactID)
		}
	}
	for _, a := range snap.Activities {
		if !belongsToUser(a.UserID) {
			continue
		}
		if a.DeletedAt != nil {
			plan.Report.appendIssue(ImportIssue{
				Record:   SourceRef{System: "meerkat", ExternalID: activityRef(a.ID)}.String(),
				Field:    "activity",
				Category: ImportIssueCategorySkipped,
				Message:  "soft-deleted in the source database",
			})
			continue
		}
		title := ""
		if a.Title != nil {
			title = *a.Title
		}
		desc := ""
		if a.Description != nil {
			desc = *a.Description
		}
		location := ""
		if a.Location != nil {
			location = *a.Location
		}
		date := ""
		if a.Date != nil {
			date = normalizeSourceDate(*a.Date)
		}
		var refs []SourceRef
		for _, id := range attendeeByActivity[a.ID] {
			refs = append(refs, refOf(id))
		}
		plan.Activities = append(plan.Activities, MappedActivity{
			Ref:         SourceRef{System: "meerkat", ExternalID: activityRef(a.ID)},
			Contacts:    refs,
			Title:       title,
			Description: desc,
			Location:    location,
			Date:        date,
		})
	}

	for _, r := range snap.Reminders {
		if !belongsToUser(r.UserID) || r.ContactID == nil {
			continue
		}
		if r.DeletedAt != nil {
			plan.Report.appendIssue(ImportIssue{
				Record:   SourceRef{System: "meerkat", ExternalID: reminderRef(r.ID)}.String(),
				Field:    "reminder",
				Category: ImportIssueCategorySkipped,
				Message:  "soft-deleted in the source database",
			})
			continue
		}
		if !contactByID[*r.ContactID] {
			continue
		}
		message := ""
		if r.Message != nil {
			message = *r.Message
		}
		remindAt := ""
		if r.RemindAt != nil {
			remindAt = normalizeSourceDate(*r.RemindAt)
		}
		recurrence := ""
		if r.Recurrence != nil {
			recurrence = *r.Recurrence
		}
		if !validRecurrence(recurrence) {
			plan.Report.appendIssue(ImportIssue{
				Record:   SourceRef{System: "meerkat", ExternalID: reminderRef(r.ID)}.String(),
				Field:    "reminder.recurrence",
				Category: ImportIssueCategoryInvalid,
				Message:  "unrecognized recurrence " + recurrence,
			})
			continue
		}
		var reoccur *bool
		if r.ReoccurFromCompletion != nil {
			b := *r.ReoccurFromCompletion != 0
			reoccur = &b
		}
		plan.Reminders = append(plan.Reminders, MappedReminder{
			Ref:                   SourceRef{System: "meerkat", ExternalID: reminderRef(r.ID)},
			Contact:               refOf(*r.ContactID),
			Message:               message,
			RemindAt:              remindAt,
			Recurrence:            recurrence,
			ReoccurFromCompletion: reoccur,
		})
	}

	// Circles: Meerkat stores groupings as a JSON array of names on each
	// contact. One Circle entity per unique name, members resolved by name.
	circleMembers := map[string][]SourceRef{}
	for _, c := range snap.Contacts {
		if !belongsToUser(c.UserID) || c.DeletedAt != nil {
			continue
		}
		for _, name := range meerkatCircles(c) {
			circleMembers[name] = append(circleMembers[name], refOf(c.ID))
		}
	}
	for name, members := range circleMembers {
		plan.Circles = append(plan.Circles, MappedCircle{
			Ref:     SourceRef{System: "meerkat", ExternalID: circleRef(name)},
			Name:    name,
			Members: members,
		})
	}

	return plan
}

// -- per-concept mappers ------------------------------------------------------

// contactRef / noteRef / ... build namespaced source-row identities, so a
// contact id 7 and a note id 7 never collide in the import_source_links
// ledger (the natural-key uniqueness spans system + external_id only).
func contactRef(id int64) string      { return "contact/" + itoa(id) }
func relationshipRef(id int64) string { return "relationship/" + itoa(id) }
func noteRef(id int64) string         { return "note/" + itoa(id) }
func activityRef(id int64) string     { return "activity/" + itoa(id) }
func reminderRef(id int64) string     { return "reminder/" + itoa(id) }
func customFieldRef(contactID int64, key string) string {
	return "custom_field/" + itoa(contactID) + "/" + key
}
func preferenceRef(contactID int64, key string) string {
	return "preference/" + itoa(contactID) + "/" + key
}
func circleRef(name string) string { return "circle/" + name }

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

// mapMeerkatContact maps one contact row onto a neutral Record plus the
// CRM-local flags. Mapped losses are appended to plan.Report.
func mapMeerkatContact(c meerkat.Contact, ref SourceRef, plan *ImportSourcePlan) MappedContact {
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return strings.TrimSpace(*p)
	}

	record := &contactmodel.Record{}
	card := contactmodel.Card{}
	if c.VCardUID != nil && *c.VCardUID != "" {
		card.UID = *c.VCardUID
	}

	name := &contactmodel.Name{}
	add := func(kind, value string) {
		if value != "" {
			name.Components = append(name.Components, contactmodel.NameComponent{Kind: kind, Value: value})
		}
	}
	add("title", str(c.Prefix))
	add("given", str(c.Firstname))
	add("given2", str(c.MiddleName))
	add("surname", str(c.Lastname))
	add("credential", str(c.Suffix))
	if len(name.Components) > 0 {
		card.Name = name
	}
	if n := str(c.Nickname); n != "" {
		card.Nicknames = []contactmodel.Nickname{{Name: n}}
	}

	card.Emails = append(card.Emails, parseMeerkatEmails(c.EmailsJSON)...)
	if len(card.Emails) == 0 && c.Email != nil && *c.Email != "" {
		card.Emails = []contactmodel.Email{{Address: *c.Email}}
	}
	card.Phones = append(card.Phones, parseMeerkatPhones(c.PhonesJSON)...)
	if len(card.Phones) == 0 && c.Phone != nil && *c.Phone != "" {
		card.Phones = []contactmodel.Phone{{Number: *c.Phone}}
	}
	card.ImppAddresses = parseMeerkatIMPPs(c.IMPPsJSON)
	card.Addresses = parseMeerkatAddresses(c.AddressesJSON)
	if len(card.Addresses) == 0 && c.Address != nil && *c.Address != "" {
		card.Addresses = []contactmodel.Address{{Full: *c.Address}}
	}
	card.Links = parseMeerkatLinks(c.URLsJSON)

	if b := mapMeerkatPartialDate(c.Birthday); b != nil {
		card.Anniversaries = append(card.Anniversaries, contactmodel.Anniversary{Kind: "birth", Date: contactmodel.AnniversaryDate{Partial: b}})
	}
	if a := mapMeerkatPartialDate(c.Anniversary); a != nil {
		card.Anniversaries = append(card.Anniversaries, contactmodel.Anniversary{Kind: "wedding", Date: contactmodel.AnniversaryDate{Partial: a}})
	}

	if org := str(c.Organization); org != "" || str(c.Department) != "" {
		o := contactmodel.Organization{Name: org}
		if d := str(c.Department); d != "" {
			o.Units = []contactmodel.OrgUnit{{Name: d}}
		}
		card.Organizations = []contactmodel.Organization{o}
	}
	var titles []contactmodel.Title
	if t := str(c.JobTitle); t != "" {
		titles = append(titles, contactmodel.Title{Name: t, Kind: "title"})
	}
	if r := str(c.Role); r != "" {
		titles = append(titles, contactmodel.Title{Name: r, Kind: "role"})
	}
	card.Titles = titles

	record.Card = card
	record.Envelope = contactmodel.CRMEnvelope{
		Gender:             NormalizeGender(str(c.Gender)),
		HowWeMet:           str(c.HowWeMet),
		WorkInformation:    str(c.WorkInfo),
		ContactInformation: str(c.ContactInfo),
		Circles:            meerkatCircles(c),
	}
	record.Passthrough = parseMeerkatVCardExtra(c.VCardExtra)

	mc := MappedContact{
		Ref:    ref,
		Record: record,
	}
	if c.Archived != nil && *c.Archived != 0 {
		mc.Archived = true
	}
	if c.Photo != nil && *c.Photo != "" {
		plan.Report.appendIssue(ImportIssue{
			Record:   ref.String(),
			Field:    "photo",
			Category: ImportIssueCategoryUnsupported,
			Message:  "photo files live in the source server's filesystem, not the database — not importable from the DB file",
		})
	}
	// Food preferences land as a real Preference (the modern home for what
	// was Meerkat's single free-text food_preference column).
	if fp := str(c.FoodPref); fp != "" {
		plan.Preferences = append(plan.Preferences, MappedPreference{
			Ref:      SourceRef{System: "meerkat", ExternalID: preferenceRef(c.ID, "food")},
			Contact:  ref,
			Category: models.PreferenceCategoryFood,
			Key:      "food_preferences",
			Value:    fp,
		})
	}
	return mc
}

// meerkatCircles parses a contact's circles JSON column ([]string).
func meerkatCircles(c meerkat.Contact) []string {
	if c.CirclesJSON == nil || *c.CirclesJSON == "" || *c.CirclesJSON == "null" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(*c.CirclesJSON), &out); err != nil {
		return nil
	}
	return out
}

// mapMeerkatCustomFields maps a contact's custom_fields JSON map onto the
// plan's CustomFields (FieldDefinition per key + FieldValue per value).
func mapMeerkatCustomFields(plan *ImportSourcePlan, c meerkat.Contact, ref SourceRef) {
	if c.CustomFields == nil || *c.CustomFields == "" || *c.CustomFields == "null" {
		return
	}
	var fields map[string]string
	if err := json.Unmarshal([]byte(*c.CustomFields), &fields); err != nil {
		return
	}
	for key, value := range fields {
		if strings.TrimSpace(value) == "" {
			continue
		}
		plan.CustomFields = append(plan.CustomFields, MappedCustomField{
			Ref:     SourceRef{System: "meerkat", ExternalID: customFieldRef(c.ID, key)},
			Contact: ref,
			Key:     key,
			Label:   key,
			Value:   value,
		})
	}
}

// meerkatEdgeCandidate is one mapped relationship edge before reciprocal
// collapse: edgeID is the source relationships row id (the ledger identity),
// source/target are the edge's endpoints (source = the named person), typ the
// matched registered token.
type meerkatEdgeCandidate struct {
	edgeID         int64
	source, target int64
	typ            string
}

// collapseReciprocalCandidates drops one half of a reciprocal pair: two
// candidate edges between the same two contacts whose types are the same or
// each other's inverse. The local graph derives the inverse from one stored
// edge, so both halves would render the relationship twice; the surviving
// half is the one with the lower source id (a stable, arbitrary choice, like
// Monica's collapse rule). A pair with a single edge — or two edges of
// unrelated types — is left untouched.
func collapseReciprocalCandidates(candidates []meerkatEdgeCandidate) []meerkatEdgeCandidate {
	if len(candidates) < 2 {
		return candidates
	}
	// group by unordered pair {min,max}
	type key struct{ a, b int64 }
	groups := map[key][]int{}
	for i, c := range candidates {
		a, b := c.source, c.target
		if a > b {
			a, b = b, a
		}
		groups[key{a, b}] = append(groups[key{a, b}], i)
	}
	drop := map[int]bool{}
	for _, idxs := range groups {
		if len(idxs) < 2 {
			continue
		}
		for i := 0; i < len(idxs); i++ {
			for j := i + 1; j < len(idxs); j++ {
				ci, cj := candidates[idxs[i]], candidates[idxs[j]]
				if ci.typ == cj.typ || ci.typ == models.InverseRelationType(cj.typ) {
					// keep the lower source id; drop the other
					if ci.source > cj.source {
						drop[idxs[i]] = true
					} else {
						drop[idxs[j]] = true
					}
				}
			}
		}
	}
	out := make([]meerkatEdgeCandidate, 0, len(candidates))
	for i, c := range candidates {
		if !drop[i] {
			out = append(out, c)
		}
	}
	return out
}

// -- JSON array parsers (Meerkat's flat model serialized the same structs
// this repo's flat model does) ------------------------------------------------

type flatEmail struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

func parseMeerkatEmails(raw *string) []contactmodel.Email {
	if raw == nil || *raw == "" || *raw == "null" {
		return nil
	}
	var list []flatEmail
	if err := json.Unmarshal([]byte(*raw), &list); err != nil {
		return nil
	}
	var out []contactmodel.Email
	for _, e := range list {
		if strings.TrimSpace(e.Value) != "" {
			out = append(out, contactmodel.Email{Address: strings.TrimSpace(e.Value), Label: e.Type})
		}
	}
	return out
}

type flatPhone struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

func parseMeerkatPhones(raw *string) []contactmodel.Phone {
	if raw == nil || *raw == "" || *raw == "null" {
		return nil
	}
	var list []flatPhone
	if err := json.Unmarshal([]byte(*raw), &list); err != nil {
		return nil
	}
	var out []contactmodel.Phone
	for _, p := range list {
		if strings.TrimSpace(p.Value) != "" {
			out = append(out, contactmodel.Phone{Number: strings.TrimSpace(p.Value), Label: p.Type})
		}
	}
	return out
}

type flatIMPP struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

func parseMeerkatIMPPs(raw *string) []contactmodel.OnlineService {
	if raw == nil || *raw == "" || *raw == "null" {
		return nil
	}
	var list []flatIMPP
	if err := json.Unmarshal([]byte(*raw), &list); err != nil {
		return nil
	}
	var out []contactmodel.OnlineService
	for _, i := range list {
		if strings.TrimSpace(i.Value) != "" {
			out = append(out, contactmodel.OnlineService{Service: i.Type, URI: strings.TrimSpace(i.Value)})
		}
	}
	return out
}

type flatURL struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

func parseMeerkatLinks(raw *string) []contactmodel.Resource {
	if raw == nil || *raw == "" || *raw == "null" {
		return nil
	}
	var list []flatURL
	if err := json.Unmarshal([]byte(*raw), &list); err != nil {
		return nil
	}
	var out []contactmodel.Resource
	for _, u := range list {
		if strings.TrimSpace(u.Value) != "" {
			out = append(out, contactmodel.Resource{URI: strings.TrimSpace(u.Value), Label: u.Type})
		}
	}
	return out
}

type flatAddress struct {
	Type    string `json:"type"`
	Street  string `json:"street"`
	City    string `json:"city"`
	Region  string `json:"region"`
	Postal  string `json:"postal"`
	Country string `json:"country"`
}

func parseMeerkatAddresses(raw *string) []contactmodel.Address {
	if raw == nil || *raw == "" || *raw == "null" {
		return nil
	}
	var list []flatAddress
	if err := json.Unmarshal([]byte(*raw), &list); err != nil {
		return nil
	}
	var out []contactmodel.Address
	for _, a := range list {
		if strings.TrimSpace(a.Street+a.City+a.Region+a.Postal+a.Country) == "" {
			continue
		}
		addr := contactmodel.Address{}
		addComp := func(kind, value string) {
			if value != "" {
				addr.Components = append(addr.Components, contactmodel.AddressComponent{Kind: kind, Value: value})
			}
		}
		addComp("name", strings.TrimSpace(a.Street))
		addComp("locality", strings.TrimSpace(a.City))
		addComp("region", strings.TrimSpace(a.Region))
		addComp("postcode", strings.TrimSpace(a.Postal))
		addComp("country", strings.TrimSpace(a.Country))
		if a.Type != "" {
			addr.Contexts = []string{a.Type}
		}
		out = append(out, addr)
	}
	return out
}

// mapMeerkatPartialDate parses a Meerkat birthday/anniversary string (the
// ISO "YYYY-MM-DD" or year-less "--MM-DD" format this repo's validator also
// accepts) into a PartialDate. Anything else maps to nil — the value stays
// on the flat source and is reported by the caller if meaningful.
func mapMeerkatPartialDate(raw *string) *contactmodel.PartialDate {
	if raw == nil {
		return nil
	}
	norm := NormalizeBirthday(*raw)
	if norm == "" {
		return nil
	}
	if strings.HasPrefix(norm, "--") {
		var month, day int
		if _, err := fmt.Sscanf(strings.TrimPrefix(norm, "--"), "%d-%d", &month, &day); err != nil {
			return nil
		}
		return &contactmodel.PartialDate{Month: &month, Day: &day}
	}
	if len(norm) == 10 && norm[4] == '-' && norm[7] == '-' {
		var year, month, day int
		if _, err := fmt.Sscanf(norm, "%d-%d-%d", &year, &month, &day); err != nil {
			return nil
		}
		return &contactmodel.PartialDate{Year: &year, Month: &month, Day: &day}
	}
	return nil
}

// normalizeSourceDate trims a source timestamp and returns it as-is. The
// engine's parseSourceTime already understands every format Meerkat writes
// (RFC3339, "2006-01-02 15:04:05", and date-only), so no rewriting is needed —
// this exists purely as the single trim point before a date enters the plan.
func normalizeSourceDate(s string) string {
	return strings.TrimSpace(s)
}

// validRecurrence reports whether a recurrence token is in the local
// vocabulary (models.Reminder's oneof).
func validRecurrence(t string) bool {
	switch t {
	case "once", "weekly", "monthly", "quarterly", "six-months", "yearly":
		return true
	}
	return false
}

// -- legacy vCard extra passthrough (best-effort, same shape as models'
// legacyVCardExtra) ----------------------------------------------------------

type legacyVCardExtra struct {
	Properties map[string][]legacyVCardField `json:"properties,omitempty"`
}

type legacyVCardField struct {
	Value  string              `json:"Value"`
	Params map[string][]string `json:"Params,omitempty"`
	Group  string              `json:"Group,omitempty"`
}

func parseMeerkatVCardExtra(raw *string) contactmodel.Passthrough {
	var pt contactmodel.Passthrough
	if raw == nil || *raw == "" {
		return pt
	}
	var extra legacyVCardExtra
	if err := json.Unmarshal([]byte(*raw), &extra); err != nil {
		return pt
	}
	for name, fields := range extra.Properties {
		for _, f := range fields {
			params := map[string]any{}
			for k, v := range f.Params {
				params[k] = v
			}
			if f.Group != "" {
				params["group"] = f.Group
			}
			valueJSON, err := json.Marshal(f.Value)
			if err != nil { // # pragma: no cover — json.Marshal of a string cannot fail
				continue
			}
			pt.VCard = append(pt.VCard, contactmodel.JCardProp{
				Name:   name,
				Params: params,
				Type:   "text",
				Value:  valueJSON,
			})
		}
	}
	return pt
}
