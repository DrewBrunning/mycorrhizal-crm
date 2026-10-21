package contactmodel

import "time"

// CRMEnvelope holds Mycorrhizal-specific data that is NOT part of any contact-exchange
// standard. Format adapters MUST ignore it entirely.
type CRMEnvelope struct {
	// Kind is the envelope-side entity kind: human|animal.
	// Distinct from Card.Kind (model.go — the standard vCard/
	// JSContact KIND: individual|group|org|location|application|device,
	// which has no pet/animal value) —, docs/adrs/0001-neutral-hub-and-spoke-contact-model.md Unenforced at the API boundary, same as Card.Kind
	// and every other CRMEnvelope field: unrecognized values are accepted
	// and preserved, not rejected (see controllers/
	// contact_controller_validation_test.go's header for why).
	Kind               string   `json:"kind,omitempty"`
	Circles            []string `json:"circles,omitempty"`
	HowWeMet           string   `json:"how_we_met,omitempty"`
	WorkInformation    string   `json:"work_information,omitempty"`
	ContactInformation string   `json:"contact_information,omitempty"`
	// Gender is the CRM's free-text gender field (issue #515). It is
	// deliberately NOT vCard GENDER or JSContact speakToAs — those are the
	// standardized grammatical-gender/pronoun concepts mapped to
	// Card.SpeakToAs ("gramgender"/"pronouns" rows, docs/specs/
	// rfc6350-baseline.md's closing note), while this is the CRM's own
	// free-text classification with no correspondence-table row. It lives in
	// the envelope so it round-trips through the neutral Record
	// (Record -> Contact -> Record) and is exposed on the REST wire via
	// crm.gender, exactly as the older CRM-only fields above do. It has no
	// Card home, so a vCard/JSContact FILE export drops it by design — the
	// export path reports that loss by name (models.EnvelopeExportLoss-
	// Diagnostics) rather than silently.
	Gender string `json:"gender,omitempty"`
	// Periods are start/end ranges for Card entries (addresses, employers,
	// titles) — user-authored content with no RFC 9553/9554/9555 home, so it
	// lives in the envelope rather than the standards Card
	// (docs/adrs/0025-temporal-periods.md). Keyed by the entry's neutral ID,
	// never by array position. Dropped from file export by design and
	// reported by name (models.EnvelopeExportLossDiagnostics).
	Periods []EntryPeriod `json:"periods,omitempty"`
	// Ambient is the user's explicit "I see them all the time" flag (issue
	// #1733, ADR 0023 amendment). A pointer so a write can say "I have no
	// opinion": a nil (absent) value PRESERVES the stored flag, which is what
	// lets a client built before this field existed (Android rebuilds crm on
	// every save) save a contact without silently clearing it. A non-nil value,
	// true or false, is authoritative. Responses carry it only when true (absent
	// reads as false), so a non-ambient contact's envelope is unchanged. The flat
	// Contact.Ambient column is the store; the format adapters ignore it (CRM
	// data with no standards home).
	Ambient *bool `json:"ambient,omitempty"`
	// TargetCloseness is the user's relationship goal for the contact (issue
	// #1796, ADR 0023 amendment 2026-10-16): immediate|close|casual|distant,
	// absent = no target. Read-only on POST/PUT /contacts: it is written only by
	// PUT/DELETE /contacts/:id/target-closeness. The flat Contact.TargetCloseness
	// column is the store; the format adapters ignore it (no standards home).
	TargetCloseness *string `json:"target_closeness,omitempty"`
	// PersonalInfoMeta is CRM-side metadata for Card.PersonalInfo entries,
	// keyed by the entry's stable ID (issue #1738). Hobbies are canonical on
	// Card.PersonalInfo because that is what CardDAV round-trips; their
	// sensitivity, favorite flag, notes and provenance have no RFC home, so
	// they live here instead of being projected into a Card. An absent entry
	// means normal sensitivity with no metadata. On a write, an ABSENT map
	// preserves the stored one (a client that predates this field rebuilds crm
	// on every save); an explicit empty object clears it. Format adapters
	// ignore the envelope entirely, so this never leaves the instance through
	// a vCard/JSContact export or sync.
	PersonalInfoMeta map[string]PersonalInfoMeta `json:"personalInfoMeta,omitempty"`
	// Reminders/Activities/Relationships remain separate GORM tables keyed by contact ID;
	// they are NOT embedded here.
}

// PersonalInfoMeta is the CRM-only metadata attached to one Card.PersonalInfo
// entry (issue #1738). Every field is optional; the zero value means normal
// sensitivity and no metadata.
type PersonalInfoMeta struct {
	Sensitivity   string     `json:"sensitivity,omitempty"`
	Favorite      bool       `json:"favorite,omitempty"`
	Notes         string     `json:"notes,omitempty"`
	Source        string     `json:"source,omitempty"`
	Confidence    *float64   `json:"confidence,omitempty"`
	LastConfirmed *time.Time `json:"last_confirmed,omitempty"`
}

// NormalizedSensitivity returns the explicit sensitivity token, treating an
// empty value as "normal".
func (m PersonalInfoMeta) NormalizedSensitivity() string {
	if m.Sensitivity == "" {
		return "normal"
	}
	return m.Sensitivity
}
