package services

import (
	"context"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Tests for the denormalized contact-column rebuild (issue #497). Real
// migrated schema via dbtest (CLAUDE.md backend trap #1).

func derivedRebuildUser(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	u := models.User{Username: "rebuild", Password: "password123!A", Email: "rebuild@example.com"}
	require.NoError(t, db.Create(&u).Error)
	return u
}

// mkRichContact creates a contact through the ordinary write path with a Card
// that populates every denormalized column.
func mkRichContact(t *testing.T, db *gorm.DB, userID uint, given, surname, email string) models.Contact {
	t.Helper()
	c := models.Contact{UserID: userID}
	models.ApplyRecordToContact(&c, &contactmodel.Record{Card: contactmodel.Card{
		Name: &contactmodel.Name{Components: []contactmodel.NameComponent{
			{Kind: "given", Value: given}, {Kind: "surname", Value: surname},
		}},
		Emails: []contactmodel.Email{{Address: email, Label: "work"}},
		Phones: []contactmodel.Phone{{Number: "+15550100199", Label: "cell"}},
		Addresses: []contactmodel.Address{{Components: []contactmodel.AddressComponent{
			{Kind: "name", Value: "1 Elm St"}, {Kind: "locality", Value: "Town"},
		}}},
	}}, "")
	require.NoError(t, db.Create(&c).Error)
	return c
}

func TestRebuildDerivedContactColumns_NoOpOnCleanDB(t *testing.T) {
	db := dbtest.New(t)
	u := derivedRebuildUser(t, db)
	mkRichContact(t, db, u.ID, "Ada", "Lovelace", "ada@example.com")
	mkRichContact(t, db, u.ID, "Bob", "Ross", "bob@example.com")

	stats, err := RebuildDerivedContactColumns(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, int64(2), stats.ContactsScanned)
	assert.Equal(t, int64(0), stats.ContactsUpdated, "a clean DB is a fixpoint: nothing to rewrite")
	assert.Nil(t, stats.ColumnUpdates)
}

func TestRebuildDerivedContactColumns_RepairsDriftAndIsIdempotent(t *testing.T) {
	db := dbtest.New(t)
	u := derivedRebuildUser(t, db)
	c := mkRichContact(t, db, u.ID, "Ada", "Lovelace", "ada@example.com")

	// Simulate a hook-bypassing write that left several columns stale.
	require.NoError(t, db.Exec(`UPDATE contacts
		SET firstname = 'WRONG', fn = 'WRONG', email = 'wrong@x.com',
		    addresses_flat = 'stale', phones_normalized = 'stale', sort_name = 'zzz'
		WHERE id = ?`, c.ID).Error)

	stats, err := RebuildDerivedContactColumns(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.ContactsUpdated)
	for _, col := range []string{"firstname", "fn", "email", "addresses_flat", "phones_normalized", "sort_name"} {
		assert.Equal(t, int64(1), stats.ColumnUpdates[col], "column %s should have been rewritten", col)
	}

	var got models.Contact
	require.NoError(t, db.First(&got, c.ID).Error)
	assert.Equal(t, "Ada", got.Firstname)
	assert.Equal(t, "Ada Lovelace", got.FN)
	assert.Equal(t, "ada@example.com", got.Email)
	assert.Equal(t, "lovelace", got.SortName)
	assert.Empty(t, got.RecomputeDerivedColumns(), "no residual drift after a rebuild")

	// Idempotent: a second pass rewrites nothing.
	stats2, err := RebuildDerivedContactColumns(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats2.ContactsUpdated)
}

// After a rebuild, the INV-A5 consistency probe reports the corpus clean —
// the rebuild converges on exactly the fixpoint the probe checks for.
func TestRebuildDerivedContactColumns_ConvergesToProbeClean(t *testing.T) {
	db := dbtest.New(t)
	u := derivedRebuildUser(t, db)
	for _, name := range []string{"Ada", "Bob", "Cy", "Dee"} {
		c := mkRichContact(t, db, u.ID, name, "Sur", name+"@x.com")
		require.NoError(t, db.Exec(
			`UPDATE contacts SET sort_name = 'x', addresses_flat = 'x', fn = 'x' WHERE id = ?`, c.ID).Error)
	}

	before, err := checkDerivedContactColumns(context.Background(), db, config.Config{})
	require.NoError(t, err)
	require.NotEmpty(t, before, "the corrupted corpus must trip the probe")

	_, err = RebuildDerivedContactColumns(context.Background(), db)
	require.NoError(t, err)

	after, err := checkDerivedContactColumns(context.Background(), db, config.Config{})
	require.NoError(t, err)
	assert.Empty(t, after, "the probe must report clean after a rebuild")
}

func TestRebuildDerivedContactColumns_SkipsUnreadableCardRow(t *testing.T) {
	db := dbtest.New(t)
	u := derivedRebuildUser(t, db)
	bad := mkRichContact(t, db, u.ID, "Cor", "Rupt", "cor@x.com")
	good := mkRichContact(t, db, u.ID, "Ada", "Lovelace", "ada@example.com")
	require.NoError(t, db.Exec("UPDATE contacts SET card = ? WHERE id = ?", "{not json", bad.ID).Error)
	require.NoError(t, db.Exec("UPDATE contacts SET sort_name = 'zzz' WHERE id = ?", good.ID).Error)

	stats, err := RebuildDerivedContactColumns(context.Background(), db)
	require.NoError(t, err, "one unreadable card must not fail the whole rebuild")
	assert.Equal(t, int64(1), stats.ContactsScanned, "the unreadable row is skipped, not scanned")
	assert.Equal(t, int64(1), stats.ContactsUpdated)

	var g models.Contact
	require.NoError(t, db.First(&g, good.ID).Error)
	assert.Equal(t, "lovelace", g.SortName)
}

func TestRebuildDerivedContactColumns_ContextCancelled(t *testing.T) {
	db := dbtest.New(t)
	u := derivedRebuildUser(t, db)
	mkRichContact(t, db, u.ID, "Ada", "Lovelace", "ada@example.com")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RebuildDerivedContactColumns(ctx, db)
	require.ErrorIs(t, err, context.Canceled)
}

func TestRebuildDerivedContactColumnsExclusive_SecondCallSkipped(t *testing.T) {
	db := dbtest.New(t)
	u := derivedRebuildUser(t, db)
	mkRichContact(t, db, u.ID, "Ada", "Lovelace", "ada@example.com")

	derivedColumnsRebuildMu.Lock()
	defer derivedColumnsRebuildMu.Unlock()

	_, err := RebuildDerivedContactColumnsExclusive(context.Background(), db)
	require.ErrorIs(t, err, ErrJobSkipped)
}

// The contacts_fts triggers fire on the rebuild's raw UPDATE, so a repaired
// addresses_flat becomes searchable without a separate search-index rebuild.
func TestRebuildDerivedContactColumns_PropagatesToSearchIndex(t *testing.T) {
	db := dbtest.New(t)
	u := derivedRebuildUser(t, db)
	c := mkRichContact(t, db, u.ID, "Ada", "Lovelace", "ada@example.com")
	require.NoError(t, db.Exec("UPDATE contacts SET addresses_flat = '' WHERE id = ?", c.ID).Error)
	require.NoError(t, db.Exec("UPDATE contacts_fts SET addresses_flat = '' WHERE rowid = ?", c.ID).Error)

	_, err := RebuildDerivedContactColumns(context.Background(), db)
	require.NoError(t, err)

	var indexed string
	require.NoError(t, db.Raw("SELECT addresses_flat FROM contacts_fts WHERE rowid = ?", c.ID).Scan(&indexed).Error)
	assert.Contains(t, indexed, "Elm St", "the FTS index must reflect the rebuilt addresses_flat")
}
