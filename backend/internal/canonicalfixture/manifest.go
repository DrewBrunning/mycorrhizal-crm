// Package canonicalfixture is the Go loader for the shared canonical
// pathological contact dataset (TEST-02, issue #430).
//
// The dataset itself is a hand-authored, reviewable JSON manifest living in
// testdata/canonical-fixture/manifest.json — the same "one canonical copy, each
// consumer points its own loader at it" arrangement as the contract fixtures
// (testdata/contract-fixtures/, issue #266), so a future TS/Kotlin loader can
// parse the same file rather than forking the dataset. This package is the Go
// consumer side: it parses the manifest, populates a real migrated database
// through the same code paths the REST API uses (ApplyRecordToContact, not
// direct field mutation — CLAUDE.md backend trap #2), and hands back a Dataset
// for tests to assert against.
//
// Every migration (MIG-01/02/03), import/export (DATA-01..04), round-trip
// (TEST-03), merge-golden (TEST-05) and performance (PERF-01) suite is expected
// to consume this fixture rather than defining its own contacts — the manifest
// is load-bearing, and the round-trip and trap tests in this package are what
// keep it that way. See testdata/canonical-fixture/README.md for the full
// contract.
package canonicalfixture

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"
)

// ManifestVersion is the version of the manifest schema this loader
// understands. The manifest carries a matching "version" field; bumping the
// schema means bumping both, and that diff is the reviewable record the
// "version it" requirement (issue #430) is about.
//
// Version 2 (the v1.2.0 fixture refresh) adds: the self-contact pointer,
// cadence policies, reach-out suggestions and occasion obligations/events
// sections; life-event end dates; preference proficiency levels; custom-field
// display positions; and the optional demo-relative `days_ago` timing on
// contacts and activities (resolved against an injectable clock, so the demo
// dataset stays fresh while tests pin a fixed reference time).
const ManifestVersion = 2

// ManifestRelPath is the manifest's repo-root-relative path, mirroring
// contractfixtures.FixturesDir's role for the contract fixtures.
const ManifestRelPath = "testdata/canonical-fixture/manifest.json"

// Manifest is the parsed form of testdata/canonical-fixture/manifest.json.
type Manifest struct {
	Version             int                       `json:"version"`
	Description         string                    `json:"description,omitempty"`
	User                ManifestUser              `json:"user"`
	SelfContact         string                    `json:"self_contact,omitempty"`
	Contacts            []ContactEntry            `json:"contacts"`
	Notes               []NoteEntry               `json:"notes,omitempty"`
	LifeEvents          []LifeEventEntry          `json:"life_events,omitempty"`
	Gifts               []GiftEntry               `json:"gifts,omitempty"`
	Relationships       []RelationshipEntry       `json:"relationships,omitempty"`
	Households          []HouseholdEntry          `json:"households,omitempty"`
	Circles             []CircleEntry             `json:"circles,omitempty"`
	Tags                []TagEntry                `json:"tags,omitempty"`
	CustomFields        []CustomFieldEntry        `json:"custom_fields,omitempty"`
	Preferences         []PreferenceEntry         `json:"preferences,omitempty"`
	ExternalIdentities  []ExternalIdentityEntry   `json:"external_identities,omitempty"`
	Attachments         []AttachmentEntry         `json:"attachments,omitempty"`
	Activities          []ActivityEntry           `json:"activities,omitempty"`
	CadencePolicies     []CadencePolicyEntry      `json:"cadence_policies,omitempty"`
	ReachOutSuggestions []ReachOutSuggestionEntry `json:"reach_out_suggestions,omitempty"`
	OccasionObligations []OccasionObligationEntry `json:"occasion_obligations,omitempty"`
	OccasionEvents      []OccasionEventEntry      `json:"occasion_events,omitempty"`
}

// ManifestUser is the user every row in the fixture is scoped to.
type ManifestUser struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

// ContactEntry is one contact in the manifest. Card/CRM/Passthrough are the
// neutral contactmodel.Record fields verbatim (same JSON shape the nested REST
// API accepts), so the loader can hand them to ApplyRecordToContact unchanged.
type ContactEntry struct {
	Name    string `json:"name"`
	Comment string `json:"comment,omitempty"`

	// SoftDeleted marks a contact that is created and then tombstoned exactly
	// the way DeleteContact does (its dependent rows soft/hard-deleted with
	// it) — the trap #7 "soft-deleted rows alongside live ones" dataset.
	SoftDeleted bool `json:"soft_deleted,omitempty"`

	// RecreatesVCardUIDOf names a soft-deleted contact whose vcard_uid this
	// contact deliberately re-uses, pinning the partial unique index
	// idx_contacts_vcard_uid_user (WHERE deleted_at IS NULL). The named
	// contact must appear earlier in the manifest and be soft_deleted.
	RecreatesVCardUIDOf string `json:"recreates_vcard_uid_of,omitempty"`

	// UpdatedDaysAgo, when set, back-dates the contact's updated_at to
	// `now - N days` after creation. It is the raw input to the health
	// score's LastUpdated facet (ADR 0023): a value >= 180 means the facet is
	// floored at 30 (russula-leaning), 0 means 100. Relative so the demo
	// dataset stays meaningful as wall-clock time advances; tests pin `now`
	// via PopulateAt.
	UpdatedDaysAgo *int `json:"updated_days_ago,omitempty"`

	// Favorite marks the contact as a dashboard favorite (issue #173) — a
	// CRM-local flag with no neutral-Card home, so it is a manifest-level
	// field rather than a card/envelope one.
	Favorite bool `json:"favorite,omitempty"`

	Card        contactmodel.Card        `json:"card"`
	CRM         contactmodel.CRMEnvelope `json:"crm"`
	Passthrough contactmodel.Passthrough `json:"passthrough,omitempty"`
}

// Record returns the entry as a neutral contactmodel.Record.
func (e ContactEntry) Record() *contactmodel.Record {
	return &contactmodel.Record{Card: e.Card, Envelope: e.CRM, Passthrough: e.Passthrough}
}

// NoteEntry is one Note row. Contact references the manifest contact by name.
type NoteEntry struct {
	Contact     string    `json:"contact"`
	Content     string    `json:"content"`
	Date        time.Time `json:"date"`
	SoftDeleted bool      `json:"soft_deleted,omitempty"`
}

// LifeEventEntry is one LifeEvent row. RelatedEntities are manifest contact
// names, resolved to VCardUIDs by the loader.
type LifeEventEntry struct {
	Contact  string                    `json:"contact"`
	Type     string                    `json:"type,omitempty"`
	Category string                    `json:"category,omitempty"`
	Date     *contactmodel.PartialDate `json:"date,omitempty"`
	// EndDate turns the event into a span ("worked at Acme 2019-2024"), per
	// ADR 0025 temporal periods.
	EndDate         *contactmodel.PartialDate `json:"end_date,omitempty"`
	Description     string                    `json:"description,omitempty"`
	Remind          bool                      `json:"remind,omitempty"`
	Source          string                    `json:"source,omitempty"`
	RelatedEntities []string                  `json:"related_entities,omitempty"`
}

// GiftEntry is one Gift row.
type GiftEntry struct {
	Contact     string     `json:"contact"`
	Status      string     `json:"status,omitempty"`
	Occasion    string     `json:"occasion,omitempty"`
	Description string     `json:"description"`
	URL         string     `json:"url,omitempty"`
	Notes       string     `json:"notes,omitempty"`
	Date        *time.Time `json:"date,omitempty"`
	ValueCents  int64      `json:"value_cents,omitempty"`
	Currency    string     `json:"currency,omitempty"`
	SoftDeleted bool       `json:"soft_deleted,omitempty"`
}

// RelationshipEntry is one RelationshipEdge. Source/Target are manifest
// contact names. Status/Sensitivity/Provenance default to
// confirmed/normal/user-confirmed when omitted, matching the model defaults.
type RelationshipEntry struct {
	Source      string         `json:"source"`
	Target      string         `json:"target"`
	Type        string         `json:"type"`
	Status      string         `json:"status,omitempty"`      // confirmed|suggested
	Sensitivity string         `json:"sensitivity,omitempty"` // normal|private|secret
	Provenance  string         `json:"provenance,omitempty"`  // RelationshipEdge.Source
	Directional bool           `json:"directional,omitempty"`
	Confidence  float64        `json:"confidence,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Comment     string         `json:"comment,omitempty"`
}

// HouseholdEntry is one Household plus its members.
type HouseholdEntry struct {
	Name    string                 `json:"name"`
	Type    string                 `json:"type"`
	Address *contactmodel.Address  `json:"address,omitempty"`
	Members []HouseholdMemberEntry `json:"members,omitempty"`
}

// HouseholdMemberEntry is one HouseholdMember row.
type HouseholdMemberEntry struct {
	Contact string `json:"contact"`
	Role    string `json:"role,omitempty"`
	Since   string `json:"since,omitempty"`
	Until   string `json:"until,omitempty"`
}

// CircleEntry is one Circle plus its members (manifest contact names).
type CircleEntry struct {
	Name    string   `json:"name"`
	Members []string `json:"members,omitempty"`
}

// TagEntry is one Tag plus the contacts tagged with it.
type TagEntry struct {
	Name     string   `json:"name"`
	Contacts []string `json:"contacts,omitempty"`
}

// CustomFieldEntry is one FieldDefinition plus its per-contact FieldValues.
type CustomFieldEntry struct {
	Key         string                  `json:"key"`
	Label       string                  `json:"label"`
	Type        string                  `json:"type"`
	Constraints models.FieldConstraints `json:"constraints,omitempty"`
	Projection  string                  `json:"projection,omitempty"`
	Sensitivity string                  `json:"sensitivity,omitempty"`
	// Position orders the definition in the user's custom-field list (issue
	// #1210). Omitted means the DB default 0.
	Position *int              `json:"position,omitempty"`
	Values   []FieldValueEntry `json:"values,omitempty"`
}

// FieldValueEntry is one FieldValue for the enclosing CustomFieldEntry.
type FieldValueEntry struct {
	Contact string          `json:"contact"`
	Value   json.RawMessage `json:"value"`
}

// PreferenceEntry is one Preference row.
type PreferenceEntry struct {
	Contact     string   `json:"contact"`
	Category    string   `json:"category"`
	Key         string   `json:"key,omitempty"`
	Value       string   `json:"value"`
	Notes       string   `json:"notes,omitempty"`
	Source      string   `json:"source,omitempty"`
	Confidence  *float64 `json:"confidence,omitempty"`
	Sensitivity string   `json:"sensitivity,omitempty"`
	// Level is the optional proficiency facet for hobby-shaped preferences
	// (issue #246): high|medium|low. Only meaningful when the category
	// supports a level (models.PreferenceCategorySupportsLevel).
	Level       *string `json:"level,omitempty"`
	SoftDeleted bool    `json:"soft_deleted,omitempty"`
}

// ExternalIdentityEntry is one ExternalIdentity row.
type ExternalIdentityEntry struct {
	Contact    string         `json:"contact"`
	System     string         `json:"system"`
	ExternalID string         `json:"external_id"`
	URL        string         `json:"url,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	SyncStatus string         `json:"sync_status,omitempty"`
}

// AttachmentEntry is one Attachment metadata row. The manifest stores metadata
// only (StoredName, OriginalName, ContentType, SizeBytes) — the physical bytes
// live on disk in the real app and are deliberately not part of this fixture;
// see README.md.
type AttachmentEntry struct {
	Contact      string `json:"contact"`
	StoredName   string `json:"stored_name"`
	OriginalName string `json:"original_name"`
	ContentType  string `json:"content_type"`
	SizeBytes    int64  `json:"size_bytes"`
	SoftDeleted  bool   `json:"soft_deleted,omitempty"`
}

// ActivityEntry is one Activity (interaction) row. Contacts are manifest
// contact names.
type ActivityEntry struct {
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Location    string    `json:"location,omitempty"`
	Date        time.Time `json:"date"`
	// DaysAgo, when set, is the demo-relative alternative to Date: the
	// activity happened `now - N days` ago, resolved against the PopulateAt
	// clock. Exactly one of Date/DaysAgo must be set. This is what lets the
	// fixture drive the health score's Recency/Frequency facets without the
	// bands drifting as wall-clock time advances.
	DaysAgo     *int     `json:"days_ago,omitempty"`
	Type        string   `json:"type,omitempty"`
	Contacts    []string `json:"contacts,omitempty"`
	ExternalRef string   `json:"external_ref,omitempty"`
	SoftDeleted bool     `json:"soft_deleted,omitempty"`
}

// CadencePolicyEntry is one CadencePolicy row: "stay in touch with this
// contact every N days" (T19). The health score's Recency/Frequency facets use
// TargetIntervalDays as the target interval (ADR 0023).
type CadencePolicyEntry struct {
	Contact            string   `json:"contact"`
	TargetIntervalDays int      `json:"target_interval_days"`
	QualifyingTypes    []string `json:"qualifying_types,omitempty"`
	SoftDeleted        bool     `json:"soft_deleted,omitempty"`
}

// ReachOutSuggestionEntry is one ReachOutSuggestion row (issue #177). In the
// fixture these are authored directly (the real rows are generated by the
// audit-diff detector); a `pending` row is a discrete health-score warning
// (ReachOut facet, fixed 30) and feeds the dashboard's reach-out block.
//
// AuditEventID is a provenance pointer with no DB foreign key; the fixture
// defaults it to 0 when omitted.
type ReachOutSuggestionEntry struct {
	Contact      string `json:"contact"`
	Kind         string `json:"kind"` // organization|title|address
	OldValue     string `json:"old_value,omitempty"`
	NewValue     string `json:"new_value,omitempty"`
	Status       string `json:"status,omitempty"` // pending|dismissed, default pending
	AuditEventID uint   `json:"audit_event_id,omitempty"`
}

// OccasionObligationEntry is one OccasionObligation row — a standing recurring
// obligation toward a contact (ADR 0024, issue #1222): a holiday card, a
// birthday gift, an annual invite.
type OccasionObligationEntry struct {
	Contact      string `json:"contact"`
	Kind         string `json:"kind"` // card|gift|invite
	Label        string `json:"label"`
	AnchorMonth  *int   `json:"anchor_month,omitempty"`
	AnchorDay    *int   `json:"anchor_day,omitempty"`
	LeadTimeDays int    `json:"lead_time_days,omitempty"`
	// Active defaults to true when omitted. A pointer so an explicit false
	// survives (a plain bool would be indistinguishable from "unset").
	Active      *bool  `json:"active,omitempty"`
	Sensitivity string `json:"sensitivity,omitempty"`
	Notes       string `json:"notes,omitempty"`
	SoftDeleted bool   `json:"soft_deleted,omitempty"`
}

// OccasionEventEntry is one OccasionEvent plus its invitee/RSVP ledger (ADR
// 0026, issue #1228). StartsAt/EndsAt are RFC 3339 instants; StartsInDays /
// EndsInDays are the demo-relative alternative resolved against the PopulateAt
// clock, so an upcoming event stays upcoming.
type OccasionEventEntry struct {
	Title        string                       `json:"title"`
	StartsAt     *time.Time                   `json:"starts_at,omitempty"`
	StartsInDays *int                         `json:"starts_in_days,omitempty"`
	EndsAt       *time.Time                   `json:"ends_at,omitempty"`
	EndsInDays   *int                         `json:"ends_in_days,omitempty"`
	Location     string                       `json:"location,omitempty"`
	Sensitivity  string                       `json:"sensitivity,omitempty"`
	Notes        string                       `json:"notes,omitempty"`
	Attendees    []OccasionEventAttendeeEntry `json:"attendees,omitempty"`
	SoftDeleted  bool                         `json:"soft_deleted,omitempty"`
}

// OccasionEventAttendeeEntry is one invitee/RSVP row for an OccasionEvent.
type OccasionEventAttendeeEntry struct {
	Contact string `json:"contact"`
	RSVP    string `json:"rsvp,omitempty"` // pending|accepted|declined|maybe, default pending
}

// Load parses a manifest from r and validates its cross-references. It does
// not touch the database; see Populate.
func Load(r io.Reader) (*Manifest, error) {
	var m Manifest
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		return nil, fmt.Errorf("canonicalfixture: parsing manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Read loads and parses the checked-in manifest by locating the repo root from
// the current working directory. It works from anywhere under the repository
// (backend/, backend/<pkg>, or the repo root), mirroring how each consumer of
// the contract fixtures resolves the shared directory in its own way.
func Read() (*Manifest, error) {
	path, err := FindManifest()
	if err != nil {
		return nil, err // # pragma: no cover — a test CWD is always inside the repo, so FindManifest cannot fail here
	}
	f, err := os.Open(path) // #nosec G304 -- path is not request input: FindManifest resolves the checked-in manifest by walking up from the test process's own working directory, and it just stat'ed this exact path.
	if err != nil {
		return nil, fmt.Errorf("canonicalfixture: opening %s: %w", path, err) // # pragma: no cover — a path FindManifest just stat'ed is openable
	}
	defer f.Close() //nolint:errcheck // read-only handle: Close cannot lose data
	return Load(f)
}

// FindManifest locates testdata/canonical-fixture/manifest.json by walking up
// from the current working directory until the repo root is found.
func FindManifest() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("canonicalfixture: resolving working directory: %w", err) // # pragma: no cover — Getwd fails only when the CWD was deleted out from under the process
	}
	for {
		candidate := filepath.Join(dir, ManifestRelPath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("canonicalfixture: manifest %s not found from %s (run tests from the backend/ dir or the repo root)", ManifestRelPath, dir) // # pragma: no cover — tests run inside the repo, so the walk always terminates at the repo root
		}
		dir = parent
	}
}

// The loader writes these enum-shaped fields straight to the DB, bypassing
// the API's own `validate:"oneof=..."` struct tags on the equivalent model
// fields — these sets mirror those tags (not the models package's own second
// copy; it exposes plain string constants, no validator/set) so Validate can
// catch a typo'd token the same way the API would reject it. Kept next to the
// exported constants they mirror, not hard-coded literals.
var (
	reachOutKinds = map[string]bool{
		models.ReachOutKindOrganization: true,
		models.ReachOutKindTitle:        true,
		models.ReachOutKindAddress:      true,
	}
	reachOutStatuses = map[string]bool{
		models.ReachOutStatusPending:   true,
		models.ReachOutStatusDismissed: true,
	}
	occasionEventRSVPs = map[string]bool{
		models.OccasionEventRSVPPending:  true,
		models.OccasionEventRSVPAccepted: true,
		models.OccasionEventRSVPDeclined: true,
		models.OccasionEventRSVPMaybe:    true,
	}
	preferenceLevels = map[string]bool{
		models.PreferenceLevelHigh:   true,
		models.PreferenceLevelMedium: true,
		models.PreferenceLevelLow:    true,
	}
)

// Validate checks the manifest's version and that every cross-reference
// resolves to a declared contact. Failures are returned as one joined error so
// a malformed manifest is fixed in one pass, not one compile-test iteration.
//
// occasion_obligations[].kind is deliberately NOT validated against a closed
// list here: OccasionObligation.Kind is an open, unvalidated classifier by
// design (see the doc comment on models.OccasionObligationKindCard — "the
// column accepts any string so a future kind doesn't need a migration"),
// exactly like LifeEvent.Type/Preference.Category. Enforcing a closed set in
// the fixture loader would be stricter than the API it's standing in for.
func (m *Manifest) Validate() error {
	if m.Version != ManifestVersion {
		return fmt.Errorf("canonicalfixture: unsupported manifest version %d (this loader understands %d)", m.Version, ManifestVersion)
	}
	if m.User.Username == "" || m.User.Email == "" {
		return fmt.Errorf("canonicalfixture: manifest user must set username and email")
	}
	if len(m.Contacts) == 0 {
		return fmt.Errorf("canonicalfixture: manifest declares no contacts")
	}

	names := make(map[string]bool, len(m.Contacts))
	for i, c := range m.Contacts {
		if c.Name == "" {
			return fmt.Errorf("canonicalfixture: contact %d has no name", i)
		}
		if names[c.Name] {
			return fmt.Errorf("canonicalfixture: duplicate contact name %q", c.Name)
		}
		names[c.Name] = true
	}
	for i, c := range m.Contacts {
		if c.RecreatesVCardUIDOf == "" {
			continue
		}
		prev := c.RecreatesVCardUIDOf
		if !names[prev] {
			return fmt.Errorf("canonicalfixture: contact %q recreates unknown vcard_uid of %q", c.Name, prev)
		}
		if i < indexOfName(m.Contacts, prev) {
			return fmt.Errorf("canonicalfixture: contact %q recreates the vcard_uid of %q, which must appear earlier in the manifest (soft-delete then recreate)", c.Name, prev)
		}
	}

	ref := func(section string, name string) error {
		if !names[name] {
			return fmt.Errorf("canonicalfixture: %s references unknown contact %q", section, name)
		}
		return nil
	}
	var errs []error
	for _, n := range m.Notes {
		if err := ref("note", n.Contact); err != nil {
			errs = append(errs, err)
		}
	}
	for _, l := range m.LifeEvents {
		if err := ref("life_event", l.Contact); err != nil {
			errs = append(errs, err)
		}
		for _, rel := range l.RelatedEntities {
			if err := ref("life_event related_entity", rel); err != nil {
				errs = append(errs, err)
			}
		}
	}
	for _, g := range m.Gifts {
		if err := ref("gift", g.Contact); err != nil {
			errs = append(errs, err)
		}
	}
	for _, r := range m.Relationships {
		if err := ref("relationship source", r.Source); err != nil {
			errs = append(errs, err)
		}
		if err := ref("relationship target", r.Target); err != nil {
			errs = append(errs, err)
		}
	}
	for _, h := range m.Households {
		for _, mem := range h.Members {
			if err := ref("household member", mem.Contact); err != nil {
				errs = append(errs, err)
			}
		}
	}
	for _, c := range m.Circles {
		for _, member := range c.Members {
			if err := ref("circle member", member); err != nil {
				errs = append(errs, err)
			}
		}
	}
	for _, t := range m.Tags {
		for _, contact := range t.Contacts {
			if err := ref("tag contact", contact); err != nil {
				errs = append(errs, err)
			}
		}
	}
	for _, f := range m.CustomFields {
		for _, v := range f.Values {
			if err := ref("custom field value", v.Contact); err != nil {
				errs = append(errs, err)
			}
		}
	}
	for _, p := range m.Preferences {
		if err := ref("preference", p.Contact); err != nil {
			errs = append(errs, err)
		}
		// Level mirrors Preference.Level's own `validate:"omitempty,oneof=..."`
		// tag (models/preference.go), plus the category gate
		// models.PreferenceCategorySupportsLevel enforces at the API layer —
		// both are bypassed by this loader's direct DB writes.
		if p.Level != nil {
			if !preferenceLevels[*p.Level] {
				errs = append(errs, fmt.Errorf("canonicalfixture: preference for %q has unknown level %q", p.Contact, *p.Level))
			} else if !models.PreferenceCategorySupportsLevel(p.Category) {
				errs = append(errs, fmt.Errorf("canonicalfixture: preference for %q sets level %q but category %q does not support a level", p.Contact, *p.Level, p.Category))
			}
		}
	}
	for _, e := range m.ExternalIdentities {
		if err := ref("external_identity", e.Contact); err != nil {
			errs = append(errs, err)
		}
	}
	for _, a := range m.Attachments {
		if err := ref("attachment", a.Contact); err != nil {
			errs = append(errs, err)
		}
	}
	for _, a := range m.Activities {
		for _, contact := range a.Contacts {
			if err := ref("activity contact", contact); err != nil {
				errs = append(errs, err)
			}
		}
		if a.Date.IsZero() && a.DaysAgo == nil {
			errs = append(errs, fmt.Errorf("canonicalfixture: activity %q must set date or days_ago", a.Title))
		}
		if !a.Date.IsZero() && a.DaysAgo != nil {
			errs = append(errs, fmt.Errorf("canonicalfixture: activity %q sets both date and days_ago (exactly one is required)", a.Title))
		}
	}
	if m.SelfContact != "" {
		if err := ref("self_contact", m.SelfContact); err != nil {
			errs = append(errs, err)
		}
	}
	for _, c := range m.CadencePolicies {
		if err := ref("cadence_policy", c.Contact); err != nil {
			errs = append(errs, err)
		}
	}
	for _, c := range m.CadencePolicies {
		if c.TargetIntervalDays <= 0 {
			errs = append(errs, fmt.Errorf("canonicalfixture: cadence_policy for %q has non-positive target_interval_days %d (models.CadencePolicy requires gt=0)", c.Contact, c.TargetIntervalDays))
		}
	}
	for _, r := range m.ReachOutSuggestions {
		if err := ref("reach_out_suggestion", r.Contact); err != nil {
			errs = append(errs, err)
		}
		// Kind/Status mirror ReachOutSuggestion's own `validate:"oneof=..."`
		// struct tags (models/reach_out_suggestion.go) — the loader writes
		// these straight to the DB, bypassing that validation, so Validate is
		// the only thing standing between a typo and a silently-broken row.
		if !reachOutKinds[r.Kind] {
			errs = append(errs, fmt.Errorf("canonicalfixture: reach_out_suggestion for %q has unknown kind %q", r.Contact, r.Kind))
		}
		if r.Status != "" && !reachOutStatuses[r.Status] {
			errs = append(errs, fmt.Errorf("canonicalfixture: reach_out_suggestion for %q has unknown status %q", r.Contact, r.Status))
		}
	}
	for _, o := range m.OccasionObligations {
		if err := ref("occasion_obligation", o.Contact); err != nil {
			errs = append(errs, err)
		}
	}
	for _, e := range m.OccasionEvents {
		if e.StartsAt == nil && e.StartsInDays == nil {
			errs = append(errs, fmt.Errorf("canonicalfixture: occasion_event %q must set starts_at or starts_in_days", e.Title))
		}
		if e.StartsAt != nil && e.StartsInDays != nil {
			errs = append(errs, fmt.Errorf("canonicalfixture: occasion_event %q sets both starts_at and starts_in_days", e.Title))
		}
		if e.EndsAt != nil && e.EndsInDays != nil {
			errs = append(errs, fmt.Errorf("canonicalfixture: occasion_event %q sets both ends_at and ends_in_days", e.Title))
		}
		// An end before its start can only be caught when both ends are
		// expressed in the same form — comparing an absolute instant against
		// a demo-relative day offset needs a clock, which Validate doesn't
		// have (see PopulateAt). Mirrors validateOccasionEventTimes's
		// ends_at >= starts_at rule (controllers/occasion_event_controller.go);
		// equal is allowed there too.
		if e.StartsAt != nil && e.EndsAt != nil && e.EndsAt.Before(*e.StartsAt) {
			errs = append(errs, fmt.Errorf("canonicalfixture: occasion_event %q ends (%s) before it starts (%s)", e.Title, e.EndsAt, e.StartsAt))
		}
		if e.StartsInDays != nil && e.EndsInDays != nil && *e.EndsInDays < *e.StartsInDays {
			errs = append(errs, fmt.Errorf("canonicalfixture: occasion_event %q ends_in_days %d is before starts_in_days %d", e.Title, *e.EndsInDays, *e.StartsInDays))
		}
		for _, a := range e.Attendees {
			if err := ref("occasion_event attendee", a.Contact); err != nil {
				errs = append(errs, err)
			}
			// RSVP mirrors OccasionEventAttendee.RSVP's own
			// `validate:"oneof=..."` tag (models/occasion_event.go).
			if a.RSVP != "" && !occasionEventRSVPs[a.RSVP] {
				errs = append(errs, fmt.Errorf("canonicalfixture: occasion_event %q attendee %q has unknown rsvp %q", e.Title, a.Contact, a.RSVP))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("canonicalfixture: manifest validation failed: %w", errors.Join(errs...))
	}
	return nil
}

func indexOfName(contacts []ContactEntry, name string) int {
	for i, c := range contacts {
		if c.Name == name {
			return i
		}
	}
	return -1 // # pragma: no cover — only called with a name Validate already proved is present
}
