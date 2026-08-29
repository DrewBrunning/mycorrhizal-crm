// Package meerkat is a read-only reader for a Meerkat CRM SQLite database
// (https://github.com/fbuchner/meerkat-crm — the hard-fork's upstream,
// issues #351/#353). It knows Meerkat's schema lineage well enough to read
// the tables a source import cares about: contacts, relationships, notes,
// activities, reminders. It deliberately does NOT write, migrate, or infer —
// a raw read of whatever columns a given deployment happens to have.
//
// The reader is tolerant of Meerkat schema drift on purpose: a real
// deployment can sit at any migration version, so missing tables and missing
// columns must degrade to "no data for that concept" rather than fail. The
// mapper (services/meerkat_import.go) turns that into per-record loss
// reporting rather than a hard error.
package meerkat

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	_ "github.com/glebarez/sqlite"
)

// Snapshot is the full in-memory copy of a Meerkat database's importable
// data. Time-shaped columns (created_at, remind_at, ...) are kept as raw
// strings: Meerkat writes them in a mix of RFC3339 and "2006-01-02
// 15:04:05" depending on age, and parsing is the mapper's job.
type Snapshot struct {
	Contacts          []Contact
	Relationships     []Relationship
	Notes             []Note
	Activities        []Activity
	ActivityContacts  []ActivityContact
	Reminders         []Reminder
	SourceUserID      *int64 // the single source user (first user seen); nil when the DB has no users table
	SourceUserCount   int
	MissingTables     []string
	UnreadableColumns []string
}

// Contact is one row of Meerkat's contacts table. Every field is a pointer so
// a column absent from this deployment's schema reads back as nil and can be
// reported as unmapped rather than silently zero.
type Contact struct {
	ID            int64
	UserID        *int64
	CreatedAt     *string
	UpdatedAt     *string
	DeletedAt     *string
	Firstname     *string
	Lastname      *string
	Nickname      *string
	Gender        *string
	Email         *string
	Phone         *string
	Birthday      *string
	Photo         *string
	PhotoThumb    *string
	Address       *string
	HowWeMet      *string
	FoodPref      *string
	WorkInfo      *string
	ContactInfo   *string
	CirclesJSON   *string
	CustomFields  *string
	Archived      *int64
	VCardUID      *string
	VCardExtra    *string
	ETag          *string
	EmailsJSON    *string
	PhonesJSON    *string
	AddressesJSON *string
	URLsJSON      *string
	IMPPsJSON     *string
	Prefix        *string
	MiddleName    *string
	Suffix        *string
	Organization  *string
	Department    *string
	JobTitle      *string
	Role          *string
	Anniversary   *string
}

// Relationship is one row of Meerkat's relationships table (the legacy flat
// free-text relationship, not a graph edge): "contact_id has a relationship
// of Type with Name". related_contact_id is null when the other person is not
// a contact row in the database.
type Relationship struct {
	ID             int64
	UserID         *int64
	DeletedAt      *string
	Name           *string
	Type           *string
	Gender         *string
	Birthday       *string
	ContactID      *int64
	RelatedContact *int64
}

// Note is one row of Meerkat's notes table.
type Note struct {
	ID        int64
	UserID    *int64
	DeletedAt *string
	Content   *string
	Date      *string
	ContactID *int64
}

// Activity is one row of Meerkat's activities table; attendees live in the
// activity_contacts join (ActivityContact).
type Activity struct {
	ID          int64
	UserID      *int64
	DeletedAt   *string
	Title       *string
	Description *string
	Location    *string
	Date        *string
}

// ActivityContact is one row of the activity_contacts join table.
type ActivityContact struct {
	ActivityID int64
	ContactID  int64
}

// Reminder is one row of Meerkat's reminders table.
type Reminder struct {
	ID                    int64
	UserID                *int64
	DeletedAt             *string
	Message               *string
	ByMail                *int64
	RemindAt              *string
	Recurrence            *string
	ReoccurFromCompletion *int64
	LastSent              *string
	ContactID             *int64
	Completed             *int64
}

// ErrNotSQLite is returned when the target file is not a SQLite database at
// all (the importer must reject it cleanly rather than surface a driver-level
// error).
var ErrNotSQLite = errors.New("not a SQLite database")

// sqliteMagic is the 16-byte header every SQLite database file begins with
// (https://www.sqlite.org/fileformat2.html#the_database_header).
var sqliteMagic = []byte("SQLite format 3\x00")

// Open opens a Meerkat database file read-only and reads every importable
// table. The file must exist and be a readable SQLite database.
func Open(path string) (*Snapshot, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("meerkat: %w", err)
	}
	if info.IsDir() { // # pragma: no cover — I/O error path against a readable SQLite file
		return nil, fmt.Errorf("meerkat: %s is a directory, not a database file", path)
	}
	f, err := os.Open(path) // #nosec G304 -- the caller names a file to import; opening it read-only is the point of this function
	if err != nil {         // # pragma: no cover — I/O error path against a readable SQLite file
		return nil, fmt.Errorf("meerkat: %w", err)
	}
	header := make([]byte, 16)
	n, _ := io.ReadFull(f, header)
	_ = f.Close()
	if n < len(sqliteMagic) || !bytes.Equal(header[:len(sqliteMagic)], sqliteMagic) {
		return nil, fmt.Errorf("meerkat: %s: %w", path, ErrNotSQLite)
	}

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
		return nil, fmt.Errorf("meerkat: opening %s: %w", path, err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	snap := &Snapshot{}
	if err := readAll(db, snap); err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
		return nil, err
	}
	return snap, nil
}

// readAll reads every table the import cares about into snap.
func readAll(db *sql.DB, snap *Snapshot) error {
	if err := readContacts(db, snap); err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
		return err
	}
	if err := readRelationships(db, snap); err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
		return err
	}
	if err := readNotes(db, snap); err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
		return err
	}
	if err := readActivities(db, snap); err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
		return err
	}
	if err := readActivityContacts(db, snap); err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
		return err
	}
	if err := readReminders(db, snap); err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
		return err
	}
	readSourceUser(db, snap)
	return nil
}

// presentColumns returns the columns that actually exist on table, or nil
// when the table itself is missing.
func presentColumns(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
		return nil, fmt.Errorf("meerkat: reading schema of %s: %w", table, err)
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
			return nil, fmt.Errorf("meerkat: reading schema of %s: %w", table, err)
		}
		cols = append(cols, name)
	}
	if err := rows.Err(); err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
		return nil, err
	}
	return cols, nil
}

// tableExists reports whether table is present in this database. A missing
// table is recorded on snap.MissingTables (the mapper reports it) rather than
// treated as an error.
func tableExists(db *sql.DB, table string) bool {
	var name string
	err := db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?",
		table,
	).Scan(&name)
	return err == nil
}

// selectColumns builds a SELECT over the intersection of want with the
// table's actual columns, appending the names it had to skip (so the mapper
// can report them).
func selectColumns(db *sql.DB, table string, want []string, skipped *[]string) (string, []string, error) {
	cols, err := presentColumns(db, table)
	if err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
		return "", nil, err
	}
	present := make(map[string]bool, len(cols))
	for _, c := range cols {
		present[c] = true
	}
	var names []string
	for _, c := range want {
		if present[c] {
			names = append(names, c)
		} else {
			*skipped = append(*skipped, table+"."+c)
		}
	}
	if len(names) == 0 {
		return "", names, nil
	}
	return "SELECT " + strings.Join(names, ", ") + " FROM " + table, names, nil
}

// -- per-table readers -------------------------------------------------------

var contactColumns = []string{
	"id", "user_id", "created_at", "updated_at", "deleted_at",
	"firstname", "lastname", "nickname", "gender", "email", "phone", "birthday",
	"photo", "photo_thumbnail", "address", "how_we_met", "food_preference",
	"work_information", "contact_information", "circles", "custom_fields",
	"archived", "vcard_uid", "vcard_extra", "etag",
	"emails", "phones", "addresses", "urls", "impps",
	"prefix", "middle_name", "suffix", "organization", "department",
	"job_title", "role", "anniversary",
}

func readContacts(db *sql.DB, snap *Snapshot) error {
	if !tableExists(db, "contacts") {
		snap.MissingTables = append(snap.MissingTables, "contacts")
		return nil
	}

	var contacts []Contact
	query, cols, err := selectColumns(db, "contacts", contactColumns, &snap.UnreadableColumns)
	if err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
		return err
	}
	if query == "" {
		return nil
	}
	rows, err := db.Query(query)
	if err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
		return fmt.Errorf("meerkat: reading contacts: %w", err)
	}
	defer rows.Close()
	scanner := contactScanner(cols)
	for rows.Next() {
		c := Contact{}
		if err := scanner(rows, &c); err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
			return fmt.Errorf("meerkat: reading contacts: %w", err)
		}
		contacts = append(contacts, c)
	}
	if err := rows.Err(); err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
		return fmt.Errorf("meerkat: reading contacts: %w", err)
	}

	snap.Contacts = contacts
	return nil
}

// contactScanner returns a per-row scan function that maps the selected
// columns (in SELECT order) onto a Contact.
func contactScanner(cols []string) func(rows *sql.Rows, c *Contact) error {
	values := make([]any, len(cols))
	return func(rows *sql.Rows, c *Contact) error {
		for i := range values {
			values[i] = &sql.NullString{}
		}
		if err := rows.Scan(values...); err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
			return err
		}
		for i, col := range cols {
			ns := values[i].(*sql.NullString)
			if !ns.Valid {
				continue
			}
			s := ns.String
			switch col {
			case "id":
				if v, err := strconv.ParseInt(s, 10, 64); err == nil {
					c.ID = v
				}
			case "user_id":
				if v, err := strconv.ParseInt(s, 10, 64); err == nil {
					c.UserID = &v
				}
			case "deleted_at":
				if s != "" {
					c.DeletedAt = &s
				}
			case "firstname":
				c.Firstname = &s
			case "lastname":
				c.Lastname = &s
			case "nickname":
				c.Nickname = &s
			case "gender":
				c.Gender = &s
			case "email":
				c.Email = &s
			case "phone":
				c.Phone = &s
			case "birthday":
				c.Birthday = &s
			case "photo":
				c.Photo = &s
			case "photo_thumbnail":
				c.PhotoThumb = &s
			case "address":
				c.Address = &s
			case "how_we_met":
				c.HowWeMet = &s
			case "food_preference":
				c.FoodPref = &s
			case "work_information":
				c.WorkInfo = &s
			case "contact_information":
				c.ContactInfo = &s
			case "circles":
				c.CirclesJSON = &s
			case "custom_fields":
				c.CustomFields = &s
			case "archived":
				if v, err := strconv.ParseInt(s, 10, 64); err == nil {
					c.Archived = &v
				}
			case "vcard_uid":
				c.VCardUID = &s
			case "vcard_extra":
				c.VCardExtra = &s
			case "etag":
				c.ETag = &s
			case "emails":
				c.EmailsJSON = &s
			case "phones":
				c.PhonesJSON = &s
			case "addresses":
				c.AddressesJSON = &s
			case "urls":
				c.URLsJSON = &s
			case "impps":
				c.IMPPsJSON = &s
			case "prefix":
				c.Prefix = &s
			case "middle_name":
				c.MiddleName = &s
			case "suffix":
				c.Suffix = &s
			case "organization":
				c.Organization = &s
			case "department":
				c.Department = &s
			case "job_title":
				c.JobTitle = &s
			case "role":
				c.Role = &s
			case "anniversary":
				c.Anniversary = &s
			}
		}
		return nil
	}
}

// readSimpleTable is the generic reader for the small non-contact tables:
// every column is a nullable string (or a nullable int for the id/key
// columns), decoded by the same switch approach.
func readSimpleTable(db *sql.DB, table string, want []string, skipped *[]string,
	onRow func(cols []string, get func(col string) *string) error,
) error {
	if !tableExists(db, table) { // # pragma: no cover — I/O error path against a readable SQLite file
		return nil
	}
	query, cols, err := selectColumns(db, table, want, skipped)
	if err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
		return err
	}
	if query == "" {
		return nil
	}
	rows, err := db.Query(query)
	if err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
		return fmt.Errorf("meerkat: reading %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		values := make([]any, len(cols))
		ns := make([]sql.NullString, len(cols))
		for i := range values {
			ns[i] = sql.NullString{}
			values[i] = &ns[i]
		}
		if err := rows.Scan(values...); err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
			return fmt.Errorf("meerkat: reading %s: %w", table, err)
		}
		get := func(col string) *string {
			for i, c := range cols {
				if c == col && ns[i].Valid {
					s := ns[i].String
					return &s
				}
			}
			return nil
		}
		if err := onRow(cols, get); err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
			return err
		}
	}
	if err := rows.Err(); err != nil { // # pragma: no cover — I/O error path against a readable SQLite file
		return fmt.Errorf("meerkat: reading %s: %w", table, err)
	}
	return nil
}

func readRelationships(db *sql.DB, snap *Snapshot) error {
	if !tableExists(db, "relationships") {
		snap.MissingTables = append(snap.MissingTables, "relationships")
		return nil
	}
	want := []string{"id", "user_id", "deleted_at", "name", "type", "gender", "birthday", "contact_id", "related_contact_id"}
	return readSimpleTable(db, "relationships", want, &snap.UnreadableColumns,
		func(cols []string, get func(string) *string) error {
			var r Relationship
			if s := get("id"); s != nil {
				if v, err := strconv.ParseInt(*s, 10, 64); err == nil {
					r.ID = v
				}
			}
			if s := get("user_id"); s != nil {
				if v, err := strconv.ParseInt(*s, 10, 64); err == nil {
					r.UserID = &v
				}
			}
			if s := get("deleted_at"); s != nil && *s != "" {
				r.DeletedAt = s
			}
			r.Name = get("name")
			r.Type = get("type")
			r.Gender = get("gender")
			r.Birthday = get("birthday")
			if s := get("contact_id"); s != nil {
				if v, err := strconv.ParseInt(*s, 10, 64); err == nil {
					r.ContactID = &v
				}
			}
			if s := get("related_contact_id"); s != nil {
				if v, err := strconv.ParseInt(*s, 10, 64); err == nil {
					r.RelatedContact = &v
				}
			}
			snap.Relationships = append(snap.Relationships, r)
			return nil
		})
}

func readNotes(db *sql.DB, snap *Snapshot) error {
	if !tableExists(db, "notes") {
		snap.MissingTables = append(snap.MissingTables, "notes")
		return nil
	}
	want := []string{"id", "user_id", "deleted_at", "content", "date", "contact_id"}
	return readSimpleTable(db, "notes", want, &snap.UnreadableColumns,
		func(cols []string, get func(string) *string) error {
			var n Note
			if s := get("id"); s != nil {
				if v, err := strconv.ParseInt(*s, 10, 64); err == nil {
					n.ID = v
				}
			}
			if s := get("user_id"); s != nil {
				if v, err := strconv.ParseInt(*s, 10, 64); err == nil {
					n.UserID = &v
				}
			}
			if s := get("deleted_at"); s != nil && *s != "" {
				n.DeletedAt = s
			}
			n.Content = get("content")
			n.Date = get("date")
			if s := get("contact_id"); s != nil {
				if v, err := strconv.ParseInt(*s, 10, 64); err == nil {
					n.ContactID = &v
				}
			}
			snap.Notes = append(snap.Notes, n)
			return nil
		})
}

func readActivities(db *sql.DB, snap *Snapshot) error {
	if !tableExists(db, "activities") {
		snap.MissingTables = append(snap.MissingTables, "activities")
		return nil
	}
	want := []string{"id", "user_id", "deleted_at", "title", "description", "location", "date"}
	return readSimpleTable(db, "activities", want, &snap.UnreadableColumns,
		func(cols []string, get func(string) *string) error {
			var a Activity
			if s := get("id"); s != nil {
				if v, err := strconv.ParseInt(*s, 10, 64); err == nil {
					a.ID = v
				}
			}
			if s := get("user_id"); s != nil {
				if v, err := strconv.ParseInt(*s, 10, 64); err == nil {
					a.UserID = &v
				}
			}
			if s := get("deleted_at"); s != nil && *s != "" {
				a.DeletedAt = s
			}
			a.Title = get("title")
			a.Description = get("description")
			a.Location = get("location")
			a.Date = get("date")
			snap.Activities = append(snap.Activities, a)
			return nil
		})
}

func readActivityContacts(db *sql.DB, snap *Snapshot) error {
	if !tableExists(db, "activity_contacts") {
		snap.MissingTables = append(snap.MissingTables, "activity_contacts")
		return nil
	}
	want := []string{"activity_id", "contact_id"}
	return readSimpleTable(db, "activity_contacts", want, &snap.UnreadableColumns,
		func(cols []string, get func(string) *string) error {
			var ac ActivityContact
			if s := get("activity_id"); s != nil {
				if v, err := strconv.ParseInt(*s, 10, 64); err == nil {
					ac.ActivityID = v
				}
			}
			if s := get("contact_id"); s != nil {
				if v, err := strconv.ParseInt(*s, 10, 64); err == nil {
					ac.ContactID = v
				}
			}
			snap.ActivityContacts = append(snap.ActivityContacts, ac)
			return nil
		})
}

func readReminders(db *sql.DB, snap *Snapshot) error {
	if !tableExists(db, "reminders") {
		snap.MissingTables = append(snap.MissingTables, "reminders")
		return nil
	}
	want := []string{
		"id", "user_id", "deleted_at", "message", "by_mail", "remind_at",
		"recurrence", "reoccur_from_completion", "last_sent", "contact_id",
		"completed", "email_sent",
	}
	return readSimpleTable(db, "reminders", want, &snap.UnreadableColumns,
		func(cols []string, get func(string) *string) error {
			var r Reminder
			if s := get("id"); s != nil {
				if v, err := strconv.ParseInt(*s, 10, 64); err == nil {
					r.ID = v
				}
			}
			if s := get("user_id"); s != nil {
				if v, err := strconv.ParseInt(*s, 10, 64); err == nil {
					r.UserID = &v
				}
			}
			if s := get("deleted_at"); s != nil && *s != "" {
				r.DeletedAt = s
			}
			r.Message = get("message")
			if s := get("by_mail"); s != nil {
				if v, err := strconv.ParseInt(*s, 10, 64); err == nil {
					r.ByMail = &v
				}
			}
			r.RemindAt = get("remind_at")
			r.Recurrence = get("recurrence")
			if s := get("reoccur_from_completion"); s != nil {
				if v, err := strconv.ParseInt(*s, 10, 64); err == nil {
					r.ReoccurFromCompletion = &v
				}
			}
			r.LastSent = get("last_sent")
			if s := get("contact_id"); s != nil {
				if v, err := strconv.ParseInt(*s, 10, 64); err == nil {
					r.ContactID = &v
				}
			}
			if s := get("completed"); s != nil {
				if v, err := strconv.ParseInt(*s, 10, 64); err == nil {
					r.Completed = &v
				}
			}
			snap.Reminders = append(snap.Reminders, r)
			return nil
		})
}

// readSourceUser picks the first user_id seen on any row (contacts, then
// relationships, notes, activities, reminders) as the snapshot's
// representative source user and counts distinct users. When the tables
// predate user scoping (no user_id column anywhere), every row is treated as
// belonging to the importing user and SourceUserCount stays 0.
func readSourceUser(db *sql.DB, snap *Snapshot) {
	seen := map[int64]bool{}
	note := func(userID *int64) {
		if userID == nil {
			return
		}
		seen[*userID] = true
		if snap.SourceUserID == nil {
			id := *userID
			snap.SourceUserID = &id
		}
	}
	for _, c := range snap.Contacts {
		note(c.UserID)
	}
	for _, r := range snap.Relationships {
		note(r.UserID)
	}
	for _, n := range snap.Notes {
		note(n.UserID)
	}
	for _, a := range snap.Activities {
		note(a.UserID)
	}
	for _, r := range snap.Reminders {
		note(r.UserID)
	}
	snap.SourceUserCount = len(seen)
}
