package models

import (
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Tests for the derived-column recompute (issue #497) — the read-only core
// the rebuild and the INV-A5 consistency probe share. Real migrated schema
// (CLAUDE.md backend trap #1) so the at-rest serializer and DeriveProjection
// run exactly as they do in production.

func mkDerivedContact(t *testing.T, db *gorm.DB) *Contact {
	t.Helper()
	u := User{Username: "derived", Password: "password123!A", Email: "derived@example.com"}
	require.NoError(t, db.Create(&u).Error)
	c := &Contact{UserID: u.ID}
	ApplyRecordToContact(c, richCardOnlyRecord(), "")
	require.NoError(t, db.Create(c).Error)
	var loaded Contact
	require.NoError(t, db.First(&loaded, c.ID).Error)
	return &loaded
}

// A contact last written through the ordinary GORM path is a fixpoint: the
// recompute finds nothing to change.
func TestRecomputeDerivedColumns_FreshContactIsAFixpoint(t *testing.T) {
	db := dbtest.New(t)
	c := mkDerivedContact(t, db)
	require.Empty(t, c.RecomputeDerivedColumns(),
		"a freshly-saved contact's denormalized columns must already match its Card")
}

// Corrupting any one denormalized column — the exact shape of a raw-SQL
// migration or a hook-bypassing bulk import — is detected, named, and paired
// with the value it should hold.
func TestRecomputeDerivedColumns_DetectsEachDrift(t *testing.T) {
	cases := []struct {
		col     string
		set     string // raw value to force into the column
		wantVal string // value RecomputeDerivedColumns should propose
	}{
		{"firstname", "Wrong", "Ada"},
		{"fn", "Wrong", "Ada"},
		{"email", "wrong@example.com", "ada@example.com"},
		{"phone", "+15559999999", "+15550100100"},
		{"addresses_flat", "wrong flat", "123 Main St, PO Box 42, Apt 3B, 4, Springfield"},
		{"sort_name", "zzzz", "ada"},
	}
	for _, tc := range cases {
		t.Run(tc.col, func(t *testing.T) {
			db := dbtest.New(t)
			c := mkDerivedContact(t, db)
			require.NoError(t, db.Exec(
				"UPDATE contacts SET "+tc.col+" = ? WHERE id = ?", tc.set, c.ID).Error)

			var reloaded Contact
			require.NoError(t, db.First(&reloaded, c.ID).Error)

			fixes := reloaded.RecomputeDerivedColumns()
			var got *DerivedColumnFix
			for i := range fixes {
				if fixes[i].Column == tc.col {
					got = &fixes[i]
				}
			}
			require.NotNil(t, got, "expected %s reported divergent, got %+v", tc.col, fixes)
			assert.Equal(t, tc.wantVal, got.Want)
		})
	}
}

// sort_name is compared case-insensitively so migration 000021's ASCII-only
// lower() backfill of a non-ASCII name is not perpetually flagged as drift.
func TestRecomputeDerivedColumns_SortNameCaseFoldedNotFlagged(t *testing.T) {
	db := dbtest.New(t)
	u := User{Username: "sn", Password: "password123!A", Email: "sn@example.com"}
	require.NoError(t, db.Create(&u).Error)
	c := &Contact{UserID: u.ID}
	ApplyRecordToContact(c, &contactmodel.Record{Card: contactmodel.Card{
		Name: &contactmodel.Name{Components: []contactmodel.NameComponent{
			{Kind: "surname", Value: "Öberg"},
		}},
	}}, "")
	require.NoError(t, db.Create(c).Error)

	// What migration 000021's SQLite lower() would have left for "Öberg":
	// the non-ASCII letter's case unchanged.
	require.NoError(t, db.Exec("UPDATE contacts SET sort_name = ? WHERE id = ?", "Öberg", c.ID).Error)

	var reloaded Contact
	require.NoError(t, db.First(&reloaded, c.ID).Error)
	for _, f := range reloaded.RecomputeDerivedColumns() {
		assert.NotEqual(t, "sort_name", f.Column,
			"an ASCII-lower vs Unicode-lower sort_name difference is cosmetic, not drift")
	}

	// A genuinely stale key (wrong name entirely) still shows.
	require.NoError(t, db.Exec("UPDATE contacts SET sort_name = ? WHERE id = ?", "different", c.ID).Error)
	require.NoError(t, db.First(&reloaded, c.ID).Error)
	var flagged bool
	for _, f := range reloaded.RecomputeDerivedColumns() {
		if f.Column == "sort_name" {
			flagged = true
		}
	}
	assert.True(t, flagged, "a sort_name that does not case-fold to the derived key is drift")
}
