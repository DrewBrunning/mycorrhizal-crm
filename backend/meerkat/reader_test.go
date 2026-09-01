package meerkat

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestDB creates an empty SQLite file and returns an open handle.
func newTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "src.db")
	db, err := sql.Open("sqlite", "file:"+path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

func TestOpen_RejectsMissingAndNonSQLiteFiles(t *testing.T) {
	_, err := Open(filepath.Join(t.TempDir(), "nope.db"))
	assert.Error(t, err, "missing file must error")

	bad := filepath.Join(t.TempDir(), "bad.db")
	require.NoError(t, os.WriteFile(bad, []byte("this is not a sqlite database"), 0o644))
	_, err = Open(bad)
	assert.ErrorIs(t, err, ErrNotSQLite, "a non-SQLite file must be rejected as not-a-SQLite-database")
}

func TestOpen_ReadsContactsAndToleratesMissingTables(t *testing.T) {
	db, path := newTestDB(t)
	_, err := db.Exec(`CREATE TABLE contacts (
		id INTEGER PRIMARY KEY, user_id INTEGER, deleted_at DATETIME,
		firstname TEXT, lastname TEXT, nickname TEXT, gender TEXT,
		email TEXT, phone TEXT, birthday TEXT, photo TEXT, address TEXT,
		how_we_met TEXT, food_preference TEXT, work_information TEXT,
		contact_information TEXT, circles TEXT, custom_fields TEXT,
		archived INTEGER, vcard_uid TEXT, vcard_extra TEXT, etag TEXT,
		emails TEXT, phones TEXT, addresses TEXT, urls TEXT, impps TEXT,
		prefix TEXT, middle_name TEXT, suffix TEXT, organization TEXT,
		department TEXT, job_title TEXT, role TEXT, anniversary TEXT
	)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO contacts
		(id, user_id, firstname, lastname, nickname, birthday, circles, custom_fields)
		VALUES (1, 7, 'Ada', 'Lovelace', 'Enchantress', '1815-12-10', '["Family"]', '{"Zodiac":"Pisces"}')`)
	require.NoError(t, err)
	// No relationships/notes/activities/reminders tables at all (an old
	// deployment); Open must degrade to empty, not fail.
	require.NoError(t, db.Close())

	snap, err := Open(path)
	require.NoError(t, err)
	require.Len(t, snap.Contacts, 1)
	c := snap.Contacts[0]
	assert.Equal(t, int64(1), c.ID)
	require.NotNil(t, c.UserID)
	assert.Equal(t, int64(7), *c.UserID)
	assert.Equal(t, "Ada", *c.Firstname)
	assert.Equal(t, "1815-12-10", *c.Birthday)
	assert.Equal(t, `["Family"]`, *c.CirclesJSON)
	assert.Contains(t, snap.MissingTables, "relationships")
	assert.Contains(t, snap.MissingTables, "notes")
	assert.Empty(t, snap.Relationships)
}

func TestOpen_ReadsRelationshipsNotesActivitiesReminders(t *testing.T) {
	db, path := newTestDB(t)
	stmts := []string{
		`CREATE TABLE relationships (id INTEGER PRIMARY KEY, user_id INTEGER, deleted_at DATETIME, name TEXT, type TEXT, contact_id INTEGER, related_contact_id INTEGER)`,
		`CREATE TABLE notes (id INTEGER PRIMARY KEY, user_id INTEGER, deleted_at DATETIME, content TEXT, date DATETIME, contact_id INTEGER)`,
		`CREATE TABLE activities (id INTEGER PRIMARY KEY, user_id INTEGER, deleted_at DATETIME, title TEXT, date DATETIME)`,
		`CREATE TABLE activity_contacts (activity_id INTEGER, contact_id INTEGER)`,
		`CREATE TABLE reminders (id INTEGER PRIMARY KEY, user_id INTEGER, deleted_at DATETIME, message TEXT, remind_at DATETIME, recurrence TEXT, by_mail INTEGER, reoccur_from_completion INTEGER, last_sent DATETIME, contact_id INTEGER, completed INTEGER, email_sent INTEGER)`,
	}
	for _, s := range stmts {
		_, err := db.Exec(s)
		require.NoError(t, err)
	}
	_, err := db.Exec(`INSERT INTO relationships (id, user_id, name, type, contact_id, related_contact_id) VALUES (1, 7, 'Jane', 'father', 2, 1)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO relationships (id, user_id, deleted_at, name, type, contact_id, related_contact_id) VALUES (2, 7, '2024-01-01 00:00:00', 'Gone', 'friend', 1, 2)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, user_id, content, date, contact_id) VALUES (1, 7, 'hello', '2023-04-01 12:00:00', 2)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes (id, user_id, deleted_at, content, date, contact_id) VALUES (2, 7, '2024-01-01 00:00:00', 'gone', '2023-04-01 12:00:00', 2)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO activities (id, user_id, title, date) VALUES (1, 7, 'Lunch', '2023-04-02 12:00:00')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO activities (id, user_id, deleted_at, title, date) VALUES (2, 7, '2024-01-01 00:00:00', 'Gone', '2023-04-02 12:00:00')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO activity_contacts (activity_id, contact_id) VALUES (1, 2)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO reminders (id, user_id, message, remind_at, recurrence, by_mail, reoccur_from_completion, completed, contact_id) VALUES (1, 7, 'Call', '2023-05-01 09:00:00', 'yearly', 1, 0, 1, 2)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO reminders (id, user_id, deleted_at, message, remind_at, recurrence, contact_id) VALUES (2, NULL, '2024-01-01 00:00:00', 'gone', '2023-05-01 09:00:00', 'yearly', 2)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	snap, err := Open(path)
	require.NoError(t, err)
	require.Len(t, snap.Relationships, 2)
	assert.Equal(t, "father", *snap.Relationships[0].Type)
	require.NotNil(t, snap.Relationships[0].RelatedContact)
	assert.Equal(t, int64(1), *snap.Relationships[0].RelatedContact)
	require.NotNil(t, snap.Relationships[1].DeletedAt)
	require.Len(t, snap.Notes, 2)
	require.NotNil(t, snap.Notes[1].DeletedAt)
	require.Len(t, snap.Activities, 2)
	require.Len(t, snap.ActivityContacts, 1)
	assert.Equal(t, int64(2), snap.ActivityContacts[0].ContactID)
	require.Len(t, snap.Reminders, 2)
	assert.Equal(t, "yearly", *snap.Reminders[0].Recurrence)
	require.NotNil(t, snap.Reminders[0].ByMail)
	assert.Equal(t, int64(1), *snap.Reminders[0].ByMail)
	require.NotNil(t, snap.Reminders[0].ReoccurFromCompletion)
	assert.Equal(t, int64(0), *snap.Reminders[0].ReoccurFromCompletion)
	require.NotNil(t, snap.Reminders[0].Completed)
	assert.Equal(t, int64(1), *snap.Reminders[0].Completed)
	require.NotNil(t, snap.Reminders[1].DeletedAt)

	require.NotNil(t, snap.SourceUserID)
	assert.Equal(t, int64(7), *snap.SourceUserID)
	assert.Equal(t, 1, snap.SourceUserCount)
}

// TestOpen_ReadsEveryContactColumn pins the full-column decode path: a row
// carrying every optional column (photo_thumbnail, vcard_extra, etag, all the
// JSON arrays, deleted_at) must decode each onto the right pointer.
func TestOpen_DegenerateTableWithNoMatchingColumns(t *testing.T) {
	db, path := newTestDB(t)
	// An activity_contacts table that shares no columns with what the reader
	// looks for: the reader must degrade to no rows, not fail. Same for a
	// degenerate contacts table (covers the empty-query paths in both the
	// contacts reader and the generic table reader).
	_, err := db.Exec(`CREATE TABLE activity_contacts (unrelated TEXT)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO activity_contacts (unrelated) VALUES ('x')`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE contacts (unrelated TEXT)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	snap, err := Open(path)
	require.NoError(t, err)
	assert.Empty(t, snap.ActivityContacts)
	assert.Empty(t, snap.Contacts)
	assert.Contains(t, snap.UnreadableColumns, "activity_contacts.activity_id")
	assert.Contains(t, snap.UnreadableColumns, "activity_contacts.contact_id")
}

// TestOpen_ReadsEveryContactColumn pins the full-column decode path: a row
// carrying every optional column (photo_thumbnail, vcard_extra, etag, all the
// JSON arrays, deleted_at) must decode each onto the right pointer.
func TestOpen_ReadsEveryContactColumn(t *testing.T) {
	db, path := newTestDB(t)
	_, err := db.Exec(`CREATE TABLE contacts (
		id INTEGER PRIMARY KEY, user_id INTEGER, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
		firstname TEXT, lastname TEXT, nickname TEXT, gender TEXT, email TEXT, phone TEXT, birthday TEXT,
		photo TEXT, photo_thumbnail TEXT, address TEXT, how_we_met TEXT, food_preference TEXT,
		work_information TEXT, contact_information TEXT, circles TEXT, custom_fields TEXT,
		archived INTEGER, vcard_uid TEXT, vcard_extra TEXT, etag TEXT,
		emails TEXT, phones TEXT, addresses TEXT, urls TEXT, impps TEXT,
		prefix TEXT, middle_name TEXT, suffix TEXT, organization TEXT, department TEXT,
		job_title TEXT, role TEXT, anniversary TEXT
	)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO contacts (
		id, user_id, deleted_at, firstname, lastname, nickname, gender, email, phone, birthday,
		photo, photo_thumbnail, address, how_we_met, food_preference, work_information, contact_information,
		circles, custom_fields, archived, vcard_uid, vcard_extra, etag,
		emails, phones, addresses, urls, impps, prefix, middle_name, suffix,
		organization, department, job_title, role, anniversary
	) VALUES (
		1, 7, '2024-01-01 00:00:00', 'Ada', 'Lovelace', 'Enchantress', 'female', 'a@b.c', '+1', '1815-12-10',
		'photos/a.jpg', 'thumb', '1 Main St', 'how', 'food', 'work', 'info',
		'["F"]', '{"K":"V"}', 1, '11111111-1111-1111-1111-111111111111', '{"properties":{}}', 'e-1-1',
		'[{"type":"home","value":"a@b.c"}]', '[{"type":"cell","value":"+1"}]', '[]', '[{"type":"home","value":"https://a"}]', '[{"type":"tg","value":"@a"}]',
		'Dr', 'M', 'Jr', 'Org', 'Dept', 'Title', 'Role', '1835-07-08'
	)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	snap, err := Open(path)
	require.NoError(t, err)
	require.Len(t, snap.Contacts, 1)
	c := snap.Contacts[0]
	require.NotNil(t, c.DeletedAt)
	assert.Equal(t, "Enchantress", *c.Nickname)
	assert.Equal(t, "photos/a.jpg", *c.Photo)
	assert.Equal(t, "thumb", *c.PhotoThumb)
	assert.Equal(t, `{"properties":{}}`, *c.VCardExtra)
	assert.Equal(t, "e-1-1", *c.ETag)
	require.NotNil(t, c.Archived)
	assert.Equal(t, int64(1), *c.Archived)
	assert.Equal(t, "Dr", *c.Prefix)
	assert.Equal(t, "Jr", *c.Suffix)
	assert.Equal(t, "Org", *c.Organization)
	assert.Equal(t, "Dept", *c.Department)
	assert.Equal(t, "Title", *c.JobTitle)
	assert.Equal(t, "Role", *c.Role)
	assert.Equal(t, "1835-07-08", *c.Anniversary)
	assert.Equal(t, `[{"type":"home","value":"a@b.c"}]`, *c.EmailsJSON)
	assert.Equal(t, `[{"type":"cell","value":"+1"}]`, *c.PhonesJSON)
	assert.Equal(t, `[]`, *c.AddressesJSON)
	assert.Equal(t, `[{"type":"home","value":"https://a"}]`, *c.URLsJSON)
	assert.Equal(t, `[{"type":"tg","value":"@a"}]`, *c.IMPPsJSON)
	assert.Equal(t, `["F"]`, *c.CirclesJSON)
	assert.Equal(t, `{"K":"V"}`, *c.CustomFields)
}

func TestOpen_ReadsUsersForThePicker(t *testing.T) {
	db, path := newTestDB(t)
	_, err := db.Exec(`
		CREATE TABLE contacts (id INTEGER PRIMARY KEY, firstname TEXT);
		CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT, email TEXT, name TEXT);
		INSERT INTO users (id, username, email, name) VALUES
			(1, 'alice', 'alice@example.com', 'Alice A'),
			(2, 'bob', 'bob@example.com', NULL);
	`)
	require.NoError(t, err)

	snap, err := Open(path)
	require.NoError(t, err)
	require.Len(t, snap.Users, 2)
	assert.Equal(t, int64(1), snap.Users[0].ID)
	assert.Equal(t, "alice", *snap.Users[0].Username)
	assert.Equal(t, "Alice A", *snap.Users[0].Name)
	assert.Nil(t, snap.Users[1].Name)
}

func TestOpen_UsersFallBackToFirstLastName(t *testing.T) {
	db, path := newTestDB(t)
	_, err := db.Exec(`
		CREATE TABLE contacts (id INTEGER PRIMARY KEY);
		CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT, first_name TEXT, last_name TEXT);
		INSERT INTO users (id, username, first_name, last_name) VALUES (5, 'carol', 'Carol', 'C');
	`)
	require.NoError(t, err)

	snap, err := Open(path)
	require.NoError(t, err)
	require.Len(t, snap.Users, 1)
	require.NotNil(t, snap.Users[0].Name)
	assert.Equal(t, "Carol C", *snap.Users[0].Name)
}

func TestOpen_NoUsersTable(t *testing.T) {
	db, path := newTestDB(t)
	_, err := db.Exec(`CREATE TABLE contacts (id INTEGER PRIMARY KEY);`)
	require.NoError(t, err)

	snap, err := Open(path)
	require.NoError(t, err)
	assert.Empty(t, snap.Users)
}
