package controllers

// Issue #416: secret/token export-exclusion pinning tests. None of the
// export handlers (ExportData, ExportContactsAsVCF, ExportContactsAsJSContact)
// query models.User/models.ApiToken at all -- they only ever touch
// contact-adjacent tables -- so this is expected to already pass. The value
// is having it pinned: a future change that widens what ExportData reads
// (e.g. "export everything about my account") could otherwise regress this
// silently.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	exportSecretTestTokenHash = "sekrit-token-hash-should-never-leak-9f8e7d6c"
	exportSecretTestTOTP      = "sekrit-totp-encrypted-blob-should-never-leak"
)

func TestExports_NeverLeakApiTokenOrTOTPSecret(t *testing.T) {
	db, router := setupRouter()
	registerVCFRoute(router, "")
	registerJSContactRoute(router, "")
	router.GET("/export", ExportData)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	totp := exportSecretTestTOTP
	user.TOTPSecretEncrypted = &totp
	require.NoError(t, db.Save(&user).Error)

	require.NoError(t, db.Create(&models.ApiToken{
		UserID:    user.ID,
		Name:      "test token",
		TokenHash: exportSecretTestTokenHash,
	}).Error)

	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Anderson"}).Error)

	cases := []struct {
		name string
		path string
	}{
		{"csv combined export", "/export"},
		{"vcf export, default", "/export/vcf"},
		{"vcf export, include_sensitive=true", "/export/vcf?include_sensitive=true"},
		{"jscontact export, default", "/export/jscontact"},
		{"jscontact export, include_sensitive=true", "/export/jscontact?include_sensitive=true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tc.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			body := w.Body.String()
			assert.NotContains(t, body, exportSecretTestTokenHash, "API token hash must never appear in an export")
			assert.NotContains(t, body, exportSecretTestTOTP, "TOTP secret must never appear in an export")
		})
	}
}

// TestExportData_UserJSONFieldsAreTaggedJSONIgnore is a narrower, static
// pinning of the same guarantee at the model layer: even if some future
// code path did marshal a models.User directly into a response, the two
// secret fields are tagged json:"-" so encoding/json would omit them
// regardless of query-layer discipline.
func TestExportData_UserJSONFieldsAreTaggedJSONIgnore(t *testing.T) {
	totp := exportSecretTestTOTP
	user := models.User{TOTPSecretEncrypted: &totp}
	raw, err := json.Marshal(user)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), exportSecretTestTOTP)

	token := models.ApiToken{TokenHash: exportSecretTestTokenHash}
	raw, err = json.Marshal(token)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), exportSecretTestTokenHash)
}
