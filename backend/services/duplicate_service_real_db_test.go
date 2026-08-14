package services

import (
	"context"
	"fmt"
	"mycorrhizal/database"
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
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

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
