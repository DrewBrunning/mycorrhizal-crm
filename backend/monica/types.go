// Package monica is a read-only client and data model for the Monica CRM REST
// API (https://www.monicahq.com/api), used by the Monica source import
// (mapping: issue #351; assistant: issue #549).
//
// types.go is the wire model plus snapshot loading — the shapes the mapping
// (services/monica_import.go) consumes. client.go (issue #549) adds the live
// transport: a rate-limited, SSRF-guarded, paginating client that produces a
// Snapshot from a running Monica instance. The backend mapping operates on a
// Snapshot regardless of origin, so the checked-in fixture
// (testdata/monica-fixture/snapshot.json) still proves the mapping without a
// live instance.
//
// Wire structs only declare the fields the mapping consumes and keep
// everything optional/tolerant, since payloads vary slightly across Monica
// versions.
package monica

import (
	"encoding/json"
	"fmt"
	"io"
)

// Meta is the pagination block returned by Monica list endpoints.
type Meta struct {
	CurrentPage int `json:"current_page"`
	LastPage    int `json:"last_page"`
	Total       int `json:"total"`
}

// Paged is the envelope of every Monica list response.
type Paged[T any] struct {
	Data []T  `json:"data"`
	Meta Meta `json:"meta"`
}

// SpecialDate is Monica's flexible date type for birthdays and similar dates:
// the date may be exact, year-unknown (month/day only), or derived from an age.
type SpecialDate struct {
	Date          *string `json:"date"` // RFC3339 or YYYY-MM-DD, nil if unknown
	IsAgeBased    bool    `json:"is_age_based"`
	IsYearUnknown bool    `json:"is_year_unknown"`
}

// Tag is a Monica tag.
type Tag struct {
	Name string `json:"name"`
}

// Address is a Monica postal address.
type Address struct {
	Name       string `json:"name"` // label, e.g. "home"
	Street     string `json:"street"`
	City       string `json:"city"`
	Province   string `json:"province"`
	PostalCode string `json:"postal_code"`
	Country    *struct {
		Name string `json:"name"`
	} `json:"country"`
}

// ContactField is a Monica contact information entry (email, phone, social
// handle, ...); the type is fully denormalized into contact_field_type.
type ContactField struct {
	Content          string `json:"content"`
	ContactFieldType struct {
		Name     string  `json:"name"`     // "Email", "Phone", "Twitter", ...
		Protocol *string `json:"protocol"` // "mailto:", "tel:", or nil
		Type     *string `json:"type"`     // "email", "phone", or nil
	} `json:"contact_field_type"`
}

// ContactRef is the minimal contact reference embedded in related entities.
type ContactRef struct {
	ID           int    `json:"id"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	CompleteName string `json:"complete_name"`
}

// Contact is a Monica contact as returned by GET /contacts?with=contactfields.
type Contact struct {
	ID              int    `json:"id"`
	FirstName       string `json:"first_name"`
	LastName        string `json:"last_name"`
	Nickname        string `json:"nickname"`
	Gender          string `json:"gender"`      // display name, e.g. "Man"
	GenderType      string `json:"gender_type"` // "M", "F", "O"
	IsPartial       bool   `json:"is_partial"`
	IsDead          bool   `json:"is_dead"`
	IsStarred       bool   `json:"is_starred"`
	Description     string `json:"description"`
	FoodPreferences string `json:"food_preferences"`
	Information     struct {
		Dates struct {
			Birthdate    SpecialDate `json:"birthdate"`
			DeceasedDate SpecialDate `json:"deceased_date"`
		} `json:"dates"`
		Career struct {
			Job     *string `json:"job"`
			Company *string `json:"company"`
		} `json:"career"`
		Avatar struct {
			URL    *string `json:"url"`
			Source *string `json:"source"` // "default", "photo", "gravatar", ...
		} `json:"avatar"`
		HowYouMet struct {
			GeneralInformation *string `json:"general_information"`
		} `json:"how_you_met"`
	} `json:"information"`
	Tags          []Tag          `json:"tags"`
	Addresses     []Address      `json:"addresses"`
	ContactFields []ContactField `json:"contactFields"`
}

// Activity is a shared event linked to one or more contacts.
type Activity struct {
	ID           int    `json:"id"`
	Summary      string `json:"summary"`
	Description  string `json:"description"`
	HappenedAt   string `json:"happened_at"` // YYYY-MM-DD
	ActivityType *struct {
		Name string `json:"name"`
	} `json:"activity_type"`
	Attendees struct {
		Contacts []ContactRef `json:"contacts"`
	} `json:"attendees"`
}

// Note is a free-form note attached to a contact.
type Note struct {
	ID          int         `json:"id"`
	Body        string      `json:"body"`
	IsFavorited bool        `json:"is_favorited"`
	CreatedAt   string      `json:"created_at"`
	Contact     *ContactRef `json:"contact"`
}

// Reminder is a one-time or recurring reminder for a contact.
type Reminder struct {
	ID               int         `json:"id"`
	Title            string      `json:"title"`
	Description      string      `json:"description"`
	FrequencyType    string      `json:"frequency_type"` // one_time, week, month, year
	FrequencyNumber  int         `json:"frequency_number"`
	InitialDate      *string     `json:"initial_date"`
	NextExpectedDate *string     `json:"next_expected_date"`
	Contact          *ContactRef `json:"contact"`
}

// Call is a logged phone call with a contact.
type Call struct {
	ID       int         `json:"id"`
	Content  string      `json:"content"`
	CalledAt string      `json:"called_at"`
	Contact  *ContactRef `json:"contact"`
}

// Task is a to-do item, optionally linked to a contact.
type Task struct {
	ID          int         `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Completed   bool        `json:"completed"`
	CompletedAt *string     `json:"completed_at"`
	CreatedAt   string      `json:"created_at"`
	Contact     *ContactRef `json:"contact"`
}

// Gift is a gift idea or given/received gift for a contact.
type Gift struct {
	ID      int     `json:"id"`
	Name    string  `json:"name"`
	Comment string  `json:"comment"`
	URL     string  `json:"url"`
	Amount  *string `json:"amount"`
	Status  string  `json:"status"` // "idea", "offered", "received" (newer versions)
	Date    *string `json:"date"`
	// Older Monica versions use booleans instead of status.
	IsAnIdea        *bool       `json:"is_an_idea"`
	HasBeenOffered  *bool       `json:"has_been_offered"`
	HasBeenReceived *bool       `json:"has_been_received"`
	CreatedAt       string      `json:"created_at"`
	Contact         *ContactRef `json:"contact"`
}

// Debt is money owed to or by a contact.
type Debt struct {
	ID                 int         `json:"id"`
	InDebt             string      `json:"in_debt"` // "yes" = user owes the contact
	Status             string      `json:"status"`
	Amount             float64     `json:"amount"`
	AmountWithCurrency string      `json:"amount_with_currency"`
	Reason             string      `json:"reason"`
	CreatedAt          string      `json:"created_at"`
	Contact            *ContactRef `json:"contact"`
}

// Relationship links two Monica contacts. "contact_is" IS the
// relationship_type of "of_contact" (e.g. John is the father of Jane): the
// row is listed on the subject's page (snapshot.Relationships[subjectID]),
// contact_is is the subject, of_contact is the other party, and the type
// describes how the subject relates to the other party.
type Relationship struct {
	RelationshipType struct {
		Name string `json:"name"`
	} `json:"relationship_type"`
	ContactIs *ContactRef `json:"contact_is"`
	OfContact *ContactRef `json:"of_contact"`
}

// EntityCounts holds the per-entity totals Monica reports (meta.total on each
// list endpoint), used for the pre-fetch summary the assistant shows and for
// the fetch-duration estimate.
type EntityCounts struct {
	Contacts   int `json:"contacts"`
	Activities int `json:"activities"`
	Notes      int `json:"notes"`
	Reminders  int `json:"reminders"`
	Calls      int `json:"calls"`
	Tasks      int `json:"tasks"`
	Gifts      int `json:"gifts"`
	Debts      int `json:"debts"`
}

// Total is the number of records a full fetch would hold in memory.
func (c EntityCounts) Total() int {
	return c.Contacts + c.Activities + c.Notes + c.Reminders +
		c.Calls + c.Tasks + c.Gifts + c.Debts
}

// Snapshot is the complete in-memory copy of a Monica account's importable
// data, in the shape the live fetch produces. Relationships are keyed by the
// subject contact's Monica ID.
type Snapshot struct {
	Contacts      []Contact
	Activities    []Activity
	Notes         []Note
	Reminders     []Reminder
	Calls         []Call
	Tasks         []Task
	Gifts         []Gift
	Debts         []Debt
	Relationships map[int][]Relationship
}

// LoadSnapshot parses a Snapshot from JSON (the fixture format). Unknown
// fields are ignored so a fixture written for a newer Monica version still
// loads.
func LoadSnapshot(r io.Reader) (*Snapshot, error) {
	snap := &Snapshot{Relationships: map[int][]Relationship{}}
	dec := json.NewDecoder(r)
	if err := dec.Decode(snap); err != nil {
		return nil, fmt.Errorf("monica: parsing snapshot: %w", err)
	}
	if snap.Relationships == nil {
		snap.Relationships = map[int][]Relationship{}
	}
	return snap, nil
}
