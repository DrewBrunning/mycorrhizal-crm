package services

import (
	"testing"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #2000 service-level coverage: the error branches the controller
// tests cannot reach without breaking the schema.

func fixedAssignmentFixture(t *testing.T) (*gorm.DB, uint, *ImportFixedAssignment) {
	t.Helper()
	db, userID := newGroupingsDB(t)
	circle := models.Circle{UserID: userID, Name: "Fixed"}
	require.NoError(t, db.Create(&circle).Error)
	tag := models.Tag{UserID: userID, Name: "fixed-tag"}
	require.NoError(t, db.Create(&tag).Error)
	return db, userID, &ImportFixedAssignment{Circles: []models.Circle{circle}, Tags: []models.Tag{tag}}
}

func TestApplyImportFixedAssignment_EmptyIsNoOp_AndNeedsVCardUID(t *testing.T) {
	db, userID, fixed := fixedAssignmentFixture(t)

	assert.True(t, (*ImportFixedAssignment)(nil).Empty())
	assert.True(t, (&ImportFixedAssignment{}).Empty())
	assert.NoError(t, ApplyImportFixedAssignment(db, userID, &models.Contact{}, nil))

	err := ApplyImportFixedAssignment(db, userID, &models.Contact{}, fixed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no VCardUID")
}

func TestResolveImportFixedAssignment_NoIDsIsNil_AndDBErrorsSurface(t *testing.T) {
	db, userID, fixed := fixedAssignmentFixture(t)

	got, appErr := ResolveImportFixedAssignment(db, userID, nil, []string{})
	assert.Nil(t, got)
	assert.Nil(t, appErr)

	dbtest.HideTable(t, db, "circles")
	_, appErr = ResolveImportFixedAssignment(db, userID, []string{fixed.Circles[0].ID}, nil)
	require.NotNil(t, appErr)
	assert.Equal(t, apperrors.ErrCodeDatabase, appErr.Code)

	dbtest.HideTable(t, db, "tags")
	_, appErr = ResolveImportFixedAssignment(db, userID, nil, []string{fixed.Tags[0].ID})
	require.NotNil(t, appErr)
	assert.Equal(t, apperrors.ErrCodeDatabase, appErr.Code)
}

func TestBuildImportFixedAssignmentPreview_CountsOnlyImportableRows(t *testing.T) {
	_, _, fixed := fixedAssignmentFixture(t)
	assert.Nil(t, BuildImportFixedAssignmentPreview(nil, nil))

	got := BuildImportFixedAssignmentPreview(fixed, []models.ImportRowPreview{
		{SuggestedAction: "add"},
		{SuggestedAction: "update"},
		{SuggestedAction: "skip"},
		{SuggestedAction: "add", ValidationErrors: []string{"bad"}},
	})
	require.NotNil(t, got)
	assert.Equal(t, 2, got.ContactCount)
	assert.Equal(t, 4, got.MembershipCount)
}

// A failing membership write must be reported per row without aborting the
// batch -- the same contract MaterializeImportedGroupings has.
func TestConfirm_FixedAssignmentWriteFailure_IsReportedPerRow(t *testing.T) {
	log := testImportLogger()
	req := func(id string, action string, fixed *ImportFixedAssignment) models.ImportConfirmRequest {
		return models.ImportConfirmRequest{
			SessionID:       id,
			Actions:         []models.RowImportAction{{RowIndex: 0, Action: action}},
			AssignCircleIDs: []string{fixed.Circles[0].ID},
		}
	}

	t.Run("confirm add and update", func(t *testing.T) {
		db, userID, fixed := fixedAssignmentFixture(t)
		models.DisableAudit(db)
		existing := models.Contact{UserID: userID, Firstname: "Old", Email: "old@example.com"}
		require.NoError(t, db.Create(&existing).Error)
		m := NewImportSessionManager()

		addID := m.CreateVCFSession(userID, []VCFContactData{{Contact: &models.Contact{Firstname: "New", Email: "new@example.com"}}},
			[]models.ImportRowPreview{{RowIndex: 0, SuggestedAction: "add"}})
		updID := m.CreateVCFSession(userID, []VCFContactData{{Contact: &models.Contact{Firstname: "Newer", Email: "old@example.com"}}},
			[]models.ImportRowPreview{{RowIndex: 0, SuggestedAction: "update", DuplicateMatch: &models.DuplicateMatch{ExistingContactID: existing.ID}}})
		dbtest.HideTable(t, db, "circle_members")

		res, appErr := m.Confirm(db, userID, req(addID, "add", fixed), log)
		require.Nil(t, appErr)
		assert.Equal(t, 1, res.Created)
		require.Len(t, res.Errors, 1)
		assert.Contains(t, res.Errors[0], "apply fixed circles/tags")

		res, appErr = m.Confirm(db, userID, req(updID, "update", fixed), log)
		require.Nil(t, appErr)
		assert.Equal(t, 1, res.Updated)
		require.Len(t, res.Errors, 1)
		assert.Contains(t, res.Errors[0], "apply fixed circles/tags")
	})

	t.Run("confirm vcf add and update", func(t *testing.T) {
		db, userID, fixed := fixedAssignmentFixture(t)
		models.DisableAudit(db)
		existing := models.Contact{UserID: userID, Firstname: "Old", Email: "old2@example.com"}
		require.NoError(t, db.Create(&existing).Error)
		m := NewImportSessionManager()

		addID := m.CreateVCFSession(userID, []VCFContactData{{Contact: &models.Contact{Firstname: "New", Email: "new2@example.com"}}},
			[]models.ImportRowPreview{{RowIndex: 0, SuggestedAction: "add"}})
		updID := m.CreateRecordsSession(userID, []VCFContactData{{Contact: &models.Contact{Firstname: "Newer", Email: "old2@example.com"}}},
			[]models.ImportRowPreview{{RowIndex: 0, SuggestedAction: "update", DuplicateMatch: &models.DuplicateMatch{ExistingContactID: existing.ID}}})
		dbtest.HideTable(t, db, "circle_members")
		cfg := testImportConfig(t)

		res, appErr := m.ConfirmVCF(db, userID, req(addID, "add", fixed), cfg, log)
		require.Nil(t, appErr)
		assert.Equal(t, 1, res.Created)
		require.Len(t, res.Errors, 1)
		assert.Contains(t, res.Errors[0], "apply fixed circles/tags")

		res, appErr = m.ConfirmVCF(db, userID, req(updID, "update", fixed), cfg, log)
		require.Nil(t, appErr)
		assert.Equal(t, 1, res.Updated)
		require.Len(t, res.Errors, 1)
		assert.Contains(t, res.Errors[0], "apply fixed circles/tags")
	})
}

func TestConfirm_PreviewResolveErrors_Surface(t *testing.T) {
	db, userID, _ := fixedAssignmentFixture(t)
	models.DisableAudit(db)
	m := NewImportSessionManager()
	id := m.CreateCSVSession(userID, []string{"First Name", "Last Name", "Email"}, [][]string{{"A", "B", "a@example.com"}})
	unknown := "00000000-0000-4000-8000-000000000000"

	_, appErr := m.PreviewCSV(db, userID, models.ImportPreviewRequest{SessionID: id, Mappings: csvMappings(), AssignTagIDs: []string{unknown}})
	require.NotNil(t, appErr)
	assert.Equal(t, apperrors.ErrCodeInvalidInput, appErr.Code)

	_, appErr = m.PreviewCSV(db, userID, models.ImportPreviewRequest{SessionID: id, Mappings: csvMappings()})
	require.Nil(t, appErr)
	_, appErr = m.Confirm(db, userID, models.ImportConfirmRequest{SessionID: id, AssignCircleIDs: []string{unknown}}, testImportLogger())
	require.NotNil(t, appErr)
	assert.Equal(t, apperrors.ErrCodeInvalidInput, appErr.Code)
}
