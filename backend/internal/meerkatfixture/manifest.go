// Package meerkatfixture is the Go loader for the shared Meerkat source-import
// fixture (issue #353).
//
// A Meerkat import reads a *Meerkat-format SQLite database* — that is the
// chosen import path (ADR 0007). The fixture for it therefore cannot be a
// check-in binary: like the canonical fixture (issue #430) and the contract
// fixtures (issue #266), it is a reviewable JSON manifest
// (testdata/meerkat-fixture/manifest.json) plus a loader. This package builds
// a real Meerkat-schema SQLite file from the manifest — a database the actual
// meerkat.Open + MapMeerkatSnapshot + ExecuteSourceImport pipeline consumes
// exactly as it would a production deployment's file. See
// testdata/meerkat-fixture/README.md for the contract.
//
// The manifest describes the Meerkat tables the import cares about (contacts,
// relationships, notes, activities, activity_contacts, reminders) in Meerkat's
// own schema terms, including the pathological shapes the canonical fixture
// demands where Meerkat can express them: multi-valued contact fields, custom
// fields, reciprocal and dangling relationships, Unicode, very long values,
// empty/null values, soft-deleted rows, and a second source user the importer
// must not silently mix into one local account.
package meerkatfixture

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ManifestVersion is the version of the manifest schema this loader
// understands; the manifest carries a matching "version" field (the #430
// "version it" requirement).
const ManifestVersion = 1

// ManifestRelPath is the manifest's repo-root-relative path.
const ManifestRelPath = "testdata/meerkat-fixture/manifest.json"

// Manifest is the parsed form of testdata/meerkat-fixture/manifest.json.
type Manifest struct {
	Version       int               `json:"version"`
	Description   string            `json:"description,omitempty"`
	User          ManifestUser      `json:"user"`
	Contacts      []ContactEntry    `json:"contacts"`
	Relationships []RelationshipRow `json:"relationships,omitempty"`
	Notes         []NoteRow         `json:"notes,omitempty"`
	Activities    []ActivityRow     `json:"activities,omitempty"`
	Reminders     []ReminderRow     `json:"reminders,omitempty"`
}

// ManifestUser is the source user every imported row is scoped to (the
// fixture's meerkat users table has exactly this one row, mirroring the
// canonical fixture's single-user arrangement).
type ManifestUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// ContactEntry is one contacts row in Meerkat's own schema terms. Every field
// mirrors a column the reader (meerkat package) exposes, so the loader writes
// the same bytes a real Meerkat deployment would.
type ContactEntry struct {
	Name string `json:"name"` // manifest-local name, used by cross-references only

	ID             int    `json:"id"`
	UserID         int    `json:"user_id,omitempty"` // defaults to the manifest user; a different value creates a second source user (multi-user filter pin)
	Deleted        bool   `json:"deleted,omitempty"`
	Firstname      string `json:"firstname,omitempty"`
	Lastname       string `json:"lastname,omitempty"`
	Nickname       string `json:"nickname,omitempty"`
	Gender         string `json:"gender,omitempty"`
	Email          string `json:"email,omitempty"`
	Phone          string `json:"phone,omitempty"`
	Birthday       string `json:"birthday,omitempty"`
	Address        string `json:"address,omitempty"`
	Photo          string `json:"photo,omitempty"`
	PhotoThumb     string `json:"photo_thumbnail,omitempty"`
	HowWeMet       string `json:"how_we_met,omitempty"`
	FoodPreference string `json:"food_preference,omitempty"`
	WorkInfo       string `json:"work_information,omitempty"`
	ContactInfo    string `json:"contact_information,omitempty"`
	VCardUID       string `json:"vcard_uid,omitempty"`
	VCardExtra     string `json:"vcard_extra,omitempty"`
	Archived       bool   `json:"archived,omitempty"`
	Prefix         string `json:"prefix,omitempty"`
	MiddleName     string `json:"middle_name,omitempty"`
	Suffix         string `json:"suffix,omitempty"`
	Organization   string `json:"organization,omitempty"`
	Department     string `json:"department,omitempty"`
	JobTitle       string `json:"job_title,omitempty"`
	Role           string `json:"role,omitempty"`
	Anniversary    string `json:"anniversary,omitempty"`

	// Multi-value JSON columns, serialized the way Meerkat's gorm stores
	// them.
	Emails    []ValueType `json:"emails,omitempty"`
	Phones    []ValueType `json:"phones,omitempty"`
	URLs      []ValueType `json:"urls,omitempty"`
	IMPPs     []ValueType `json:"impps,omitempty"`
	Addresses []Address   `json:"addresses,omitempty"`

	// Circles is Meerkat's grouping column (a JSON array of names).
	Circles []string `json:"circles,omitempty"`
	// CustomFields is Meerkat's user-defined key/value column.
	CustomFields map[string]string `json:"custom_fields,omitempty"`
}

// ValueType is Meerkat's serialized email/phone/url/impp entry shape
// ({type, value}).
type ValueType struct {
	Type  string `json:"type,omitempty"`
	Value string `json:"value,omitempty"`
}

// Address is Meerkat's serialized ContactAddress shape.
type Address struct {
	Type    string `json:"type,omitempty"`
	Street  string `json:"street,omitempty"`
	City    string `json:"city,omitempty"`
	Region  string `json:"region,omitempty"`
	Postal  string `json:"postal,omitempty"`
	Country string `json:"country,omitempty"`
}

// RelationshipRow is one relationships row (the legacy flat table). Contacts
// are referenced by their manifest name.
type RelationshipRow struct {
	ID        int    `json:"id"`
	Contact   string `json:"contact"` // owner
	RelatedTo string `json:"related_to"`
	Type      string `json:"type"`
	Name      string `json:"name,omitempty"` // the other party's name when RelatedTo is ""
	Dangling  bool   `json:"dangling,omitempty"`
	Deleted   bool   `json:"deleted,omitempty"`
}

// NoteRow is one notes row.
type NoteRow struct {
	ID      int    `json:"id"`
	Contact string `json:"contact"`
	Content string `json:"content"`
	Date    string `json:"date"`
	Deleted bool   `json:"deleted,omitempty"`
}

// ActivityRow is one activities row plus its attendees (activity_contacts).
type ActivityRow struct {
	ID          int      `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Location    string   `json:"location,omitempty"`
	Date        string   `json:"date"`
	Contacts    []string `json:"contacts,omitempty"`
	Deleted     bool     `json:"deleted,omitempty"`
}

// ReminderRow is one reminders row.
type ReminderRow struct {
	ID                    int    `json:"id"`
	Contact               string `json:"contact"`
	Message               string `json:"message"`
	RemindAt              string `json:"remind_at"`
	Recurrence            string `json:"recurrence"`
	ReoccurFromCompletion bool   `json:"reoccur_from_completion,omitempty"`
	Deleted               bool   `json:"deleted,omitempty"`
}

// Load parses a manifest from r and validates its cross-references.
func Load(r io.Reader) (*Manifest, error) {
	var m Manifest
	if err := json.NewDecoder(r).Decode(&m); err != nil { // # pragma: no cover — defensive
		return nil, fmt.Errorf("meerkatfixture: parsing manifest: %w", err)
	}
	if err := m.Validate(); err != nil { // # pragma: no cover — defensive
		return nil, err
	}
	return &m, nil
}

// Read loads the checked-in manifest by locating the repo root from the
// current working directory (mirroring canonicalfixture.Read).
func Read() (*Manifest, error) {
	path, err := FindManifest()
	if err != nil { // # pragma: no cover — defensive
		return nil, err // # pragma: no cover — a test CWD is always inside the repo
	}
	f, err := os.Open(path) // #nosec G304 -- checked-in manifest resolved by walking up from the test process's own CWD
	if err != nil {         // # pragma: no cover — defensive
		return nil, fmt.Errorf("meerkatfixture: opening %s: %w", path, err) // # pragma: no cover
	}
	defer f.Close()
	return Load(f)
}

// FindManifest locates testdata/meerkat-fixture/manifest.json by walking up
// from the current working directory.
func FindManifest() (string, error) {
	dir, err := os.Getwd()
	if err != nil { // # pragma: no cover — defensive
		return "", fmt.Errorf("meerkatfixture: resolving working directory: %w", err) // # pragma: no cover
	}
	for {
		candidate := filepath.Join(dir, ManifestRelPath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir { // # pragma: no cover — defensive
			return "", fmt.Errorf("meerkatfixture: manifest %s not found from %s (run tests from the backend/ dir or the repo root)", ManifestRelPath, dir) // # pragma: no cover
		}
		dir = parent
	}
}

// Validate checks the manifest's version and cross-references.
func (m *Manifest) Validate() error {
	if m.Version != ManifestVersion { // # pragma: no cover — defensive
		return fmt.Errorf("meerkatfixture: unsupported manifest version %d (this loader understands %d)", m.Version, ManifestVersion)
	}
	if m.User.Username == "" || m.User.Email == "" { // # pragma: no cover — defensive
		return fmt.Errorf("meerkatfixture: manifest user must set username and email")
	}
	if len(m.Contacts) == 0 { // # pragma: no cover — defensive
		return fmt.Errorf("meerkatfixture: manifest declares no contacts")
	}

	names := make(map[string]bool, len(m.Contacts))
	ids := make(map[int]bool, len(m.Contacts))
	for i, c := range m.Contacts {
		if c.Name == "" { // # pragma: no cover — defensive
			return fmt.Errorf("meerkatfixture: contact %d has no name", i)
		}
		if names[c.Name] { // # pragma: no cover — defensive
			return fmt.Errorf("meerkatfixture: duplicate contact name %q", c.Name)
		}
		if ids[c.ID] { // # pragma: no cover — defensive
			return fmt.Errorf("meerkatfixture: duplicate contact id %d", c.ID)
		}
		names[c.Name] = true
		ids[c.ID] = true
	}

	ref := func(section, name string) error {
		if !names[name] { // # pragma: no cover — defensive
			return fmt.Errorf("meerkatfixture: %s references unknown contact %q", section, name)
		}
		return nil
	}
	idOf := func(section, name string) (int, error) {
		if err := ref(section, name); err != nil { // # pragma: no cover — defensive
			return 0, err
		}
		for _, c := range m.Contacts {
			if c.Name == name {
				return c.ID, nil
			}
		}
		return 0, fmt.Errorf("meerkatfixture: %s references contact %q without an id", section, name) // # pragma: no cover
	}
	var errs []error
	for _, r := range m.Relationships {
		if _, err := idOf("relationship contact", r.Contact); err != nil { // # pragma: no cover — defensive
			errs = append(errs, err)
		}
		if !r.Dangling {
			if _, err := idOf("relationship related_to", r.RelatedTo); err != nil { // # pragma: no cover — defensive
				errs = append(errs, err)
			}
		}
	}
	for _, n := range m.Notes {
		if _, err := idOf("note contact", n.Contact); err != nil { // # pragma: no cover — defensive
			errs = append(errs, err)
		}
	}
	for _, a := range m.Activities {
		for _, c := range a.Contacts {
			if _, err := idOf("activity contact", c); err != nil { // # pragma: no cover — defensive
				errs = append(errs, err)
			}
		}
	}
	for _, r := range m.Reminders {
		if _, err := idOf("reminder contact", r.Contact); err != nil { // # pragma: no cover — defensive
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 { // # pragma: no cover — defensive
		return fmt.Errorf("meerkatfixture: manifest validation failed: %w", errors.Join(errs...))
	}
	return nil
}

// idByName returns the manifest contact ID for a manifest contact name.
func (m *Manifest) idByName(name string) (int, bool) {
	for _, c := range m.Contacts {
		if c.Name == name {
			return c.ID, true
		}
	}
	return 0, false // # pragma: no cover — Validate already proved every reference resolves
}
