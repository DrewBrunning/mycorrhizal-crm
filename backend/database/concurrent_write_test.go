package database

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Concurrent writes must queue, not fail.
//
// The DSN carries busy_timeout(5000), but that alone does not cover GORM's
// write path. A deferred transaction (SQLite's default BEGIN) takes a read
// lock and only tries to upgrade at its first write; the busy handler is not
// invoked for that upgrade, so it fails instantly with SQLITE_BUSY regardless
// of the timeout. GORM wraps even a single Create in an implicit transaction,
// so two concurrent POST /contacts could return 500 in a few milliseconds —
// observed intermittently under the e2e suite's parallel workers.
//
// _txlock=immediate makes transactions take the write lock up front, which is
// a case the busy handler does retry. This test is what pins that: remove
// _txlock=immediate from openDSN and it fails with "database is locked".
func TestConcurrentWritesDoNotReturnSQLiteBusy(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "concurrent.db")

	db, err := InitDB(dbPath)
	require.NoError(t, err)
	defer func() {
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	}()

	user := models.User{Username: "concurrent", Email: "concurrent@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)

	const writers = 8
	const perWriter = 15

	var wg sync.WaitGroup
	errCh := make(chan error, writers*perWriter)

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				contact := models.Contact{
					UserID:    user.ID,
					Firstname: "Concurrent",
					Lastname:  string(rune('A' + worker)),
				}
				if err := db.Create(&contact).Error; err != nil {
					errCh <- err
				}
			}
		}(w)
	}

	wg.Wait()
	close(errCh)

	var busyErrors []string
	var otherErrors []string
	for err := range errCh {
		if strings.Contains(err.Error(), "database is locked") || strings.Contains(err.Error(), "SQLITE_BUSY") {
			busyErrors = append(busyErrors, err.Error())
		} else {
			otherErrors = append(otherErrors, err.Error())
		}
	}

	assert.Emptyf(t, busyErrors,
		"concurrent writes must queue on the busy handler, not fail: %d/%d writes returned SQLITE_BUSY",
		len(busyErrors), writers*perWriter)
	assert.Empty(t, otherErrors, "no other write errors expected")

	var count int64
	require.NoError(t, db.Model(&models.Contact{}).Count(&count).Error)
	assert.Equal(t, int64(writers*perWriter), count, "every concurrent write must land")
}
