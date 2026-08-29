package meerkatfixture

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/glebarez/sqlite"
)

// Populate builds a Meerkat-schema SQLite database at path from the manifest.
// The database is created fresh (the file must not already exist); it contains
// exactly the tables and rows the manifest declares, in Meerkat's own schema
// shape, so the real meerkat.Open reader consumes it unchanged. Every write
// runs before the connection is closed, so the file on disk is complete.
func Populate(path string, m *Manifest) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("meerkatfixture: refusing to overwrite existing file %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil { // # pragma: no cover — defensive
		return fmt.Errorf("meerkatfixture: creating fixture dir: %w", err)
	}

	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil { // # pragma: no cover — defensive
		return fmt.Errorf("meerkatfixture: opening %s: %w", path, err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if err := createSchema(db); err != nil { // # pragma: no cover — defensive
		return err
	}
	if err := insertUsers(db, m); err != nil { // # pragma: no cover — defensive
		return err
	}
	if err := insertContacts(db, m); err != nil { // # pragma: no cover — defensive
		return err
	}
	if err := insertRelationships(db, m); err != nil { // # pragma: no cover — defensive
		return err
	}
	if err := insertNotes(db, m); err != nil { // # pragma: no cover — defensive
		return err
	}
	if err := insertActivities(db, m); err != nil { // # pragma: no cover — defensive
		return err
	}
	if err := insertReminders(db, m); err != nil { // # pragma: no cover — defensive
		return err
	}
	return nil
}

// createSchema creates the subset of Meerkat's schema the import reads. The
// column lists mirror upstream meerkat-crm's migrations at their latest state.
func createSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			username TEXT UNIQUE NOT NULL, password TEXT NOT NULL, email TEXT UNIQUE NOT NULL,
			language TEXT DEFAULT 'en', is_admin INTEGER DEFAULT 0, date_format TEXT DEFAULT 'eu',
			custom_field_names TEXT DEFAULT '[]', enabled_contact_fields TEXT
		)`,
		`CREATE TABLE contacts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			firstname TEXT NOT NULL COLLATE NOCASE, lastname TEXT COLLATE NOCASE,
			nickname TEXT COLLATE NOCASE, gender TEXT, email TEXT COLLATE NOCASE,
			phone TEXT, birthday TEXT, photo TEXT, photo_thumbnail TEXT, address TEXT,
			how_we_met TEXT, food_preference TEXT, work_information TEXT,
			contact_information TEXT, circles TEXT, user_id INTEGER,
			vcard_uid TEXT, vcard_extra TEXT, etag TEXT,
			archived INTEGER DEFAULT 0 NOT NULL,
			emails TEXT DEFAULT '[]', phones TEXT DEFAULT '[]', addresses TEXT DEFAULT '[]',
			urls TEXT DEFAULT '[]', impps TEXT DEFAULT '[]',
			prefix TEXT DEFAULT '', middle_name TEXT DEFAULT '', suffix TEXT DEFAULT '',
			organization TEXT DEFAULT '', department TEXT DEFAULT '',
			job_title TEXT DEFAULT '', role TEXT DEFAULT '', anniversary TEXT DEFAULT '',
			custom_fields TEXT DEFAULT '{}'
		)`,
		`CREATE TABLE relationships (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			name TEXT NOT NULL, type TEXT NOT NULL, gender TEXT, birthday TEXT,
			contact_id INTEGER NOT NULL, related_contact_id INTEGER, user_id INTEGER
		)`,
		`CREATE TABLE notes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			content TEXT NOT NULL, date DATETIME NOT NULL, contact_id INTEGER, user_id INTEGER
		)`,
		`CREATE TABLE activities (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			title TEXT NOT NULL, description TEXT, location TEXT,
			date DATETIME NOT NULL, user_id INTEGER
		)`,
		`CREATE TABLE activity_contacts (
			activity_id INTEGER NOT NULL, contact_id INTEGER NOT NULL,
			PRIMARY KEY (activity_id, contact_id)
		)`,
		`CREATE TABLE reminders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
			message TEXT NOT NULL, by_mail INTEGER DEFAULT 0, remind_at DATETIME NOT NULL,
			recurrence TEXT NOT NULL, reoccur_from_completion INTEGER DEFAULT 1,
			last_sent DATETIME, contact_id INTEGER NOT NULL,
			completed BOOLEAN DEFAULT false NOT NULL, user_id INTEGER,
			email_sent BOOLEAN DEFAULT FALSE NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil { // # pragma: no cover — defensive
			return fmt.Errorf("meerkatfixture: creating schema: %w", err)
		}
	}
	return nil
}

func insertUsers(db *sql.DB, m *Manifest) error {
	seen := map[int]bool{m.User.ID: true}
	rows := []struct {
		id              int
		username, email string
	}{
		{m.User.ID, m.User.Username, m.User.Email},
	}
	for _, c := range m.Contacts {
		if c.UserID != 0 && !seen[c.UserID] {
			seen[c.UserID] = true
			rows = append(rows, struct {
				id              int
				username, email string
			}{
				c.UserID, "user" + itoa(c.UserID), "user" + itoa(c.UserID) + "@example.com",
			})
		}
	}
	for _, r := range rows {
		if _, err := db.Exec(
			`INSERT INTO users (id, created_at, updated_at, username, password, email)
			 VALUES (?, datetime('now'), datetime('now'), ?, ?, ?)`,
			r.id, r.username, "fixture-password", r.email,
		); err != nil { // # pragma: no cover — defensive
			return fmt.Errorf("meerkatfixture: inserting user %d: %w", r.id, err)
		}
	}
	return nil
}

func itoa(v int) string { return fmt.Sprintf("%d", v) }

func jsonOrNull(v any) any {
	b, err := json.Marshal(v)
	if err != nil { // # pragma: no cover — defensive
		return nil
	}
	return string(b)
}

func insertContacts(db *sql.DB, m *Manifest) error {
	for _, c := range m.Contacts {
		userID := c.UserID
		if userID == 0 {
			userID = m.User.ID
		}
		deletedAt := any(nil)
		if c.Deleted {
			deletedAt = "2024-01-01 00:00:00"
		}
		archived := 0
		if c.Archived { // # pragma: no cover — defensive
			archived = 1
		}
		_, err := db.Exec(`INSERT INTO contacts (
			id, created_at, updated_at, deleted_at,
			firstname, lastname, nickname, gender, email, phone, birthday, address,
			photo, photo_thumbnail,
			how_we_met, food_preference, work_information, contact_information,
			circles, user_id, vcard_uid, vcard_extra, archived,
			emails, phones, addresses, urls, impps,
			prefix, middle_name, suffix, organization, department, job_title, role,
			anniversary, custom_fields
		) VALUES (?, datetime('now'), datetime('now'), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.ID, deletedAt,
			c.Firstname, c.Lastname, c.Nickname, c.Gender, c.Email, c.Phone, c.Birthday, c.Address,
			c.Photo, c.PhotoThumb,
			c.HowWeMet, c.FoodPreference, c.WorkInfo, c.ContactInfo,
			jsonOrNull(c.Circles), userID, c.VCardUID, c.VCardExtra, archived,
			jsonOrNull(c.Emails), jsonOrNull(c.Phones), jsonOrNull(c.Addresses),
			jsonOrNull(c.URLs), jsonOrNull(c.IMPPs),
			c.Prefix, c.MiddleName, c.Suffix, c.Organization, c.Department, c.JobTitle, c.Role,
			c.Anniversary, jsonOrNull(c.CustomFields),
		)
		if err != nil { // # pragma: no cover — defensive
			return fmt.Errorf("meerkatfixture: inserting contact %q: %w", c.Name, err)
		}
	}
	return nil
}

func insertRelationships(db *sql.DB, m *Manifest) error {
	for _, r := range m.Relationships {
		contactID, _ := m.idByName(r.Contact)
		var related *int
		if !r.Dangling {
			id, _ := m.idByName(r.RelatedTo)
			related = &id
		}
		deletedAt := any(nil)
		if r.Deleted { // # pragma: no cover — defensive
			deletedAt = "2024-01-01 00:00:00"
		}
		_, err := db.Exec(`INSERT INTO relationships (
			id, created_at, updated_at, deleted_at, name, type,
			contact_id, related_contact_id, user_id
		) VALUES (?, datetime('now'), datetime('now'), ?, ?, ?, ?, ?, ?)`,
			r.ID, deletedAt, r.Name, r.Type, contactID, related, m.User.ID,
		)
		if err != nil { // # pragma: no cover — defensive
			return fmt.Errorf("meerkatfixture: inserting relationship %d: %w", r.ID, err)
		}
	}
	return nil
}

func insertNotes(db *sql.DB, m *Manifest) error {
	for _, n := range m.Notes {
		contactID, _ := m.idByName(n.Contact)
		deletedAt := any(nil)
		if n.Deleted {
			deletedAt = "2024-01-01 00:00:00"
		}
		_, err := db.Exec(`INSERT INTO notes (
			id, created_at, updated_at, deleted_at, content, date, contact_id, user_id
		) VALUES (?, datetime('now'), datetime('now'), ?, ?, ?, ?, ?)`,
			n.ID, deletedAt, n.Content, n.Date, contactID, m.User.ID,
		)
		if err != nil { // # pragma: no cover — defensive
			return fmt.Errorf("meerkatfixture: inserting note %d: %w", n.ID, err)
		}
	}
	return nil
}

func insertActivities(db *sql.DB, m *Manifest) error {
	for _, a := range m.Activities {
		deletedAt := any(nil)
		if a.Deleted { // # pragma: no cover — defensive
			deletedAt = "2024-01-01 00:00:00"
		}
		_, err := db.Exec(`INSERT INTO activities (
			id, created_at, updated_at, deleted_at, title, description, location, date, user_id
		) VALUES (?, datetime('now'), datetime('now'), ?, ?, ?, ?, ?, ?)`,
			a.ID, deletedAt, a.Title, a.Description, a.Location, a.Date, m.User.ID,
		)
		if err != nil { // # pragma: no cover — defensive
			return fmt.Errorf("meerkatfixture: inserting activity %d: %w", a.ID, err)
		}
		for _, c := range a.Contacts {
			contactID, _ := m.idByName(c)
			if _, err := db.Exec(
				`INSERT INTO activity_contacts (activity_id, contact_id) VALUES (?, ?)`,
				a.ID, contactID,
			); err != nil { // # pragma: no cover — defensive
				return fmt.Errorf("meerkatfixture: linking activity %d: %w", a.ID, err)
			}
		}
	}
	return nil
}

func insertReminders(db *sql.DB, m *Manifest) error {
	for _, r := range m.Reminders {
		contactID, _ := m.idByName(r.Contact)
		deletedAt := any(nil)
		if r.Deleted { // # pragma: no cover — defensive
			deletedAt = "2024-01-01 00:00:00"
		}
		reoccur := 1
		if !r.ReoccurFromCompletion {
			reoccur = 0
		}
		_, err := db.Exec(`INSERT INTO reminders (
			id, created_at, updated_at, deleted_at, message, remind_at, recurrence,
			reoccur_from_completion, contact_id, user_id
		) VALUES (?, datetime('now'), datetime('now'), ?, ?, ?, ?, ?, ?, ?)`,
			r.ID, deletedAt, r.Message, r.RemindAt, r.Recurrence, reoccur, contactID, m.User.ID,
		)
		if err != nil { // # pragma: no cover — defensive
			return fmt.Errorf("meerkatfixture: inserting reminder %d: %w", r.ID, err)
		}
	}
	return nil
}
