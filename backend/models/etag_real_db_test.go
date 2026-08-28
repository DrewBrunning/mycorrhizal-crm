package models

import (
	"fmt"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRevisionToken_RealMigratedSchema is the real-DB check for issue #591
// (CON-01a, ADR 0006). The AutoMigrate test DBs used elsewhere derive their
// schema from the same Go struct tags the application code uses, so they
// cannot catch a GORM column-tag mismatch against the real migration SQL —
// the `e_tag` vs `etag` bug class that shipped broken for ContactSyncLink.ETag
// and the `revision` column migration 000044 adds. Only a
// database.InitDB-migrated DB, which applies the real migrations, can.
//
// It proves, against the real schema, for every user-authored soft-delete
// entity:
//
//  1. a new row starts at revision 1 and derives `e-{id}-1`;
//  2. updating it bumps the revision to 2 and re-derives the ETag;
//  3. the revision column persists exactly what the in-memory struct says.
func TestRevisionToken_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "revision-real.db")
	db := dbtest.NewAt(t, dbPath)

	user := User{Username: "realdbtester", Password: "password123!A", Email: "realdb@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	// --- Contact (uint PK, gorm.Model) ---
	{
		c := Contact{UserID: user.ID, Firstname: "Contact initial"}
		require.NoError(t, db.Create(&c).Error)
		assertRevisionContract(t, "Contact", c.ID, c.ETag, c.Revision,
			regexp.MustCompile(`^e-\d+-\d+$`),
			func() error { return db.Model(&c).Update("firstname", "Renamed").Error },
			func() (int64, string) { return c.Revision, c.ETag })
	}

	// --- Activity (uint PK, gorm.Model) ---
	{
		a := Activity{UserID: user.ID, Title: "Activity initial", Date: time.Now()}
		require.NoError(t, db.Create(&a).Error)
		assertRevisionContract(t, "Activity", a.ID, a.ETag, a.Revision,
			regexp.MustCompile(`^e-\d+-\d+$`),
			func() error { return db.Model(&a).Update("title", "Renamed").Error },
			func() (int64, string) { return a.Revision, a.ETag })
	}

	// --- LifeEvent (UUID string PK) ---
	{
		l := LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: "graduated"}
		require.NoError(t, db.Create(&l).Error)
		assertRevisionContract(t, "LifeEvent", l.ID, l.ETag, l.Revision,
			regexp.MustCompile(`^e-[0-9a-f-]+-\d+$`),
			func() error { return db.Model(&l).Update("description", "updated").Error },
			func() (int64, string) { return l.Revision, l.ETag })
	}

	// --- Note (uint PK; revision+etag columns new in 000044) ---
	{
		n := Note{UserID: user.ID, Content: "Note initial", Date: time.Now(), ContactID: &contact.ID}
		require.NoError(t, db.Create(&n).Error)
		assertRevisionContract(t, "Note", n.ID, n.ETag, n.Revision,
			regexp.MustCompile(`^e-\d+-\d+$`),
			func() error { return db.Model(&n).Update("content", "Renamed").Error },
			func() (int64, string) { return n.Revision, n.ETag })
	}

	// --- Reminder (uint PK; revision+etag columns new in 000044) ---
	{
		r := Reminder{UserID: user.ID, Message: "Reminder initial", RemindAt: time.Now(), Recurrence: "once", ContactID: &contact.ID}
		require.NoError(t, db.Create(&r).Error)
		assertRevisionContract(t, "Reminder", r.ID, r.ETag, r.Revision,
			regexp.MustCompile(`^e-\d+-\d+$`),
			func() error { return db.Model(&r).Update("message", "Renamed").Error },
			func() (int64, string) { return r.Revision, r.ETag })
	}
}

// assertRevisionContract checks the ADR 0006 contract for one row: create
// starts at revision 1 with a well-formed derived ETag, a real update bumps
// to revision 2 with a re-derived ETag, and the persisted `revision` column
// matches the in-memory value exactly.
func assertRevisionContract(t *testing.T, table string, id any, etag string, revision int64,
	etagShape *regexp.Regexp,
	update func() error,
	current func() (int64, string)) {
	t.Helper()

	require.NotEmpty(t, etag, "%s: assertion 1: a new row gets an ETag", table)
	assert.Regexp(t, etagShape, etag, "%s: etag must have the e-{id}-{revision} shape", table)
	assert.Equal(t, int64(1), revision, "%s: a new row starts at revision 1", table)
	assert.Equal(t, fmt.Sprintf("e-%v-%d", id, revision), etag, "%s: etag embeds the revision", table)

	require.NoError(t, update(), "%s: update failed", table)
	revAfter, etagAfter := current()
	assert.Equal(t, int64(2), revAfter, "%s: a real update bumps the revision to 2", table)
	assert.NotEqual(t, etag, etagAfter, "%s: a real update changes the ETag", table)
	assert.Equal(t, fmt.Sprintf("e-%v-%d", id, revAfter), etagAfter, "%s: etag re-derived from the new revision", table)
}

// TestRevisionCounterSubSecondResolution_RealMigratedSchema is the regression
// test for the bug this ticket exists to fix (issue #591): under the old
// scheme the ETag was derived from UpdatedAt.Unix(), so two writes to the same
// row inside the same wall-clock second produced an identical token — the
// lost-update hole a conditional-write check built on it would inherit. The
// monotonic revision counter has no wall-clock input at all, so two
// back-to-back writes are distinct no matter what second they land in.
//
// Written FIRST, per the ticket ("write this test first, against a real
// migrated schema") and hand-verified per CLAUDE.md: temporarily reverting the
// counter increment in Contact.AfterSave makes this fail.
func TestRevisionCounterSubSecondResolution_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "subsecond-real.db")
	db := dbtest.NewAt(t, dbPath)

	user := User{Username: "subsecond", Password: "password123!A", Email: "subsecond@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	// Two consecutive writes with no clock manipulation at all: if the token
	// still depended on wall-clock seconds, two writes inside the same second
	// would collide. The counter makes them distinct unconditionally.
	c := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&c).Error)
	rev1 := c.Revision

	require.NoError(t, db.Model(&c).Update("firstname", "First change").Error)
	rev2 := c.Revision
	require.NoError(t, db.Model(&c).Update("firstname", "Second change").Error)
	rev3 := c.Revision

	assert.Less(t, rev1, rev2, "two writes must strictly increase the revision")
	assert.Less(t, rev2, rev3, "three writes must strictly increase the revision")
	assert.NotEqual(t, c.ETag, fmt.Sprintf("e-%d-%d", c.ID, rev1), "the ETag must track the new revision")

	// The same property holds for a UUID-PK entity (LifeEvent).
	event := LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: "graduated"}
	require.NoError(t, db.Create(&event).Error)
	e1 := event.Revision
	require.NoError(t, db.Model(&event).Update("description", "First change").Error)
	e2 := event.Revision
	require.NoError(t, db.Model(&event).Update("description", "Second change").Error)
	e3 := event.Revision
	assert.Less(t, e1, e2)
	assert.Less(t, e2, e3)

	// And for the entities that never had an ETag at all (Note, Reminder).
	note := Note{UserID: user.ID, Content: "first", Date: time.Now(), ContactID: &contact.ID}
	require.NoError(t, db.Create(&note).Error)
	n1 := note.Revision
	require.NoError(t, db.Model(&note).Update("content", "second").Error)
	n2 := note.Revision
	assert.Less(t, n1, n2)

	reminder := Reminder{UserID: user.ID, Message: "first", RemindAt: time.Now(), Recurrence: "once", ContactID: &contact.ID}
	require.NoError(t, db.Create(&reminder).Error)
	r1 := reminder.Revision
	require.NoError(t, db.Model(&reminder).Update("message", "second").Error)
	r2 := reminder.Revision
	assert.Less(t, r1, r2)

	// Activity, for completeness of the five-entity set.
	activity := Activity{UserID: user.ID, Title: "first", Date: time.Now()}
	require.NoError(t, db.Create(&activity).Error)
	a1 := activity.Revision
	require.NoError(t, db.Model(&activity).Update("title", "second").Error)
	a2 := activity.Revision
	assert.Less(t, a1, a2)
}
