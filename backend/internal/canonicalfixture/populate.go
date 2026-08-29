package canonicalfixture

import (
	"fmt"

	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Dataset is what Populate returns: the created user, every contact keyed by
// its manifest name, and every created entity — including tombstoned ones
// (their DeletedAt is set in place, so an Unscoped() read is not needed to
// inspect the dataset itself). Consumers that want a fresh read should query
// db with their own scoping.
type Dataset struct {
	User               models.User
	Contacts           map[string]models.Contact
	Notes              []models.Note
	LifeEvents         []models.LifeEvent
	Gifts              []models.Gift
	Relationships      []models.RelationshipEdge
	Households         []models.Household
	Circles            []models.Circle
	Tags               []models.Tag
	FieldDefinitions   []models.FieldDefinition
	Preferences        []models.Preference
	ExternalIdentities []models.ExternalIdentity
	Attachments        []models.Attachment
	Activities         []models.Activity
}

// Populate loads the manifest's dataset into db, which MUST be a real migrated
// schema (database.InitDB / internal/dbtest — CLAUDE.md backend trap #1), and
// returns the created rows. Contacts are created through ApplyRecordToContact
// (trap #2), exactly like the REST API; every entity is scoped to one freshly
// created user. A soft-deleted contact is tombstoned the way DeleteContact
// tombstones one: its dependent user-authored content is soft-deleted and its
// join rows are hard-deleted, so the resulting database is the realistic
// post-delete state MIG/DATA suites want to migrate and read.
//
// All work runs in a single transaction; on any error nothing is persisted.
func Populate(db *gorm.DB, m *Manifest) (*Dataset, error) {
	if m == nil {
		return nil, fmt.Errorf("canonicalfixture: nil manifest")
	}
	var ds *Dataset
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		ds, err = populate(tx, m)
		return err
	})
	if err != nil {
		return nil, err
	}
	return ds, nil
}

func populate(db *gorm.DB, m *Manifest) (*Dataset, error) {
	ds := &Dataset{Contacts: map[string]models.Contact{}}

	user := models.User{
		Username: m.User.Username,
		// The password is never validated by the loader (validation lives in
		// the middleware layer the loader bypasses); it is stored verbatim so
		// the fixture user is a usable login for suites that hit the HTTP API.
		Password: "fixture-password-1!",
		Email:    m.User.Email,
	}
	if err := db.Create(&user).Error; err != nil {
		return nil, fmt.Errorf("canonicalfixture: creating user: %w", err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
	}
	ds.User = user

	softDeleted := map[string]bool{}
	for _, entry := range m.Contacts {
		if entry.SoftDeleted {
			softDeleted[entry.Name] = true
		}
	}

	for _, entry := range m.Contacts {
		contact, err := createContact(db, user.ID, entry, ds, softDeleted)
		if err != nil {
			return nil, fmt.Errorf("canonicalfixture: creating contact %q: %w", entry.Name, err)
		}
		ds.Contacts[entry.Name] = contact
		if entry.SoftDeleted {
			// Tombstone the contact row immediately (phase A) so a later
			// contact that recreates this vcard_uid can be created; the
			// dependent-row cascade runs once every section is populated
			// (phase B, cascadeContact below).
			if err := db.Delete(&contact).Error; err != nil {
				return nil, fmt.Errorf("canonicalfixture: tombstoning contact %q: %w", entry.Name, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
			}
			ds.Contacts[entry.Name] = contact
		}
	}

	uidOf := func(section, name string) (string, error) {
		c, ok := ds.Contacts[name]
		if !ok {
			return "", fmt.Errorf("canonicalfixture: %s references unknown contact %q", section, name)
		}
		return c.VCardUID, nil
	}

	for _, entry := range m.Notes {
		contact, ok := ds.Contacts[entry.Contact]
		if !ok {
			return nil, fmt.Errorf("canonicalfixture: note references unknown contact %q", entry.Contact)
		}
		note := models.Note{
			UserID:    user.ID,
			Content:   entry.Content,
			Date:      entry.Date,
			ContactID: &contact.ID,
		}
		if err := db.Create(&note).Error; err != nil {
			return nil, fmt.Errorf("canonicalfixture: creating note for %q: %w", entry.Contact, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
		}
		if entry.SoftDeleted {
			if err := db.Delete(&note).Error; err != nil {
				return nil, fmt.Errorf("canonicalfixture: tombstoning note for %q: %w", entry.Contact, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
			}
		}
		ds.Notes = append(ds.Notes, note)
	}

	for _, entry := range m.LifeEvents {
		uid, err := uidOf("life_event", entry.Contact)
		if err != nil {
			return nil, err
		}
		evt := models.LifeEvent{
			UserID:           user.ID,
			EntityID:         uid,
			Type:             entry.Type,
			Category:         entry.Category,
			Date:             entry.Date,
			Description:      entry.Description,
			Remind:           entry.Remind,
			Source:           entry.Source,
			RelatedEntityIDs: nil,
		}
		for _, name := range entry.RelatedEntities {
			relatedUID, err := uidOf("life_event related_entity", name)
			if err != nil {
				return nil, err
			}
			evt.RelatedEntityIDs = append(evt.RelatedEntityIDs, relatedUID)
		}
		if err := db.Create(&evt).Error; err != nil {
			return nil, fmt.Errorf("canonicalfixture: creating life event for %q: %w", entry.Contact, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
		}
		ds.LifeEvents = append(ds.LifeEvents, evt)
	}

	for _, entry := range m.Gifts {
		uid, err := uidOf("gift", entry.Contact)
		if err != nil {
			return nil, err
		}
		gift := models.Gift{
			UserID:      user.ID,
			EntityID:    uid,
			Status:      entry.Status,
			Occasion:    entry.Occasion,
			Description: entry.Description,
			URL:         entry.URL,
			Notes:       entry.Notes,
			Date:        entry.Date,
			ValueCents:  entry.ValueCents,
			Currency:    entry.Currency,
		}
		if err := db.Create(&gift).Error; err != nil {
			return nil, fmt.Errorf("canonicalfixture: creating gift for %q: %w", entry.Contact, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
		}
		if entry.SoftDeleted {
			if err := db.Delete(&gift).Error; err != nil {
				return nil, fmt.Errorf("canonicalfixture: tombstoning gift for %q: %w", entry.Contact, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
			}
		}
		ds.Gifts = append(ds.Gifts, gift)
	}

	for _, entry := range m.Relationships {
		sourceUID, err := uidOf("relationship source", entry.Source)
		if err != nil {
			return nil, err
		}
		targetUID, err := uidOf("relationship target", entry.Target)
		if err != nil {
			return nil, err
		}
		status := entry.Status
		if status == "" {
			status = models.RelationshipStatusConfirmed
		}
		sensitivity := entry.Sensitivity
		if sensitivity == "" {
			sensitivity = models.RelationshipSensitivityNormal
		}
		provenance := entry.Provenance
		if provenance == "" {
			provenance = models.RelationshipSourceUserConfirmed
		}
		edge := models.RelationshipEdge{
			UserID:      user.ID,
			SourceID:    sourceUID,
			TargetID:    targetUID,
			Type:        entry.Type,
			Directional: entry.Directional,
			Metadata:    entry.Metadata,
			Source:      provenance,
			Confidence:  entry.Confidence,
			Status:      status,
			Sensitivity: sensitivity,
		}
		if err := db.Create(&edge).Error; err != nil {
			return nil, fmt.Errorf("canonicalfixture: creating relationship %q->%q: %w", entry.Source, entry.Target, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
		}
		ds.Relationships = append(ds.Relationships, edge)
	}

	for _, entry := range m.Households {
		household := models.Household{
			UserID:  user.ID,
			Name:    entry.Name,
			Type:    entry.Type,
			Address: entry.Address,
		}
		if err := db.Create(&household).Error; err != nil {
			return nil, fmt.Errorf("canonicalfixture: creating household %q: %w", entry.Name, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
		}
		for _, member := range entry.Members {
			uid, err := uidOf("household member", member.Contact)
			if err != nil {
				return nil, err
			}
			m := models.HouseholdMember{
				HouseholdID:    household.ID,
				UserID:         user.ID,
				MemberVCardUID: uid,
				Role:           member.Role,
				Since:          member.Since,
				Until:          member.Until,
			}
			if err := db.Create(&m).Error; err != nil {
				return nil, fmt.Errorf("canonicalfixture: adding %q to household %q: %w", member.Contact, entry.Name, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
			}
		}
		ds.Households = append(ds.Households, household)
	}

	for _, entry := range m.Circles {
		circle := models.Circle{UserID: user.ID, Name: entry.Name}
		if err := db.Create(&circle).Error; err != nil {
			return nil, fmt.Errorf("canonicalfixture: creating circle %q: %w", entry.Name, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
		}
		for _, member := range entry.Members {
			uid, err := uidOf("circle member", member)
			if err != nil {
				return nil, err
			}
			m := models.CircleMember{CircleID: circle.ID, UserID: user.ID, MemberVCardUID: uid}
			if err := db.Create(&m).Error; err != nil {
				return nil, fmt.Errorf("canonicalfixture: adding %q to circle %q: %w", member, entry.Name, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
			}
		}
		ds.Circles = append(ds.Circles, circle)
	}

	for _, entry := range m.Tags {
		tag := models.Tag{UserID: user.ID, Name: entry.Name}
		if err := db.Create(&tag).Error; err != nil {
			return nil, fmt.Errorf("canonicalfixture: creating tag %q: %w", entry.Name, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
		}
		for _, contactName := range entry.Contacts {
			uid, err := uidOf("tag contact", contactName)
			if err != nil {
				return nil, err
			}
			t := models.ContactTag{TagID: tag.ID, UserID: user.ID, ContactVCardUID: uid}
			if err := db.Create(&t).Error; err != nil {
				return nil, fmt.Errorf("canonicalfixture: tagging %q with %q: %w", contactName, entry.Name, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
			}
		}
		ds.Tags = append(ds.Tags, tag)
	}

	for _, entry := range m.CustomFields {
		def := models.FieldDefinition{
			UserID:      user.ID,
			Label:       entry.Label,
			Key:         entry.Key,
			Target:      models.FieldDefinitionTargetContact,
			Type:        entry.Type,
			Constraints: entry.Constraints,
			Projection:  entry.Projection,
			Sensitivity: entry.Sensitivity,
		}
		if err := db.Create(&def).Error; err != nil {
			return nil, fmt.Errorf("canonicalfixture: creating custom field %q: %w", entry.Key, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
		}
		for _, v := range entry.Values {
			uid, err := uidOf("custom field value", v.Contact)
			if err != nil {
				return nil, err
			}
			fv := models.FieldValue{
				FieldDefinitionID: def.ID,
				UserID:            user.ID,
				EntityID:          uid,
				Value:             v.Value,
			}
			if err := db.Create(&fv).Error; err != nil {
				return nil, fmt.Errorf("canonicalfixture: setting custom field %q on %q: %w", entry.Key, v.Contact, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
			}
		}
		ds.FieldDefinitions = append(ds.FieldDefinitions, def)
	}

	for _, entry := range m.Preferences {
		uid, err := uidOf("preference", entry.Contact)
		if err != nil {
			return nil, err
		}
		pref := models.Preference{
			UserID:      user.ID,
			EntityID:    uid,
			Category:    entry.Category,
			Key:         entry.Key,
			Value:       entry.Value,
			Notes:       entry.Notes,
			Source:      entry.Source,
			Confidence:  entry.Confidence,
			Sensitivity: entry.Sensitivity,
		}
		if err := db.Create(&pref).Error; err != nil {
			return nil, fmt.Errorf("canonicalfixture: creating preference for %q: %w", entry.Contact, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
		}
		if entry.SoftDeleted {
			if err := db.Delete(&pref).Error; err != nil {
				return nil, fmt.Errorf("canonicalfixture: tombstoning preference for %q: %w", entry.Contact, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
			}
		}
		ds.Preferences = append(ds.Preferences, pref)
	}

	for _, entry := range m.ExternalIdentities {
		uid, err := uidOf("external_identity", entry.Contact)
		if err != nil {
			return nil, err
		}
		ident := models.ExternalIdentity{
			UserID:     user.ID,
			EntityID:   uid,
			System:     entry.System,
			ExternalID: entry.ExternalID,
			URL:        entry.URL,
			Metadata:   entry.Metadata,
			SyncStatus: entry.SyncStatus,
		}
		if err := db.Create(&ident).Error; err != nil {
			return nil, fmt.Errorf("canonicalfixture: creating external identity for %q: %w", entry.Contact, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
		}
		ds.ExternalIdentities = append(ds.ExternalIdentities, ident)
	}

	for _, entry := range m.Attachments {
		uid, err := uidOf("attachment", entry.Contact)
		if err != nil {
			return nil, err
		}
		att := models.Attachment{
			UserID:          user.ID,
			ContactVCardUID: uid,
			StoredName:      entry.StoredName,
			OriginalName:    entry.OriginalName,
			ContentType:     entry.ContentType,
			SizeBytes:       entry.SizeBytes,
		}
		if err := db.Create(&att).Error; err != nil {
			return nil, fmt.Errorf("canonicalfixture: creating attachment for %q: %w", entry.Contact, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
		}
		if entry.SoftDeleted {
			if err := db.Delete(&att).Error; err != nil {
				return nil, fmt.Errorf("canonicalfixture: tombstoning attachment for %q: %w", entry.Contact, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
			}
		}
		ds.Attachments = append(ds.Attachments, att)
	}

	for _, entry := range m.Activities {
		activity := models.Activity{
			UserID:      user.ID,
			Title:       entry.Title,
			Description: entry.Description,
			Location:    entry.Location,
			Date:        entry.Date,
			Type:        entry.Type,
			ExternalRef: entry.ExternalRef,
		}
		if err := db.Create(&activity).Error; err != nil {
			return nil, fmt.Errorf("canonicalfixture: creating activity %q: %w", entry.Title, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
		}
		var contacts []models.Contact
		for _, name := range entry.Contacts {
			contact, ok := ds.Contacts[name]
			if !ok {
				return nil, fmt.Errorf("canonicalfixture: activity %q references unknown contact %q", entry.Title, name)
			}
			contacts = append(contacts, contact)
		}
		if len(contacts) > 0 {
			if err := db.Model(&activity).Association("Contacts").Replace(contacts); err != nil {
				return nil, fmt.Errorf("canonicalfixture: linking contacts to activity %q: %w", entry.Title, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
			}
		}
		if entry.SoftDeleted {
			if err := db.Delete(&activity).Error; err != nil {
				return nil, fmt.Errorf("canonicalfixture: tombstoning activity %q: %w", entry.Title, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
			}
		}
		ds.Activities = append(ds.Activities, activity)
	}

	// Phase B: cascade every soft-deleted contact's dependent rows exactly the
	// way DeleteContact's deleteContactAssociations does — user-authored
	// content is soft-deleted (the undo button), join/edge rows are hard
	// deleted (the client re-pulls them). This is the realistic post-delete
	// state the manifest's soft_deleted contacts exist to pin.
	for name := range softDeleted {
		contact := ds.Contacts[name]
		if err := cascadeContact(db, user.ID, contact); err != nil {
			return nil, fmt.Errorf("canonicalfixture: cascading soft-deleted contact %q: %w", name, err) // # pragma: no cover — a freshly-migrated DB accepts every manifest row; failure here means a broken invariant
		}
	}

	return ds, nil
}

// createContact builds and persists one manifest contact through
// ApplyRecordToContact (trap #2 — never direct field mutation). A contact
// with RecreatesVCardUIDOf inherits the tombstoned contact's vcard_uid, which
// pins the partial unique index idx_contacts_vcard_uid_user.
func createContact(db *gorm.DB, userID uint, entry ContactEntry, ds *Dataset, softDeleted map[string]bool) (models.Contact, error) {
	record := entry.Record()
	if entry.RecreatesVCardUIDOf != "" {
		prev, ok := ds.Contacts[entry.RecreatesVCardUIDOf]
		if !ok || !softDeleted[entry.RecreatesVCardUIDOf] {
			return models.Contact{}, fmt.Errorf("recreates_vcard_uid_of %q must name an earlier soft-deleted contact", entry.RecreatesVCardUIDOf)
		}
		record.Card.UID = prev.VCardUID
	}

	contact := models.Contact{UserID: userID}
	models.ApplyRecordToContact(&contact, record, "")
	if err := db.Create(&contact).Error; err != nil {
		return models.Contact{}, err // # pragma: no cover — a DELETE over an intact schema cannot fail; failure here means a broken invariant
	}
	return contact, nil
}

// cascadeContact mirrors controllers.deleteContactAssociations for the entity
// types the manifest can create: soft-delete the user-authored content
// (Note/LifeEvent/Preference/Gift/Attachment), hard-delete the join- and
// edge-shaped rows (RelationshipEdge/HouseholdMember/CircleMember/ContactTag/
// FieldValue/ExternalIdentity + activity_contacts). The contact row itself is
// already tombstoned (phase A). Not a wholesale copy of the controller — the
// manifest cannot create every entity in the controller's list, so this covers
// exactly the rows the loader can have created.
func cascadeContact(db *gorm.DB, userID uint, contact models.Contact) error {
	uid := contact.VCardUID

	if err := db.Where("contact_id = ? AND user_id = ?", contact.ID, userID).Delete(&models.Note{}).Error; err != nil {
		return err // # pragma: no cover — a DELETE over an intact schema cannot fail; failure here means a broken invariant
	}
	if err := db.Exec(
		"DELETE FROM activity_contacts WHERE contact_id = ? AND activity_id IN (SELECT id FROM activities WHERE user_id = ?)",
		contact.ID, userID,
	).Error; err != nil {
		return err // # pragma: no cover — a DELETE over an intact schema cannot fail; failure here means a broken invariant
	}
	if err := db.Where("(source_id = ? OR target_id = ?) AND user_id = ?", uid, uid, userID).Delete(&models.RelationshipEdge{}).Error; err != nil {
		return err // # pragma: no cover — a DELETE over an intact schema cannot fail; failure here means a broken invariant
	}
	if err := db.Where("member_vcard_uid = ? AND user_id = ?", uid, userID).Delete(&models.HouseholdMember{}).Error; err != nil {
		return err // # pragma: no cover — a DELETE over an intact schema cannot fail; failure here means a broken invariant
	}
	if err := db.Where("member_vcard_uid = ? AND user_id = ?", uid, userID).Delete(&models.CircleMember{}).Error; err != nil {
		return err // # pragma: no cover — a DELETE over an intact schema cannot fail; failure here means a broken invariant
	}
	if err := db.Where("contact_vcard_uid = ? AND user_id = ?", uid, userID).Delete(&models.ContactTag{}).Error; err != nil {
		return err // # pragma: no cover — a DELETE over an intact schema cannot fail; failure here means a broken invariant
	}
	if err := db.Where("entity_id = ? AND user_id = ?", uid, userID).Delete(&models.LifeEvent{}).Error; err != nil {
		return err // # pragma: no cover — a DELETE over an intact schema cannot fail; failure here means a broken invariant
	}
	if err := db.Where("entity_id = ? AND user_id = ?", uid, userID).Delete(&models.Preference{}).Error; err != nil {
		return err // # pragma: no cover — a DELETE over an intact schema cannot fail; failure here means a broken invariant
	}
	if err := db.Where("entity_id = ? AND user_id = ?", uid, userID).Delete(&models.FieldValue{}).Error; err != nil {
		return err // # pragma: no cover — a DELETE over an intact schema cannot fail; failure here means a broken invariant
	}
	if err := db.Where("entity_id = ? AND user_id = ?", uid, userID).Delete(&models.ExternalIdentity{}).Error; err != nil {
		return err // # pragma: no cover — a DELETE over an intact schema cannot fail; failure here means a broken invariant
	}
	if err := db.Where("entity_id = ? AND user_id = ?", uid, userID).Delete(&models.Gift{}).Error; err != nil {
		return err // # pragma: no cover — a DELETE over an intact schema cannot fail; failure here means a broken invariant
	}
	if err := db.Where("contact_vcard_uid = ? AND user_id = ?", uid, userID).Delete(&models.Attachment{}).Error; err != nil {
		return err // # pragma: no cover — a DELETE over an intact schema cannot fail; failure here means a broken invariant
	}
	return nil
}
