package services

import (
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// End-to-end coverage for the T3 round trip, driven through the real import
// wizard (CreateCSVSession -> PreviewCSV -> Confirm) against a real migrated
// schema rather than AutoMigrate.
//
// What was broken: a CSV with a "Circles" column landed its values in the flat
// Contact.Circles JSON column and created no Circle/CircleMember rows, while
// ContactsPage's filter, NetworkGraph's grouping and ContactHeader's chips all
// read the Circle entities. Imported circles were therefore invisible in the
// running app — the import reported success and the data went nowhere the user
// could see. Nothing in the suite noticed because every existing import test
// asserted on Contact.Circles, the column that had stopped mattering.

func TestImportCSV_MaterializesCirclesIntoRealEntities(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "roundtrip", Email: "roundtrip@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)

	m := NewImportSessionManager()
	log := testImportLogger()

	headers := []string{"First Name", "Last Name", "Circles", "Tags"}
	rows := [][]string{
		{"Alice", "Smith", "Family, Climbing", "vegan"},
		{"Bob", "Jones", "Family", ""},
	}
	id := m.CreateCSVSession(user.ID, headers, rows)

	mappings := []models.ColumnMapping{
		{CSVColumn: "First Name", ContactField: "firstname"},
		{CSVColumn: "Last Name", ContactField: "lastname"},
		{CSVColumn: "Circles", ContactField: "circles"},
		{CSVColumn: "Tags", ContactField: "tags"},
	}
	_, appErr := m.PreviewCSV(db, user.ID, models.ImportPreviewRequest{SessionID: id, Mappings: mappings})
	require.Nil(t, appErr)

	result, appErr := m.Confirm(db, user.ID, models.ImportConfirmRequest{
		SessionID: id,
		Actions: []models.RowImportAction{
			{RowIndex: 0, Action: "add"},
			{RowIndex: 1, Action: "add"},
		},
	}, log)
	require.Nil(t, appErr)
	require.Empty(t, result.Errors)
	require.Equal(t, 2, result.Created)

	// The entities the UI actually reads must now exist.
	var circles []models.Circle
	require.NoError(t, db.Where("user_id = ?", user.ID).Order("name").Find(&circles).Error)
	require.Len(t, circles, 2, "Climbing and Family must exist as real Circles")
	assert.Equal(t, "Climbing", circles[0].Name)
	assert.Equal(t, "Family", circles[1].Name)

	var alice, bob models.Contact
	require.NoError(t, db.Where("firstname = ?", "Alice").First(&alice).Error)
	require.NoError(t, db.Where("firstname = ?", "Bob").First(&bob).Error)

	aliceCircles, err := CircleNamesForContact(db, user.ID, alice.VCardUID)
	require.NoError(t, err)
	assert.Equal(t, []string{"Climbing", "Family"}, aliceCircles)

	bobCircles, err := CircleNamesForContact(db, user.ID, bob.VCardUID)
	require.NoError(t, err)
	assert.Equal(t, []string{"Family"}, bobCircles, "both rows must join the same Family circle")

	// "Tags" is its own destination now, not folded into circles.
	aliceTags, err := TagNamesForContact(db, user.ID, alice.VCardUID)
	require.NoError(t, err)
	assert.Equal(t, []string{"vegan"}, aliceTags)

	bobTags, err := TagNamesForContact(db, user.ID, bob.VCardUID)
	require.NoError(t, err)
	assert.Empty(t, bobTags)
}

// A "Tags" column used to be mapped onto the flat circles field, so importing
// tags produced circles. The synonym table now splits by target.
func TestImportCSV_TagColumnDoesNotBecomeACircle(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "tagsonly", Email: "tagsonly@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)

	m := NewImportSessionManager()
	log := testImportLogger()

	id := m.CreateCSVSession(user.ID, []string{"First Name", "Labels"}, [][]string{{"Carol", "vip, vegan"}})

	_, appErr := m.PreviewCSV(db, user.ID, models.ImportPreviewRequest{
		SessionID: id,
		Mappings: []models.ColumnMapping{
			{CSVColumn: "First Name", ContactField: "firstname"},
			// "Labels" resolves to the tags target via the synonym table.
			{CSVColumn: "Labels", ContactField: "tags"},
		},
	})
	require.Nil(t, appErr)

	result, appErr := m.Confirm(db, user.ID, models.ImportConfirmRequest{
		SessionID: id,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
	}, log)
	require.Nil(t, appErr)
	require.Empty(t, result.Errors)

	var carol models.Contact
	require.NoError(t, db.Where("firstname = ?", "Carol").First(&carol).Error)

	tags, err := TagNamesForContact(db, user.ID, carol.VCardUID)
	require.NoError(t, err)
	assert.Equal(t, []string{"vegan", "vip"}, tags)

	var circleCount int64
	require.NoError(t, db.Model(&models.Circle{}).Where("user_id = ?", user.ID).Count(&circleCount).Error)
	assert.Zero(t, circleCount, "a Labels column must not create Circles")
}

// The header synonym table is what routes a header to a destination; assert the split
// directly so a future edit cannot quietly fold tags back onto circles.
func TestHeaderSynonymsSplitCircleAndTagVocabularies(t *testing.T) {
	for header, want := range map[string]string{
		"circles":    "circles",
		"groups":     "circles",
		"kreise":     "circles",
		"gruppen":    "circles",
		"tags":       "tags",
		"labels":     "tags",
		"category":   "tags",
		"categories": "tags",
	} {
		assert.Equalf(t, want, headerToField[header],
			"header %q must map to the %q target", header, want)
	}
}
