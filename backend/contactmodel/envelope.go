package contactmodel

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
	// Reminders/Activities/Relationships remain separate GORM tables keyed by contact ID;
	// they are NOT embedded here.
}
