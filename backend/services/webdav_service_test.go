package services

import (
	"mycorrhizal/models"
	"net/http"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newWebDAVTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.WebDAVConfig{}, &models.ExternalIdentity{}))
	return db
}

func seedWebDAVUser(t *testing.T, db *gorm.DB) (models.User, models.Contact) {
	t.Helper()
	user := models.User{Username: "webdav-user", Password: "password123!A", Email: "webdav@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)
	return user, contact
}

func connectWebDAVForUser(t *testing.T, db *gorm.DB, userID uint, baseURL, username, password string) {
	t.Helper()
	if password == "" {
		password = "test-pass"
	}
	_, err := UpsertWebDAVConfig(db, paperlessTestConfig().JWTSecretKey, userID, models.WebDAVConfigInput{
		BaseURL: baseURL, Username: username, AppPassword: password,
	})
	require.NoError(t, err)
}

// seedWebDAVFake populates the fake with a root, a Documents folder, and a
// file inside it.
func seedWebDAVFakeServer(fake *fakeWebDAVServer, username string) {
	modified := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC1123)
	fake.Items["/remote.php/dav/files/"+username] = &fakeWebDAVItem{Name: username, IsDir: true}
	fake.Items["/remote.php/dav/files/"+username+"/Documents"] = &fakeWebDAVItem{Name: "Documents", IsDir: true, Modified: modified}
	fake.Items["/remote.php/dav/files/"+username+"/Documents/contract.pdf"] = &fakeWebDAVItem{Name: "contract.pdf", IsDir: false, Size: 4096, Modified: modified, FileID: "00000123"}
}

func TestNormalizeWebDAVBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "plain https", in: "https://nc.example", want: "https://nc.example"},
		{name: "trailing slash stripped", in: "https://nc.example/", want: "https://nc.example"},
		{name: "scheme-less rejected", in: "nc.example", wantErr: true},
		{name: "empty rejected", in: "", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeWebDAVBaseURL(tc.in)
			if tc.wantErr {
				assert.ErrorIs(t, err, ErrWebDAVInvalidURL)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestListWebDAVDirForUser_BrowsesFake(t *testing.T) {
	db := newWebDAVTestDB(t)
	user, _ := seedWebDAVUser(t, db)

	fake := newFakeWebDAVServer(t, "alice", "sekret")
	defer fake.Close()
	seedWebDAVFakeServer(fake, "alice")
	connectWebDAVForUser(t, db, user.ID, fake.URL(), "alice", "sekret")

	// Root listing excludes the root itself.
	items, err := ListWebDAVDirForUser(db, paperlessTestConfig(), user.ID, "/")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Documents", items[0].Name)
	assert.Equal(t, "dir", items[0].Type)
	assert.Equal(t, "/Documents/", items[0].Path)
	assert.Equal(t, "alice", fake.LastUser, "the request must authenticate with the stored username")
	assert.Equal(t, "sekret", fake.LastPass)

	// Inside Documents: the file with size + file id.
	items, err = ListWebDAVDirForUser(db, paperlessTestConfig(), user.ID, "/Documents")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "contract.pdf", items[0].Name)
	assert.EqualValues(t, 4096, items[0].Size)
	assert.Equal(t, "00000123", items[0].FileID)
	assert.Equal(t, "/Documents/contract.pdf", items[0].Path)
	require.NotEmpty(t, items[0].ModifiedAt)
	parsed, err := time.Parse(time.RFC3339, items[0].ModifiedAt)
	require.NoError(t, err)
	assert.Equal(t, 2026, parsed.Year())
}

func TestLinkWebDAVItem_StoresPathAndDeepLink(t *testing.T) {
	db := newWebDAVTestDB(t)
	user, contact := seedWebDAVUser(t, db)

	fake := newFakeWebDAVServer(t, "alice", "sekret")
	defer fake.Close()
	connectWebDAVForUser(t, db, user.ID, fake.URL(), "alice", "sekret")

	identity, err := LinkWebDAVItem(db, paperlessTestConfig(), user.ID, contact.VCardUID, WebDAVLinkMetadata{
		Path: "/Documents/contract.pdf", Name: "contract.pdf", Type: "file", Size: 4096, FileID: "00000123",
	})
	require.NoError(t, err)
	assert.Equal(t, ExternalSystemWebDAV, identity.System)
	assert.Equal(t, "/Documents/contract.pdf", identity.ExternalID)
	assert.Contains(t, identity.URL, "/apps/files/?dir=%2FDocuments&openfile=00000123")

	// A folder deep-links to its own directory, no openfile.
	dirIdentity, err := LinkWebDAVItem(db, paperlessTestConfig(), user.ID, contact.VCardUID, WebDAVLinkMetadata{
		Path: "/Documents", Name: "Documents", Type: "dir",
	})
	require.NoError(t, err)
	assert.Contains(t, dirIdentity.URL, "/apps/files/?dir=%2FDocuments")
	assert.NotContains(t, dirIdentity.URL, "openfile")
}

func TestTestWebDAVConnection_SuccessAndDiagnosis(t *testing.T) {
	db := newWebDAVTestDB(t)
	user, _ := seedWebDAVUser(t, db)

	fake := newFakeWebDAVServer(t, "alice", "sekret")
	defer fake.Close()
	seedWebDAVFakeServer(fake, "alice")
	connectWebDAVForUser(t, db, user.ID, fake.URL(), "alice", "sekret")

	result, err := TestWebDAVConnection(db, paperlessTestConfig(), user.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.OK)

	// Wrong app password → diagnosed failure (non-error, OK:false).
	require.NoError(t, db.Model(&models.WebDAVConfig{}).Where("user_id = ?", user.ID).
		Update("app_password_encrypted", mustEncrypt(t, "test-jwt-secret-0123456789abcdef0123456789abcdef", "wrong")).Error)
	result2, err := TestWebDAVConnection(db, paperlessTestConfig(), user.ID)
	require.NoError(t, err)
	require.NotNil(t, result2)
	assert.False(t, result2.OK)
}

func TestWebDAVClient_RejectsUnexpectedStatus(t *testing.T) {
	fake := newFakeWebDAVServer(t, "alice", "sekret")
	defer fake.Close()
	fake.FailWithStatus = http.StatusBadGateway

	client, err := NewWebDAVClient(fake.URL(), "alice", "sekret", false)
	require.NoError(t, err)
	_, err = client.ListDir("/")
	assert.ErrorIs(t, err, ErrWebDAVRequestFailed)
}
