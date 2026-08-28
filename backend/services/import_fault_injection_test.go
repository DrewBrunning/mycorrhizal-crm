package services

import (
	"errors"
	"testing"

	"mycorrhizal/internal/faults"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfirmInjectedDBErrorFailsClosed pins the CSV-confirm half of the
// services.import.confirm seam (issue #434): an armed fault at the transaction
// boundary fails the whole confirm closed — every contact row the confirm
// would have created rolls back, the session is left unconsumed, and a retry
// after the fault clears applies the same import cleanly. Silent partial
// state is the exact failure TEST-06 exists to catch, and this test asserts
// its opposite: either all rows land, or none do.
func TestConfirmInjectedDBErrorFailsClosed(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	db := setupImportSessionTestDB(t)
	m := NewImportSessionManager()
	log := testImportLogger()

	headers := []string{"First Name", "Last Name", "Email"}
	rows := [][]string{
		{"Alice", "Smith", "alice@example.com"},
		{"Bob", "Jones", "bob@example.com"},
	}
	id := m.CreateCSVSession(1, headers, rows)
	_, appErr := m.PreviewCSV(db, 1, models.ImportPreviewRequest{SessionID: id, Mappings: csvMappings()})
	require.Nil(t, appErr)

	req := models.ImportConfirmRequest{
		SessionID: id,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}, {RowIndex: 1, Action: "add"}},
	}

	injected := errors.New("injected db failure mid-import")
	faults.ArmError(faultImportConfirm, injected)

	result, appErr := m.Confirm(db, 1, req, log)
	assert.Nil(t, result, "an injected fault must not return a success result")
	require.NotNil(t, appErr, "an injected fault must surface as an error")
	assert.ErrorIs(t, appErr, injected, "the injected error must cross the boundary unwrapped for errors.Is")

	var count int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", 1).Count(&count).Error)
	assert.Equal(t, int64(0), count, "a failed-closed confirm must not leave partial contact rows")

	// The session was NOT consumed by the failed confirm — a retry with the
	// fault cleared applies the same import.
	faults.Disarm(faultImportConfirm)
	result, appErr = m.Confirm(db, 1, req, log)
	require.Nil(t, appErr)
	require.NotNil(t, result)
	assert.Equal(t, 2, result.Created)
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", 1).Count(&count).Error)
	assert.Equal(t, int64(2), count, "the retried confirm must land every row")
}

// TestConfirmVCFInjectedDBErrorFailsClosed pins the same contract on the
// VCF/records confirm path, which additionally runs photo processing after
// the transaction — the injection must fail before any of that can run.
func TestConfirmVCFInjectedDBErrorFailsClosed(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	db := setupImportSessionTestDB(t)
	m := NewImportSessionManager()
	log := testImportLogger()
	cfg := testImportConfig(t)

	contact := &models.Contact{Firstname: "Alice", Lastname: "Smith", Email: "alice@example.com"}
	id := m.CreateVCFSession(1, []VCFContactData{{Contact: contact}}, []models.ImportRowPreview{{RowIndex: 0, SuggestedAction: "add"}})

	req := models.ImportConfirmRequest{
		SessionID: id,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
	}

	injected := errors.New("injected db failure mid-import")
	faults.ArmError(faultImportConfirm, injected)

	result, appErr := m.ConfirmVCF(db, 1, req, cfg, log)
	assert.Nil(t, result)
	require.NotNil(t, appErr)
	assert.ErrorIs(t, appErr, injected)

	var count int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", 1).Count(&count).Error)
	assert.Equal(t, int64(0), count, "a failed-closed VCF confirm must not leave partial contact rows")

	faults.Disarm(faultImportConfirm)
	result, appErr = m.ConfirmVCF(db, 1, req, cfg, log)
	require.Nil(t, appErr)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Created)
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", 1).Count(&count).Error)
	assert.Equal(t, int64(1), count, "the retried VCF confirm must land the row")
}

// TestConfirmInjectedFaultDoesNotLeakToOtherSessions guards the seam's
// isolation: a fault armed for one confirm run must not affect a subsequent,
// unrelated confirm once disarmed — the Reset/Disarm contract.
func TestConfirmInjectedFaultDoesNotLeakToOtherSessions(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	db := setupImportSessionTestDB(t)
	m := NewImportSessionManager()
	log := testImportLogger()

	headers := []string{"First Name"}
	id := m.CreateCSVSession(1, headers, [][]string{{"Carol"}})
	_, appErr := m.PreviewCSV(db, 1, models.ImportPreviewRequest{SessionID: id, Mappings: []models.ColumnMapping{{CSVColumn: "First Name", ContactField: "firstname"}}})
	require.Nil(t, appErr)

	req := models.ImportConfirmRequest{SessionID: id, Actions: []models.RowImportAction{{RowIndex: 0, Action: "add"}}}

	// Arm, confirm (fails), disarm — exactly the test lifecycle above.
	faults.ArmError(faultImportConfirm, errors.New("boom"))
	_, appErr = m.Confirm(db, 1, req, log)
	require.NotNil(t, appErr)
	faults.Reset()

	// Unrelated confirm after Reset must be unaffected.
	result, appErr := m.Confirm(db, 1, req, log)
	require.Nil(t, appErr)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Created)
}
