package services

import (
	"context"
	"fmt"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
)

// countingLogger wraps a gorm logger and counts every traced statement, so a
// test can pin the number of queries a scan issues (T93's bounded-query
// guarantee — see TestFindDuplicatePairs_QueryCountIsBounded).
type countingLogger struct {
	gormLogger.Interface
	count *int
}

func (l countingLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	*l.count++
	l.Interface.Trace(ctx, begin, fc, err)
}

// TestFindDuplicatePairs_QueryCountIsBounded pins T93's "the scan issues a
// bounded number of queries regardless of contact count" guarantee: one query
// per tier (email/name/phone) plus one summary fetch plus one dismissal fetch
// — five total, a constant. An O(n²) regression (DetectDuplicate per contact,
// or a full table load inside a loop) would blow past this by orders of
// magnitude on the 150 contacts seeded here, so the assertion catches it.
func TestFindDuplicatePairs_QueryCountIsBounded(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "dup-count.db")
	db := dbtest.NewAt(t, dbPath)

	user := models.User{Username: "dupcount", Password: "password123!A", Email: "dupcount@example.com"}
	require.NoError(t, db.Create(&user).Error)

	// 150 contacts: 50 sharing one email, 50 sharing one name, 50 sharing one
	// phone key — every tier triggers, so the worst-case scan path runs.
	for i := 0; i < 50; i++ {
		require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Shared", Lastname: "Email", Email: "shared@example.com"}).Error)
	}
	for i := 0; i < 50; i++ {
		require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Same", Lastname: "Name", Email: fmt.Sprintf("solo-%02d@example.com", i)}).Error)
	}
	for i := 0; i < 50; i++ {
		require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Phone", Lastname: "Guy", Phones: []models.ContactPhone{{Value: fmt.Sprintf("+1 800 555 %04d", i)}}}).Error)
	}

	// A second connection to the same file with a counting logger. journal_mode
	// (WAL) is persisted in the file, so this handle reads the seeded data
	// with no special pragmas.
	count := 0
	countDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: countingLogger{Interface: gormLogger.Default, count: &count},
	})
	require.NoError(t, err)

	_, err = FindDuplicatePairs(countDB, user.ID)
	require.NoError(t, err)

	assert.LessOrEqual(t, count, 6, "duplicate scan must issue a constant, small number of queries regardless of contact count")
}

// TestFindDuplicatePairs_NeverPairsContactWithItself pins T113: the phone
// tier must not pair a contact with itself. FlattenPhones emits a duplicate
// token when a contact has two numbers that reduce to the same PhoneKey (e.g.
// "+1 800 555 1234" → digits "18005551234" + key "8005551234" next to
// "800-555-1234" → digits "8005551234" whose key equals its digits), so
// without the DISTINCT guard the scan grouped that one contact's own rows and
// produced a pair whose two sides are the same person — which surfaced in the
// review UI as a same-person "duplicate" whose Merge failed with "merge_id
// must differ from keep_id".
func TestFindDuplicatePairs_NeverPairsContactWithItself(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "dup-selfpair.db")
	db := dbtest.NewAt(t, dbPath)

	user := models.User{Username: "selfpair", Password: "password123!A", Email: "selfpair@example.com"}
	require.NoError(t, db.Create(&user).Error)

	// Self-key: two numbers that reduce to the same last-10 key, so
	// FlattenPhones emits the key token twice for this one contact.
	self := models.Contact{UserID: user.ID, Firstname: "Jordan", Lastname: "Duplicate",
		Phones: []models.ContactPhone{
			{Type: "mobile", Value: "+1 800 555 1234"},
			{Type: "home", Value: "800-555-1234"},
		}}
	require.NoError(t, db.Create(&self).Error)

	// Real duplicate: a genuinely different contact sharing the same key.
	other := models.Contact{UserID: user.ID, Firstname: "Meike", Lastname: "Other",
		Phones: []models.ContactPhone{{Type: "mobile", Value: "(800) 555-1234"}}}
	require.NoError(t, db.Create(&other).Error)

	pairs, err := FindDuplicatePairs(db, user.ID)
	require.NoError(t, err)

	// No pair may be a contact against itself, whatever the tier.
	for _, p := range pairs {
		assert.NotEqual(t, p.A.UID, p.B.UID,
			"duplicate scan must never pair a contact with itself: %s == %s", p.A.UID, p.B.UID)
	}

	// The genuine cross-contact phone match must still be detected.
	var found bool
	for _, p := range pairs {
		aUID, bUID := p.A.UID, p.B.UID
		if (aUID == self.VCardUID && bUID == other.VCardUID) || (aUID == other.VCardUID && bUID == self.VCardUID) {
			found = true
		}
	}
	assert.True(t, found, "shared PhoneKey between two different contacts must still be reported as a duplicate")
}
