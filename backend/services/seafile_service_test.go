package services

import (
	"mycorrhizal/models"
	"net/http"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newSeafileTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.SeafileConfig{}, &models.ExternalIdentity{}))
	return db
}

func seedSeafileUser(t *testing.T, db *gorm.DB) (models.User, models.Contact) {
	t.Helper()
	user := models.User{Username: "seafile-user", Password: "password123!A", Email: "seafile@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)
	return user, contact
}

func connectSeafileForUser(t *testing.T, db *gorm.DB, userID uint, baseURL, token string) {
	t.Helper()
	if token == "" {
		token = "test-token"
	}
	_, err := UpsertSeafileConfig(db, paperlessTestConfig().JWTSecretKey, userID, models.SeafileConfigInput{
		BaseURL: baseURL, APIToken: token,
	})
	require.NoError(t, err)
}

func TestNormalizeSeafileBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "plain https", in: "https://seafile.example", want: "https://seafile.example"},
		{name: "trailing slash stripped", in: "https://seafile.example/", want: "https://seafile.example"},
		{name: "scheme-less rejected", in: "seafile.example", wantErr: true},
		{name: "empty rejected", in: "", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeSeafileBaseURL(tc.in)
			if tc.wantErr {
				assert.ErrorIs(t, err, ErrSeafileInvalidURL)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestListSeafileLibrariesAndDirForUser_BrowsesFake(t *testing.T) {
	db := newSeafileTestDB(t)
	user, _ := seedSeafileUser(t, db)

	fake := newFakeSeafileServer(t, "sekret")
	defer fake.Close()
	fake.Libs["repo-1"] = &fakeSeafileLibrary{Name: "Personal"}
	fake.Dir["repo-1:/"] = []*fakeSeafileItem{
		{Name: "Documents", Type: "dir"},
		{Name: "contract.pdf", Type: "file", Size: 2048, MTime: 2000},
	}
	connectSeafileForUser(t, db, user.ID, fake.URL(), "sekret")

	libs, err := ListSeafileLibrariesForUser(db, paperlessTestConfig(), user.ID)
	require.NoError(t, err)
	require.Len(t, libs, 1)
	assert.Equal(t, "Personal", libs[0].Name)

	items, err := ListSeafileDirForUser(db, paperlessTestConfig(), user.ID, "repo-1", "/")
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "contract.pdf", items[1].Name)
	assert.EqualValues(t, 2048, items[1].Size)
	assert.Equal(t, "sekret", fake.LastToken, "the request must authenticate with the stored token")
}

func TestLinkSeafileItem_StoresRepoRelativeLinkAndMetadata(t *testing.T) {
	db := newSeafileTestDB(t)
	user, contact := seedSeafileUser(t, db)

	fake := newFakeSeafileServer(t, "sekret")
	defer fake.Close()
	connectSeafileForUser(t, db, user.ID, fake.URL(), "sekret")

	identity, err := LinkSeafileItem(db, paperlessTestConfig(), user.ID, contact.VCardUID, SeafileLinkMetadata{
		RepoID: "repo-1", Path: "/Documents/contract.pdf", Name: "contract.pdf", Type: "file", Size: 2048, MTime: 1000,
	})
	require.NoError(t, err)
	assert.Equal(t, "repo-1:/Documents/contract.pdf", identity.ExternalID)
	assert.Equal(t, fake.URL()+"/lib/repo-1/Documents/contract.pdf", identity.URL)
	assert.Equal(t, "contract.pdf", identity.Metadata["name"])

	// A directory keeps a trailing slash in the web URL so the Seafile web app
	// renders it as a folder.
	dirIdentity, err := LinkSeafileItem(db, paperlessTestConfig(), user.ID, contact.VCardUID, SeafileLinkMetadata{
		RepoID: "repo-1", Path: "/Documents", Name: "Documents", Type: "dir",
	})
	require.NoError(t, err)
	assert.Equal(t, fake.URL()+"/lib/repo-1/Documents/", dirIdentity.URL)
}

// TestSeafileWebURL_EscapesSpecialCharacters pins the URL-building fix for
// filenames with spaces/special characters: the stored deep-link URL must be a
// valid, percent-encoded http URL, not a raw URL with a space that would break
// down at render time.
func TestSeafileWebURL_EscapesSpecialCharacters(t *testing.T) {
	base := "https://seafile.example"

	file := seafileWebURL(base, "repo-1", "/Documents/My Contract (signed).pdf", "file")
	assert.Equal(t, "https://seafile.example/lib/repo-1/Documents/My%20Contract%20%28signed%29.pdf", file)

	dir := seafileWebURL(base, "repo-1", "/My Folder", "dir")
	assert.Equal(t, "https://seafile.example/lib/repo-1/My%20Folder/", dir)

	root := seafileWebURL(base, "repo-1", "/", "dir")
	assert.Equal(t, "https://seafile.example/lib/repo-1/", root)
}

func TestTestSeafileConnection_SuccessAndDiagnosis(t *testing.T) {
	db := newSeafileTestDB(t)
	user, _ := seedSeafileUser(t, db)

	fake := newFakeSeafileServer(t, "sekret")
	defer fake.Close()
	connectSeafileForUser(t, db, user.ID, fake.URL(), "sekret")

	result, err := TestSeafileConnection(db, paperlessTestConfig(), user.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.OK)

	// Wrong token → diagnosed auth failure (non-error, OK:false).
	require.NoError(t, db.Model(&models.SeafileConfig{}).Where("user_id = ?", user.ID).
		Update("api_token_encrypted", mustEncrypt(t, "test-jwt-secret-0123456789abcdef0123456789abcdef", "wrong")).Error)
	result2, err := TestSeafileConnection(db, paperlessTestConfig(), user.ID)
	require.NoError(t, err)
	require.NotNil(t, result2)
	assert.False(t, result2.OK)
	assert.Equal(t, "auth", result2.Stage)
}

func TestSeafileClient_RejectsUnexpectedStatus(t *testing.T) {
	fake := newFakeSeafileServer(t, "sekret")
	defer fake.Close()
	fake.FailWithStatus = http.StatusBadGateway

	client, err := NewSeafileClient(fake.URL(), "sekret", false)
	require.NoError(t, err)
	_, err = client.ListLibraries()
	assert.ErrorIs(t, err, ErrSeafileRequestFailed)
}
