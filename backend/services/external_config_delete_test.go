package services

import (
	"errors"
	"path/filepath"
	"testing"

	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupExternalConfigTestDB opens a real migrated schema (the config tables
// use gorm.Model soft delete with partial unique indexes from hand-written
// migrations — CLAUDE.md backend trap 1).
func setupExternalConfigTestDB(t *testing.T) (*gorm.DB, models.User) {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), "external-config.db"))
	require.NoError(t, err)
	user := models.User{Username: "extconfig", Password: "password123!A", Email: "extconfig@example.com"}
	require.NoError(t, db.Create(&user).Error)
	return db, user
}

// TestDeleteImmichConfig pins that deleting a user's Immich config is a soft
// delete (the row is retained, only deleted_at set) and only touches that
// user's row — their ExternalIdentity/ExternalActivity history is kept.
func TestDeleteImmichConfig(t *testing.T) {
	db, user := setupExternalConfigTestDB(t)
	cfg := immichTestConfig()

	_, err := UpsertImmichConfig(db, cfg.JWTSecretKey, user.ID, models.ImmichConfigInput{
		BaseURL: "https://immich.example", APIKey: "key-1",
	})
	require.NoError(t, err)

	other := models.User{Username: "extother", Password: "password123!A", Email: "extother@example.com"}
	require.NoError(t, db.Create(&other).Error)
	_, err = UpsertImmichConfig(db, cfg.JWTSecretKey, other.ID, models.ImmichConfigInput{
		BaseURL: "https://immich.example", APIKey: "key-2",
	})
	require.NoError(t, err)

	require.NoError(t, DeleteImmichConfig(db, user.ID))

	// The user's config is gone from the normal (non-Unscoped) query...
	got, err := GetImmichConfigForUser(db, user.ID)
	require.NoError(t, err)
	assert.Nil(t, got)

	// ...but the row is soft-deleted, not removed.
	var count int64
	require.NoError(t, db.Unscoped().Model(&models.ImmichConfig{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count, "DeleteImmichConfig is a soft delete")

	// The other user's config is untouched.
	got, err = GetImmichConfigForUser(db, other.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestDeletePaperlessConfig(t *testing.T) {
	db, user := setupExternalConfigTestDB(t)
	cfg := paperlessTestConfig()

	_, err := UpsertPaperlessConfig(db, cfg.JWTSecretKey, user.ID, models.PaperlessConfigInput{
		BaseURL: "https://paperless.example", APIToken: "tok-1",
	})
	require.NoError(t, err)

	require.NoError(t, DeletePaperlessConfig(db, user.ID))

	got, err := GetPaperlessConfigForUser(db, user.ID)
	require.NoError(t, err)
	assert.Nil(t, got)

	var count int64
	require.NoError(t, db.Unscoped().Model(&models.PaperlessConfig{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count, "DeletePaperlessConfig is a soft delete")
}

func TestDeleteSeafileConfig(t *testing.T) {
	db, user := setupExternalConfigTestDB(t)
	cfg := paperlessTestConfig() // JWT secret only

	_, err := UpsertSeafileConfig(db, cfg.JWTSecretKey, user.ID, models.SeafileConfigInput{
		BaseURL: "https://seafile.example", APIToken: "tok-1",
	})
	require.NoError(t, err)

	require.NoError(t, DeleteSeafileConfig(db, user.ID))

	got, err := GetSeafileConfigForUser(db, user.ID)
	require.NoError(t, err)
	assert.Nil(t, got)

	var count int64
	require.NoError(t, db.Unscoped().Model(&models.SeafileConfig{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count, "DeleteSeafileConfig is a soft delete")
}

func TestDeleteWebDAVConfig(t *testing.T) {
	db, user := setupExternalConfigTestDB(t)
	cfg := paperlessTestConfig() // JWT secret only

	_, err := UpsertWebDAVConfig(db, cfg.JWTSecretKey, user.ID, models.WebDAVConfigInput{
		BaseURL: "https://nextcloud.example", Username: "u", AppPassword: "p",
	})
	require.NoError(t, err)

	require.NoError(t, DeleteWebDAVConfig(db, user.ID))

	got, err := GetWebDAVConfigForUser(db, user.ID)
	require.NoError(t, err)
	assert.Nil(t, got)

	var count int64
	require.NoError(t, db.Unscoped().Model(&models.WebDAVConfig{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count, "DeleteWebDAVConfig is a soft delete")
}

func TestNormalizeWebDAVPathForKey(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: "/"},
		{name: "leading slash kept", in: "/Documents", want: "/Documents"},
		{name: "missing leading slash added", in: "Documents", want: "/Documents"},
		{name: "trailing slash stripped", in: "/Documents/", want: "/Documents"},
		{name: "whitespace trimmed", in: "  /Documents  ", want: "/Documents"},
		{name: "root stays root", in: "/", want: "/"},
		{name: "nested path", in: "/a/b/c/", want: "/a/b/c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, NormalizeWebDAVPathForKey(tc.in))
		})
	}
}

// TestListImmichPeopleForUser exercises the L1 picker path end-to-end against
// the permanent fake Immich server.
func TestListImmichPeopleForUser(t *testing.T) {
	db, user := setupExternalConfigTestDB(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	fake := newFakeImmichServer(t, "sekret")
	defer fake.Close()
	fake.addPerson("p1", "Alice", 5, nil, nil)
	fake.addPerson("p2", "Bob", 2, nil, nil)

	cfg := immichTestConfig()
	_, err := UpsertImmichConfig(db, cfg.JWTSecretKey, user.ID, models.ImmichConfigInput{
		BaseURL: fake.URL(), APIKey: "sekret",
	})
	require.NoError(t, err)

	people, err := ListImmichPeopleForUser(db, cfg, user.ID)
	require.NoError(t, err)
	require.Len(t, people, 2)
	assert.Equal(t, "sekret", fake.LastAPIKey, "the client must send the user's API key")
}

func TestListImmichPeopleForUser_NoConnection(t *testing.T) {
	db, user := setupExternalConfigTestDB(t)
	cfg := immichTestConfig()

	_, err := ListImmichPeopleForUser(db, cfg, user.ID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrImmichUnauthorized), "expected an unauthorized sentinel for a missing connection, got %v", err)
}

// TestFetchImmichThumbnail exercises the linked-contact photo fetch: the
// person is resolved through the contact's ExternalIdentity, and the fetch
// runs with the user's own connection credentials.
func TestFetchImmichThumbnail(t *testing.T) {
	db, user := setupExternalConfigTestDB(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	fake := newFakeImmichServer(t, "sekret")
	defer fake.Close()
	// connectImmichForUser links external_id "person-alice" (see
	// immich_service_test.go), so the fake must serve that exact person ID.
	fake.addPerson("person-alice", "Alice", 5, nil, []byte("fake-jpeg-bytes"))

	cfg := immichTestConfig()
	connectImmichForUser(t, db, user.ID, contact.VCardUID, fake.URL(), "sekret")

	data, contentType, err := FetchImmichThumbnail(db, cfg, user.ID, contact.VCardUID)
	require.NoError(t, err)
	assert.Equal(t, "fake-jpeg-bytes", string(data))
	assert.Equal(t, "image/jpeg", contentType)
}

func TestFetchImmichThumbnail_UnlinkedContact(t *testing.T) {
	db, user := setupExternalConfigTestDB(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	cfg := immichTestConfig()
	_, _, err := FetchImmichThumbnail(db, cfg, user.ID, contact.VCardUID)
	require.Error(t, err, "a contact with no Immich identity link must not be fetchable")
}
