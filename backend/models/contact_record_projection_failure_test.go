package models

import (
	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The three join-table projections degrade to the caller's existing values
// when their lookup fails, rather than dropping them or failing the whole
// record build. These paths used to be exercised only by accident — the old
// AutoMigrate test schemas simply lacked the tables — so they're pinned
// explicitly here against the real schema with the table hidden.

func TestProjectTags_LookupFailureKeepsExisting(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	dbtest.HideTable(t, db, "contact_tags")

	got := projectTags(db, "any-uid", []string{"passthrough-kw"})
	assert.Equal(t, []string{"passthrough-kw"}, got)
}

func TestProjectPreferences_LookupFailureKeepsExisting(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	dbtest.HideTable(t, db, "preferences")

	existing := []contactmodel.PersonalInfo{{Kind: "hobby", Value: "chess"}}
	got := projectPreferences(db, "any-uid", existing, false)
	assert.Equal(t, existing, got)
}

func TestProjectCustomFields_LookupFailureKeepsExisting(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	dbtest.HideTable(t, db, "field_values")

	existing := []contactmodel.JCardProp{{Name: "x-existing"}}
	got := projectCustomFields(db, "any-uid", existing, false)
	assert.Equal(t, existing, got)
}

// End to end: a failed projection lookup still yields a usable record.
func TestRecordForContact_ProjectionLookupFailureStillBuildsRecord(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	user := User{Username: "tester", Password: "password123!A", Email: "tester@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	dbtest.HideTable(t, db, "contact_tags")
	record := RecordForContact(&contact, "", db)
	require.NotNil(t, record)
	assert.Empty(t, record.Card.Keywords)
}
