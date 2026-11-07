package models_test

import (
	"errors"
	"strings"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #2290: the re-confirmation hooks run inside the sensitivity write's
// transaction. A failure of any one statement they issue must fail the write
// (never leave an item reclassified with the section still served), and must
// never panic.

type nthFailure struct {
	count, target int
	hit           bool
	hitSQL        string
}

func (f *nthFailure) fail(tx *gorm.DB) {
	f.count++
	if f.count == f.target {
		f.hit = true
		f.hitSQL = tx.Statement.SQL.String()
		tx.AddError(errors.New("injected statement failure"))
	}
}

func (f *nthFailure) install(db *gorm.DB) {
	_ = db.Callback().Query().After("gorm:query").Register("rc:fail_query", f.fail)
	_ = db.Callback().Create().After("gorm:create").Register("rc:fail_create", f.fail)
	_ = db.Callback().Update().After("gorm:update").Register("rc:fail_update", f.fail)
	_ = db.Callback().Row().After("gorm:row").Register("rc:fail_row", f.fail)
}

func TestReconfirmHooks_SingleStatementFailuresFailTheWrite(t *testing.T) {
	type world struct {
		db      *gorm.DB
		owner   models.User
		contact models.Contact
		pref    models.Preference
		edge    models.RelationshipEdge
		def     models.FieldDefinition
	}
	build := func(t *testing.T) *world {
		db := dbtest.New(t)
		a, b := seedGrantUsers(t, db)
		w := &world{db: db, owner: a}
		w.contact = models.Contact{UserID: a.ID, Firstname: "Alice"}
		require.NoError(t, db.Create(&w.contact).Error)
		other := models.Contact{UserID: a.ID, Firstname: "Other"}
		require.NoError(t, db.Create(&other).Error)
		g := models.ShareGrant{FromUserID: a.ID, ToUserID: b.ID, EntityType: models.ShareGrantEntityContact, EntityID: w.contact.VCardUID,
			Sections: []string{"addresses", "related_to", "personal_info", "custom_fields"}, Status: models.ShareGrantStatusActive}
		require.NoError(t, db.Create(&g).Error)
		w.pref = models.Preference{UserID: a.ID, EntityID: w.contact.VCardUID, Category: models.PreferenceCategoryHobby, Value: "chess", Sensitivity: "normal"}
		require.NoError(t, db.Create(&w.pref).Error)
		w.edge = models.RelationshipEdge{UserID: a.ID, SourceID: w.contact.VCardUID, TargetID: other.VCardUID, Type: "parent_of",
			Source: models.RelationshipSourceUserConfirmed, Confidence: 1, Status: models.RelationshipStatusConfirmed, Sensitivity: "normal"}
		require.NoError(t, db.Create(&w.edge).Error)
		w.def = models.FieldDefinition{UserID: a.ID, Label: "F", Key: "f", Target: models.FieldDefinitionTargetContact,
			Type: models.FieldTypeString, Projection: "vcard:X-F", Sensitivity: "normal"}
		require.NoError(t, db.Create(&w.def).Error)
		require.NoError(t, db.Model(&w.contact).Updates(map[string]any{"firstname": "Alice"}).Error)
		return w
	}
	writes := map[string]func(w *world) error{
		"preference": func(w *world) error { return w.db.Model(&w.pref).Update("sensitivity", "private").Error },
		"edge":       func(w *world) error { return w.db.Model(&w.edge).Update("sensitivity", "secret").Error },
		"field-def":  func(w *world) error { return w.db.Model(&w.def).Update("sensitivity", "private").Error },
		"address": func(w *world) error {
			var c models.Contact
			if err := w.db.First(&c, w.contact.ID).Error; err != nil {
				return err
			}
			c.Addresses = []models.ContactAddress{{Street: "1 St", City: "X", Sensitivity: "private"}}
			return w.db.Save(&c).Error
		},
	}
	for name, write := range writes {
		t.Run(name, func(t *testing.T) {
			// Baseline: the write succeeds and marks the section.
			w := build(t)
			require.NoError(t, write(w))
			var g models.ShareGrant
			require.NoError(t, w.db.First(&g, "from_user_id = ?", w.owner.ID).Error)
			require.NotEmpty(t, g.ReconfirmSections, "baseline marks a section")

			for n := 1; n < 40; n++ {
				w := build(t)
				inj := &nthFailure{target: n}
				inj.install(w.db)
				var err error
				assert.NotPanics(t, func() { err = write(w) })
				if !inj.hit {
					return
				}
				// The audit trail is deliberately best-effort (an audit failure never
				// rolls back the audited write), so only the hooks' own statements
				// and the write itself must propagate.
				if strings.Contains(inj.hitSQL, "audit_events") {
					continue
				}
				if inj.hitSQL == "" || strings.Contains(inj.hitSQL, "share_grants") || strings.Contains(inj.hitSQL, "sensitivity") ||
					strings.Contains(inj.hitSQL, "addresses") || strings.Contains(inj.hitSQL, "UPDATE") {
					assert.Error(t, err, "statement %d (%q) failing must fail the write, not be swallowed", n, inj.hitSQL)
				}
			}
			t.Fatal("write still issuing statements after 39")
		})
	}
}
