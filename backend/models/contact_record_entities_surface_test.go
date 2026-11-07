package models

import (
	"encoding/json"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/jscontact"
	"mycorrhizal/vcard3"
	"mycorrhizal/vcard4"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// surfaceFixture builds a contact with an own ORG/ADR plus a primary
// organisation and place link whose entities and links carry data (notes, url,
// role label, dates, sensitive-looking tokens) that must NEVER leave.
func surfaceFixture(t *testing.T) (*projFixture, ContactEntityLink, ContactEntityLink, Entity, Entity) {
	t.Helper()
	f := newProjFixture(t)
	pref := 1
	f.contactWithCard(t, contactmodel.Card{
		Organizations: []contactmodel.Organization{{ID: "own-org", Name: "Own Co"}},
		Addresses: []contactmodel.Address{{ID: "own-adr", Pref: &pref,
			Components: []contactmodel.AddressComponent{{Kind: "name", Value: "9 Elm"}, {Kind: "locality", Value: "Shelbyville"}}}},
	})
	org := Entity{UserID: f.user.ID, Kind: EntityKindOrganisation, Name: "Acme Corp"}
	require.NoError(t, org.SetCore(&OrganisationCore{
		Organization: contactmodel.Organization{Name: "Acme Corp", Units: []contactmodel.OrgUnit{{Name: "Rocketry"}}},
		URL:          "https://secret-url.example", Notes: "ENTITY-NOTES-TOKEN"}))
	require.NoError(t, f.db.Create(&org).Error)
	place := Entity{UserID: f.user.ID, Kind: EntityKindPlace, Name: "Cabin"}
	require.NoError(t, place.SetCore(&PlaceCore{
		Address: contactmodel.Address{Components: []contactmodel.AddressComponent{{Kind: "name", Value: "1 Main St"}, {Kind: "locality", Value: "Springfield"}}},
		Notes:   "PLACE-NOTES-TOKEN"}))
	require.NoError(t, f.db.Create(&place).Error)
	y := 2019
	lo := ContactEntityLink{UserID: f.user.ID, ContactVCardUID: f.contact.VCardUID, EntityID: org.ID, Kind: org.Kind,
		Role: "contractor", RoleLabel: "ROLE-LABEL-TOKEN", IsPrimary: true,
		Period: &contactmodel.TemporalRange{Start: &contactmodel.PartialDate{Year: &y}}}
	require.NoError(t, f.db.Create(&lo).Error)
	lp := ContactEntityLink{UserID: f.user.ID, ContactVCardUID: f.contact.VCardUID, EntityID: place.ID, Kind: place.Kind,
		Role: "resident", IsPrimary: true}
	require.NoError(t, f.db.Create(&lp).Error)
	return f, lo, lp, org, place
}

func TestProjection_VCard4AndJSContactCarryProjectedORGAndADR(t *testing.T) {
	f, lo, lp, org, place := surfaceFixture(t)
	rec := f.record(t, FieldSelectionAll())

	vc4, diags, err := vcard4.Adapter{}.Export(rec)
	require.NoError(t, err)
	out := string(vc4)
	assert.Contains(t, out, "PROP-ID=own-org:Own Co", "own value leads")
	assert.Contains(t, out, "Acme Corp;Rocketry")
	assert.Contains(t, out, "PROP-ID=ent-"+lo.ID)
	assert.Contains(t, out, "PROP-ID=ent-"+lp.ID)
	assert.Less(t, strings.Index(out, "Own Co"), strings.Index(out, "Acme Corp"), "own ORG precedes the projection")
	for _, d := range diags {
		assert.NotEqual(t, "org", d.Concept, "vCard 4.0 emits both ORGs without a diagnostic: %v", d)
	}

	js, _, err := jscontact.Adapter{}.Export(rec)
	require.NoError(t, err)
	var card map[string]any
	require.NoError(t, json.Unmarshal(js, &card))
	orgs := card["organizations"].(map[string]any)
	assert.Contains(t, orgs, "ent-"+lo.ID, "JSContact map key is the reserved element ID")
	assert.Contains(t, orgs, "own-org")
	adrs := card["addresses"].(map[string]any)
	assert.Contains(t, adrs, "ent-"+lp.ID)

	// Nothing entity-shaped leaves except the projected ORG/ADR: not the entity
	// ids, notes, url, role, role label or dates, on any of the three formats.
	vc3, _, err := vcard3.Adapter{}.Export(rec)
	require.NoError(t, err)
	for name, blob := range map[string]string{"vcard4": out, "jscontact": string(js), "vcard3": string(vc3)} {
		for _, secret := range []string{org.ID, place.ID, "ENTITY-NOTES-TOKEN", "PLACE-NOTES-TOKEN", "secret-url.example",
			"ROLE-LABEL-TOKEN", "contractor", "resident"} {
			assert.NotContains(t, blob, secret, "%s must not carry %q", name, secret)
		}
	}
}

func TestProjection_VCard3DropsProjectedORGBehindOwnORGWithWarn(t *testing.T) {
	f, _, _, _, _ := surfaceFixture(t)
	rec := f.record(t, FieldSelectionAll())
	out, diags, err := vcard3.Adapter{}.Export(rec)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "ORG:Own Co")
	assert.NotContains(t, s, "Acme Corp", "projected ORG is not emitted behind an own ORG on 3.0")
	assert.Contains(t, s, "Springfield", "projected ADR still travels (3.0 has repeated ADR)")
	var found bool
	for _, d := range diags {
		if d.Severity == "warn" && d.Concept == "org" && strings.Contains(d.Message, "Acme Corp") {
			found = true
		}
	}
	assert.True(t, found, "a warn diagnostic names the dropped projection: %v", diags)
}

func TestProjection_VCard3EmitsProjectedORGWhenContactHasNoOwn(t *testing.T) {
	f := newProjFixture(t)
	f.contactWithCard(t, contactmodel.Card{})
	f.link(t, f.org(t, "Acme", "normal"), true, "normal", nil)
	out, diags, err := vcard3.Adapter{}.Export(f.record(t, FieldSelectionAll()))
	require.NoError(t, err)
	assert.Contains(t, string(out), "ORG:Acme;R&D", "the common case: the entity's value shows")
	for _, d := range diags {
		assert.NotEqual(t, "org", d.Concept)
	}
}

// project -> export -> import -> apply: the stored Card is unchanged, for
// vCard 4.0 (ids round-trip), JSContact, and vCard 3.0 (ids stripped).
func TestProjection_ExportImportRoundTripLeavesStoredCardUnchanged(t *testing.T) {
	type roundtrip struct {
		name string
		do   func(*contactmodel.Record) (*contactmodel.Record, error)
	}
	for _, rt := range []roundtrip{
		{"vcard4", func(r *contactmodel.Record) (*contactmodel.Record, error) {
			b, _, err := vcard4.Adapter{}.Export(r)
			if err != nil {
				return nil, err
			}
			out, _, err := vcard4.Adapter{}.Import(b)
			return out, err
		}},
		{"jscontact", func(r *contactmodel.Record) (*contactmodel.Record, error) {
			b, _, err := jscontact.Adapter{}.Export(r)
			if err != nil {
				return nil, err
			}
			out, _, err := jscontact.Adapter{}.Import(b)
			return out, err
		}},
		{"vcard3", func(r *contactmodel.Record) (*contactmodel.Record, error) {
			b, _, err := vcard3.Adapter{}.Export(r)
			if err != nil {
				return nil, err
			}
			out, _, err := vcard3.Adapter{}.Import(b)
			return out, err
		}},
	} {
		t.Run(rt.name, func(t *testing.T) {
			f, _, _, _, _ := surfaceFixture(t)
			var stored Contact
			require.NoError(t, f.db.First(&stored, f.contact.ID).Error)
			before, err := json.Marshal(stored.Card.Organizations)
			require.NoError(t, err)
			beforeAdr, err := json.Marshal(stored.Card.Addresses)
			require.NoError(t, err)

			imported, err := rt.do(RecordForContactForSync(&stored, "", f.db))
			require.NoError(t, err)
			SuppressEntityProjectionEcho(f.db, &stored, imported)
			ApplyRecordToContact(&stored, imported, "")
			require.NoError(t, f.db.Save(&stored).Error)

			var after Contact
			require.NoError(t, f.db.First(&after, f.contact.ID).Error)
			gotOrgs, _ := json.Marshal(after.Card.Organizations)
			gotAdrs, _ := json.Marshal(after.Card.Addresses)
			require.Len(t, after.Card.Organizations, 1, "no projected ORG was written back")
			require.Len(t, after.Card.Addresses, 1, "no projected ADR was written back")
			// Element IDs and pref may be re-minted by an adapter that regenerates
			// them; the VALUES must be untouched.
			assert.Equal(t, "Own Co", after.Card.Organizations[0].Name)
			assert.Equal(t, "9 Elm", after.Card.Addresses[0].Components[0].Value)
			if rt.name == "vcard4" || rt.name == "jscontact" {
				assert.JSONEq(t, string(before), string(gotOrgs))
				assert.JSONEq(t, string(beforeAdr), string(gotAdrs))
			}
		})
	}
}

func TestRecordForContactBackup_DoesNotProject(t *testing.T) {
	f, _, _, _, _ := surfaceFixture(t)
	var c Contact
	require.NoError(t, f.db.First(&c, f.contact.ID).Error)
	rec := RecordForContactBackup(&c, "", f.db)
	require.Len(t, rec.Card.Organizations, 1)
	require.Len(t, rec.Card.Addresses, 1)
	assert.Len(t, RecordForContact(&c, "", f.db).Card.Organizations, 2, "the owner REST read does project")
}
