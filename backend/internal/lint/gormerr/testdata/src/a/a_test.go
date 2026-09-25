package a

import "gorm.io/gorm"

// Test files are out of scope: no diagnostic expected here.
func inTest(db *gorm.DB, c *Contact) {
	db.Save(c)
}
