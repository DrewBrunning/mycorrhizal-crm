// Hand-verification snippets for the
// mycorrhizal-gorm-save-updates-error-unchecked rule (issue #370).
package tests

func bareSaveSwallowsError(db *DB, contact *Contact) {
	// ruleid: mycorrhizal-gorm-save-updates-error-unchecked
	db.Save(contact)
}

func bareUpdatesSwallowsError(tx *DB, contact *Contact) {
	// ruleid: mycorrhizal-gorm-save-updates-error-unchecked
	tx.Updates(contact)
}

func checkedSaveIsClean(db *DB, contact *Contact) error {
	// ok: mycorrhizal-gorm-save-updates-error-unchecked
	if err := db.Save(contact).Error; err != nil {
		return err
	}
	return nil
}

func checkedUpdatesIsClean(tx *DB, contact *Contact) error {
	// ok: mycorrhizal-gorm-save-updates-error-unchecked
	return tx.Updates(contact).Error
}

func assignedThenCheckedIsClean(db *DB, contact *Contact) error {
	// ok: mycorrhizal-gorm-save-updates-error-unchecked
	result := db.Save(contact)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func unrelatedSaveIsClean(dir string, data []byte) {
	// ok: mycorrhizal-gorm-save-updates-error-unchecked
	attachments.Save(data, dir)
}
