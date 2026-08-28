package models

import (
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/jscontact"
	"mycorrhizal/vcard4"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fileExporter is the minimal adapter surface this test needs: the shared
// contactmodel.Importer/Exporter pair, satisfied by both vcard4.Adapter and
// jscontact.Adapter without importing them twice.
type fileExporter interface {
	contactmodel.Importer
	contactmodel.Exporter
}

// TestContactFileRoundTrip_VCard4AndJSContact is issue #515's file-level
// acceptance criterion: a contact with every canonical field populated is
// exported to vCard 4 and JSContact and re-imported, and every canonical
// field that has a Card home survives — verified at the concept level (the
// per-field byte-level fidelity of each adapter is that adapter's own suite's
// job: roundtrip_test.go plus the import_*/export_* correspondence tests).
//
// Envelope-only fields (Gender, Circles, HowWeMet, ...) are excluded from the
// file by design ("Format adapters MUST ignore it entirely"); their loss is
// covered on the record level by TestApplyRecordToContact_RoundTrip (they
// survive Record -> Contact -> Record) and named on the file level by
// EnvelopeExportLossDiagnostics — never silent.
func TestContactFileRoundTrip_VCard4AndJSContact(t *testing.T) {
	original := fullyPopulatedContact()
	photoDir := t.TempDir()
	record := RecordFromContact(original, photoDir)

	tests := []struct {
		name    string
		adapter fileExporter
	}{
		{"vcard4", vcard4.Adapter{}},
		{"jscontact", jscontact.Adapter{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, exportDiags, err := tc.adapter.Export(record)
			require.NoError(t, err, "export failed; diags: %v", exportDiags)
			require.NotEmpty(t, data, "export produced empty output")

			reimported, importDiags, err := tc.adapter.Import(data)
			require.NoError(t, err, "re-import failed; diags: %v", importDiags)

			c := reimported.Card

			// uid
			assert.Equal(t, record.Card.UID, c.UID, "uid must survive the %s round trip", tc.name)

			// name.*
			require.NotNil(t, c.Name, "name must survive")
			components := map[string]string{}
			for _, comp := range c.Name.Components {
				components[comp.Kind] = comp.Value
			}
			assert.Equal(t, "Jane", components["given"], "name.given")
			assert.Equal(t, "Doe", components["surname"], "name.surname")
			assert.Equal(t, "Quinn", components["given2"], "name.given2")
			assert.Equal(t, "Dr.", components["title"], "name.title")
			assert.Equal(t, "Jr.", components["credential"], "name.credential")

			// nickname
			require.Len(t, c.Nicknames, 1, "nickname must survive")
			assert.Equal(t, "Janie", c.Nicknames[0].Name)

			// org / org.unit
			require.Len(t, c.Organizations, 1, "organization must survive")
			assert.Equal(t, "Acme Corp", c.Organizations[0].Name)
			require.Len(t, c.Organizations[0].Units, 1)
			assert.Equal(t, "Engineering", c.Organizations[0].Units[0].Name)

			// title / role
			titles := map[string]string{}
			for _, ti := range c.Titles {
				titles[ti.Kind] = ti.Name
			}
			assert.Equal(t, "Staff Engineer", titles["title"])
			assert.Equal(t, "Tech Lead", titles["role"])

			// email
			require.Len(t, c.Emails, 1, "email must survive")
			assert.Equal(t, "jane@example.com", c.Emails[0].Address)

			// phone
			require.Len(t, c.Phones, 1, "phone must survive")
			assert.Equal(t, "+15551234567", c.Phones[0].Number)

			// impp
			require.Len(t, c.ImppAddresses, 1, "impp must survive")
			assert.Equal(t, "xmpp:jane@example.com", c.ImppAddresses[0].URI)

			// link (URL): URI survives in both formats
			require.Len(t, c.Links, 1, "link must survive")
			assert.Equal(t, "https://jane.example.com", c.Links[0].URI)

			// adr: the structured components (street/city/region/postcode/
			// country plus the T79 sub-street parts) must survive
			require.Len(t, c.Addresses, 1, "address must survive")
			addrComponents := map[string]string{}
			for _, comp := range c.Addresses[0].Components {
				addrComponents[comp.Kind] = comp.Value
			}
			assert.Equal(t, "1 Main St", addrComponents["name"])
			assert.Equal(t, "PO Box 42", addrComponents["postOfficeBox"])
			assert.Equal(t, "Apt 3B", addrComponents["apartment"])
			assert.Equal(t, "Floor 2", addrComponents["floor"])
			assert.Equal(t, "Springfield", addrComponents["locality"])
			assert.Equal(t, "IL", addrComponents["region"])
			assert.Equal(t, "62704", addrComponents["postcode"])
			assert.Equal(t, "US", addrComponents["country"])

			// anniversary.birth / anniversary.wedding
			anniversaries := map[string]*contactmodel.Anniversary{}
			for i := range c.Anniversaries {
				anniversaries[c.Anniversaries[i].Kind] = &c.Anniversaries[i]
			}
			require.Contains(t, anniversaries, "birth", "anniversary.birth must survive")
			require.NotNil(t, anniversaries["birth"].Date.Partial)
			assert.Equal(t, 1990, *anniversaries["birth"].Date.Partial.Year)
			require.Contains(t, anniversaries, "wedding", "anniversary.wedding must survive")
			require.NotNil(t, anniversaries["wedding"].Date.Partial)
			assert.Equal(t, 9, *anniversaries["wedding"].Date.Partial.Month)

			// Envelope fields are deliberately NOT in the file — the
			// adapters ignore the envelope — so the re-imported record's
			// envelope must be empty. This is exactly the drop the export
			// path names via EnvelopeExportLossDiagnostics (issue #515).
			assert.Empty(t, reimported.Envelope)
		})
	}
}
