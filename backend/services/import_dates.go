package services

import (
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"
)

// Issues #2631 / #2653 (maintainer decision "option C"): an impossible or
// unparseable birthday or anniversary is neither a row error nor silently
// dropped. The contact imports without that date and the row's `diagnostics`
// carries a warn Diagnostic {concept: "birthday"|"anniversary", message:
// "<concept> not imported: invalid value"}. Clients that already render row
// diagnostics show it with no contract change; the concept is the stable key.
//
// Dates the importers read: CSV Birthday and Anniversary columns (flat
// contact), vCard 3/4 BDAY and ANNIVERSARY (vCard 3: X-ANNIVERSARY), JSContact
// anniversaries and the device-contacts records' Card.Anniversaries. Death
// dates are not imported into a flat field and are left alone. The Monica /
// Meerkat / bundle source engine has its own per-record issue report.

const (
	importDateConceptBirthday    = "birthday"
	importDateConceptAnniversary = "anniversary"
)

func importDateWarning(concept string) contactmodel.Diagnostic {
	return contactmodel.Diagnostic{Severity: "warn", Concept: concept, Message: concept + " not imported: invalid value"}
}

func daysInMonth(month int, leap bool) int {
	switch month {
	case 2:
		if leap {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

// isValidCalendarDate reports whether s is a real "YYYY-MM-DD" date or a
// "--MM-DD" year-less one (Feb 29 allowed without a year).
func isValidCalendarDate(s string) bool {
	if !IsValidBirthdayFormat(s) {
		return false
	}
	if s[0] == '-' {
		return validDayOfMonth(nil, atoi2(s[2:4]), atoi2(s[5:7]))
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return false
	}
	return true
}

func atoi2(s string) int { return int(s[0]-'0')*10 + int(s[1]-'0') }

// validDayOfMonth checks month 1-12 and a day that exists in that month (with
// leap-year rules when the year is known; Feb 29 is allowed when it is not).
func validDayOfMonth(year *int, month, day int) bool {
	if month < 1 || month > 12 || day < 1 {
		return false
	}
	leap := year == nil || isLeapYear(*year)
	return day <= daysInMonth(month, leap)
}

// anniversaryDateValid reports whether a neutral date can be stored as a real
// date. An empty date (no timestamp, no partial) carries nothing and is left
// to the caller; a partial with no component at all, or a timestamp that is
// not RFC 3339, is what an unparseable BDAY looks like after the adapter.
func anniversaryDateValid(d contactmodel.AnniversaryDate) bool {
	if d.Timestamp != nil {
		_, err := time.Parse(time.RFC3339, *d.Timestamp)
		return err == nil
	}
	p := d.Partial
	if p == nil {
		return true
	}
	if p.Year == nil && p.Month == nil && p.Day == nil {
		return false
	}
	if p.Year != nil && *p.Year < 0 {
		return false
	}
	if p.Month != nil && (*p.Month < 1 || *p.Month > 12) {
		return false
	}
	if p.Day != nil {
		if *p.Day < 1 || *p.Day > 31 {
			return false
		}
		if p.Month != nil {
			return validDayOfMonth(p.Year, *p.Month, *p.Day)
		}
	}
	return true
}

// DropInvalidRecordDates removes birth and wedding anniversaries whose date is
// impossible or unparseable from rec (so neither the flat columns nor the
// stored Card carry it) and returns one warn Diagnostic per affected concept.
func DropInvalidRecordDates(rec *contactmodel.Record) []contactmodel.Diagnostic {
	if rec == nil || len(rec.Card.Anniversaries) == 0 {
		return nil
	}
	var diags []contactmodel.Diagnostic
	seen := map[string]bool{}
	kept := rec.Card.Anniversaries[:0:0]
	for _, a := range rec.Card.Anniversaries {
		concept := ""
		switch a.Kind {
		case "birth":
			concept = importDateConceptBirthday
		case "wedding":
			concept = importDateConceptAnniversary
		}
		if concept == "" || anniversaryDateValid(a.Date) {
			kept = append(kept, a)
			continue
		}
		if !seen[concept] {
			seen[concept] = true
			diags = append(diags, importDateWarning(concept))
		}
	}
	rec.Card.Anniversaries = kept
	return diags
}

// dropInvalidFlatDates is the flat-contact counterpart for CSV rows (which have
// no Card): an impossible or unparseable Birthday/Anniversary is cleared and
// reported.
func dropInvalidFlatDates(c *models.Contact) []contactmodel.Diagnostic {
	var diags []contactmodel.Diagnostic
	if c.Birthday != "" && !isValidCalendarDate(c.Birthday) {
		c.Birthday = ""
		diags = append(diags, importDateWarning(importDateConceptBirthday))
	}
	if c.Anniversary != "" && !isValidCalendarDate(c.Anniversary) {
		c.Anniversary = ""
		diags = append(diags, importDateWarning(importDateConceptAnniversary))
	}
	return diags
}
