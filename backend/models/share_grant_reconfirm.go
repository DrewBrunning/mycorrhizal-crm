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
	if ownerID == 0 || !IsSensitiveSection(section) {
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
func sensitivityOf(tx *gorm.DB, table, idColumn string, id any) string {
	var s string
	_ = tx.Session(&gorm.Session{NewDB: true}).Table(table).Select("sensitivity").Where(idColumn+" = ?", id).Limit(1).Scan(&s).Error
	return s
}

// captureSensitivity stores the row's pre-write sensitivity on the statement
// instance for the matching After hook. "" means the row is new.
func captureSensitivity(tx *gorm.DB, table string, id any, isNew bool) {
	old := ""
	if !isNew {
		old = sensitivityOf(tx, table, "id", id)
	}
	tx.InstanceSet(shareGrantOldSensitivityKey+table, old)
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
	if !reclassified(oldSensitivity(tx, "relationship_edges"), sensitivityOf(tx, "relationship_edges", "id", r.ID)) {
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
	if !reclassified(oldSensitivity(tx, "preferences"), sensitivityOf(tx, "preferences", "id", p.ID)) {
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
	if !reclassified(oldSensitivity(tx, "field_definitions"), sensitivityOf(tx, "field_definitions", "id", d.ID)) {
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
	captureSensitivity(tx, table, id, id == "" || !rowExists(tx, table, id))
	return nil
}

func rowExists(tx *gorm.DB, table string, id any) bool {
	var n int64
	_ = tx.Session(&gorm.Session{NewDB: true}).Table(table).Where("id = ?", id).Limit(1).Count(&n).Error
	return n > 0
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

// RecordShareGrantAudit writes an audit event on actorID's trail for a share
// grant. The snapshot carries grant fields only, never entity content or
// labels (ADR 0042 decision 11). The accept/decline/expire/reconfirm audit
// operations and per-party trails land with #2291; until its CHECK-rebuild
// migration, reconfirm is recorded as an update.
func RecordShareGrantAudit(tx *gorm.DB, g *ShareGrant, actorID uint, op string) {
	snap, err := json.Marshal(map[string]any{
		"id": g.ID, "entity_type": g.EntityType, "status": g.Status,
		"sections": g.Sections, "reconfirm_sections": g.ReconfirmSections,
		"expires_at": g.ExpiresAt, "revision": g.Revision,
	})
	if err != nil {
		return
	}
	recordAudit(tx, AuditEntityShareGrant, g.ID, op, actorID, string(snap))
}
