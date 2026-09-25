package a

import (
	"gorm.io/gorm"
)

type Contact struct{ ID uint }

// A look-alike type in another package must not be flagged.
type FakeDB struct{}

func (f *FakeDB) Save(v interface{}) *FakeDB { return f }

func positives(db *gorm.DB, c *Contact) {
	db.Save(c)                                  // want `result of \(\*gorm.DB\).Save is discarded`
	db.Create(c)                                // want `result of \(\*gorm.DB\).Create is discarded`
	db.Where("id = ?", 1).Delete(&Contact{})    // want `result of \(\*gorm.DB\).Delete is discarded`
	db.Model(c).Updates(map[string]any{"a": 1}) // want `result of \(\*gorm.DB\).Updates is discarded`
	db.Model(c).Update("a", 1)                  // want `result of \(\*gorm.DB\).Update is discarded`
	db.Model(c).UpdateColumn("a", 1)            // want `result of \(\*gorm.DB\).UpdateColumn is discarded`
	db.Exec("DELETE FROM contacts")             // want `result of \(\*gorm.DB\).Exec is discarded`
	db.First(c, 1)                              // want `result of \(\*gorm.DB\).First is discarded`
	db.Find(&[]Contact{})                       // want `result of \(\*gorm.DB\).Find is discarded`
	var n int64
	db.Model(c).Count(&n)       // want `result of \(\*gorm.DB\).Count is discarded`
	db.Raw("SELECT 1").Scan(&n) // want `result of \(\*gorm.DB\).Scan is discarded`
	db.Model(c).Pluck("id", &n) // want `result of \(\*gorm.DB\).Pluck is discarded`
	db.FirstOrCreate(c)         // want `result of \(\*gorm.DB\).FirstOrCreate is discarded`
	db.Unscoped().Delete(c)     // want `result of \(\*gorm.DB\).Delete is discarded`
	(db.Save(c))                // want `result of \(\*gorm.DB\).Save is discarded`
	_ = db.Save(c)              // want `blank-assigned result of \(\*gorm.DB\).Save is discarded`
	defer db.Delete(c)          // want `deferred result of \(\*gorm.DB\).Delete is discarded`
	go db.Exec("VACUUM")        // want `go result of \(\*gorm.DB\).Exec is discarded`
	tx := db.Begin()
	tx.Commit() // want `result of \(\*gorm.DB\).Commit is discarded`

	_ = db.Transaction(func(tx *gorm.DB) error {
		tx.Save(c) // want `result of \(\*gorm.DB\).Save is discarded`
		return nil
	})
}

func negatives(db *gorm.DB, c *Contact, f *FakeDB) error {
	if err := db.Save(c).Error; err != nil {
		return err
	}
	res := db.Where("id = ?", 1).Delete(&Contact{})
	if res.Error != nil {
		return res.Error
	}
	if db.Model(c).Updates(map[string]any{"a": 1}).RowsAffected == 0 {
		return nil
	}
	// An explicit discard of .Error is a visible, reviewable decision.
	_ = db.Create(c).Error
	// Builders and Rollback run nothing worth checking on their own.
	db.Where("id = ?", 1)
	tx := db.Begin()
	tx.Rollback()
	defer tx.Rollback()
	// Returning / passing the *gorm.DB on hands the check to the caller.
	use(db.First(c))
	// Look-alike method on a non-gorm type.
	f.Save(c)
	return db.Save(c).Error
}

func use(*gorm.DB) {}
