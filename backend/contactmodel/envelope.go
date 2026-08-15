package contactmodel

// CRMEnvelope holds Mycorrhizal-specific data that is NOT part of any contact-exchange
// standard. Format adapters MUST ignore it entirely.
type CRMEnvelope struct {
	// Kind is the envelope-side entity kind: human|animal.
	// Distinct from Card.Kind (model.go — the standard vCard/
	// JSContact KIND: individual|group|org|location|application|device,
	// which has no pet/animal value) — WP-82, docs/adrs/0001-neutral-hub-and-spoke-contact-model.md Unenforced at the API boundary, same as Card.Kind
	// and every other CRMEnvelope field: unrecognized values are accepted
	// and preserved, not rejected (see controllers/
	// contact_controller_validation_test.go's header for why).
	Kind               string   `json:"kind,omitempty"`
	Circles            []string `json:"circles,omitempty"`
	HowWeMet           string   `json:"how_we_met,omitempty"`
	WorkInformation    string   `json:"work_information,omitempty"`
	ContactInformation string   `json:"contact_information,omitempty"`
	// Reminders/Activities/Relationships remain separate GORM tables keyed by contact ID;
	// they are NOT embedded here.
}
