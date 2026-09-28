package models

import (
	"encoding/json"
	"time"

	"mycorrhizal/contactmodel"
)

// The account bundle (issue #1259, ADR 0028 Decision 3) is the full-fidelity,
// re-importable, per-user export of everything the app stores, shaped after
// services.ImportSourcePlan so the same import engine can land it. It is the
// user's own data going to the user's own destination (another server, a fresh
// local profile, a file), so — like the flat CSV export (issue #861) — it
// includes every sensitivity level and `status: suggested` rows with no
// include_sensitive opt-in. Withholding there would be silent data loss.
//
// Every entity carries its stable portable ID: contacts by vcard_uid, UUID-PK
// entities by their UUID primary key, activities by Activity.UUID, and
// notes/reminders/reminder_completions by their uuid column (migration 000067,
// issue #1260). Cross-entity references use those stable IDs, never uint PKs,
// so the bundle is independent of the destination's row ids.
const (
	// AccountBundleFormat is the `format` discriminator a client must check
	// before treating a file as a bundle.
	AccountBundleFormat = "mycorrhizal-account"
	// AccountBundleVersion is the only bundle version `mycorrhizal` import
	// accepts; any other value is rejected 422.
	AccountBundleVersion = 1
)

// AccountBundle is the top-level wire document. The response body carries the
// same shape written to a file by the Android "Export account bundle" flow.
type AccountBundle struct {
	Format      string                    `json:"format"`
	Version     int                       `json:"version"`
	ExportedAt  time.Time                 `json:"exported_at"`
	Plan        AccountBundlePlan         `json:"plan"`
	Attachments []AccountBundleAttachment `json:"attachments"`
	Omitted     AccountBundleOmissions    `json:"omitted"`
}

// AccountBundleOmissions records what the bundle deliberately did not carry,
// so nothing is dropped silently.
type AccountBundleOmissions struct {
	// Attachments is always true in v1: attachment bytes are not embedded,
	// only their metadata (above) travels.
	Attachments bool `json:"attachments"`
	// PhotosOmitted counts profile photos dropped for exceeding the size cap.
	PhotosOmitted int `json:"photos_omitted"`
}

// AccountBundleAttachment is attachment metadata only — the bytes live on
// disk and are not embedded (ADR 0028 Decision 3).
type AccountBundleAttachment struct {
	ID              uint      `json:"id"`
	ContactVCardUID string    `json:"contact_vcard_uid"`
	OriginalName    string    `json:"original_name"`
	ContentType     string    `json:"content_type"`
	SizeBytes       int64     `json:"size_bytes"`
	CreatedAt       time.Time `json:"created_at"`
}

// AccountBundlePlan is the entity payload, mirroring services.ImportSourcePlan
// and extending it to every user-authored entity the plan did not yet cover.
// Collections are deliberately never nil when marshalled (the DTOs are built
// with make), because the import mapper distinguishes an absent section from
// an empty one nowhere — it just reads zero rows (CLAUDE.md frontend trap #8).
type AccountBundlePlan struct {
	Contacts               []AccountBundleContact            `json:"contacts"`
	Relationships          []AccountBundleRelationship       `json:"relationships"`
	Notes                  []AccountBundleNote               `json:"notes"`
	Reminders              []AccountBundleReminder           `json:"reminders"`
	ReminderCompletions    []AccountBundleReminderCompletion `json:"reminder_completions"`
	Activities             []AccountBundleActivity           `json:"activities"`
	LifeEvents             []AccountBundleLifeEvent          `json:"life_events"`
	Gifts                  []AccountBundleGift               `json:"gifts"`
	Preferences            []AccountBundlePreference         `json:"preferences"`
	ConversationAgenda     []AccountBundleAgendaItem         `json:"conversation_agenda"`
	CadencePolicies        []AccountBundleCadencePolicy      `json:"cadence_policies"`
	DataDecayPolicies      []AccountBundleDataDecayPolicy    `json:"data_decay_policies"`
	Households             []AccountBundleHousehold          `json:"households"`
	Circles                []AccountBundleCircle             `json:"circles"`
	Tags                   []AccountBundleTag                `json:"tags"`
	CustomFieldDefinitions []AccountBundleFieldDefinition    `json:"custom_field_definitions"`
	CustomFieldValues      []AccountBundleFieldValue         `json:"custom_field_values"`
	Occasions              []AccountBundleOccasion           `json:"occasions"`
	OccasionEvents         []AccountBundleOccasionEvent      `json:"occasion_events"`
}

// AccountBundleContact carries the neutral Record (via RecordForContact, never
// RecordFromContact — CLAUDE.md backend trap 3) plus the CRM-local flags with
// no neutral home.
type AccountBundleContact struct {
	UID          string                   `json:"uid"`
	ETag         string                   `json:"etag,omitempty"`
	Gender       string                   `json:"gender,omitempty"`
	Card         contactmodel.Card        `json:"card"`
	CRM          contactmodel.CRMEnvelope `json:"crm"`
	Passthrough  contactmodel.Passthrough `json:"passthrough,omitempty"`
	Archived     bool                     `json:"archived"`
	IsFavorite   bool                     `json:"is_favorite"`
	PhotoOmitted bool                     `json:"photo_omitted,omitempty"`
}

// AccountBundleRelationship is one directed edge; the stable IDs are contact
// vcard_uids and the edge keeps its own UUID primary key.
type AccountBundleRelationship struct {
	ID          string                 `json:"id"`
	SourceID    string                 `json:"source_id"`
	TargetID    string                 `json:"target_id"`
	Type        string                 `json:"type"`
	Directional bool                   `json:"directional"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Source      string                 `json:"source"`
	Confidence  float64                `json:"confidence"`
	Status      string                 `json:"status"`
	Sensitivity string                 `json:"sensitivity"`
}

// AccountBundleNote is one note; contact_uid is the owning contact's vcard_uid
// (Nullable in the model, so optional here).
type AccountBundleNote struct {
	UUID       string    `json:"uuid"`
	ContactUID string    `json:"contact_uid,omitempty"`
	Content    string    `json:"content"`
	Date       time.Time `json:"date"`
}

// AccountBundleReminder is one reminder. ReminderUUID is its own stable ID;
// LifeEventID / OccasionID reference stable IDs of those entities.
type AccountBundleReminder struct {
	UUID                  string     `json:"uuid"`
	ContactUID            string     `json:"contact_uid"`
	Message               string     `json:"message"`
	ByMail                *bool      `json:"by_mail,omitempty"`
	RemindAt              time.Time  `json:"remind_at"`
	Recurrence            string     `json:"recurrence"`
	ReoccurFromCompletion *bool      `json:"reoccur_from_completion,omitempty"`
	Completed             bool       `json:"completed"`
	LastSent              *time.Time `json:"last_sent,omitempty"`
	LifeEventID           *string    `json:"life_event_id,omitempty"`
	OccasionObligationID  *string    `json:"occasion_obligation_id,omitempty"`
}

// AccountBundleReminderCompletion is one completion. ReminderUUID references
// the reminder's stable UUID; ContactUID the subject contact.
type AccountBundleReminderCompletion struct {
	UUID         string    `json:"uuid"`
	ReminderUUID string    `json:"reminder_uuid,omitempty"`
	ContactUID   string    `json:"contact_uid"`
	Message      string    `json:"message"`
	CompletedAt  time.Time `json:"completed_at"`
}

// AccountBundleActivity is one shared activity; attendee_uids are contact
// vcard_uids.
type AccountBundleActivity struct {
	UUID         string    `json:"uuid"`
	Title        string    `json:"title"`
	Description  string    `json:"description,omitempty"`
	Location     string    `json:"location,omitempty"`
	Date         time.Time `json:"date"`
	Type         string    `json:"type,omitempty"`
	ExternalRef  string    `json:"external_ref,omitempty"`
	AttendeeUIDs []string  `json:"attendee_uids"`
}

// AccountBundleLifeEvent is one life event keyed by its UUID primary key.
type AccountBundleLifeEvent struct {
	ID               string                    `json:"id"`
	EntityID         string                    `json:"entity_id"`
	Type             string                    `json:"type,omitempty"`
	Category         string                    `json:"category,omitempty"`
	Date             *contactmodel.PartialDate `json:"date,omitempty"`
	EndDate          *contactmodel.PartialDate `json:"end_date,omitempty"`
	Description      string                    `json:"description,omitempty"`
	Source           string                    `json:"source,omitempty"`
	RelatedEntityIDs []string                  `json:"related_entity_ids,omitempty"`
	Remind           bool                      `json:"remind"`
}

// AccountBundleGift is one gift keyed by its UUID primary key.
type AccountBundleGift struct {
	ID           string     `json:"id"`
	EntityID     string     `json:"entity_id"`
	Status       string     `json:"status"`
	Occasion     string     `json:"occasion,omitempty"`
	Description  string     `json:"description"`
	URL          string     `json:"url,omitempty"`
	Notes        string     `json:"notes,omitempty"`
	Date         *time.Time `json:"date,omitempty"`
	ValueCents   int64      `json:"value_cents,omitempty"`
	Currency     string     `json:"currency,omitempty"`
	LifeEventID  string     `json:"life_event_id,omitempty"`
	ActivityUUID string     `json:"activity_uuid,omitempty"`
}

// AccountBundlePreference is one preference keyed by its UUID primary key.
type AccountBundlePreference struct {
	ID            string     `json:"id"`
	EntityID      string     `json:"entity_id"`
	Category      string     `json:"category"`
	Key           string     `json:"key,omitempty"`
	Value         string     `json:"value"`
	Level         *string    `json:"level,omitempty"`
	Notes         string     `json:"notes,omitempty"`
	Source        string     `json:"source,omitempty"`
	Confidence    *float64   `json:"confidence,omitempty"`
	LastConfirmed *time.Time `json:"last_confirmed,omitempty"`
	Sensitivity   string     `json:"sensitivity"`
}

// AccountBundleAgendaItem is one conversation-agenda item keyed by its UUID.
type AccountBundleAgendaItem struct {
	ID           string     `json:"id"`
	EntityID     string     `json:"entity_id"`
	Content      string     `json:"content"`
	ReferenceURL string     `json:"reference_url,omitempty"`
	DiscussedAt  *time.Time `json:"discussed_at,omitempty"`
	ActivityUUID string     `json:"activity_uuid,omitempty"`
}

// AccountBundleCadencePolicy is one cadence policy keyed by its UUID.
type AccountBundleCadencePolicy struct {
	ID                 string   `json:"id"`
	EntityID           string   `json:"entity_id"`
	TargetIntervalDays int      `json:"target_interval_days"`
	QualifyingTypes    []string `json:"qualifying_types,omitempty"`
}

// AccountBundleDataDecayPolicy is one data-decay policy keyed by its UUID.
type AccountBundleDataDecayPolicy struct {
	ID             string     `json:"id"`
	EntityID       string     `json:"entity_id"`
	IntervalDays   int        `json:"interval_days"`
	LastVerifiedAt *time.Time `json:"last_verified_at,omitempty"`
	Active         bool       `json:"active"`
}

// AccountBundleHousehold is one household keyed by its UUID, with members keyed
// by contact vcard_uid.
type AccountBundleHousehold struct {
	ID      string                         `json:"id"`
	Name    string                         `json:"name"`
	Type    string                         `json:"type"`
	Address *contactmodel.Address          `json:"address,omitempty"`
	Members []AccountBundleHouseholdMember `json:"members"`
}

// AccountBundleHouseholdMember is one membership in a household.
type AccountBundleHouseholdMember struct {
	MemberVCardUID string `json:"member_vcard_uid"`
	Role           string `json:"role,omitempty"`
	Since          string `json:"since,omitempty"`
	Until          string `json:"until,omitempty"`
}

// AccountBundleCircle is one circle keyed by its UUID, with member vcard_uids.
type AccountBundleCircle struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	MemberUIDs []string `json:"member_uids"`
}

// AccountBundleTag is one tag keyed by its UUID, with tagged contact uid.
type AccountBundleTag struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	ContactUIDs []string `json:"contact_uids"`
}

// AccountBundleFieldDefinition is the custom-field schema half, keyed by its
// UUID. Carried so an imported bundle re-creates the same typed definitions
// rather than inventing an internal-only one per value (the source-import
// engine's default).
type AccountBundleFieldDefinition struct {
	ID          string           `json:"id"`
	Label       string           `json:"label"`
	Key         string           `json:"key"`
	Target      string           `json:"target"`
	Type        string           `json:"type"`
	Constraints FieldConstraints `json:"constraints,omitempty"`
	Projection  string           `json:"projection"`
	Sensitivity string           `json:"sensitivity"`
	Position    int              `json:"position"`
}

// AccountBundleFieldValue is one custom-field value; the definition is
// referenced by stable ID and the entity by contact vcard_uid.
type AccountBundleFieldValue struct {
	FieldDefinitionID string          `json:"field_definition_id"`
	EntityID          string          `json:"entity_id"`
	Value             json.RawMessage `json:"value"`
}

// AccountBundleOccasion is one occasion obligation keyed by its UUID.
type AccountBundleOccasion struct {
	ID                string `json:"id"`
	EntityID          string `json:"entity_id"`
	Kind              string `json:"kind"`
	Label             string `json:"label"`
	AnchorMonth       *int   `json:"anchor_month,omitempty"`
	AnchorDay         *int   `json:"anchor_day,omitempty"`
	LinkedLifeEventID string `json:"linked_life_event_id,omitempty"`
	LeadTimeDays      int    `json:"lead_time_days"`
	Active            bool   `json:"active"`
	Sensitivity       string `json:"sensitivity"`
	Notes             string `json:"notes,omitempty"`
}

// AccountBundleOccasionEvent is one occasion event keyed by its UUID, with
// attendees keyed by contact vcard_uid.
type AccountBundleOccasionEvent struct {
	ID          string                  `json:"id"`
	Title       string                  `json:"title"`
	StartsAt    time.Time               `json:"starts_at"`
	EndsAt      *time.Time              `json:"ends_at,omitempty"`
	Location    string                  `json:"location,omitempty"`
	Sensitivity string                  `json:"sensitivity"`
	Notes       string                  `json:"notes,omitempty"`
	Attendees   []AccountBundleAttendee `json:"attendees"`
}

// AccountBundleAttendee is one occasion-event attendee.
type AccountBundleAttendee struct {
	EntityID string `json:"entity_id"`
	RSVP     string `json:"rsvp"`
}
