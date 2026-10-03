package models

import (
	"fmt"

	"mycorrhizal/contactmodel"
)

// Field-selection section tokens  / T9 (selective field export +
// sensitivity gating, T9).
//
// These are the coarse-grained, Google-Contacts-picker-style sections a user
// can choose when exporting a contact card — deliberately NOT per-value
// granularity. One token covers a whole Card top-level section (plus, for
// organizations, the linked titles), so a single selection applies identically
// to every exporter through the shared neutral Card.
//
// The frontend hardcodes a mirror of this list (CLAUDE.md frontend trap #4)
// in src/api/export.ts — keep EXPORT_FIELD_SECTIONS in sync when adding a
// token here.
//
// Card data with no section token is identity/metadata and is always exported
// regardless of selection: name/uid/nicknames/kind/prodId/created/updated/
// language, and the exotic resource collections (Calendars, FreeBusyURLs,
// SchedulingAddresses, CryptoKeys, Directories, Localizations) that the picker
// deliberately does not claim to cover.
const (
	SectionEmails         = "emails"
	SectionPhones         = "phones"
	SectionAddresses      = "addresses"
	SectionOrganizations  = "organizations"
	SectionAnniversaries  = "anniversaries"
	SectionMedia          = "media"
	SectionOnlineServices = "online_services"
	SectionLinks          = "links"
	SectionNotes          = "notes"
	SectionKeywords       = "keywords"
	SectionRelatedTo      = "related_to"
	SectionPersonalInfo   = "personal_info"
	SectionSpeakToAs      = "speak_to_as"
	SectionMembers        = "members"
	SectionLanguages      = "languages"
	SectionCustomFields   = "custom_fields"
)

// sensitiveSections are the sections whose data can carry a sensitivity
// above "normal". Only these are affected by FieldSelection.IncludeSensitive —
// the opt-in override that lets one export include private/secret items just
// this once. Tags (SectionKeywords) and every other section have no
// sensitivity dimension today.
//
// SectionAddresses is here because ADR 0031 gave ContactAddress a Sensitivity
// (and, with it, a geo: coordinate): an above-normal address must not leave the
// instance through the neutral-Card exports/shares any more than an
// above-normal relationship edge, hobby preference, or vCard custom field may.
// The filtering itself happens in ApplyFieldSelection below; the map view, the
// flat CSV backup and the account bundle are the documented owner-facing
// exceptions and never pass a FieldSelection.
var sensitiveSections = map[string]bool{
	SectionAddresses:    true,
	SectionRelatedTo:    true,
	SectionPersonalInfo: true,
	SectionCustomFields: true,
}

// FieldSections returns the full ordered list of section tokens.
func FieldSections() []string {
	return []string{
		SectionEmails, SectionPhones, SectionAddresses, SectionOrganizations,
		SectionAnniversaries, SectionMedia, SectionOnlineServices, SectionLinks,
		SectionNotes, SectionKeywords, SectionRelatedTo, SectionPersonalInfo,
		SectionSpeakToAs, SectionMembers, SectionLanguages, SectionCustomFields,
	}
}

// IsSensitiveSection reports whether a section's data can carry a
// sensitivity above "normal", i.e. whether it is gated behind the
// IncludeSensitive opt-in override.
func IsSensitiveSection(token string) bool {
	return sensitiveSections[token]
}

var validFieldSections = func() map[string]bool {
	m := make(map[string]bool, len(FieldSections()))
	for _, s := range FieldSections() {
		m[s] = true
	}
	return m
}()

// FieldSelection is one export's "which fields" picker state (T9).
//
// Sections maps each selected section token to true. Absent/unknown tokens
// are simply not selected. A nil map is the zero value and means "no sections
// selected"; use FieldSelectionAll for the all-on default.
//
// IncludeSensitive is the explicit opt-in override: when true,
// projection steps that normally filter to sensitivity='normal' (relationship
// edges, hobby preferences, vCard-projected custom fields, and — via
// ApplyFieldSelection — private/secret postal addresses with their geo:
// coordinates) also include their private/secret items. This is the backend
// half of the foot-gun guard — it is a separate, intentional flag that no
// amount of ordinary section-checking can imply.
type FieldSelection struct {
	Sections         map[string]bool
	IncludeSensitive bool
}

// NewFieldSelection returns an empty selection (no sections selected).
func NewFieldSelection() *FieldSelection {
	return &FieldSelection{Sections: make(map[string]bool)}
}

// FieldSelectionAll returns a selection with every section enabled — the
// default for an export call with no selection params, preserving pre-T9
// behavior exactly.
func FieldSelectionAll() *FieldSelection {
	sel := NewFieldSelection()
	for _, s := range FieldSections() {
		sel.Sections[s] = true
	}
	return sel
}

// Enable selects a section token. It returns an error for an unknown token
// rather than silently ignoring it, so a typo can't silently narrow an export.
func (f *FieldSelection) Enable(token string) error {
	if !validFieldSections[token] {
		return fmt.Errorf("unknown export field section %q", token)
	}
	f.Sections[token] = true
	return nil
}

// Has reports whether a section token is selected.
func (f *FieldSelection) Has(token string) bool {
	return f != nil && f.Sections[token]
}

// ApplyFieldSelection returns a COPY of record with every section not
// selected in sel cleared from Card (and, for the custom_fields section, from
// Passthrough). It also default-denies private/secret postal addresses (and
// their geo: coordinates) when IncludeSensitive is false — the address half of
// the same rule the relationship-edge / hobby / custom-field projections
// already enforce. It never mutates record or its nested slices, matching the
// projection functions' "returns a new slice rather than mutating existing"
// discipline. A nil selection returns record unchanged, which is what keeps
// the owner-facing read paths (REST detail, map) and the full-fidelity
// backups (flat CSV, account bundle) unfiltered — none of them passes a
// FieldSelection.
//
// This is the single filter point the whole ticket is built around: it runs
// BEFORE any exporter, and because vcard3/vcard4/jscontact all consume the
// same neutral Card, one function applies identically to all three formats
// with zero changes to any adapter.
func ApplyFieldSelection(record *contactmodel.Record, sel *FieldSelection) *contactmodel.Record {
	if record == nil || sel == nil {
		return record
	}

	card := record.Card
	passthrough := record.Passthrough

	if !sel.Has(SectionEmails) {
		card.Emails = nil
	}
	if !sel.Has(SectionPhones) {
		card.Phones = nil
	}
	if !sel.Has(SectionAddresses) {
		card.Addresses = nil
	} else if !sel.IncludeSensitive {
		card.Addresses = filterSensitiveAddresses(card.Addresses)
	}
	if !sel.Has(SectionOrganizations) {
		card.Organizations = nil
		card.Titles = nil
	}
	if !sel.Has(SectionAnniversaries) {
		card.Anniversaries = nil
	}
	if !sel.Has(SectionMedia) {
		card.Media = nil
	}
	if !sel.Has(SectionOnlineServices) {
		card.ImppAddresses = nil
		card.SocialProfiles = nil
		card.OtherOnlineServices = nil
	}
	if !sel.Has(SectionLinks) {
		card.Links = nil
		card.ContactURIs = nil
	}
	if !sel.Has(SectionNotes) {
		card.Notes = nil
	}
	if !sel.Has(SectionKeywords) {
		card.Keywords = nil
	}
	if !sel.Has(SectionRelatedTo) {
		card.RelatedTo = nil
	}
	if !sel.Has(SectionPersonalInfo) {
		card.PersonalInfo = nil
	}
	if !sel.Has(SectionSpeakToAs) {
		card.SpeakToAs = nil
	}
	if !sel.Has(SectionMembers) {
		card.Members = nil
	}
	if !sel.Has(SectionLanguages) {
		card.PreferredLanguages = nil
	}
	if !sel.Has(SectionCustomFields) {
		passthrough.VCard = nil
		passthrough.JSContact = nil
	}

	return &contactmodel.Record{
		Card:        card,
		Envelope:    record.Envelope,
		Passthrough: passthrough,
		UID:         record.UID,
		ETag:        record.ETag,
	}
}

// FilterSensitiveAddresses returns addrs with every above-normal entry
// removed. It is the address-only half of ApplyFieldSelection, exported for
// callers that build an owner-facing record but still need this one
// outward-copy guarantee — currently the MCP get_contact tool with
// include_sensitive=false (the record it builds is otherwise the owner's own
// view, which keeps addresses so a full-overwrite edit cannot drop them).
func FilterSensitiveAddresses(addrs []contactmodel.Address) []contactmodel.Address {
	return filterSensitiveAddresses(addrs)
}

// filterSensitiveAddresses returns a copy of addrs with every entry whose
// Sensitivity is above "normal" removed. It backs the address half of
// ApplyFieldSelection's default-deny: an ADR 0031 private/secret address —
// including its geo: coordinate — must not leave the instance through the
// neutral-Card exports/shares unless the caller passed
// ?include_sensitive=true, exactly like the relationship-edge, hobby and
// custom-field filters. Returns the input unchanged (never a fresh slice) when
// there is nothing to drop, so the common case stays allocation-free. The
// caller's slice is never mutated (CLAUDE.md trap #3: read projections do not
// write back into Contact.Card).
func filterSensitiveAddresses(addrs []contactmodel.Address) []contactmodel.Address {
	drop := 0
	for _, a := range addrs {
		if a.Sensitivity != "" && a.Sensitivity != RelationshipSensitivityNormal {
			drop++
		}
	}
	if drop == 0 {
		return addrs
	}
	out := make([]contactmodel.Address, 0, len(addrs)-drop)
	for _, a := range addrs {
		if a.Sensitivity != "" && a.Sensitivity != RelationshipSensitivityNormal {
			continue
		}
		out = append(out, a)
	}
	return out
}
