package services

import (
	"context"
	"strings"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	csvUID  = "c0ffee00-0000-4000-8000-0000000000aa"
	csvEnt1 = "11111111-1111-4111-8111-111111111111"
	csvEnt2 = "22222222-2222-4222-8222-222222222222"
)

func csvBackup(entities, links, fields string) string {
	return "=== CONTACTS ===\nID,Firstname\n1,Ada\n\n" +
		"=== ENTITIES ===\nID,Kind,Name,Sensitivity,Core (JSON),Notes,Created At,Updated At\n" + entities +
		"\n=== ENTITY LINKS ===\nID,Contact ID,Contact Name,Entity ID,Entity Name,Kind,Role,Role Label,Primary,Start Date,End Date,Sensitivity,Contact UID\n" + links +
		"\n=== ENTITY FIELDS ===\nEntity ID,Definition Key,Value\n" + fields
}

func TestParseEntityCSVSections_Pre17ExportHasNone(t *testing.T) {
	s, issues, err := ParseEntityCSVSections(strings.NewReader("=== CONTACTS ===\nID,Firstname\n1,Ada\n\n=== RELATIONSHIPS ===\nID,Type\nx,friend_of\n"))
	require.NoError(t, err)
	assert.Empty(t, issues)
	assert.Empty(t, s.Entities)
	assert.Empty(t, s.Links)
	assert.Empty(t, s.Values)
}

func TestParseCSVPartialDate(t *testing.T) {
	type ymd struct{ y, m, d int } // 0 = absent
	for in, want := range map[string]ymd{
		"2020": {2020, 0, 0}, "2020-03": {2020, 3, 0}, "2020-03-04": {2020, 3, 4},
		"--03-04": {0, 3, 4}, "--03": {0, 3, 0}, "---04": {0, 0, 4},
	} {
		p, ok := parseCSVPartialDate(in)
		require.True(t, ok, in)
		require.NotNil(t, p, in)
		val := func(v *int) int {
			if v == nil {
				return 0
			}
			return *v
		}
		assert.Equal(t, want, ymd{val(p.Year), val(p.Month), val(p.Day)}, in)
	}
	p, ok := parseCSVPartialDate("")
	assert.True(t, ok)
	assert.Nil(t, p)
	_, ok = parseCSVPartialDate("March 2020")
	assert.False(t, ok)
}

func TestRestoreEntityCSV_RulesAndIssues(t *testing.T) {
	db := dbtest.New(t)
	u := bundleTestUser(t, db, "csv-restore")
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, VCardUID: csvUID, Firstname: "Ada"}).Error)

	body := csvBackup(
		csvEnt1+",organisation,Acme,normal,\"{\"\"organization\"\":{\"\"name\"\":\"\"Acme\"\"}}\",,,\n"+
			csvEnt2+",robot,Bad,normal,{},,,\n"+
			"short,row\n",
		"l1,1,Ada,"+csvEnt1+",Acme,organisation,employee,,true,2020-01,,normal,"+csvUID+"\n"+
			"l2,9,Ghost,"+csvEnt1+",Acme,organisation,employee,,false,,,normal,no-such-contact\n"+
			"l3,1,Ada,"+csvEnt1+",Acme,organisation,employee,,false,March 2020,,normal,"+csvUID+"\n"+
			"l4,1,Ada,"+csvEnt1+",Acme,organisation,astronaut,,false,,,normal,"+csvUID+"\n",
		csvEnt1+",missing_def,\"\"\"x\"\"\"\n")

	rep, err := RestoreEntityCSV(context.Background(), db, u.ID, strings.NewReader(body))
	require.NoError(t, err)
	assert.Equal(t, 1, rep.EntitiesCreated)
	assert.Equal(t, 1, rep.LinksCreated)
	assert.Zero(t, rep.FieldValuesCreated)
	fields := map[string]bool{}
	for _, i := range rep.Issues {
		fields[i.Field] = true
	}
	for _, f := range []string{"row", "entity", "entity_link.contact", "entity_link.period", "entity_link.role", "entity_field_value.field_definition_id"} {
		assert.True(t, fields[f], "missing issue for %s: %+v", f, rep.Issues)
	}

	// A second run: the ledger skips the entity and link, nothing duplicates.
	rep, err = RestoreEntityCSV(context.Background(), db, u.ID, strings.NewReader(body))
	require.NoError(t, err)
	assert.Zero(t, rep.EntitiesCreated+rep.LinksCreated)
	var n int64
	require.NoError(t, db.Model(&models.Entity{}).Where("user_id = ?", u.ID).Count(&n).Error)
	assert.EqualValues(t, 1, n)
}

func TestUnneutralizeCSV(t *testing.T) {
	assert.Equal(t, "=1+1", unneutralizeCSV("'=1+1"))
	assert.Equal(t, "-5", unneutralizeCSVJSON("'-5"))
	assert.Equal(t, "'plain", unneutralizeCSV("'plain"), "an apostrophe before an ordinary character is data")
	assert.Equal(t, "x", unneutralizeCSV("x"))
}
