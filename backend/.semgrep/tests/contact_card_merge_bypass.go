// Hand-verification snippets for the mycorrhizal-contact-card-merge-bypass
// rule (issue #370). See the verification command in
// .semgrep/mycorrhizal-traps.yaml's header comment.
package tests

func directAssignBypassesMerge(c *Contact, photoDir string) {
	// ruleid: mycorrhizal-contact-card-merge-bypass
	c.Card = RecordFromContact(c, photoDir).Card
}

func viaLocalVarBypassesMerge(c *Contact, photoDir string) {
	fresh := RecordFromContact(c, photoDir)
	// ruleid: mycorrhizal-contact-card-merge-bypass
	c.Card = fresh.Card
}

func viaMergeIsClean(c *Contact, photoDir string) {
	fresh := RecordFromContact(c, photoDir)
	// ok: mycorrhizal-contact-card-merge-bypass
	merged := mergeRecordFromFlat(Record{Card: c.Card}, *fresh)
	c.Card = merged.Card
}

func freshInMemoryRecordIsClean(photoDir string) {
	contact := &Contact{}
	// ok: mycorrhizal-contact-card-merge-bypass
	record := RecordFromContact(contact, photoDir)
	record.Envelope.Kind = "animal"
	ApplyRecordToContact(contact, record, "")
}
