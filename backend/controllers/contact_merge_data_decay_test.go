package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContactMerge_DataDecayPolicyConflict covers issue #352's merge-conflict
// type, mirroring TestContactMerge_CadencePolicyConflict: DataDecayPolicy
// carries the same one-per-contact partial unique index shape
// (migration 000065) as CadencePolicy, so it can't be unioned or plain
// re-pointed either. Three independent pairs: pair A shows the commit is
// rejected until resolved; pair B resolves it toward the loser's policy and
// confirms the keeper actually ends up with the loser's values; pair C
// covers a stale resolution value (matches neither side) being a hard
// rejection rather than a silent guess.
func TestContactMerge_DataDecayPolicyConflict(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "contact-merge-data-decay-conflict.db")
	db := dbtest.NewAt(t, dbPath)
	closeTestDBAtTeardown(t, db)

	user := models.User{Username: "decaymergetester", Password: "password123!A", Email: "decaymerge@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST("/contacts/merge/preview", withValidated(func() any { return &models.ContactMergeRequest{} }), PreviewContactMerge)
	router.POST("/contacts/merge", withValidated(func() any { return &models.ContactMergeRequest{} }), CommitContactMerge)

	doJSON := func(path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
		req, _ := http.NewRequest("POST", path, &buf)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// --- Pair A: rejected until resolved ---
	aliceA := models.Contact{UserID: user.ID, Firstname: "AliceA"}
	bobA := models.Contact{UserID: user.ID, Firstname: "AliceA"} // same firstname: no scalar conflict to muddy this test
	require.NoError(t, db.Create(&aliceA).Error)
	require.NoError(t, db.Create(&bobA).Error)
	require.NoError(t, db.Create(&models.DataDecayPolicy{UserID: user.ID, EntityID: aliceA.VCardUID, IntervalDays: 30, Active: true}).Error)
	require.NoError(t, db.Create(&models.DataDecayPolicy{UserID: user.ID, EntityID: bobA.VCardUID, IntervalDays: 90, Active: true}).Error)

	previewA := doJSON("/contacts/merge/preview", models.ContactMergeRequest{KeepID: aliceA.ID, MergeID: bobA.ID})
	require.Equal(t, http.StatusOK, previewA.Code, previewA.Body.String())
	var resolutionA models.ContactMergePreviewResponse
	require.NoError(t, json.Unmarshal(previewA.Body.Bytes(), &resolutionA))
	require.Len(t, resolutionA.Resolution.Conflicts, 1, "differing data decay policies on both sides must surface as exactly one conflict")
	assert.Equal(t, "data_decay_policy", resolutionA.Resolution.Conflicts[0].Field)
	assert.Equal(t, "Every 30 days (active)", resolutionA.Resolution.Conflicts[0].KeeperValue)
	assert.Equal(t, "Every 90 days (active)", resolutionA.Resolution.Conflicts[0].LoserValue)

	rejectA := doJSON("/contacts/merge", models.ContactMergeRequest{KeepID: aliceA.ID, MergeID: bobA.ID})
	require.Equal(t, http.StatusBadRequest, rejectA.Code, rejectA.Body.String())
	assert.Contains(t, rejectA.Body.String(), "data_decay_policy")

	// --- Pair B: resolved toward the loser's (paused) policy ---
	aliceB := models.Contact{UserID: user.ID, Firstname: "AliceB"}
	bobB := models.Contact{UserID: user.ID, Firstname: "AliceB"}
	require.NoError(t, db.Create(&aliceB).Error)
	require.NoError(t, db.Create(&bobB).Error)
	keeperPolicy := models.DataDecayPolicy{UserID: user.ID, EntityID: aliceB.VCardUID, IntervalDays: 14, Active: true}
	require.NoError(t, db.Create(&keeperPolicy).Error)
	loserPolicy := models.DataDecayPolicy{UserID: user.ID, EntityID: bobB.VCardUID, IntervalDays: 60, Active: false}
	require.NoError(t, db.Create(&loserPolicy).Error)

	commitB := doJSON("/contacts/merge", models.ContactMergeRequest{
		KeepID: aliceB.ID, MergeID: bobB.ID,
		Resolutions: map[string]string{"data_decay_policy": "Every 60 days (paused)"},
	})
	require.Equal(t, http.StatusOK, commitB.Code, commitB.Body.String())

	var survivingPolicy models.DataDecayPolicy
	require.NoError(t, db.Where("entity_id = ? AND user_id = ?", aliceB.VCardUID, user.ID).First(&survivingPolicy).Error)
	assert.Equal(t, 60, survivingPolicy.IntervalDays, "the chosen (loser's) policy must win, not the keeper's original")
	assert.False(t, survivingPolicy.Active)
	assert.Equal(t, loserPolicy.ID, survivingPolicy.ID, "the surviving row must be the loser's re-pointed, not a new one")

	var keeperPolicyGone int64
	require.NoError(t, db.Model(&models.DataDecayPolicy{}).Where("id = ?", keeperPolicy.ID).Count(&keeperPolicyGone).Error)
	assert.EqualValues(t, 0, keeperPolicyGone, "the keeper's original (unchosen) policy must be gone, not left behind as a duplicate")

	// --- Pair C: a resolution value that matches neither side is rejected,
	// not silently treated as "keeper wins" (the stale-preview case).
	aliceC := models.Contact{UserID: user.ID, Firstname: "AliceC"}
	bobC := models.Contact{UserID: user.ID, Firstname: "AliceC"}
	require.NoError(t, db.Create(&aliceC).Error)
	require.NoError(t, db.Create(&bobC).Error)
	require.NoError(t, db.Create(&models.DataDecayPolicy{UserID: user.ID, EntityID: aliceC.VCardUID, IntervalDays: 7, Active: true}).Error)
	loserPolicyC := models.DataDecayPolicy{UserID: user.ID, EntityID: bobC.VCardUID, IntervalDays: 21, Active: true}
	require.NoError(t, db.Create(&loserPolicyC).Error)

	commitC := doJSON("/contacts/merge", models.ContactMergeRequest{
		KeepID: aliceC.ID, MergeID: bobC.ID,
		Resolutions: map[string]string{"data_decay_policy": "Every 14 days (active)"},
	})
	require.Equal(t, http.StatusInternalServerError, commitC.Code, commitC.Body.String())

	var aliceCPolicy, bobCPolicy models.DataDecayPolicy
	require.NoError(t, db.Where("entity_id = ? AND user_id = ?", aliceC.VCardUID, user.ID).First(&aliceCPolicy).Error)
	assert.Equal(t, 7, aliceCPolicy.IntervalDays, "a rejected commit must not have touched the keeper's policy")
	require.NoError(t, db.Where("id = ?", loserPolicyC.ID).First(&bobCPolicy).Error)
	assert.Equal(t, 21, bobCPolicy.IntervalDays, "a rejected commit must not have dropped the loser's policy either")

	// --- Pair D: only the loser has a policy -- silent adoption, no conflict.
	aliceD := models.Contact{UserID: user.ID, Firstname: "AliceD"}
	bobD := models.Contact{UserID: user.ID, Firstname: "AliceD"}
	require.NoError(t, db.Create(&aliceD).Error)
	require.NoError(t, db.Create(&bobD).Error)
	onlyLoserPolicy := models.DataDecayPolicy{UserID: user.ID, EntityID: bobD.VCardUID, IntervalDays: 45, Active: true}
	require.NoError(t, db.Create(&onlyLoserPolicy).Error)

	previewD := doJSON("/contacts/merge/preview", models.ContactMergeRequest{KeepID: aliceD.ID, MergeID: bobD.ID})
	require.Equal(t, http.StatusOK, previewD.Code, previewD.Body.String())
	var resolutionD models.ContactMergePreviewResponse
	require.NoError(t, json.Unmarshal(previewD.Body.Bytes(), &resolutionD))
	assert.Empty(t, resolutionD.Resolution.Conflicts, "only one side having a policy must not surface as a conflict")

	commitD := doJSON("/contacts/merge", models.ContactMergeRequest{KeepID: aliceD.ID, MergeID: bobD.ID})
	require.Equal(t, http.StatusOK, commitD.Code, commitD.Body.String())

	var adoptedPolicy models.DataDecayPolicy
	require.NoError(t, db.Where("entity_id = ? AND user_id = ?", aliceD.VCardUID, user.ID).First(&adoptedPolicy).Error)
	assert.Equal(t, onlyLoserPolicy.ID, adoptedPolicy.ID, "the loser's policy must be silently adopted onto the keeper")
	assert.Equal(t, 45, adoptedPolicy.IntervalDays)

	// --- Pair E: a real DB failure computing the data-decay conflict (not a
	// "not found") must abort both preview and commit with a 500, not be
	// swallowed -- pins appendDataDecayPolicyConflict's error-wrapping branch
	// in both PreviewContactMerge and CommitContactMerge.
	aliceE := models.Contact{UserID: user.ID, Firstname: "AliceE"}
	bobE := models.Contact{UserID: user.ID, Firstname: "AliceE"}
	require.NoError(t, db.Create(&aliceE).Error)
	require.NoError(t, db.Create(&bobE).Error)

	dbtest.HideTable(t, db, "data_decay_policies")

	previewE := doJSON("/contacts/merge/preview", models.ContactMergeRequest{KeepID: aliceE.ID, MergeID: bobE.ID})
	require.Equal(t, http.StatusInternalServerError, previewE.Code, previewE.Body.String())

	commitE := doJSON("/contacts/merge", models.ContactMergeRequest{KeepID: aliceE.ID, MergeID: bobE.ID})
	require.Equal(t, http.StatusInternalServerError, commitE.Code, commitE.Body.String())
}
