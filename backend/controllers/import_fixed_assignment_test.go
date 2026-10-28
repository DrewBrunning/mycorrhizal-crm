package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #2000: fixed circles/tags assigned to every imported contact.

func uploadFixedAssignmentCSV(t *testing.T, router http.Handler, content []byte) string {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/upload", "contacts.csv", content))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp models.ImportUploadResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.SessionID
}

func fixedAssignmentMappings() []models.ColumnMapping {
	return []models.ColumnMapping{
		{CSVColumn: "First Name", ContactField: "firstname"},
		{CSVColumn: "Last Name", ContactField: "lastname"},
		{CSVColumn: "Email", ContactField: "email"},
	}
}

func seedFixedAssignmentEntities(t *testing.T, db *gorm.DB, userID uint) (models.Circle, models.Tag) {
	t.Helper()
	circle := models.Circle{UserID: userID, Name: "Conference 2026"}
	require.NoError(t, db.Create(&circle).Error)
	tag := models.Tag{UserID: userID, Name: "imported"}
	require.NoError(t, db.Create(&tag).Error)
	return circle, tag
}

func previewFixed(t *testing.T, router http.Handler, sessionID string, circleIDs, tagIDs []string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, newJSONRequest(t, "/contacts/import/preview", models.ImportPreviewRequest{
		SessionID:       sessionID,
		Mappings:        fixedAssignmentMappings(),
		AssignCircleIDs: circleIDs,
		AssignTagIDs:    tagIDs,
	}))
	return w
}

func confirmFixed(t *testing.T, router http.Handler, sessionID string, actions []models.RowImportAction, circleIDs, tagIDs []string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, newJSONRequest(t, "/contacts/import/confirm", models.ImportConfirmRequest{
		SessionID:       sessionID,
		Actions:         actions,
		AssignCircleIDs: circleIDs,
		AssignTagIDs:    tagIDs,
	}))
	return w
}

func TestImportFixedAssignment_PreviewShowsAssignmentAndCounts(t *testing.T) {
	db, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})
	var user models.User
	db.First(&user)
	circle, tag := seedFixedAssignmentEntities(t, db, user.ID)

	sid := uploadFixedAssignmentCSV(t, router, csvFixture(
		[3]string{"A", "One", "a1@example.com"},
		[3]string{"B", "Two", "b2@example.com"},
	))
	w := previewFixed(t, router, sid, []string{circle.ID}, []string{tag.ID})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.FixedAssignment)
	require.Len(t, resp.FixedAssignment.Circles, 1)
	assert.Equal(t, circle.ID, resp.FixedAssignment.Circles[0].ID)
	assert.Equal(t, "Conference 2026", resp.FixedAssignment.Circles[0].Name)
	require.Len(t, resp.FixedAssignment.Tags, 1)
	assert.Equal(t, "imported", resp.FixedAssignment.Tags[0].Name)
	assert.Equal(t, 2, resp.FixedAssignment.ContactCount)
	assert.Equal(t, 4, resp.FixedAssignment.MembershipCount, "2 contacts x (1 circle + 1 tag)")
}

func TestImportFixedAssignment_PreviewWithoutFieldsUnchanged(t *testing.T) {
	db, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})
	_ = db

	sid := uploadFixedAssignmentCSV(t, router, csvFixture([3]string{"A", "One", "a1@example.com"}))
	w := previewFixed(t, router, sid, nil, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	_, present := raw["fixed_assignment"]
	assert.False(t, present, "no fixed assignment requested -> key absent")
}

func TestImportFixedAssignment_ConfirmAppliesToEveryCreatedContact(t *testing.T) {
	db, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})
	var user models.User
	db.First(&user)
	circle, tag := seedFixedAssignmentEntities(t, db, user.ID)

	sid := uploadFixedAssignmentCSV(t, router, csvFixture(
		[3]string{"A", "One", "a1@example.com"},
		[3]string{"B", "Two", "b2@example.com"},
		[3]string{"C", "Skipped", "c3@example.com"},
	))
	require.Equal(t, http.StatusOK, previewFixed(t, router, sid, []string{circle.ID}, []string{tag.ID}).Code)

	w := confirmFixed(t, router, sid, []models.RowImportAction{
		{RowIndex: 0, Action: "add"}, {RowIndex: 1, Action: "add"}, {RowIndex: 2, Action: "skip"},
	}, []string{circle.ID}, []string{tag.ID})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var members []models.CircleMember
	require.NoError(t, db.Where("circle_id = ?", circle.ID).Find(&members).Error)
	assert.Len(t, members, 2, "skipped row must not be assigned")
	var tagged []models.ContactTag
	require.NoError(t, db.Where("tag_id = ?", tag.ID).Find(&tagged).Error)
	assert.Len(t, tagged, 2)
}

func TestImportFixedAssignment_DedupesAgainstPerRowMapping(t *testing.T) {
	db, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})
	var user models.User
	db.First(&user)
	circle, _ := seedFixedAssignmentEntities(t, db, user.ID)

	csv := []byte("First Name,Last Name,Email,Groups\nA,One,a1@example.com,conference 2026\n")
	sid := uploadFixedAssignmentCSV(t, router, csv)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, newJSONRequest(t, "/contacts/import/preview", models.ImportPreviewRequest{
		SessionID: sid,
		Mappings:  append(fixedAssignmentMappings(), models.ColumnMapping{CSVColumn: "Groups", ContactField: "circles"}),
	}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// The same circle is both a per-row value (case-insensitive) and fixed,
	// and the id is passed twice: still exactly one membership.
	cw := confirmFixed(t, router, sid, []models.RowImportAction{{RowIndex: 0, Action: "add"}},
		[]string{circle.ID, circle.ID}, nil)
	require.Equal(t, http.StatusOK, cw.Code, cw.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.CircleMember{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	var circles int64
	require.NoError(t, db.Model(&models.Circle{}).Where("user_id = ?", user.ID).Count(&circles).Error)
	assert.Equal(t, int64(1), circles, "per-row name must reuse the fixed circle, not create a twin")
}

func TestImportFixedAssignment_RejectsForeignAndUnknownIDs(t *testing.T) {
	db, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})
	var user models.User
	db.First(&user)
	circle, tag := seedFixedAssignmentEntities(t, db, user.ID)

	other := models.User{Username: "other-fixed", Email: "other-fixed@example.com", Password: "x"}
	require.NoError(t, db.Create(&other).Error)
	foreignCircle := models.Circle{UserID: other.ID, Name: "Theirs"}
	require.NoError(t, db.Create(&foreignCircle).Error)
	foreignTag := models.Tag{UserID: other.ID, Name: "theirs"}
	require.NoError(t, db.Create(&foreignTag).Error)

	sid := uploadFixedAssignmentCSV(t, router, csvFixture([3]string{"A", "One", "a1@example.com"}))
	unknown := "00000000-0000-4000-8000-000000000000"

	cases := []struct {
		name    string
		circles []string
		tags    []string
	}{
		{"foreign circle", []string{foreignCircle.ID}, nil},
		{"foreign tag", nil, []string{foreignTag.ID}},
		{"unknown circle", []string{circle.ID, unknown}, nil},
		{"unknown tag", nil, []string{tag.ID, unknown}},
		{"malformed id", []string{"not-a-uuid"}, nil},
	}
	for _, tc := range cases {
		t.Run("preview "+tc.name, func(t *testing.T) {
			assert.Equal(t, http.StatusBadRequest, previewFixed(t, router, sid, tc.circles, tc.tags).Code)
		})
	}

	require.Equal(t, http.StatusOK, previewFixed(t, router, sid, nil, nil).Code)
	for _, tc := range cases {
		t.Run("confirm "+tc.name, func(t *testing.T) {
			w := confirmFixed(t, router, sid, []models.RowImportAction{{RowIndex: 0, Action: "add"}}, tc.circles, tc.tags)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}

	// Rejected confirms consumed nothing and created nothing.
	var contacts, members int64
	require.NoError(t, db.Model(&models.Contact{}).Where("email = ?", "a1@example.com").Count(&contacts).Error)
	require.NoError(t, db.Model(&models.CircleMember{}).Where("circle_id = ?", foreignCircle.ID).Count(&members).Error)
	assert.Zero(t, contacts)
	assert.Zero(t, members)

	ok := confirmFixed(t, router, sid, []models.RowImportAction{{RowIndex: 0, Action: "add"}}, []string{circle.ID}, nil)
	assert.Equal(t, http.StatusOK, ok.Code, "session survives a rejected confirm")
}

func TestImportFixedAssignment_ReImportUpdateIsIdempotent(t *testing.T) {
	db, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})
	var user models.User
	db.First(&user)
	circle, tag := seedFixedAssignmentEntities(t, db, user.ID)

	run := func() {
		sid := uploadFixedAssignmentCSV(t, router, csvFixture([3]string{"A", "One", "a1@example.com"}))
		pw := previewFixed(t, router, sid, []string{circle.ID}, []string{tag.ID})
		require.Equal(t, http.StatusOK, pw.Code)
		var pr models.ImportPreviewResponse
		require.NoError(t, json.Unmarshal(pw.Body.Bytes(), &pr))
		action := "add"
		if pr.Rows[0].DuplicateMatch != nil {
			action = "update"
		}
		w := confirmFixed(t, router, sid, []models.RowImportAction{{RowIndex: 0, Action: action}}, []string{circle.ID}, []string{tag.ID})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	run()
	run()

	var members, tagged, contacts int64
	require.NoError(t, db.Model(&models.CircleMember{}).Where("circle_id = ?", circle.ID).Count(&members).Error)
	require.NoError(t, db.Model(&models.ContactTag{}).Where("tag_id = ?", tag.ID).Count(&tagged).Error)
	require.NoError(t, db.Model(&models.Contact{}).Where("email = ?", "a1@example.com").Count(&contacts).Error)
	assert.Equal(t, int64(1), contacts)
	assert.Equal(t, int64(1), members)
	assert.Equal(t, int64(1), tagged)
}

func TestImportFixedAssignment_RecordsSessionViaVCFConfirm(t *testing.T) {
	db, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})
	var user models.User
	db.First(&user)
	circle, tag := seedFixedAssignmentEntities(t, db, user.ID)

	vw := httptest.NewRecorder()
	router.ServeHTTP(vw, newFileUploadRequest(t, "/contacts/import/vcf/upload", "c.vcf", vcfFixture("Vic", "Card", "vic@example.com", "")))
	require.Equal(t, http.StatusOK, vw.Code, vw.Body.String())
	var up models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(vw.Body.Bytes(), &up))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, newJSONRequest(t, "/contacts/import/vcf/confirm", models.ImportConfirmRequest{
		SessionID:       up.SessionID,
		Actions:         []models.RowImportAction{{RowIndex: 0, Action: "add"}},
		AssignCircleIDs: []string{circle.ID},
		AssignTagIDs:    []string{tag.ID},
	}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var contact models.Contact
	require.NoError(t, db.Where("email = ?", "vic@example.com").First(&contact).Error)
	names, err := services.CircleNamesForContact(db, user.ID, contact.VCardUID)
	require.NoError(t, err)
	assert.Equal(t, []string{"Conference 2026"}, names)
	tags, err := services.TagNamesForContact(db, user.ID, contact.VCardUID)
	require.NoError(t, err)
	assert.Equal(t, []string{"imported"}, tags)
}
