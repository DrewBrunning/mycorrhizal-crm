package canonicalfixture

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestManifestValidateRejectsBreakages is the table-driven counterpart of the
// happy-path Read() tests: each case breaks exactly one manifest invariant and
// asserts Validate fails naming the problem, so the manifest's self-consistency
// rules are pinned both ways.
func TestManifestValidateRejectsBreakages(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(m *Manifest)
		wantErr string
	}{
		{
			name:    "unsupported version",
			mutate:  func(m *Manifest) { m.Version = 99 },
			wantErr: "unsupported manifest version",
		},
		{
			name:    "empty username",
			mutate:  func(m *Manifest) { m.User.Username = "" },
			wantErr: "user must set username and email",
		},
		{
			name:    "empty email",
			mutate:  func(m *Manifest) { m.User.Email = "" },
			wantErr: "user must set username and email",
		},
		{
			name:    "no contacts",
			mutate:  func(m *Manifest) { m.Contacts = nil },
			wantErr: "declares no contacts",
		},
		{
			name:    "contact without name",
			mutate:  func(m *Manifest) { m.Contacts[0].Name = "" },
			wantErr: `contact 0 has no name`,
		},
		{
			name:    "duplicate contact name",
			mutate:  func(m *Manifest) { m.Contacts[1].Name = m.Contacts[0].Name },
			wantErr: `duplicate contact name`,
		},
		{
			name:    "recreates unknown contact",
			mutate:  func(m *Manifest) { m.Contacts[9].RecreatesVCardUIDOf = "nobody" },
			wantErr: `recreates unknown vcard_uid of "nobody"`,
		},
		{
			name:    "recreates a later contact",
			mutate:  func(m *Manifest) { m.Contacts[0].RecreatesVCardUIDOf = "julie" },
			wantErr: "must appear earlier in the manifest",
		},
		{
			name:    "note references unknown contact",
			mutate:  func(m *Manifest) { m.Notes[0].Contact = "ghost" },
			wantErr: `note references unknown contact "ghost"`,
		},
		{
			name:    "life event references unknown contact",
			mutate:  func(m *Manifest) { m.LifeEvents[0].Contact = "ghost" },
			wantErr: `life_event references unknown contact "ghost"`,
		},
		{
			name:    "life event related entity references unknown contact",
			mutate:  func(m *Manifest) { m.LifeEvents[0].RelatedEntities = []string{"ghost"} },
			wantErr: `life_event related_entity references unknown contact "ghost"`,
		},
		{
			name:    "gift references unknown contact",
			mutate:  func(m *Manifest) { m.Gifts[0].Contact = "ghost" },
			wantErr: `gift references unknown contact "ghost"`,
		},
		{
			name:    "relationship source references unknown contact",
			mutate:  func(m *Manifest) { m.Relationships[0].Source = "ghost" },
			wantErr: `relationship source references unknown contact "ghost"`,
		},
		{
			name:    "relationship target references unknown contact",
			mutate:  func(m *Manifest) { m.Relationships[0].Target = "ghost" },
			wantErr: `relationship target references unknown contact "ghost"`,
		},
		{
			name:    "household member references unknown contact",
			mutate:  func(m *Manifest) { m.Households[0].Members[0].Contact = "ghost" },
			wantErr: `household member references unknown contact "ghost"`,
		},
		{
			name:    "circle member references unknown contact",
			mutate:  func(m *Manifest) { m.Circles[0].Members[0] = "ghost" },
			wantErr: `circle member references unknown contact "ghost"`,
		},
		{
			name:    "tag contact references unknown contact",
			mutate:  func(m *Manifest) { m.Tags[0].Contacts[0] = "ghost" },
			wantErr: `tag contact references unknown contact "ghost"`,
		},
		{
			name:    "custom field value references unknown contact",
			mutate:  func(m *Manifest) { m.CustomFields[0].Values[0].Contact = "ghost" },
			wantErr: `custom field value references unknown contact "ghost"`,
		},
		{
			name:    "preference references unknown contact",
			mutate:  func(m *Manifest) { m.Preferences[0].Contact = "ghost" },
			wantErr: `preference references unknown contact "ghost"`,
		},
		{
			name:    "external identity references unknown contact",
			mutate:  func(m *Manifest) { m.ExternalIdentities[0].Contact = "ghost" },
			wantErr: `external_identity references unknown contact "ghost"`,
		},
		{
			name:    "attachment references unknown contact",
			mutate:  func(m *Manifest) { m.Attachments[0].Contact = "ghost" },
			wantErr: `attachment references unknown contact "ghost"`,
		},
		{
			name:    "activity contact references unknown contact",
			mutate:  func(m *Manifest) { m.Activities[0].Contacts[0] = "ghost" },
			wantErr: `activity contact references unknown contact "ghost"`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// fresh copy per case so mutations never leak across subtests
			m, err := Read()
			require.NoError(t, err)
			tc.mutate(m)
			err = m.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// TestLoadRejectsMalformedJSON covers the parser's error branch.
func TestLoadRejectsMalformedJSON(t *testing.T) {
	_, err := Load(strings.NewReader("{ this is not json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing manifest")
}

// TestLoadRejectsInvalidManifest covers Load's validate step: a syntactically
// valid manifest that violates the schema rules is rejected, not accepted and
// silently misinterpreted.
func TestLoadRejectsInvalidManifest(t *testing.T) {
	m, err := Read()
	require.NoError(t, err)
	m.Version = 99
	data, err := json.Marshal(m)
	require.NoError(t, err)

	_, err = Load(bytes.NewReader(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported manifest version")
}

// TestPopulateRejectsNilManifest covers Populate's entry guard.
func TestPopulateRejectsNilManifest(t *testing.T) {
	db := dbtest.New(t)
	_, err := Populate(db, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil manifest")
}

// TestPopulateRejectsUnvalidatedManifest covers the loader's own defensive
// resolution of a manifest that was built programmatically and never passed
// through Validate: every section's contact-name -> vcard_uid resolution must
// fail loudly (naming the section and contact) rather than silently creating
// dangling rows.
func TestPopulateRejectsUnvalidatedManifest(t *testing.T) {
	db := dbtest.New(t)

	// Each case breaks exactly one reference in one section. Populate is
	// transactional, so a failing case leaves the shared db clean for the
	// next one.
	cases := []struct {
		name    string
		mutate  func(m *Manifest)
		wantErr string
	}{
		{
			name:    "note references unknown contact",
			mutate:  func(m *Manifest) { m.Notes[0].Contact = "ghost" },
			wantErr: `unknown contact "ghost"`,
		},
		{
			name:    "life event references unknown contact",
			mutate:  func(m *Manifest) { m.LifeEvents[0].Contact = "ghost" },
			wantErr: `unknown contact "ghost"`,
		},
		{
			name:    "life event related entity references unknown contact",
			mutate:  func(m *Manifest) { m.LifeEvents[0].RelatedEntities = []string{"ghost"} },
			wantErr: `unknown contact "ghost"`,
		},
		{
			name:    "gift references unknown contact",
			mutate:  func(m *Manifest) { m.Gifts[0].Contact = "ghost" },
			wantErr: `unknown contact "ghost"`,
		},
		{
			name:    "relationship source references unknown contact",
			mutate:  func(m *Manifest) { m.Relationships[0].Source = "ghost" },
			wantErr: `unknown contact "ghost"`,
		},
		{
			name:    "relationship target references unknown contact",
			mutate:  func(m *Manifest) { m.Relationships[0].Target = "ghost" },
			wantErr: `unknown contact "ghost"`,
		},
		{
			name:    "household member references unknown contact",
			mutate:  func(m *Manifest) { m.Households[0].Members[0].Contact = "ghost" },
			wantErr: `unknown contact "ghost"`,
		},
		{
			name:    "circle member references unknown contact",
			mutate:  func(m *Manifest) { m.Circles[0].Members[0] = "ghost" },
			wantErr: `unknown contact "ghost"`,
		},
		{
			name:    "tag contact references unknown contact",
			mutate:  func(m *Manifest) { m.Tags[0].Contacts[0] = "ghost" },
			wantErr: `unknown contact "ghost"`,
		},
		{
			name:    "custom field value references unknown contact",
			mutate:  func(m *Manifest) { m.CustomFields[0].Values[0].Contact = "ghost" },
			wantErr: `unknown contact "ghost"`,
		},
		{
			name:    "preference references unknown contact",
			mutate:  func(m *Manifest) { m.Preferences[0].Contact = "ghost" },
			wantErr: `unknown contact "ghost"`,
		},
		{
			name:    "external identity references unknown contact",
			mutate:  func(m *Manifest) { m.ExternalIdentities[0].Contact = "ghost" },
			wantErr: `unknown contact "ghost"`,
		},
		{
			name:    "attachment references unknown contact",
			mutate:  func(m *Manifest) { m.Attachments[0].Contact = "ghost" },
			wantErr: `unknown contact "ghost"`,
		},
		{
			name:    "activity references unknown contact",
			mutate:  func(m *Manifest) { m.Activities[0].Contacts = []string{"ghost"} },
			wantErr: `unknown contact "ghost"`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m, err := Read()
			require.NoError(t, err)
			tc.mutate(m)
			_, err = Populate(db, m)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// TestPopulateRejectsRecreatedUIDOfLiveContact covers createContact's guard:
// recreates_vcard_uid_of must name a soft-deleted contact, not a live one —
// the loader refuses to fabricate a UID collision against a living row.
func TestPopulateRejectsRecreatedUIDOfLiveContact(t *testing.T) {
	db := dbtest.New(t)
	m, err := Read()
	require.NoError(t, err)
	// Point julie at a live contact instead of the tombstoned gina. Validate
	// still passes (ada is a known name appearing earlier); Populate must
	// reject the collision.
	m.Contacts[9].RecreatesVCardUIDOf = "ada"

	_, err = Populate(db, m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must name an earlier soft-deleted contact")
}
