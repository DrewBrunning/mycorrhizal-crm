package services

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"
	"mycorrhizal/monica"
)

// This file maps a Monica CRM account snapshot (monica.Snapshot — the shape
// the live API fetch produces, and the fixture format) onto the shared
// ImportSourcePlan (issues #351/#353). It is a deliberate port of the upstream
// Meerkat Monica assistant's proven mappers (meerkat-crm#211/#216/#218),
// re-targeted: contacts land as neutral contactmodel.Record values so the
// engine persists them through ApplyRecordToContact (CLAUDE.md backend trap
// #2), and Monica's graph (relationships, notes, activities, gifts,
// reminders) maps to its real local entity rather than collapsing into notes.
//
// Direction discipline: Monica writes each relationship to both contacts'
// pages; the local graph stores one directed edge and derives the inverse, so
// importing both halves would double-render. The mapper collapses the
// reciprocal pair exactly like upstream (the lower Monica contact id's half
// survives), and the type is resolved through models.MatchLegacyRelationType
// so "father"/"spouse"/"daughter" land on the registered vocabulary with a
// related_to fallback.

// MapMonicaSnapshot maps a fetched snapshot onto an import plan. now is the
// "today" reference for reminder scheduling (nextRecurrenceOnOrAfter) and is
// injectable for deterministic tests (the untitled-activity fallback title is
// deliberately language-free, unlike upstream's localized rendering).
func MapMonicaSnapshot(snap *monica.Snapshot, now time.Time) *ImportSourcePlan {
	plan := &ImportSourcePlan{System: "monica"}
	if snap == nil {
		return plan
	}

	contactByID := map[int]monica.Contact{}
	refOf := func(id int) SourceRef { return SourceRef{System: "monica", ExternalID: monicaContactRef(id)} }
	for _, c := range snap.Contacts {
		contactByID[c.ID] = c
	}

	for _, c := range snap.Contacts {
		ref := refOf(c.ID)
		if c.IsPartial {
			plan.Report.appendIssue(ImportIssue{
				Record:   ref.String(),
				Field:    "contact",
				Category: ImportIssueCategorySkipped,
				Message:  "Monica partial contact (a name-only stub behind a relationship) — the relationship carries it",
			})
			continue
		}
		plan.Contacts = append(plan.Contacts, mapMonicaContact(c, ref, plan))
	}

	// Relationships: snapshot.Relationships is keyed by the subject
	// (contact_is — Monica's own index query is WHERE contact_is = X).
	// A row {contact_is: A, of_contact: B, type: "daughter"} means "B is A's
	// daughter": the type describes *of_contact's* role relative to
	// contact_is (verified against Monica's ApiRelationshipController +
	// Contact::setRelationship, which writes both directions). Our edge
	// stores the source's role relative to the target, so the edge is
	// of_contact → contact_is with the matched type verbatim ("daughter" →
	// child_of: B is A's child). Monica writes every relationship to both
	// contacts' pages; the reciprocal halves are collapsed (the lower
	// subject id's half survives) since the local graph derives the inverse
	// itself.
	edges := reciprocalEdgeIndex{}
	for subject, rels := range snap.Relationships {
		for _, rel := range rels {
			if rel.OfContact != nil {
				edges[[2]int{subject, rel.OfContact.ID}] = true
			}
		}
	}
	birthdays := monicaBirthdayMonthDay(snap)
	for subject, rels := range snap.Relationships {
		if _, imported := contactByID[subject]; !imported {
			continue
		}
		for _, rel := range rels {
			if rel.OfContact == nil || rel.OfContact.ID == subject {
				continue
			}
			if _, imported := contactByID[rel.OfContact.ID]; !imported {
				continue
			}
			if edges.isRedundantHalf(subject, rel.OfContact.ID) {
				continue
			}
			matched, _ := models.MatchLegacyRelationType(rel.RelationshipType.Name)
			if matched == "" {
				matched = "related_to"
			}
			plan.Relationships = append(plan.Relationships, MappedRelationship{
				Ref:         SourceRef{System: "monica", ExternalID: monicaRelationshipRef(subject, rel.OfContact.ID)},
				Source:      refOf(rel.OfContact.ID),
				Target:      refOf(subject),
				Type:        matched,
				Directional: !models.IsSymmetricRelationType(matched),
			})
		}
	}

	for _, a := range snap.Activities {
		if !monicaTimeUsable(a.HappenedAt) {
			plan.Report.appendIssue(ImportIssue{
				Record:   SourceRef{System: "monica", ExternalID: monicaActivityRef(a.ID)}.String(),
				Field:    "activity.happened_at",
				Category: ImportIssueCategoryInvalid,
				Message:  "unusable date: " + a.HappenedAt,
			})
			continue
		}
		title := strings.TrimSpace(a.Summary)
		if title == "" && a.ActivityType != nil {
			title = strings.TrimSpace(a.ActivityType.Name)
		}
		if title == "" {
			title = "Activity" // untitled fallback; upstream localized this, "Activity" keeps the mapper language-free
		}
		description := strings.TrimSpace(a.Description)
		if a.ActivityType != nil && strings.TrimSpace(a.ActivityType.Name) != "" && strings.TrimSpace(a.Summary) != "" {
			typeName := strings.TrimSpace(a.ActivityType.Name)
			if description == "" {
				description = typeName
			} else {
				description = typeName + "\n" + description
			}
		}
		var refs []SourceRef
		for _, attendee := range a.Attendees.Contacts {
			if _, ok := contactByID[attendee.ID]; ok {
				refs = append(refs, refOf(attendee.ID))
			}
		}
		plan.Activities = append(plan.Activities, MappedActivity{
			Ref:         SourceRef{System: "monica", ExternalID: monicaActivityRef(a.ID)},
			Contacts:    refs,
			Title:       title,
			Description: description,
			Date:        a.HappenedAt,
		})
	}

	for _, n := range snap.Notes {
		body := strings.TrimSpace(n.Body)
		if body == "" || n.Contact == nil {
			continue
		}
		if _, ok := contactByID[n.Contact.ID]; !ok {
			continue
		}
		date := n.CreatedAt
		if !monicaTimeUsable(date) {
			date = time.Now().Format(time.RFC3339)
		}
		plan.Notes = append(plan.Notes, MappedNote{
			Ref:     SourceRef{System: "monica", ExternalID: monicaNoteRef(n.ID)},
			Contact: refOf(n.Contact.ID),
			Content: body,
			Date:    date,
		})
	}

	for _, r := range snap.Reminders {
		if r.Contact == nil {
			continue
		}
		if _, ok := contactByID[r.Contact.ID]; !ok {
			continue
		}
		reminder, ok := mapMonicaReminder(r, now, birthdays[r.Contact.ID])
		if !ok {
			continue
		}
		plan.Reminders = append(plan.Reminders, reminder)
	}

	// Calls: Monica's logged phone calls are the InteractionTypeCall home.
	for _, c := range snap.Calls {
		if c.Contact == nil {
			continue
		}
		if _, ok := contactByID[c.Contact.ID]; !ok {
			continue
		}
		content := strings.TrimSpace(c.Content)
		date := c.CalledAt
		if !monicaTimeUsable(date) {
			date = time.Now().Format(time.RFC3339)
		}
		title := "Call"
		if content != "" {
			title = "Call: " + content
			if len([]rune(title)) > 200 {
				title = string([]rune(title)[:200])
			}
		}
		plan.Activities = append(plan.Activities, MappedActivity{
			Ref:         SourceRef{System: "monica", ExternalID: monicaCallRef(c.ID)},
			Contacts:    []SourceRef{refOf(c.Contact.ID)},
			Title:       title,
			Description: "",
			Date:        date,
			Type:        models.InteractionTypeCall,
		})
	}

	// Tasks and debts have no direct entity home; they become dated notes
	// describing the task/debt, the same lossy-but-named choice upstream made.
	for _, t := range snap.Tasks {
		if t.Contact == nil {
			continue
		}
		if _, ok := contactByID[t.Contact.ID]; !ok {
			continue
		}
		title := strings.TrimSpace(t.Title)
		if title == "" {
			continue
		}
		body := "Task: " + title
		if desc := strings.TrimSpace(t.Description); desc != "" {
			body += "\n" + desc
		}
		if t.Completed {
			body += "\n(completed)"
		}
		date := t.CreatedAt
		if t.CompletedAt != nil && monicaTimeUsable(*t.CompletedAt) {
			date = *t.CompletedAt
		} else if !monicaTimeUsable(date) {
			date = time.Now().Format(time.RFC3339)
		}
		plan.Notes = append(plan.Notes, MappedNote{
			Ref:     SourceRef{System: "monica", ExternalID: monicaTaskRef(t.ID)},
			Contact: refOf(t.Contact.ID),
			Content: body,
			Date:    date,
		})
	}
	for _, d := range snap.Debts {
		if d.Contact == nil {
			continue
		}
		if _, ok := contactByID[d.Contact.ID]; !ok {
			continue
		}
		amount := strings.TrimSpace(d.AmountWithCurrency)
		if amount == "" {
			amount = strconv.FormatFloat(d.Amount, 'f', 2, 64)
		}
		body := "Debt: " + amount
		if d.InDebt == "yes" {
			body += " (I owe them)"
		} else {
			body += " (they owe me)"
		}
		if reason := strings.TrimSpace(d.Reason); reason != "" {
			body += "\n" + reason
		}
		date := d.CreatedAt
		if !monicaTimeUsable(date) {
			date = time.Now().Format(time.RFC3339)
		}
		plan.Notes = append(plan.Notes, MappedNote{
			Ref:     SourceRef{System: "monica", ExternalID: monicaDebtRef(d.ID)},
			Contact: refOf(d.Contact.ID),
			Content: body,
			Date:    date,
		})
	}

	// Gifts: Monica's gifts are the models.Gift home (not a note as upstream
	// had to do).
	for _, g := range snap.Gifts {
		if g.Contact == nil {
			continue
		}
		if _, ok := contactByID[g.Contact.ID]; !ok {
			continue
		}
		name := strings.TrimSpace(g.Name)
		if name == "" {
			continue
		}
		status := g.Status
		if status == "" {
			switch {
			case g.HasBeenReceived != nil && *g.HasBeenReceived:
				status = "received"
			case g.HasBeenOffered != nil && *g.HasBeenOffered:
				status = "offered"
			default:
				status = "idea"
			}
		}
		mapped := MappedGift{
			Ref:         SourceRef{System: "monica", ExternalID: monicaGiftRef(g.ID)},
			Contact:     refOf(g.Contact.ID),
			Status:      mapMonicaGiftStatus(status),
			Description: name,
		}
		if comment := strings.TrimSpace(g.Comment); comment != "" {
			mapped.Notes = comment
		}
		amount := ""
		if g.Amount != nil {
			amount = strings.TrimSpace(*g.Amount)
		}
		if amount != "" && mapped.Notes != "" {
			mapped.Notes += " — Amount: " + amount
		} else if amount != "" {
			mapped.Notes = "Amount: " + amount
		}
		if g.URL != "" {
			mapped.URL = g.URL
		}
		if g.Date != nil {
			mapped.Date = *g.Date
		}
		plan.Gifts = append(plan.Gifts, mapped)
	}

	// Circles from Monica tags (the grouping concept), via the envelope and
	// as real Circle entities.
	tagMembers := map[string][]SourceRef{}
	for _, c := range snap.Contacts {
		if c.IsPartial {
			continue
		}
		for _, tag := range c.Tags {
			if strings.TrimSpace(tag.Name) == "" {
				continue
			}
			tagMembers[tag.Name] = append(tagMembers[tag.Name], refOf(c.ID))
		}
	}
	for name, members := range tagMembers {
		plan.Circles = append(plan.Circles, MappedCircle{
			Ref:     SourceRef{System: "monica", ExternalID: monicaCircleRef(name)},
			Name:    name,
			Members: members,
		})
	}

	return plan
}

// -- source refs (namespaced so entity kinds never collide in the ledger) ----

func monicaContactRef(id int) string { return "contact/" + strconv.Itoa(id) }
func monicaRelationshipRef(a, b int) string {
	return "relationship/" + strconv.Itoa(a) + "-" + strconv.Itoa(b)
}
func monicaActivityRef(id int) string    { return "activity/" + strconv.Itoa(id) }
func monicaNoteRef(id int) string        { return "note/" + strconv.Itoa(id) }
func monicaReminderRef(id int) string    { return "reminder/" + strconv.Itoa(id) }
func monicaCallRef(id int) string        { return "call/" + strconv.Itoa(id) }
func monicaTaskRef(id int) string        { return "task/" + strconv.Itoa(id) }
func monicaDebtRef(id int) string        { return "debt/" + strconv.Itoa(id) }
func monicaGiftRef(id int) string        { return "gift/" + strconv.Itoa(id) }
func monicaCircleRef(name string) string { return "circle/" + name }

// -- per-concept mappers ------------------------------------------------------

func mapMonicaContact(c monica.Contact, ref SourceRef, plan *ImportSourcePlan) MappedContact {
	record := &contactmodel.Record{}
	card := contactmodel.Card{}

	first := strings.TrimSpace(c.FirstName)
	last := strings.TrimSpace(c.LastName)
	nick := strings.TrimSpace(c.Nickname)
	if first == "" {
		if nick != "" {
			first = nick
		} else if last != "" {
			first = last
			last = ""
		}
	}
	name := &contactmodel.Name{}
	if first != "" {
		name.Components = append(name.Components, contactmodel.NameComponent{Kind: "given", Value: first})
	}
	if last != "" {
		name.Components = append(name.Components, contactmodel.NameComponent{Kind: "surname", Value: last})
	}
	if len(name.Components) > 0 {
		card.Name = name
	}
	if nick != "" {
		card.Nicknames = []contactmodel.Nickname{{Name: nick}}
	}

	gender := ""
	switch c.GenderType {
	case "M":
		gender = "male"
	case "F":
		gender = "female"
	case "O":
		gender = "other"
	default:
		gender = NormalizeGender(c.Gender)
	}

	if b := mapMonicaSpecialDateToPartial(c.Information.Dates.Birthdate); b != nil {
		card.Anniversaries = append(card.Anniversaries, contactmodel.Anniversary{Kind: "birth", Date: contactmodel.AnniversaryDate{Partial: b}})
	}
	if c.IsDead {
		if d := mapMonicaSpecialDateToPartial(c.Information.Dates.DeceasedDate); d != nil {
			card.Anniversaries = append(card.Anniversaries, contactmodel.Anniversary{Kind: "death", Date: contactmodel.AnniversaryDate{Partial: d}})
		} else {
			plan.Report.appendIssue(ImportIssue{
				Record:   ref.String(),
				Field:    "is_dead",
				Category: ImportIssueCategoryTransformed,
				Message:  "deceased marker without a date is not represented (no neutral home for a bare death flag)",
			})
		}
	}

	if c.Information.Career.Job != nil {
		card.Titles = append(card.Titles, contactmodel.Title{Name: strings.TrimSpace(*c.Information.Career.Job), Kind: "title"})
	}
	if c.Information.Career.Company != nil {
		card.Organizations = []contactmodel.Organization{{Name: strings.TrimSpace(*c.Information.Career.Company)}}
	}

	for _, addr := range c.Addresses {
		if a := mapMonicaAddress(addr); a != nil {
			card.Addresses = append(card.Addresses, *a)
		}
	}
	for _, field := range c.ContactFields {
		mapMonicaContactField(&card, field)
	}

	record.Card = card
	record.Envelope = contactmodel.CRMEnvelope{
		Gender:             gender,
		HowWeMet:           monicaOptional(c.Information.HowYouMet.GeneralInformation),
		ContactInformation: strings.TrimSpace(c.Description),
	}
	record.Envelope.Circles = mapMonicaTagNames(c.Tags)

	mc := MappedContact{Ref: ref, Record: record}
	if c.IsStarred {
		mc.Favorite = true
	}
	if c.Information.Avatar.URL != nil && c.Information.Avatar.Source != nil {
		if src := *c.Information.Avatar.Source; src == "photo" || src == "gravatar" {
			mc.PhotoURL = *c.Information.Avatar.URL
			mc.PhotoSource = src
		}
	}

	if strings.TrimSpace(c.FoodPreferences) != "" {
		plan.Preferences = append(plan.Preferences, MappedPreference{
			Ref:      SourceRef{System: "monica", ExternalID: monicaContactRef(c.ID) + "/food"},
			Contact:  ref,
			Category: models.PreferenceCategoryFood,
			Key:      "food_preferences",
			Value:    strings.TrimSpace(c.FoodPreferences),
		})
	}

	return mc
}

func monicaOptional(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func mapMonicaTagNames(tags []monica.Tag) []string {
	var out []string
	for _, t := range tags {
		if strings.TrimSpace(t.Name) != "" {
			out = append(out, strings.TrimSpace(t.Name))
		}
	}
	return out
}

// mapMonicaAddress maps one Monica address onto the neutral shape; returns
// nil for an all-empty address.
func mapMonicaAddress(a monica.Address) *contactmodel.Address {
	addr := contactmodel.Address{}
	addComp := func(kind, value string) {
		if value != "" {
			addr.Components = append(addr.Components, contactmodel.AddressComponent{Kind: kind, Value: value})
		}
	}
	addComp("name", strings.TrimSpace(a.Street))
	addComp("locality", strings.TrimSpace(a.City))
	addComp("region", strings.TrimSpace(a.Province))
	addComp("postcode", strings.TrimSpace(a.PostalCode))
	if a.Country != nil {
		addComp("country", strings.TrimSpace(a.Country.Name))
	}
	if len(addr.Components) == 0 {
		return nil
	}
	if strings.TrimSpace(a.Name) != "" {
		addr.Contexts = []string{strings.TrimSpace(a.Name)}
	}
	return &addr
}

// mapMonicaContactField routes a denormalized Monica contact field into the
// matching neutral collection (email, phone, URL, or online service).
func mapMonicaContactField(card *contactmodel.Card, field monica.ContactField) {
	content := strings.TrimSpace(field.Content)
	if content == "" {
		return
	}
	protocol := ""
	if field.ContactFieldType.Protocol != nil {
		protocol = *field.ContactFieldType.Protocol
	}
	kind := ""
	if field.ContactFieldType.Type != nil {
		kind = *field.ContactFieldType.Type
	}
	label := strings.ToLower(strings.TrimSpace(field.ContactFieldType.Name))

	switch {
	case protocol == "mailto:" || kind == "email":
		card.Emails = append(card.Emails, contactmodel.Email{Address: content})
	case protocol == "tel:" || kind == "phone":
		card.Phones = append(card.Phones, contactmodel.Phone{Number: content})
	case isHTTPURL(content):
		card.Links = append(card.Links, contactmodel.Resource{URI: content, Label: label})
	default:
		card.ImppAddresses = append(card.ImppAddresses, contactmodel.OnlineService{Service: label, URI: content})
	}
}

func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// mapMonicaSpecialDateToPartial converts Monica's flexible date to the
// neutral PartialDate: year-unknown becomes month/day only, age-based dates
// (an estimate, not a fact) map to nil.
func mapMonicaSpecialDateToPartial(d monica.SpecialDate) *contactmodel.PartialDate {
	if d.Date == nil || d.IsAgeBased {
		return nil
	}
	date := strings.TrimSpace(*d.Date)
	if len(date) < 10 {
		return nil
	}
	date = date[:10]
	var year, month, day int
	if _, err := fmt.Sscanf(date, "%d-%d-%d", &year, &month, &day); err != nil {
		return nil
	}
	if d.IsYearUnknown {
		return &contactmodel.PartialDate{Month: &month, Day: &day}
	}
	return &contactmodel.PartialDate{Year: &year, Month: &month, Day: &day}
}

// mapMonicaReminder folds Monica's frequency model into the local recurrence
// vocabulary and schedules the reminder at its first future occurrence.
// birthdayMonthDay is the "MM-DD" of the contact's birthday, or "" when it
// has none — it identifies Monica's auto-created birthday reminders (their
// titles are written in the Monica user's own language, so matching the date
// is the locale-proof signal).
func mapMonicaReminder(mr monica.Reminder, now time.Time, birthdayMonthDay string) (MappedReminder, bool) {
	if mr.Contact == nil {
		return MappedReminder{}, false
	}
	dateStr := ""
	if mr.NextExpectedDate != nil && *mr.NextExpectedDate != "" {
		dateStr = *mr.NextExpectedDate
	} else if mr.InitialDate != nil {
		dateStr = *mr.InitialDate
	}
	remindAt, err := parseSourceTime(dateStr)
	if err != nil {
		return MappedReminder{}, false
	}

	recurrence := ""
	switch mr.FrequencyType {
	case "one_time":
		recurrence = "once"
	case "week":
		recurrence = "weekly"
	case "month":
		switch mr.FrequencyNumber {
		case 3:
			recurrence = "quarterly"
		case 6:
			recurrence = "six-months"
		default:
			recurrence = "monthly"
		}
	case "year":
		recurrence = "yearly"
	default:
		return MappedReminder{}, false
	}
	if recurrence == "once" && remindAt.Before(now) {
		return MappedReminder{}, false // a past one-time reminder is a dead row
	}

	message := strings.TrimSpace(mr.Title)
	if desc := strings.TrimSpace(mr.Description); desc != "" {
		if message == "" {
			message = desc
		} else {
			message = message + ": " + desc
		}
	}
	if message == "" {
		return MappedReminder{}, false
	}
	reoccur := !isMonicaBirthdayReminder(recurrence, remindAt, birthdayMonthDay)
	return MappedReminder{
		Ref:                   SourceRef{System: "monica", ExternalID: monicaReminderRef(mr.ID)},
		Contact:               SourceRef{System: "monica", ExternalID: monicaContactRef(mr.Contact.ID)},
		Message:               message,
		RemindAt:              nextRecurrenceOnOrAfter(remindAt, recurrence, now).Format(time.RFC3339),
		Recurrence:            recurrence,
		ReoccurFromCompletion: &reoccur,
	}, true
}

// isMonicaBirthdayReminder recognizes Monica's auto-created birthday
// reminders by their shape: a yearly reminder falling on the contact's
// birthday. The local default is to reschedule a completed reminder from its
// completion date, which would walk a birthday forward a week every year —
// birthdays stay pinned to the calendar instead.
func isMonicaBirthdayReminder(recurrence string, remindAt time.Time, birthdayMonthDay string) bool {
	if recurrence != "yearly" || birthdayMonthDay == "" {
		return false
	}
	return remindAt.Format("01-02") == birthdayMonthDay
}

// monicaBirthdayMonthDay indexes the "MM-DD" of every contact's birthday
// (year-unknown "--MM-DD" and full "YYYY-MM-DD" both end in month+day).
func monicaBirthdayMonthDay(snap *monica.Snapshot) map[int]string {
	out := make(map[int]string, len(snap.Contacts))
	for _, mc := range snap.Contacts {
		birthday := mapMonicaSpecialDateToPartial(mc.Information.Dates.Birthdate)
		if birthday == nil || birthday.Month == nil || birthday.Day == nil {
			continue
		}
		out[mc.ID] = birthdayToString(*birthday.Month, *birthday.Day)
	}
	return out
}

func birthdayToString(month, day int) string {
	return fmt.Sprintf("%02d-%02d", month, day)
}

// nextRecurrenceOnOrAfter moves a recurring reminder to its first occurrence
// on or after today (ported from upstream, which documented the birthday
// walk-forward trap this avoids).
func nextRecurrenceOnOrAfter(remindAt time.Time, recurrence string, now time.Time) time.Time {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, remindAt.Location())
	if !remindAt.Before(today) {
		return remindAt
	}
	var after func(periods int) time.Time
	switch recurrence {
	case "weekly":
		after = func(p int) time.Time { return remindAt.AddDate(0, 0, 7*p) }
	case "monthly":
		after = func(p int) time.Time { return addMonths(remindAt, p) }
	case "quarterly":
		after = func(p int) time.Time { return addMonths(remindAt, 3*p) }
	case "six-months":
		after = func(p int) time.Time { return addMonths(remindAt, 6*p) }
	case "yearly":
		after = func(p int) time.Time { return addYears(remindAt, p) }
	default:
		return remindAt
	}
	next := remindAt
	for p := 1; next.Before(today); p++ {
		next = after(p)
	}
	return next
}

func mapMonicaGiftStatus(status string) string {
	switch status {
	case "idea":
		return models.GiftStatusIdea
	case "offered":
		return models.GiftStatusGiven
	case "received":
		return models.GiftStatusReceived
	case "purchased":
		return models.GiftStatusPurchased
	default:
		return models.GiftStatusIdea
	}
}

// monicaTimeUsable reports whether a Monica timestamp is parseable by the
// shared engine (RFC3339 or date-only).
func monicaTimeUsable(s string) bool {
	_, err := parseSourceTime(s)
	return err == nil
}

// reciprocalEdgeIndex records which directed relationship edges the snapshot
// contains, so the mapper can tell one half of a Monica pair from a genuinely
// one-sided relationship.
type reciprocalEdgeIndex map[[2]int]bool

// isRedundantHalf reports whether the edge owner→other is the half of a
// bidirectional Monica pair that can be dropped. Monica writes every
// relationship to both contacts' pages; the local graph derives the inverse
// from one stored edge, so importing both halves renders each relationship
// twice. The surviving half is the one owned by the lower Monica contact id —
// an arbitrary but stable choice, since snapshot.Relationships is a map and
// its iteration order changes between runs. A one-sided relationship (only
// one direction recorded) survives, because dropping it would lose it.
func (idx reciprocalEdgeIndex) isRedundantHalf(owner, other int) bool {
	return other < owner && idx[[2]int{other, owner}]
}
