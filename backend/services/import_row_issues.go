package services

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog"

	"mycorrhizal/contactmodel"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
)

// Issues #2186 / #2187: the import review's error-row fixes, the empty-name
// fallback and the per-row report. Shared by the CSV and VCF confirm paths.

const (
	importIssueSkipped = "skipped"
	importIssueFailed  = "failed"
)

// applyImportNameFallback gives a row with no name at all (no first or last
// name) its first email, else its first phone, as the first name, so a
// contact that can be identified by either is not rejected for an empty name.
// Returns true when it changed the contact. A row with neither stays invalid.
func applyImportNameFallback(c *models.Contact) bool {
	if strings.TrimSpace(c.Firstname) != "" || strings.TrimSpace(c.Lastname) != "" {
		return false
	}
	fallback := ""
	for _, e := range c.Emails {
		if v := strings.TrimSpace(e.Value); v != "" {
			fallback = v
			break
		}
	}
	if fallback == "" {
		for _, p := range c.Phones {
			if v := strings.TrimSpace(p.Value); v != "" {
				fallback = v
				break
			}
		}
	}
	if fallback == "" {
		return false
	}
	setImportContactName(c, &fallback, nil)
	return true
}

// setImportContactName sets the given/surname on both the flat columns and the
// Card name (a VCF contact's Card is authoritative on save), copying the Card
// name so a session's stored contact is never mutated through a shared pointer.
// nil leaves a part unchanged.
func setImportContactName(c *models.Contact, first, last *string) {
	name := contactmodel.Name{}
	if c.Card.Name != nil {
		name = *c.Card.Name
		name.Components = append([]contactmodel.NameComponent(nil), name.Components...)
	}
	set := func(kind, v string) {
		out := name.Components[:0:0]
		replaced := false
		for _, comp := range name.Components {
			if comp.Kind == kind {
				if !replaced && v != "" {
					comp.Value = v
					out = append(out, comp)
				}
				replaced = true
				continue
			}
			out = append(out, comp)
		}
		if !replaced && v != "" {
			out = append(out, contactmodel.NameComponent{Kind: kind, Value: v})
		}
		name.Components = out
	}
	if first != nil {
		c.Firstname = strings.TrimSpace(*first)
		set("given", c.Firstname)
	}
	if last != nil {
		c.Lastname = strings.TrimSpace(*last)
		set("surname", c.Lastname)
	}
	name.Full = strings.TrimSpace(c.Firstname + " " + c.Lastname)
	c.Card.Name = &name
}

// applyImportRowEdit returns a copy of base with the user's correction applied
// (the stored session contact is left untouched so a retried confirm starts
// from the original).
func applyImportRowEdit(base models.Contact, e models.ImportRowEdit) models.Contact {
	c := base
	if e.Firstname != nil || e.Lastname != nil {
		setImportContactName(&c, e.Firstname, e.Lastname)
	}
	if e.Email != nil {
		c.Emails, c.Card.Emails = editEmail(c.Emails, c.Card.Emails, strings.TrimSpace(*e.Email))
		c.Email = ""
		if len(c.Emails) > 0 {
			c.Email = c.Emails[0].Value
		}
	}
	if e.Phone != nil {
		c.Phones, c.Card.Phones = editPhone(c.Phones, c.Card.Phones, strings.TrimSpace(*e.Phone))
		c.Phone = ""
		if len(c.Phones) > 0 {
			c.Phone = c.Phones[0].Value
		}
	}
	return c
}

// editEmail replaces the first invalid email (else the first) with value, or
// removes it when value is empty. The Card entry holding the old address is
// updated alongside.
func editEmail(flat []models.ContactEmail, card []contactmodel.Email, value string) ([]models.ContactEmail, []contactmodel.Email) {
	flat = append([]models.ContactEmail(nil), flat...)
	card = append([]contactmodel.Email(nil), card...)
	idx := 0
	for i, e := range flat {
		if e.Value != "" && !middleware.ValidateEmail(e.Value) {
			idx = i
			break
		}
	}
	old := ""
	switch {
	case len(flat) == 0 && value != "":
		flat = append(flat, models.ContactEmail{Value: value})
	case len(flat) > 0:
		old = flat[idx].Value
		if value == "" {
			flat = append(flat[:idx], flat[idx+1:]...)
		} else {
			flat[idx].Value = value
		}
	}
	for i, e := range card {
		if e.Address == old && old != "" {
			if value == "" {
				card = append(card[:i], card[i+1:]...)
			} else {
				card[i].Address = value
			}
			break
		}
	}
	if old == "" && value != "" {
		card = append(card, contactmodel.Email{Address: value})
	}
	return flat, card
}

// editPhone is editEmail for phone numbers.
func editPhone(flat []models.ContactPhone, card []contactmodel.Phone, value string) ([]models.ContactPhone, []contactmodel.Phone) {
	flat = append([]models.ContactPhone(nil), flat...)
	card = append([]contactmodel.Phone(nil), card...)
	idx := 0
	for i, p := range flat {
		if p.Value != "" && !IsValidPhone(p.Value) {
			idx = i
			break
		}
	}
	old := ""
	switch {
	case len(flat) == 0 && value != "":
		flat = append(flat, models.ContactPhone{Value: value})
	case len(flat) > 0:
		old = flat[idx].Value
		if value == "" {
			flat = append(flat[:idx], flat[idx+1:]...)
		} else {
			flat[idx].Value = value
		}
	}
	for i, p := range card {
		if p.Number == old && old != "" {
			if value == "" {
				card = append(card[:i], card[i+1:]...)
			} else {
				card[i].Number = value
			}
			break
		}
	}
	if old == "" && value != "" {
		card = append(card, contactmodel.Phone{Number: value})
	}
	return flat, card
}

// importRowEditMap indexes the request's edits by row (a later edit for the
// same row wins).
func importRowEditMap(edits []models.ImportRowEdit) map[int]models.ImportRowEdit {
	m := make(map[int]models.ImportRowEdit, len(edits))
	for _, e := range edits {
		m[e.RowIndex] = e
	}
	return m
}

// resolveErrorRow decides what happens to a row that failed preview validation
// when the user asked to add/update it. With a valid edit it returns the fixed
// contact; otherwise it returns a non-empty reason and the row is skipped.
func resolveErrorRow(preview models.ImportRowPreview, base models.Contact, edits map[int]models.ImportRowEdit) (models.Contact, string) {
	edit, ok := edits[preview.RowIndex]
	if !ok {
		return base, strings.Join(preview.ValidationErrors, "; ")
	}
	fixed := applyImportRowEdit(base, edit)
	applyImportNameFallback(&fixed)
	if errs := ValidateImportedContact(&fixed); len(errs) > 0 {
		return base, "Still invalid after edit: " + strings.Join(errs, "; ")
	}
	return fixed, ""
}

// importPreviewString reads a string field from a row's parsed contact.
func importPreviewString(p models.ImportRowPreview, key string) string {
	if v, ok := p.ParsedContact[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// importIssueFor builds the report entry for a row.
func importIssueFor(p models.ImportRowPreview, status, reason string) models.ImportRowIssue {
	first, last := importPreviewString(p, "firstname"), importPreviewString(p, "lastname")
	email, phone := importPreviewString(p, "email"), importPreviewString(p, "phone")
	name := strings.TrimSpace(first + " " + last)
	if name == "" {
		name = email
	}
	if name == "" {
		name = phone
	}
	return models.ImportRowIssue{Row: p.RowIndex + 1, Name: name, Email: email, Phone: phone, Status: status, Reason: reason}
}

// importSkipReason says why a row the user (or the default) left as "skip" was
// not imported.
func importSkipReason(p models.ImportRowPreview) string {
	switch {
	case len(p.ValidationErrors) > 0:
		return strings.Join(p.ValidationErrors, "; ")
	case p.BatchDuplicateOf != nil:
		return fmt.Sprintf("Duplicate of row %d in this file", *p.BatchDuplicateOf+1)
	default:
		return "Skipped"
	}
}

// reportRowFailure is importRowFailure plus an Issues entry.
func reportRowFailure(log *zerolog.Logger, result *models.ImportResult, p models.ImportRowPreview, what string, err error) {
	result.Errors = append(result.Errors, importRowFailure(log, p.RowIndex+1, what, err))
	result.Issues = append(result.Issues, importIssueFor(p, importIssueFailed, "Failed to "+what))
}
