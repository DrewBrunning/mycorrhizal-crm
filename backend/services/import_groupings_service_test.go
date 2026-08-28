package services

import (
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newGroupingsDB migrates a REAL schema (database.InitDB, not AutoMigrate) —
// CircleMember.MemberVCardUID and ContactTag.ContactVCardUID both carry
// explicit `gorm:"column:..."` tags precisely because GORM's derived names
// disagree with the migration SQL, and only a real-schema test can catch that
// class of bug (CLAUDE.md backend trap 1).
func newGroupingsDB(t *testing.T) (*gorm.DB, uint) {
	t.Helper()

	db := dbtest.New(t)

	user := models.User{Username: "grouping-user", Email: "grouping@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)

	return db, user.ID
}

func createGroupingContact(t *testing.T, db *gorm.DB, userID uint, firstname string, circles, tags []string) *models.Contact {
	t.Helper()

	contact := models.Contact{UserID: userID, Firstname: firstname, Circles: circles, ImportedTags: tags}
	require.NoError(t, db.Create(&contact).Error)
	require.NotEmpty(t, contact.VCardUID, "BeforeSave must assign a VCardUID")
	return &contact
}

// The bug this pins: import parsed circle columns into the flat
// Contact.Circles JSON column and created no Circle/CircleMember rows at all,
// while every UI surface reads the entities — so imported circles were
// invisible in the running app.
func TestMaterializeImportedGroupings_CreatesCirclesAndMemberships(t *testing.T) {
	db, userID := newGroupingsDB(t)
	contact := createGroupingContact(t, db, userID, "Alice", []string{"Family", "Climbing"}, nil)

	require.NoError(t, MaterializeImportedGroupings(db, userID, contact))

	var circles []models.Circle
	require.NoError(t, db.Where("user_id = ?", userID).Order("name").Find(&circles).Error)
	require.Len(t, circles, 2)
	assert.Equal(t, "Climbing", circles[0].Name)
	assert.Equal(t, "Family", circles[1].Name)

	names, err := CircleNamesForContact(db, userID, contact.VCardUID)
	require.NoError(t, err)
	assert.Equal(t, []string{"Climbing", "Family"}, names)
}

func TestMaterializeImportedGroupings_CreatesTagsSeparatelyFromCircles(t *testing.T) {
	db, userID := newGroupingsDB(t)
	contact := createGroupingContact(t, db, userID, "Bob", []string{"Work"}, []string{"vegan", "cyclist"})

	require.NoError(t, MaterializeImportedGroupings(db, userID, contact))

	circleNames, err := CircleNamesForContact(db, userID, contact.VCardUID)
	require.NoError(t, err)
	assert.Equal(t, []string{"Work"}, circleNames, "a tag must not become a Circle")

	tagNames, err := TagNamesForContact(db, userID, contact.VCardUID)
	require.NoError(t, err)
	assert.Equal(t, []string{"cyclist", "vegan"}, tagNames, "a circle must not become a Tag")
}

// Re-importing the same file must not duplicate entities or memberships. The
// (circle_id, member_vcard_uid) unique index would reject the duplicate row
// with an error; this asserts the checked-first path keeps it a clean no-op.
func TestMaterializeImportedGroupings_IsIdempotent(t *testing.T) {
	db, userID := newGroupingsDB(t)
	contact := createGroupingContact(t, db, userID, "Carol", []string{"Family"}, []string{"vegan"})

	require.NoError(t, MaterializeImportedGroupings(db, userID, contact))
	require.NoError(t, MaterializeImportedGroupings(db, userID, contact))

	var circleCount, memberCount, tagCount, contactTagCount int64
	require.NoError(t, db.Model(&models.Circle{}).Where("user_id = ?", userID).Count(&circleCount).Error)
	require.NoError(t, db.Model(&models.CircleMember{}).Where("user_id = ?", userID).Count(&memberCount).Error)
	require.NoError(t, db.Model(&models.Tag{}).Where("user_id = ?", userID).Count(&tagCount).Error)
	require.NoError(t, db.Model(&models.ContactTag{}).Where("user_id = ?", userID).Count(&contactTagCount).Error)

	assert.Equal(t, int64(1), circleCount)
	assert.Equal(t, int64(1), memberCount)
	assert.Equal(t, int64(1), tagCount)
	assert.Equal(t, int64(1), contactTagCount)
}

// Two contacts naming the same circle must join one Circle, not create two.
func TestMaterializeImportedGroupings_ReusesExistingCircleAcrossContacts(t *testing.T) {
	db, userID := newGroupingsDB(t)
	first := createGroupingContact(t, db, userID, "Dan", []string{"Family"}, nil)
	second := createGroupingContact(t, db, userID, "Erin", []string{"family"}, nil)

	require.NoError(t, MaterializeImportedGroupings(db, userID, first))
	require.NoError(t, MaterializeImportedGroupings(db, userID, second))

	var circles []models.Circle
	require.NoError(t, db.Where("user_id = ?", userID).Find(&circles).Error)
	require.Len(t, circles, 1, "casing must not fork a second Circle named the same thing")
	assert.Equal(t, "Family", circles[0].Name, "the first spelling seen wins")

	var memberCount int64
	require.NoError(t, db.Model(&models.CircleMember{}).
		Where("circle_id = ?", circles[0].ID).Count(&memberCount).Error)
	assert.Equal(t, int64(2), memberCount, "both contacts must join the one Circle")
}

// Circles are scoped per user like everything else — one user's import must
// never join a Circle belonging to another user.
func TestMaterializeImportedGroupings_ScopedPerUser(t *testing.T) {
	db, userID := newGroupingsDB(t)
	other := models.User{Username: "other-grouping", Email: "other-grouping@example.com", Password: "x"}
	require.NoError(t, db.Create(&other).Error)

	mine := createGroupingContact(t, db, userID, "Mine", []string{"Family"}, nil)
	theirs := createGroupingContact(t, db, other.ID, "Theirs", []string{"Family"}, nil)

	require.NoError(t, MaterializeImportedGroupings(db, userID, mine))
	require.NoError(t, MaterializeImportedGroupings(db, other.ID, theirs))

	var circles []models.Circle
	require.NoError(t, db.Where("name = ?", "Family").Find(&circles).Error)
	assert.Len(t, circles, 2, "each user gets their own Family circle")

	mineNames, err := CircleNamesForContact(db, userID, mine.VCardUID)
	require.NoError(t, err)
	assert.Equal(t, []string{"Family"}, mineNames)

	// The other user's contact must not appear in this user's circle.
	theirNames, err := CircleNamesForContact(db, userID, theirs.VCardUID)
	require.NoError(t, err)
	assert.Empty(t, theirNames)
}

func TestMaterializeImportedGroupings_SkipsBlankAndDuplicateNames(t *testing.T) {
	db, userID := newGroupingsDB(t)
	contact := createGroupingContact(t, db, userID, "Frank",
		[]string{" Family ", "", "   ", "family", "Family"}, nil)

	require.NoError(t, MaterializeImportedGroupings(db, userID, contact))

	names, err := CircleNamesForContact(db, userID, contact.VCardUID)
	require.NoError(t, err)
	assert.Equal(t, []string{"Family"}, names)
}

func TestMaterializeImportedGroupings_NoGroupingsIsANoOp(t *testing.T) {
	db, userID := newGroupingsDB(t)
	contact := createGroupingContact(t, db, userID, "Grace", nil, nil)

	require.NoError(t, MaterializeImportedGroupings(db, userID, contact))

	var circleCount int64
	require.NoError(t, db.Model(&models.Circle{}).Count(&circleCount).Error)
	assert.Zero(t, circleCount)
}
