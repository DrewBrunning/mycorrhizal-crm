package dbfault_test

import (
	"errors"
	"testing"

	"mycorrhizal/internal/dbfault"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newUser makes the owner row the tags/circles below reference. Call it before
// arming the injector: its own INSERT is a statement too.
func newUser(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	u := models.User{Username: "dbfault-user", Email: "dbfault@example.com", Password: "password123"}
	require.NoError(t, db.Create(&u).Error)
	return u.ID
}

func TestFor_NilWithoutOption(t *testing.T) {
	require.Nil(t, dbfault.For(dbtest.New(t)))
}

func TestIdleInjectorPassesThroughAndRecordsNothing(t *testing.T) {
	db := dbtest.New(t, dbtest.WithFaults())
	inj := dbfault.For(db)
	require.NotNil(t, inj)
	require.Equal(t, dbfault.PluginName, inj.Name())

	u := newUser(t, db)
	require.NoError(t, db.Create(&models.Tag{UserID: u, Name: "x"}).Error)
	require.Empty(t, inj.Statements())
	_, fired := inj.Fired()
	require.False(t, fired)
}

func TestRecordThenFailNth(t *testing.T) {
	db := dbtest.New(t, dbtest.WithFaults())
	inj := dbfault.For(db)
	u := newUser(t, db)

	inj.Record()
	require.NoError(t, db.Create(&models.Tag{UserID: u, Name: "a"}).Error)
	var n int64
	require.NoError(t, db.Model(&models.Tag{}).Count(&n).Error)
	require.NoError(t, db.Exec("DELETE FROM tags WHERE name = ?", "a").Error)
	inj.Disarm()
	stmts := inj.Statements()
	require.Len(t, stmts, 3)
	require.Equal(t, []string{"create tags", "query tags", "raw tags"}, []string{stmts[0].String(), stmts[1].String(), stmts[2].String()})

	// n-th statement fails, the others pass; a failed create writes nothing.
	inj.FailNth(1)
	err := db.Create(&models.Tag{UserID: u, Name: "b"}).Error
	require.True(t, errors.Is(err, dbfault.ErrInjected), "got %v", err)
	st, fired := inj.Fired()
	require.True(t, fired)
	require.Equal(t, "create tags", st.String())
	inj.Disarm()
	require.NoError(t, db.Model(&models.Tag{}).Where("name = ?", "b").Count(&n).Error)
	require.Zero(t, n)

	// n < 1 never fires.
	inj.FailNth(0)
	require.NoError(t, db.Create(&models.Tag{UserID: u, Name: "c"}).Error)
	_, fired = inj.Fired()
	require.False(t, fired)
}

func TestFailMatchingFiresOnceOnFirstMatch(t *testing.T) {
	db := dbtest.New(t, dbtest.WithFaults())
	inj := dbfault.For(db)
	u := newUser(t, db)

	inj.FailMatching("create", "tags")
	require.NoError(t, db.Model(&models.Tag{}).Where("1 = 0").Find(&[]models.Tag{}).Error, "a non-matching verb passes")
	require.ErrorIs(t, db.Create(&models.Tag{UserID: u, Name: "d"}).Error, dbfault.ErrInjected)
	require.NoError(t, db.Create(&models.Tag{UserID: u, Name: "e"}).Error, "only the first match fails")
}

func TestRowAndRawStatementsAreInterceptedAndRawTableParsed(t *testing.T) {
	db := dbtest.New(t, dbtest.WithFaults())
	inj := dbfault.For(db)
	u := newUser(t, db)

	inj.Record()
	var c int64
	require.NoError(t, db.Raw("SELECT count(*) FROM `tags`").Row().Scan(&c))
	require.NoError(t, db.Exec("INSERT INTO circles (id, user_id, name, created_at, updated_at) VALUES ('c1', ?, 'n', datetime('now'), datetime('now'))", u).Error)
	inj.Disarm()
	stmts := inj.Statements()
	require.Len(t, stmts, 2)
	require.Equal(t, "row tags", stmts[0].String())
	require.Equal(t, "raw circles", stmts[1].String())
}

func TestInjectorIgnoresOtherGoroutines(t *testing.T) {
	db := dbtest.New(t, dbtest.WithFaults())
	inj := dbfault.For(db)
	u := newUser(t, db)
	inj.FailNth(1)

	done := make(chan error)
	go func() { done <- db.Create(&models.Tag{UserID: u, Name: "g"}).Error }()
	require.NoError(t, <-done, "a statement on another goroutine is neither counted nor failed")
	require.Empty(t, inj.Statements())

	require.ErrorIs(t, db.Create(&models.Tag{UserID: u, Name: "h"}).Error, dbfault.ErrInjected)
}
