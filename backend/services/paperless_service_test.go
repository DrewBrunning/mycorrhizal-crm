package services

import (
	"mycorrhizal/config"
	"mycorrhizal/models"
	"net/http"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// paperlessTestConfig returns a config with a JWT secret the tests can encrypt
// against (credential_crypto.go derives the key from it).
func paperlessTestConfig() config.Config {
	return config.Config{
		JWTSecretKey: "test-jwt-secret-0123456789abcdef0123456789abcdef",
	}
}

// newPaperlessTestDB migrates the models the Paperless service touches.
func newPaperlessTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.PaperlessConfig{}, &models.ExternalIdentity{}))
	return db
}

func seedPaperlessUser(t *testing.T, db *gorm.DB) (models.User, models.Contact) {
	t.Helper()
	user := models.User{Username: "paperless-user", Password: "password123!A", Email: "paperless@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)
	return user, contact
}

// connectPaperlessForUser writes a config for a user against the given fake
// server. An empty token uses a dummy (the fake server with no required token
// accepts anything; UpsertPaperlessConfig still requires a non-empty token on
// create).
func connectPaperlessForUser(t *testing.T, db *gorm.DB, userID uint, baseURL, token string) {
	t.Helper()
	if token == "" {
		token = "test-token"
	}
	_, err := UpsertPaperlessConfig(db, paperlessTestConfig().JWTSecretKey, userID, models.PaperlessConfigInput{
		BaseURL: baseURL, APIToken: token,
	})
	require.NoError(t, err)
}

func TestNormalizePaperlessBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "plain https", in: "https://paperless.example", want: "https://paperless.example"},
		{name: "trailing slash stripped", in: "https://paperless.example/", want: "https://paperless.example"},
		{name: "whitespace trimmed", in: "  https://paperless.example  ", want: "https://paperless.example"},
		{name: "path preserved", in: "https://example.com/paperless", want: "https://example.com/paperless"},
		{name: "scheme-less rejected", in: "paperless.example", wantErr: true},
		{name: "ftp rejected", in: "ftp://paperless.example", wantErr: true},
		{name: "empty rejected", in: "", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizePaperlessBaseURL(tc.in)
			if tc.wantErr {
				assert.ErrorIs(t, err, ErrPaperlessInvalidURL)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestLinkPaperlessDocument_FetchesAuthoritativeMetadataAndBuildsURL(t *testing.T) {
	db := newPaperlessTestDB(t)
	user, contact := seedPaperlessUser(t, db)

	fake := newFakePaperlessServer(t, "sekret")
	defer fake.Close()
	fake.Docs[42] = &fakePaperlessDoc{Title: "Signed Contract", FileName: "contract.pdf", Created: "2026-03-01", Added: "2026-03-10T10:00:00Z"}
	connectPaperlessForUser(t, db, user.ID, fake.URL(), "sekret")

	identity, err := LinkPaperlessDocument(db, paperlessTestConfig(), user.ID, contact.VCardUID, "42")
	require.NoError(t, err)

	assert.Equal(t, ExternalSystemPaperless, identity.System)
	assert.Equal(t, "42", identity.ExternalID)
	assert.Equal(t, fake.URL()+"/documents/42/details", identity.URL)
	assert.Equal(t, "Signed Contract", identity.Metadata["title"])
	assert.Equal(t, "contract.pdf", identity.Metadata["file_name"])
	// The metadata must come from the server, not be trusted from the client —
	// the document wasn't even sent to the service.
	require.Equal(t, "sekret", fake.LastToken, "the request must authenticate with the stored token")

	// The link is a real ExternalIdentity row.
	var persisted models.ExternalIdentity
	require.NoError(t, db.First(&persisted, "id = ?", identity.ID).Error)
	assert.Equal(t, contact.VCardUID, persisted.EntityID)
}

func TestListPaperlessDocumentsForUser_BrowsesAndSearches(t *testing.T) {
	db := newPaperlessTestDB(t)
	user, _ := seedPaperlessUser(t, db)

	fake := newFakePaperlessServer(t, "sekret")
	defer fake.Close()
	fake.Docs[1] = &fakePaperlessDoc{Title: "Lease Agreement", FileName: "lease.pdf", Created: "2026-01-15", Added: "2026-01-20T10:00:00Z"}
	fake.Docs[2] = &fakePaperlessDoc{Title: "Passport", FileName: "passport.pdf", Created: "2026-02-01", Added: "2026-02-05T10:00:00Z"}
	connectPaperlessForUser(t, db, user.ID, fake.URL(), "sekret")

	docs, err := ListPaperlessDocumentsForUser(db, paperlessTestConfig(), user.ID, "")
	require.NoError(t, err)
	require.Len(t, docs, 2)

	filtered, err := ListPaperlessDocumentsForUser(db, paperlessTestConfig(), user.ID, "passport")
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, "Passport", filtered[0].Title)
}

func TestTestPaperlessConnection_SuccessAndDiagnosis(t *testing.T) {
	db := newPaperlessTestDB(t)
	user, _ := seedPaperlessUser(t, db)

	fake := newFakePaperlessServer(t, "sekret")
	defer fake.Close()
	fake.Me = map[string]any{"user_name": "alice", "id": 2}
	connectPaperlessForUser(t, db, user.ID, fake.URL(), "sekret")

	result, err := TestPaperlessConnection(db, paperlessTestConfig(), user.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.OK)
	assert.Equal(t, "ok", result.Stage)
	assert.Contains(t, result.Message, "alice")

	// A diagnosed auth failure (wrong token) is a non-error result with
	// OK:false, not a Go error.
	require.NoError(t, db.Model(&models.PaperlessConfig{}).Where("user_id = ?", user.ID).
		Update("api_token_encrypted", mustEncrypt(t, "test-jwt-secret-0123456789abcdef0123456789abcdef", "wrong")).Error)

	result2, err := TestPaperlessConnection(db, paperlessTestConfig(), user.ID)
	require.NoError(t, err)
	require.NotNil(t, result2)
	assert.False(t, result2.OK)
	assert.Equal(t, "auth", result2.Stage)
}

// TestPaperlessClient_RejectsUnexpectedStatus pins the defensive client
// behavior: a real non-2xx response from a live instance is a RequestFailed
// (distinct from Unreachable), never a transport error.
func TestPaperlessClient_RejectsUnexpectedStatus(t *testing.T) {
	fake := newFakePaperlessServer(t, "sekret")
	defer fake.Close()
	fake.FailWithStatus = http.StatusBadGateway

	client, err := NewPaperlessClient(fake.URL(), "sekret", false)
	require.NoError(t, err)
	_, err = client.GetDocument(1)
	assert.ErrorIs(t, err, ErrPaperlessRequestFailed)
}

// TestPaperlessClient_GetDocumentMissingIsNotFound pins the 404 mapping: a
// document id the server does not know maps to ErrPaperlessNotFound, which the
// controller surfaces as a 404 (not a 503 "instance down").
func TestPaperlessClient_GetDocumentMissingIsNotFound(t *testing.T) {
	fake := newFakePaperlessServer(t, "sekret")
	defer fake.Close()

	client, err := NewPaperlessClient(fake.URL(), "sekret", false)
	require.NoError(t, err)
	_, err = client.GetDocument(999)
	assert.ErrorIs(t, err, ErrPaperlessNotFound)
}
