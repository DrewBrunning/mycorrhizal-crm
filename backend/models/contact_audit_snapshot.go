package models

import (
	"reflect"

	"mycorrhizal/contactmodel"
)

// ContactAuditSnapshot is the shape marshaled as a Contact's audit
// before-snapshot (T82, docs/fork-plan/tickets/126-T82-audit-snapshots-miss-
// nested-contact-data.md).
//
// The embedded Contact carries the flat fields at the top level exactly as
// pre-T82 snapshots did. Card/CRM/Passthrough are added as explicit top-level
// members because the Contact struct's own columns are json:"-" (they exist so
// the REST API serves the nested model through ContactRecordResponse rather
// than leaking the storage shape), which meant a plain json.Marshal(&contact)
// omitted them and the audit trail never captured nested contact data — the
// whole gap this type closes.
//
// The nested Card has photo Media entries stripped before marshaling (see
// auditSnapshot below): the photo is flat-owned (Contact.Photo/PhotoThumbnail
// and the on-disk file, already carried by the flat half of the snapshot), so
// embedding a multi-KB base64 image in every contact-update event would have
// multiplied audit_events growth for the most-updated entity in the product
// by ~100x for photo-bearing contacts. Non-photo media (logo/sound) have no
// flat home and stay in the snapshot. Undo restores the photo from the
// restored flat Photo path via RestoreFullStateFrom.
type ContactAuditSnapshot struct {
	Contact
	Card        contactmodel.Card        `json:"card"`
	CRM         contactmodel.CRMEnvelope `json:"crm"`
	Passthrough contactmodel.Passthrough `json:"passthrough"`
}

// auditSnapshot returns the value auditBeforeSave/auditAfterDelete marshal as
// this Contact's snapshot. It is a method on *Contact (implementing
// models.auditSnapshotProvider), so every snapshot — update and delete —
// captures the nested columns. Card.Media photo entries are stripped per the
// type's doc comment.
func (c *Contact) auditSnapshot() any {
	card := c.Card
	card.Media = stripPhotoMedia(card.Media)
	return ContactAuditSnapshot{
		Contact:     *c,
		Card:        card,
		CRM:         c.CRM,
		Passthrough: c.Passthrough,
	}
}

// auditSnapshot shadows the promoted *Contact method (see the type's embedded
// Contact) so a ContactAuditSnapshot is never mistaken for a Contact by
// redactedJSONForAudit. Without this shadow, the promoted method would make
// *ContactAuditSnapshot satisfy auditSnapshotProvider too, and passing a
// snapshot to redactedJSONForAudit would re-wrap it against the embedded
// Contact's zero Card/CRM/Passthrough — silently dropping the nested columns.
// A snapshot is already the final shape, so the provider path is idempotent.
func (s *ContactAuditSnapshot) auditSnapshot() any {
	return s
}

// HasNested reports whether the snapshot carries the T82 nested columns. Pre-
// T82 events were marshaled by json.Marshal(&Contact), which omitted card/
// crm/passthrough entirely, so json.Unmarshal leaves them zero-valued here —
// this is what separates "old event, nested data never captured" (preserve the
// current Card) from "new event, nested data is authoritative" (full restore)
// without mistaking the former for "the user cleared the data." A live
// contact's Card is never empty (BeforeSave always derives flat-owned
// sub-structures into it), so a non-empty Card unambiguously marks a T82 event.
func (s *ContactAuditSnapshot) HasNested() bool {
	return !reflect.DeepEqual(s.Card, contactmodel.Card{}) ||
		!reflect.DeepEqual(s.CRM, contactmodel.CRMEnvelope{}) ||
		!reflect.DeepEqual(s.Passthrough, contactmodel.Passthrough{})
}

// stripPhotoMedia removes photo entries from a card's Media for the audit
// snapshot (see ContactAuditSnapshot's doc comment). Everything else is kept.
func stripPhotoMedia(media []contactmodel.Resource) []contactmodel.Resource {
	out := make([]contactmodel.Resource, 0, len(media))
	for _, r := range media {
		if r.Kind != "photo" {
			out = append(out, r)
		}
	}
	return out
}

// RestoreFullStateFrom restores a T82 before-snapshot onto dst: the flat
// fields (same set RestoreFlatStateFrom copies) plus the nested
// Card/CRM/Passthrough columns, and marks dst so BeforeSave keeps the restored
// nested columns verbatim rather than re-deriving them from the restored flat
// fields (which would truncate them back to what the lossy flat shape can
// express). This is the full-fidelity half of T82's undo: an event written
// after the capture change is restored exactly, where pre-T82 events —
// RestoreFlatStateFrom only — can preserve-but-not-restore the nested data.
//
// The snapshot's Card had photo media stripped, so the photo is re-derived
// from dst's restored flat Photo/PhotoThumbnail (via RecordFromContact +
// mergeMedia, the same rule BeforeSave's merge uses) and re-attached — the
// restored card stays consistent with the restored Photo path for CardDAV and
// REST readers instead of silently losing the photo. Non-photo media (logo/
// sound) come from the snapshot, where they were preserved.
func (dst *Contact) RestoreFullStateFrom(src *ContactAuditSnapshot) {
	if dst == nil || src == nil {
		return
	}
	dst.RestoreFlatStateFrom(&src.Contact)
	dst.Card = src.Card
	dst.CRM = src.CRM
	dst.Passthrough = src.Passthrough
	fresh := RecordFromContact(dst, DefaultPhotoDir)
	dst.Card.Media = mergeMedia(src.Card.Media, fresh.Card.Media)
	dst.cardSetDirectly = true
}
