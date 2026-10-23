package services

import (
	"encoding/json"
	"mycorrhizal/internal/logtest"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1830: the account bundle carries users.time_format.

func importBundleInto(t *testing.T, bundle *models.AccountBundle) (models.User, error) {
	t.Helper()
	targetDB := dbtest.New(t)
	target := bundleTestUser(t, targetDB, "tf-target")
	useBundlePhotoDir(t)
	_, _, err := ExecuteSourceImportWithActions(
		t.Context(), targetDB, target.ID, MapAccountBundle(bundle), map[string]SourceContactAction{}, nil)
	var got models.User
	require.NoError(t, targetDB.First(&got, target.ID).Error)
	return got, err
}

func TestAccountBundle_TimeFormatRoundTrips(t *testing.T) {
	logtest.Guard(t) // happy-path tests fail on unexpected warn/error logs (issue #1474)
	for _, want := range []string{"12h", "24h"} {
		t.Run(want, func(t *testing.T) {
			ownerDB := dbtest.New(t)
			owner := bundleTestUser(t, ownerDB, "tf-owner")
			require.NoError(t, ownerDB.Model(&owner).Update("time_format", want).Error)

			exported, _, err := BuildAccountBundle(ownerDB, owner.ID, t.TempDir())
			require.NoError(t, err)
			assert.Equal(t, want, exported.TimeFormat)

			// Through the wire shape too.
			raw, err := json.Marshal(exported)
			require.NoError(t, err)
			var wire models.AccountBundle
			require.NoError(t, json.Unmarshal(raw, &wire))

			got, err := importBundleInto(t, &wire)
			require.NoError(t, err)
			assert.Equal(t, want, got.TimeFormat, "the destination adopts the bundle's preference")

			// And a re-export from the destination carries it again.
			targetDB := dbtest.New(t)
			target := bundleTestUser(t, targetDB, "tf-target2")
			useBundlePhotoDir(t)
			_, _, err = ExecuteSourceImportWithActions(t.Context(), targetDB, target.ID, MapAccountBundle(exported), map[string]SourceContactAction{}, nil)
			require.NoError(t, err)
			re, _, err := BuildAccountBundle(targetDB, target.ID, t.TempDir())
			require.NoError(t, err)
			assert.Equal(t, want, re.TimeFormat)
		})
	}
}

func TestAccountBundle_AutoIsExportedAndKeepsDestinationDefault(t *testing.T) {
	logtest.Guard(t) // happy-path tests fail on unexpected warn/error logs (issue #1474)
	ownerDB := dbtest.New(t)
	owner := bundleTestUser(t, ownerDB, "tf-owner-auto")
	exported, _, err := BuildAccountBundle(ownerDB, owner.ID, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "auto", exported.TimeFormat)

	got, err := importBundleInto(t, exported)
	require.NoError(t, err)
	assert.Equal(t, "auto", got.TimeFormat)
}

func TestAccountBundle_OlderBundleWithoutTimeFormatImportsAsAuto(t *testing.T) {
	logtest.Guard(t) // happy-path tests fail on unexpected warn/error logs (issue #1474)
	old := `{"format":"mycorrhizal-account","version":1,"exported_at":"2026-01-01T00:00:00Z",` +
		`"plan":{"contacts":[]},"attachments":[],"omitted":{"attachments":true,"photos_omitted":0}}`
	var bundle models.AccountBundle
	require.NoError(t, json.Unmarshal([]byte(old), &bundle))
	assert.Empty(t, bundle.TimeFormat)
	assert.Empty(t, MapAccountBundle(&bundle).TimeFormat)

	got, err := importBundleInto(t, &bundle)
	require.NoError(t, err)
	assert.Equal(t, "auto", got.TimeFormat, "a bundle without the field leaves a fresh account on auto")
}

func TestAccountBundle_TimeFormatRawJSONPresence(t *testing.T) {
	logtest.Guard(t) // happy-path tests fail on unexpected warn/error logs (issue #1474)
	db := dbtest.New(t)
	user := bundleTestUser(t, db, "tf-raw")
	require.NoError(t, db.Model(&user).Update("time_format", "24h").Error)
	exported, _, err := BuildAccountBundle(db, user.ID, t.TempDir())
	require.NoError(t, err)
	raw, err := json.Marshal(exported)
	require.NoError(t, err)
	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &top))
	require.Contains(t, top, "time_format")
	assert.JSONEq(t, `"24h"`, string(top["time_format"]))
}

func TestAccountBundle_ImportNeverOverwritesDestinationChoice(t *testing.T) {
	logtest.Guard(t) // happy-path tests fail on unexpected warn/error logs (issue #1474)
	targetDB := dbtest.New(t)
	target := bundleTestUser(t, targetDB, "tf-chosen")
	require.NoError(t, targetDB.Model(&target).Update("time_format", "12h").Error)
	useBundlePhotoDir(t)

	bundle := &models.AccountBundle{Format: models.AccountBundleFormat, Version: models.AccountBundleVersion, TimeFormat: "24h"}
	_, _, err := ExecuteSourceImportWithActions(
		t.Context(), targetDB, target.ID, MapAccountBundle(bundle), map[string]SourceContactAction{}, nil)
	require.NoError(t, err)
	var got models.User
	require.NoError(t, targetDB.First(&got, target.ID).Error)
	assert.Equal(t, "12h", got.TimeFormat, "an explicit choice on the destination wins")
}

func TestAccountBundle_UnknownTimeFormatIsDropped(t *testing.T) {
	logtest.Guard(t) // happy-path tests fail on unexpected warn/error logs (issue #1474)
	bundle := &models.AccountBundle{Format: models.AccountBundleFormat, Version: models.AccountBundleVersion, TimeFormat: "13h"}
	assert.Empty(t, MapAccountBundle(bundle).TimeFormat)
	got, err := importBundleInto(t, bundle)
	require.NoError(t, err)
	assert.Equal(t, "auto", got.TimeFormat)
}
