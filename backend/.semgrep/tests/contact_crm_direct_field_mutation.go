// Hand-verification snippets for the
// mycorrhizal-contact-crm-direct-field-mutation rule (issue #370).
package tests

func directMutationIsCaught(contact *Contact, edgeType string, isSource bool) {
	if edgeType == "owned_by" && isSource {
		// ruleid: mycorrhizal-contact-crm-direct-field-mutation
		contact.CRM.Kind = "animal"
	}
}

func readIsClean(contact *Contact) bool {
	// ok: mycorrhizal-contact-crm-direct-field-mutation
	return contact.CRM.Kind == "animal"
}

func viaApplyRecordToContactIsClean(contact *Contact, photoDir string) {
	record := RecordFromContact(contact, photoDir)
	// ok: mycorrhizal-contact-crm-direct-field-mutation
	record.Envelope.Kind = "animal"
	ApplyRecordToContact(contact, record, "")
}

func wholeEnvelopeAssignIsClean(contact *Contact, record *Record) {
	// ok: mycorrhizal-contact-crm-direct-field-mutation
	contact.CRM = record.Envelope
}
