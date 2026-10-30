package models

import "time"

// Per-contact field visibility (issue #1997).
//
// users.enabled_contact_fields is the account default for which extended
// contact fields the UI shows. ContactFieldOverride lets a user say "show" or
// "hide" for one field on ONE contact, and hide-empty mode suppresses fields
// that carry no value. Everything here is cosmetic display state: it is NOT an
// access boundary (sensitivity is still filtered in the query) and it lives
// outside the neutral Card, in its own owner-scoped table, so it can never
// reach vCard/JSContact, CardDAV/CalDAV, shares or MCP.
//
// Resolution order, most specific wins:
//
//	per-contact override > account default (enabled_contact_fields) > built-in default

// Tri-state values of a field override as the API spells them. "inherit" is
// never stored: it is the absence of a ContactFieldOverride row.
const (
	FieldStateInherit = "inherit"
	FieldStateShow    = "show"
	FieldStateHide    = "hide"
)

// HideEmptyOverrideKey is the reserved field_key under which the per-contact
// hide-empty override is stored in the same table (state "hide" = hide empty
// fields on this contact, "show" = show them even if the account hides them).
// It is deliberately not a contact field key and is rejected in the overrides
// map; the API exposes it as the separate hide_empty member.
const HideEmptyOverrideKey = "__hide_empty"

// ContactFieldKeys is the explicit allowlist of field keys an override may
// name (no mass assignment). It mirrors ContactFieldKey in
// frontend/src/contactFields.ts; TestContactFieldKeysMatchFrontend pins the
// two together.
var ContactFieldKeys = []string{
	"emails", "phones", "addresses", "links", "imppAddresses", "socialProfiles",
	"otherOnlineServices", "nickname", "gender", "birthday", "anniversary",
	"anniversaries", "prefix", "middle_name", "suffix", "organizations", "titles",
	"how_we_met", "work_information", "contact_information", "speakToAs",
	"personalInfo", "keywords", "cardNotes", "preferredLanguages", "cardKind",
	"language",
}

// DefaultEnabledContactFields is the built-in default: what an account that
// never configured enabled_contact_fields sees. It mirrors
// DEFAULT_ENABLED_CONTACT_FIELDS in frontend/src/contactFields.ts.
var DefaultEnabledContactFields = []string{
	"emails", "phones", "addresses", "nickname", "gender", "birthday",
	"speakToAs", "personalInfo", "how_we_met", "work_information",
	"contact_information",
}

// IsContactFieldKey reports whether key is an allowlisted, overridable field.
func IsContactFieldKey(key string) bool {
	for _, k := range ContactFieldKeys {
		if k == key {
			return true
		}
	}
	return false
}

// ContactFieldOverride is one explicit per-contact override: user_id +
// contact_vcard_uid + field_key -> state ("show" or "hide"; inherit is the
// absence of a row). Migration 000084.
//
// uint primary key, hard delete (trap #7): a join-shaped preference row whose
// identity IS its natural key. It is swept by the contact cascade registry,
// carried in the contact's deletion snapshot so trash restore brings it back,
// swept with the user, and exported in the account bundle.
type ContactFieldOverride struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	UserID          uint   `gorm:"not null;index;uniqueIndex:idx_contact_field_override_key,priority:1" json:"-"`
	ContactVCardUID string `gorm:"column:contact_vcard_uid;not null;uniqueIndex:idx_contact_field_override_key,priority:2" json:"contact_vcard_uid"`
	FieldKey        string `gorm:"not null;uniqueIndex:idx_contact_field_override_key,priority:3" json:"field_key"`
	State           string `gorm:"not null" json:"state"`
}

// ContactFieldVisibilityInput is the PUT body: the COMPLETE override set for
// one contact. A field absent from Overrides (or "inherit") has no override;
// an absent HideEmpty means inherit. Keys are checked against ContactFieldKeys
// in the handler so a bad key or state carries a reason code.
type ContactFieldVisibilityInput struct {
	Overrides map[string]string `json:"overrides"`
	HideEmpty string            `json:"hide_empty"`
}

// ResolvedFieldVisibility is a visibility decision for every known field key
// plus the hide-empty flag.
type ResolvedFieldVisibility struct {
	Fields    map[string]bool `json:"fields"`
	HideEmpty bool            `json:"hide_empty"`
}

// ContactFieldVisibilityResponse is the GET/PUT/DELETE body. Overrides holds
// only the explicit ("show"/"hide") rows; HideEmpty is the raw per-contact
// tri-state; Defaults is the account-level decision (account default >
// built-in) the contact inherits; Resolved applies the overrides on top and is
// what a client renders from.
type ContactFieldVisibilityResponse struct {
	Overrides map[string]string       `json:"overrides"`
	HideEmpty string                  `json:"hide_empty"`
	Defaults  ResolvedFieldVisibility `json:"defaults"`
	Resolved  ResolvedFieldVisibility `json:"resolved"`
}

// ResolveFieldVisibility applies the resolution order. accountEnabled is the
// user's enabled_contact_fields (nil = never configured, so the built-in
// default applies); overrides maps field key -> "show"/"hide" and may carry
// HideEmptyOverrideKey. Unknown override keys are ignored: a stale row can
// never widen what a contact shows beyond the allowlist.
func ResolveFieldVisibility(accountEnabled []string, accountHideEmpty bool, overrides map[string]string) (defaults, resolved ResolvedFieldVisibility) {
	base := accountEnabled
	if base == nil {
		base = DefaultEnabledContactFields
	}
	enabled := make(map[string]bool, len(base))
	for _, k := range base {
		enabled[k] = true
	}
	defaults = ResolvedFieldVisibility{Fields: make(map[string]bool, len(ContactFieldKeys)), HideEmpty: accountHideEmpty}
	resolved = ResolvedFieldVisibility{Fields: make(map[string]bool, len(ContactFieldKeys)), HideEmpty: accountHideEmpty}
	for _, k := range ContactFieldKeys {
		defaults.Fields[k] = enabled[k]
		switch overrides[k] {
		case FieldStateShow:
			resolved.Fields[k] = true
		case FieldStateHide:
			resolved.Fields[k] = false
		default:
			resolved.Fields[k] = enabled[k]
		}
	}
	switch overrides[HideEmptyOverrideKey] {
	case FieldStateShow:
		resolved.HideEmpty = false
	case FieldStateHide:
		resolved.HideEmpty = true
	}
	return defaults, resolved
}
