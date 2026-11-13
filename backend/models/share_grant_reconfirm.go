package models

import (
	"database/sql"
	"encoding/json"
	"time"

	"mycorrhizal/logger"

	"gorm.io/gorm"
)

// Re-confirmation markers (ADR 0042 decision 5, issue #2290).
//
// When an item in a sensitivity-bearing section (models.IsSensitiveSection)
// moves from `normal` to `private`/`secret` on a contact that has a live
// standing-share grant, that section token is added to the grant's
// reconfirm_sections. The read path then withholds the whole section until
// the owner calls POST /share-grants/{id}/reconfirm, so moving the item back
// to `normal` never silently resurfaces it.
//
// The marker is written at the persistence point of each sensitivity-bearing
// item (GORM hooks on the row type), inside the same transaction as the
// sensitivity write, never in a controller: REST, CardDAV pulls, imports and
// merges all reach these rows through GORM saves. The writers covered are:
//
//	addresses      Contact.BeforeSave (Contact.Addresses[].Sensitivity)
//	related_to     RelationshipEdge hooks (Sensitivity, both endpoints)
//	personal_info  Preference hooks (Sensitivity)
//	custom_fields  FieldDefinition hooks (Sensitivity; a vcard: projection)
//	organizations  ContactEntityLink and Entity hooks (organisation kind; ADR 0043)
//	addresses      ContactEntityLink and Entity hooks (place kind; ADR 0043)
//
// The two entity writers cover the projected ORG/ADR entries (issue #2564): a
// link or its entity moving to private/secret withholds the section, and an
// entity change fans out to every contact linked to it.
//
// Each hook is a cheap no-op unless the owner has a live grant on the contact.

const (
	shareGrantOldSensitivityKey = "share_grant:old_sensitivity"
)

// liveShareGrantStatuses are the statuses a marker applies to. `blocked` is
// the sender's "pending" so it is treated the same.
var liveShareGrantStatuses = ShareGrantLiveStatuses

// markShareGrantReconfirm adds section to reconfirm_sections on every live
// contact grant of ownerID that grants section. vcardUID == "" marks every
// such grant of the owner (a field definition applies to all their contacts).
// The match is scoped by from_user_id (trap #5).
func markShareGrantReconfirm(tx *gorm.DB, ownerID uint, vcardUID, section string) error {
	// organizations is not a sensitive section (an own ORG has no sensitivity)
	// but a projected organisation entry does, so it can be re-confirmed too.
	if ownerID == 0 || (!IsSensitiveSection(section) && section != SectionOrganizations) {
		return nil
	}
	db := tx.Session(&gorm.Session{NewDB: true})
	q := db.Table("share_grants").
		Where("from_user_id = ? AND entity_type = ? AND status IN ?", ownerID, ShareGrantEntityContact, liveShareGrantStatuses)
	if vcardUID != "" {
		q = q.Where("entity_id = ?", vcardUID)
	}
	var grants []ShareGrant
	if err := q.Find(&grants).Error; err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, g := range grants {
		if !containsString(g.Sections, section) || containsString(g.ReconfirmSections, section) {
			continue
		}
		next, err := json.Marshal(append(append([]string{}, g.ReconfirmSections...), section))
		if err != nil {
			return err
		}
		res := db.Table("share_grants").Where("id = ? AND from_user_id = ?", g.ID, ownerID).
			UpdateColumns(map[string]any{
				"reconfirm_sections": string(next),
				"updated_at":         now,
				"revision":           gorm.Expr("revision + 1"),
			})
		if res.Error != nil {
			return res.Error
		}
		logger.Info().Str("share_grant_id", g.ID).Str("section", section).Msg("share grant section withheld pending re-confirmation")
	}
	return nil
}

// hasLiveContactGrant reports whether ownerID has a live contact grant on
// vcardUID ("" = any contact). It gates every hook so the common no-grant
// path costs one indexed existence query.
func hasLiveContactGrant(tx *gorm.DB, ownerID uint, vcardUID string) (bool, error) {
	if ownerID == 0 {
		return false, nil
	}
	q := tx.Session(&gorm.Session{NewDB: true}).Table("share_grants").
		Where("from_user_id = ? AND entity_type = ? AND status IN ?", ownerID, ShareGrantEntityContact, liveShareGrantStatuses)
	if vcardUID != "" {
		q = q.Where("entity_id = ?", vcardUID)
	}
	var n int64
	if err := q.Limit(1).Count(&n).Error; err != nil {
		// A failed lookup must fail the write: skipping the marker would leave
		// a reclassified item exposed (and swallow a database error).
		return false, err
	}
	return n > 0, nil
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// sensitivityOf reads the stored sensitivity of one row (empty when absent).
func sensitivityOf(tx *gorm.DB, table, idColumn string, id any) (string, error) {
	var s string
	err := tx.Session(&gorm.Session{NewDB: true}).Table(table).Select("sensitivity").Where(idColumn+" = ?", id).Limit(1).Scan(&s).Error
	return s, err
}

// captureSensitivity stores the row's pre-write sensitivity on the statement
// instance for the matching After hook. "" means the row is new.
func captureSensitivity(tx *gorm.DB, table string, id any, isNew bool) error {
	old := ""
	if !isNew {
		var err error
		if old, err = sensitivityOf(tx, table, "id", id); err != nil {
			return err
		}
	}
	tx.InstanceSet(shareGrantOldSensitivityKey+table, old)
	return nil
}

func oldSensitivity(tx *gorm.DB, table string) string {
	v, _ := tx.InstanceGet(shareGrantOldSensitivityKey + table)
	s, _ := v.(string)
	return s
}

// reclassified reports whether a row moved from normal (or did not exist)
// to a non-normal sensitivity.
func reclassified(oldS, newS string) bool {
	oldNormal := oldS == "" || oldS == RelationshipSensitivityNormal
	return oldNormal && newS != "" && newS != RelationshipSensitivityNormal
}

// --- RelationshipEdge (related_to) ---

func (r *RelationshipEdge) BeforeSave(tx *gorm.DB) error {
	return beforeSensitivityWrite(tx, r.UserID, "relationship_edges", r.ID)
}

func (r *RelationshipEdge) AfterSave(tx *gorm.DB) error {
	if r.ID == "" {
		return nil
	}
	if live, err := hasLiveContactGrant(tx, r.UserID, ""); err != nil || !live {
		return err
	}
	nowS, err := sensitivityOf(tx, "relationship_edges", "id", r.ID)
	if err != nil {
		return err
	}
	if !reclassified(oldSensitivity(tx, "relationship_edges"), nowS) {
		return nil
	}
	// An edge appears in the projection of both endpoints.
	for _, uid := range []string{r.SourceID, r.TargetID} {
		if err := markShareGrantReconfirm(tx, r.UserID, uid, SectionRelatedTo); err != nil {
			return err
		}
	}
	return nil
}

// --- Preference (personal_info) ---

func (p *Preference) BeforeSave(tx *gorm.DB) error {
	return beforeSensitivityWrite(tx, p.UserID, "preferences", p.ID)
}

func (p *Preference) AfterSave(tx *gorm.DB) error {
	if p.ID == "" {
		return nil
	}
	if live, err := hasLiveContactGrant(tx, p.UserID, p.EntityID); err != nil || !live {
		return err
	}
	nowS, err := sensitivityOf(tx, "preferences", "id", p.ID)
	if err != nil {
		return err
	}
	if !reclassified(oldSensitivity(tx, "preferences"), nowS) {
		return nil
	}
	return markShareGrantReconfirm(tx, p.UserID, p.EntityID, SectionPersonalInfo)
}

// --- FieldDefinition (custom_fields) ---

func (d *FieldDefinition) BeforeSave(tx *gorm.DB) error {
	return beforeSensitivityWrite(tx, d.UserID, "field_definitions", d.ID)
}

func (d *FieldDefinition) AfterSave(tx *gorm.DB) error {
	if d.ID == "" {
		return nil
	}
	if live, err := hasLiveContactGrant(tx, d.UserID, ""); err != nil || !live {
		return err
	}
	nowS, err := sensitivityOf(tx, "field_definitions", "id", d.ID)
	if err != nil {
		return err
	}
	if !reclassified(oldSensitivity(tx, "field_definitions"), nowS) {
		return nil
	}
	return markShareGrantReconfirm(tx, d.UserID, "", SectionCustomFields)
}

// beforeSensitivityWrite captures the pre-write sensitivity when the owner has
// a live contact grant; a failed lookup fails the write.
func beforeSensitivityWrite(tx *gorm.DB, ownerID uint, table, id string) error {
	live, err := hasLiveContactGrant(tx, ownerID, "")
	if err != nil || !live {
		return err
	}
	isNew := id == ""
	if !isNew {
		exists, err := rowExists(tx, table, id)
		if err != nil {
			return err
		}
		isNew = !exists
	}
	return captureSensitivity(tx, table, id, isNew)
}

func rowExists(tx *gorm.DB, table string, id any) (bool, error) {
	var n int64
	err := tx.Session(&gorm.Session{NewDB: true}).Table(table).Where("id = ?", id).Limit(1).Count(&n).Error
	return n > 0, err
}

// --- Contact (addresses) ---

// markAddressReclassification is called from Contact.BeforeSave, after the
// derivation has assigned address ids. An address that is non-normal now and
// was normal (or unknown) before moves the contact's addresses section into
// re-confirmation.
func (c *Contact) markAddressReclassification(tx *gorm.DB) error {
	if c.ID == 0 {
		return nil
	}
	// A marker is only ever needed for an address that is non-normal now, so
	// the common save (all addresses normal) costs no extra query.
	anyNonNormal := false
	for _, a := range c.Addresses {
		if a.Sensitivity != "" && a.Sensitivity != RelationshipSensitivityNormal {
			anyNonNormal = true
			break
		}
	}
	if !anyNonNormal {
		return nil
	}
	if live, err := hasLiveContactGrant(tx, c.UserID, c.VCardUID); err != nil || !live {
		return err
	}
	var raw sql.NullString
	if err := tx.Session(&gorm.Session{NewDB: true}).Table("contacts").Select("addresses").Where("id = ?", c.ID).Limit(1).Scan(&raw).Error; err != nil {
		return err
	}
	var old []ContactAddress
	if raw.Valid && raw.String != "" {
		_ = json.Unmarshal([]byte(raw.String), &old)
	}
	oldByID := map[string]string{}
	for _, a := range old {
		if a.ID != "" {
			oldByID[a.ID] = a.Sensitivity
		}
	}
	for _, a := range c.Addresses {
		if a.Sensitivity == "" || a.Sensitivity == RelationshipSensitivityNormal {
			continue
		}
		prev, known := oldByID[a.ID]
		if a.ID == "" || !known || prev == "" || prev == RelationshipSensitivityNormal {
			return markShareGrantReconfirm(tx, c.UserID, c.VCardUID, SectionAddresses)
		}
	}
	return nil
}

// RecordShareGrantAudit writes an audit event on userID's trail for a share
// grant (entity = the grant id). The snapshot carries grant fields only, never
// entity content, ids of the shared entity or labels (ADR 0042 decision 11), so
// the audit trail cannot leak source data to a non-recipient. The internal
// blocked status is written as pending: the sender must not learn of a block
// from their own trail (decision 8), and a blocked grant is never written to
// the recipient's trail at all (the services layer decides the parties).
func RecordShareGrantAudit(tx *gorm.DB, g *ShareGrant, userID uint, op string) {
	status := g.Status
	if status == ShareGrantStatusBlocked {
		status = ShareGrantStatusPending
	}
	snap, err := json.Marshal(map[string]any{
		"id": g.ID, "entity_type": g.EntityType, "status": status,
		"from_user_id": g.FromUserID, "to_user_id": g.ToUserID,
		"sections": g.Sections, "reconfirm_sections": g.ReconfirmSections,
		"expires_at": g.ExpiresAt, "revoked_reason": g.RevokedReason, "revision": g.Revision,
	})
	if err != nil {
		return
	}
	recordAudit(tx, AuditEntityShareGrant, g.ID, op, userID, string(snap))
}

// RecordContactShareAudit writes an audit event on actorID's trail for a
// one-time ContactShare (issue #2294). The snapshot is id and status only:
// never the payload or the contact's name.
func RecordContactShareAudit(tx *gorm.DB, id, status string, actorID uint, op string) {
	snap, _ := json.Marshal(map[string]any{"id": id, "status": status}) // a string map cannot fail to marshal
	recordAudit(tx, AuditEntityContactShare, id, op, actorID, string(snap))
}

// --- ContactEntityLink and Entity (projected organizations / addresses) ---

// entityKindSection is the grant section an entity kind projects into
// (ADR 0043 decision 3); pets never project, so they map to "".
func entityKindSection(kind string) string {
	switch kind {
	case EntityKindOrganisation:
		return SectionOrganizations
	case EntityKindPlace:
		return SectionAddresses
	}
	return ""
}

// captureEntityLinkSensitivity is ContactEntityLink.BeforeSave's half.
func (l *ContactEntityLink) captureSensitivity(tx *gorm.DB) error {
	return beforeSensitivityWrite(tx, l.UserID, "contact_entity_links", l.ID)
}

// markSensitivityReclassification is ContactEntityLink.AfterSave's half: a link
// that moved from normal to private/secret withholds the contact's projected
// section of the link's kind until the owner re-confirms.
func (l *ContactEntityLink) markSensitivityReclassification(tx *gorm.DB) error {
	if l.ID == "" {
		return nil
	}
	if live, err := hasLiveContactGrant(tx, l.UserID, l.ContactVCardUID); err != nil || !live {
		return err
	}
	nowS, err := sensitivityOf(tx, "contact_entity_links", "id", l.ID)
	if err != nil {
		return err
	}
	if !reclassified(oldSensitivity(tx, "contact_entity_links"), nowS) {
		return nil
	}
	kind, uid := l.Kind, l.ContactVCardUID
	if kind == "" || uid == "" { // a partial struct (Model(&ContactEntityLink{ID: x}).Update)
		var row struct {
			Kind            string
			ContactVCardUID string `gorm:"column:contact_vcard_uid"`
		}
		if err := tx.Session(&gorm.Session{NewDB: true}).Table("contact_entity_links").
			Select("kind", "contact_vcard_uid").Where("id = ?", l.ID).Limit(1).Scan(&row).Error; err != nil {
			return err
		}
		kind, uid = row.Kind, row.ContactVCardUID
	}
	section := entityKindSection(kind)
	if section == "" || uid == "" {
		return nil
	}
	return markShareGrantReconfirm(tx, l.UserID, uid, section)
}

// captureSensitivity is Entity.BeforeSave's half.
func (e *Entity) captureSensitivity(tx *gorm.DB) error {
	return beforeSensitivityWrite(tx, e.UserID, "entities", e.ID)
}

// markSensitivityReclassification is Entity.AfterSave's half: an entity that
// moved from normal to private/secret withholds the projected section on every
// contact linked to it.
func (e *Entity) markSensitivityReclassification(tx *gorm.DB) error {
	if e.ID == "" {
		return nil
	}
	if live, err := hasLiveContactGrant(tx, e.UserID, ""); err != nil || !live {
		return err
	}
	nowS, err := sensitivityOf(tx, "entities", "id", e.ID)
	if err != nil {
		return err
	}
	if !reclassified(oldSensitivity(tx, "entities"), nowS) {
		return nil
	}
	kind := e.Kind
	if kind == "" {
		if err := tx.Session(&gorm.Session{NewDB: true}).Table("entities").Select("kind").Where("id = ?", e.ID).Limit(1).Scan(&kind).Error; err != nil {
			return err
		}
	}
	section := entityKindSection(kind)
	if section == "" {
		return nil
	}
	var uids []string
	if err := tx.Session(&gorm.Session{NewDB: true}).Table("contact_entity_links").
		Where("entity_id = ? AND user_id = ? AND deleted_at IS NULL", e.ID, e.UserID).
		Distinct().Pluck("contact_vcard_uid", &uids).Error; err != nil {
		return err
	}
	for _, uid := range uids {
		if err := markShareGrantReconfirm(tx, e.UserID, uid, section); err != nil {
			return err
		}
	}
	return nil
}
