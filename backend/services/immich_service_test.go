package services

import (
	"errors"
	"mycorrhizal/config"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// immichTestConfig returns a config with a JWT secret the tests can encrypt
// against (credential_crypto.go derives the key from it).
func immichTestConfig() config.Config {
	return config.Config{
		JWTSecretKey: "test-jwt-secret-0123456789abcdef0123456789abcdef",
	}
}

// newImmichTestDB migrates the models the Immich service touches.
func newImmichTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.ImmichConfig{}, &models.ExternalIdentity{}, &models.ExternalActivity{}))
	return db
}

func seedImmichUser(t *testing.T, db *gorm.DB) (models.User, models.Contact) {
	t.Helper()
	user := models.User{Username: "immich-user", Password: "password123!A", Email: "immich@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)
	return user, contact
}

// connectImmichForUser writes a config + linked identity for a user against
// the given fake server, returning the identity. An empty apiKey uses a
// dummy (the fake server with no required key accepts anything; UpsertImmichConfig
// still requires a non-empty key on create).
func connectImmichForUser(t *testing.T, db *gorm.DB, userID uint, contactUID, baseURL, apiKey string) models.ExternalIdentity {
	t.Helper()
	if apiKey == "" {
		apiKey = "test-key"
	}
	cfg := immichTestConfig()
	_, err := UpsertImmichConfig(db, cfg.JWTSecretKey, userID, models.ImmichConfigInput{
		BaseURL: baseURL, APIKey: apiKey,
	})
	require.NoError(t, err)
	identity, err := LinkImmichPerson(db, cfg, userID, contactUID, "person-alice", "Alice")
	require.NoError(t, err)
	return *identity
}

func TestNormalizeImmichBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "plain https", in: "https://immich.example", want: "https://immich.example"},
		{name: "trailing slash stripped", in: "https://immich.example/", want: "https://immich.example"},
		{name: "multiple trailing slashes stripped", in: "https://immich.example///", want: "https://immich.example"},
		{name: "whitespace trimmed", in: "  https://immich.example  ", want: "https://immich.example"},
		{name: "trailing /api stripped", in: "https://immich.example/api", want: "https://immich.example"},
		{name: "trailing /api/ stripped", in: "https://immich.example/api/", want: "https://immich.example"},
		{name: "trailing /api under a path prefix stripped", in: "https://example.com/immich/api", want: "https://example.com/immich"},
		{name: "path segment that merely contains api is left alone", in: "https://immich.example/apiv2", want: "https://immich.example/apiv2"},
		{name: "http scheme kept", in: "http://immich:2283", want: "http://immich:2283"},
		{name: "missing scheme is rejected, not guessed", in: "immich.example.com", wantErr: true},
		{name: "empty is rejected", in: "   ", wantErr: true},
		{name: "unsupported scheme is rejected", in: "ftp://immich.example", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeImmichBaseURL(tc.in)
			if tc.wantErr {
				assert.True(t, errors.Is(err, ErrImmichInvalidURL), "expected ErrImmichInvalidURL, got %v", err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestImmichClient_ListPeople(t *testing.T) {
	fake := newFakeImmichServer(t, "sekret")
	defer fake.Close()
	fake.addPerson("p1", "Alice", 5, nil, nil)
	fake.addPerson("p2", "Bob", 2, nil, nil)

	client, err := NewImmichClient(fake.URL(), "sekret", false)
	require.NoError(t, err)

	people, err := client.ListPeople()
	require.NoError(t, err)
	require.Len(t, people, 2)
	names := map[string]string{}
	for _, p := range people {
		names[p.ID] = p.Name
	}
	assert.Equal(t, "Alice", names["p1"])
	assert.Equal(t, "Bob", names["p2"])
	assert.Equal(t, "sekret", fake.LastAPIKey, "the client must send the API key")
}

func TestImmichClient_Unauthorized(t *testing.T) {
	fake := newFakeImmichServer(t, "sekret")
	defer fake.Close()
	fake.addPerson("p1", "Alice", 5, nil, nil)

	client, err := NewImmichClient(fake.URL(), "wrong-key", false)
	require.NoError(t, err)

	_, err = client.ListPeople()
	assert.True(t, errors.Is(err, ErrImmichUnauthorized), "expired/invalid key must map to ErrImmichUnauthorized, got %v", err)
}

func TestImmichClient_Unreachable(t *testing.T) {
	// Point at a server that's already closed.
	fake := newFakeImmichServer(t, "")
	url := fake.URL()
	fake.Close()

	client, err := NewImmichClient(url, "sekret", false)
	require.NoError(t, err)

	_, err = client.ListPeople()
	assert.True(t, errors.Is(err, ErrImmichUnreachable), "an unreachable instance must map to ErrImmichUnreachable, got %v", err)
}

func TestImmichClient_NotFound(t *testing.T) {
	fake := newFakeImmichServer(t, "")
	defer fake.Close()

	client, err := NewImmichClient(fake.URL(), "", false)
	require.NoError(t, err)

	_, err = client.GetStatistics("no-such-person")
	assert.True(t, errors.Is(err, ErrImmichNotFound), "a missing person must map to ErrImmichNotFound, got %v", err)
}

func TestImmichClient_RecentAssetsOrdersByOccurrence(t *testing.T) {
	fake := newFakeImmichServer(t, "")
	defer fake.Close()
	// Deliberately out of order on the wire: the client must re-sort.
	fake.addPerson("p1", "Alice", 3, []fakeImmichAsset{
		{ID: "old", FileCreatedAt: "2026-01-01T10:00:00Z"},
		{ID: "new", FileCreatedAt: "2026-08-03T10:00:00Z"},
		{ID: "mid", FileCreatedAt: "2026-04-01T10:00:00Z"},
	}, nil)

	client, err := NewImmichClient(fake.URL(), "", false)
	require.NoError(t, err)

	assets, err := client.RecentAssets("p1", 25)
	require.NoError(t, err)
	require.Len(t, assets, 3)
	assert.Equal(t, "new", assets[0].ID)
	assert.Equal(t, "mid", assets[1].ID)
	assert.Equal(t, "old", assets[2].ID)

	// Bounded to limit.
	limited, err := client.RecentAssets("p1", 1)
	require.NoError(t, err)
	require.Len(t, limited, 1)
	assert.Equal(t, "new", limited[0].ID)
}

func TestImmichClient_Thumbnail(t *testing.T) {
	fake := newFakeImmichServer(t, "")
	defer fake.Close()
	fake.addPerson("p1", "Alice", 1, nil, []byte{0xff, 0xd8, 0xff})

	client, err := NewImmichClient(fake.URL(), "", false)
	require.NoError(t, err)

	body, contentType, err := client.Thumbnail("p1")
	require.NoError(t, err)
	assert.Equal(t, []byte{0xff, 0xd8, 0xff}, body)
	assert.Equal(t, "image/jpeg", contentType)
}

// TestImmichClient_ThumbnailRejectsNonImageContentType pins the
// defense-in-depth hardening added during review: the Immich client's
// Thumbnail method must reject Content-Types that don't start with "image/"
// before the body reaches the controller, matching httputil.FetchImageFromURL.
func TestImmichClient_ThumbnailRejectsNonImageContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<script>alert(1)</script>"))
	}))
	defer server.Close()

	client, err := NewImmichClient(server.URL, "", false)
	require.NoError(t, err)

	_, _, err = client.Thumbnail("p1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-image content type")
}

func TestImmichClient_Ping(t *testing.T) {
	fake := newFakeImmichServer(t, "")
	defer fake.Close()

	client, err := NewImmichClient(fake.URL(), "", false)
	require.NoError(t, err)

	assert.NoError(t, client.Ping())
}

func TestImmichClient_PingUnreachable(t *testing.T) {
	fake := newFakeImmichServer(t, "")
	url := fake.URL()
	fake.Close()

	client, err := NewImmichClient(url, "", false)
	require.NoError(t, err)

	err = client.Ping()
	assert.True(t, errors.Is(err, ErrImmichUnreachable), "an unreachable instance must map to ErrImmichUnreachable, got %v", err)
}

func TestImmichClient_GetMyUser(t *testing.T) {
	fake := newFakeImmichServer(t, "sekret")
	defer fake.Close()
	fake.Me = &fakeImmichMe{Email: "alice@example.com", Name: "Alice"}

	client, err := NewImmichClient(fake.URL(), "sekret", false)
	require.NoError(t, err)

	user, err := client.GetMyUser()
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", user.Email)
}

func TestImmichClient_GetMyUserUnauthorized(t *testing.T) {
	fake := newFakeImmichServer(t, "sekret")
	defer fake.Close()

	client, err := NewImmichClient(fake.URL(), "wrong-key", false)
	require.NoError(t, err)

	_, err = client.GetMyUser()
	assert.True(t, errors.Is(err, ErrImmichUnauthorized), "an invalid key must map to ErrImmichUnauthorized, got %v", err)
}

func TestTestImmichConnection_Success(t *testing.T) {
	db := newImmichTestDB(t)
	user, contact := seedImmichUser(t, db)

	fake := newFakeImmichServer(t, "sekret")
	defer fake.Close()
	fake.Me = &fakeImmichMe{Email: "alice@example.com"}

	connectImmichForUser(t, db, user.ID, contact.VCardUID, fake.URL(), "sekret")

	result, err := TestImmichConnection(db, immichTestConfig(), user.ID)
	require.NoError(t, err)
	assert.True(t, result.OK)
	assert.Equal(t, "ok", result.Stage)
	assert.Contains(t, result.Message, "alice@example.com")
}

func TestTestImmichConnection_AuthFailure(t *testing.T) {
	db := newImmichTestDB(t)
	user, contact := seedImmichUser(t, db)

	fake := newFakeImmichServer(t, "sekret")
	defer fake.Close()

	// Stored key no longer matches (simulates a revoked/expired key).
	connectImmichForUser(t, db, user.ID, contact.VCardUID, fake.URL(), "now-stale-key")

	result, err := TestImmichConnection(db, immichTestConfig(), user.ID)
	require.NoError(t, err, "a diagnosed upstream failure is a successful check, not a Go error")
	assert.False(t, result.OK)
	assert.Equal(t, "auth", result.Stage)
	assert.Contains(t, result.Message, "API key")
}

func TestTestImmichConnection_Unreachable(t *testing.T) {
	db := newImmichTestDB(t)
	user, contact := seedImmichUser(t, db)

	fake := newFakeImmichServer(t, "")
	url := fake.URL()
	fake.Close()

	connectImmichForUser(t, db, user.ID, contact.VCardUID, url, "")

	result, err := TestImmichConnection(db, immichTestConfig(), user.ID)
	require.NoError(t, err)
	assert.False(t, result.OK)
	assert.Equal(t, "reachability", result.Stage)
}

func TestTestImmichConnection_NoConfigReturnsError(t *testing.T) {
	db := newImmichTestDB(t)
	user, _ := seedImmichUser(t, db)

	_, err := TestImmichConnection(db, immichTestConfig(), user.ID)
	require.Error(t, err, "no saved connection must be a Go error, not a diagnosed result")
	assert.True(t, errors.Is(err, ErrImmichUnauthorized))
}

func TestSyncImmichForUser_WritesActivitiesAndDedups(t *testing.T) {
	db := newImmichTestDB(t)
	user, contact := seedImmichUser(t, db)

	fake := newFakeImmichServer(t, "sekret")
	defer fake.Close()
	fake.addPerson("person-alice", "Alice", 2, []fakeImmichAsset{
		{ID: "asset-1", FileCreatedAt: "2026-08-03T10:00:00Z"},
		{ID: "asset-2", FileCreatedAt: "2026-07-01T10:00:00Z"},
	}, nil)

	connectImmichForUser(t, db, user.ID, contact.VCardUID, fake.URL(), "sekret")

	// First sync: two ExternalActivity rows (one per asset), identity metadata
	// refreshed, connection status recorded.
	err := SyncImmichForUser(db, immichTestConfig(), user.ID)
	require.NoError(t, err)

	var activities []models.ExternalActivity
	require.NoError(t, db.Where("user_id = ? AND source_system = ?", user.ID, ExternalSystemImmich).Order("occurred_at desc").Find(&activities).Error)
	require.Len(t, activities, 2)
	assert.Equal(t, "asset-1", activities[0].ExternalID)
	assert.Equal(t, ImmichActivityType, activities[0].Type)
	assert.Equal(t, contact.VCardUID, activities[0].EntityID)
	assert.Equal(t, "Alice", activities[0].Payload["person_name"])

	var identity models.ExternalIdentity
	require.NoError(t, db.Where("user_id = ? AND system = ?", user.ID, ExternalSystemImmich).First(&identity).Error)
	assert.EqualValues(t, 2, identity.Metadata["photo_count"])
	assert.Equal(t, models.ExternalIdentitySyncStatusSynced, identity.SyncStatus)
	require.NotNil(t, identity.LastSyncedAt)

	var config models.ImmichConfig
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&config).Error)
	assert.Equal(t, "success", config.LastSyncStatus)

	// Second sync (re-sync, same upstream data): the natural key dedupes — no
	// new rows, no error.
	err = SyncImmichForUser(db, immichTestConfig(), user.ID)
	require.NoError(t, err)
	var count int64
	require.NoError(t, db.Model(&models.ExternalActivity{}).Where("user_id = ? AND source_system = ?", user.ID, ExternalSystemImmich).Count(&count).Error)
	assert.EqualValues(t, 2, count, "a re-sync must upsert on the natural key, not duplicate")
}

func TestSyncImmichForUser_ExpiredKeyRecordsError(t *testing.T) {
	db := newImmichTestDB(t)
	user, contact := seedImmichUser(t, db)

	fake := newFakeImmichServer(t, "sekret")
	defer fake.Close()
	fake.addPerson("person-alice", "Alice", 1, []fakeImmichAsset{{ID: "a1", FileCreatedAt: "2026-08-03T10:00:00Z"}}, nil)

	// Store a config whose key no longer matches (simulates expiry).
	connectImmichForUser(t, db, user.ID, contact.VCardUID, fake.URL(), "now-stale-key")

	err := SyncImmichForUser(db, immichTestConfig(), user.ID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrImmichUnauthorized))

	var config models.ImmichConfig
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&config).Error)
	assert.Equal(t, "error", config.LastSyncStatus)
	assert.Contains(t, config.LastSyncError, "API key")

	// No external activities were written.
	var count int64
	require.NoError(t, db.Model(&models.ExternalActivity{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.EqualValues(t, 0, count)
}

func TestSyncImmichForUser_UnreachableRecordsError(t *testing.T) {
	db := newImmichTestDB(t)
	user, contact := seedImmichUser(t, db)

	fake := newFakeImmichServer(t, "")
	url := fake.URL()
	fake.Close()

	connectImmichForUser(t, db, user.ID, contact.VCardUID, url, "")

	err := SyncImmichForUser(db, immichTestConfig(), user.ID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrImmichUnreachable))

	var config models.ImmichConfig
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&config).Error)
	assert.Equal(t, "error", config.LastSyncStatus)
}

func TestUpsertImmichConfig_EncryptsKeyAndKeepsOnEmptyUpdate(t *testing.T) {
	db := newImmichTestDB(t)
	user, _ := seedImmichUser(t, db)
	cfg := immichTestConfig()

	ic, err := UpsertImmichConfig(db, cfg.JWTSecretKey, user.ID, models.ImmichConfigInput{
		BaseURL: "https://immich.example/", APIKey: "plaintext-key",
	})
	require.NoError(t, err)
	assert.NotContains(t, ic.APIKeyEncrypted, "plaintext-key", "the API key must never be stored in plaintext")
	assert.True(t, ic.HasAPIKey())

	decrypted, err := DecryptCredential(cfg.JWTSecretKey, ic.APIKeyEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "plaintext-key", decrypted)

	// Empty APIKey on update keeps the stored key.
	ic2, err := UpsertImmichConfig(db, cfg.JWTSecretKey, user.ID, models.ImmichConfigInput{BaseURL: "https://immich.example"})
	require.NoError(t, err)
	assert.Equal(t, ic.APIKeyEncrypted, ic2.APIKeyEncrypted, "an empty key on update must keep the stored one")

	// Create with no key is rejected.
	other := models.User{Username: "immich-user2", Password: "password123!A", Email: "immich2@example.com"}
	require.NoError(t, db.Create(&other).Error)
	_, err = UpsertImmichConfig(db, cfg.JWTSecretKey, other.ID, models.ImmichConfigInput{BaseURL: "https://immich.example"})
	require.Error(t, err)
}

func TestLinkImmichPerson_WritesExternalIdentity(t *testing.T) {
	db := newImmichTestDB(t)
	user, contact := seedImmichUser(t, db)
	cfg := immichTestConfig()

	_, err := UpsertImmichConfig(db, cfg.JWTSecretKey, user.ID, models.ImmichConfigInput{BaseURL: "https://immich.example", APIKey: "k"})
	require.NoError(t, err)

	identity, err := LinkImmichPerson(db, cfg, user.ID, contact.VCardUID, "person-alice", "Alice")
	require.NoError(t, err)
	assert.Equal(t, ExternalSystemImmich, identity.System)
	assert.Equal(t, "person-alice", identity.ExternalID)
	assert.Equal(t, "https://immich.example/people/person-alice", identity.URL)
	assert.Equal(t, "Alice", identity.Metadata["person_name"])

	var stored models.ExternalIdentity
	require.NoError(t, db.Where("user_id = ? AND system = ?", user.ID, ExternalSystemImmich).First(&stored).Error)
	assert.Equal(t, contact.VCardUID, stored.EntityID)
}

func TestFetchImmichPersonSummary_LiveAndCachedDegradation(t *testing.T) {
	db := newImmichTestDB(t)
	user, contact := seedImmichUser(t, db)

	fake := newFakeImmichServer(t, "")
	defer fake.Close()
	fake.addPerson("person-alice", "Alice", 7, []fakeImmichAsset{{ID: "latest-1", FileCreatedAt: "2026-08-03T10:00:00Z"}}, nil)

	connectImmichForUser(t, db, user.ID, contact.VCardUID, fake.URL(), "")
	var identity models.ExternalIdentity
	require.NoError(t, db.Where("user_id = ? AND system = ?", user.ID, ExternalSystemImmich).First(&identity).Error)

	// Live summary against the reachable instance.
	summary := FetchImmichPersonSummary(db, immichTestConfig(), user.ID, &identity)
	assert.Equal(t, 7, summary.PhotoCount)
	assert.Equal(t, "latest-1", summary.LatestAssetID)
	require.NotNil(t, summary.LatestAt)

	// Instance goes down: must degrade to the cached metadata, not error.
	identity.Metadata["photo_count"] = 7
	identity.Metadata["latest_asset_id"] = "latest-1"
	require.NoError(t, db.Save(&identity).Error)
	fake.FailWithStatus = 503

	degraded := FetchImmichPersonSummary(db, immichTestConfig(), user.ID, &identity)
	assert.Equal(t, "Alice", degraded.PersonName, "the name is still available from cached metadata")
	assert.EqualValues(t, 7, degraded.PhotoCount, "photo count degrades to the cache on failure")
}

func TestSyncImmichWithRateLimit_SkipsWhenRecentlyRun(t *testing.T) {
	db := newImmichJobLockDB(t)
	user, contact := seedImmichUser(t, db)

	fake := newFakeImmichServer(t, "sekret")
	defer fake.Close()
	fake.addPerson("person-alice", "Alice", 1, []fakeImmichAsset{{ID: "a1", FileCreatedAt: "2026-08-03T10:00:00Z"}}, nil)

	connectImmichForUser(t, db, user.ID, contact.VCardUID, fake.URL(), "sekret")

	require.NoError(t, db.Create(&models.JobExecution{
		JobName: models.JobNameImmichSync, LastRunAt: time.Now(),
	}).Error)

	cfg := immichTestConfig()
	require.NotPanics(t, func() { SyncImmichWithRateLimit(db, cfg) })

	var count int64
	require.NoError(t, db.Model(&models.ExternalActivity{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.EqualValues(t, 0, count, "the rate-limited job must not write any rows")

	var job models.JobExecution
	require.NoError(t, db.Where("job_name = ?", models.JobNameImmichSync).First(&job).Error)
	assert.Nil(t, job.LockedAt, "lock must not have been re-acquired while rate-limited")
}

func TestSyncImmichWithRateLimit_RunsWhenNoPriorRun(t *testing.T) {
	db := newImmichJobLockDB(t)
	user, contact := seedImmichUser(t, db)

	fake := newFakeImmichServer(t, "sekret")
	defer fake.Close()
	fake.addPerson("person-alice", "Alice", 1, []fakeImmichAsset{{ID: "a1", FileCreatedAt: "2026-08-03T10:00:00Z"}}, nil)

	connectImmichForUser(t, db, user.ID, contact.VCardUID, fake.URL(), "sekret")

	cfg := immichTestConfig()
	require.NotPanics(t, func() { SyncImmichWithRateLimit(db, cfg) })

	var count int64
	require.NoError(t, db.Model(&models.ExternalActivity{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.GreaterOrEqual(t, count, int64(1), "the job must create at least one ExternalActivity row")

	var job models.JobExecution
	require.NoError(t, db.Where("job_name = ?", models.JobNameImmichSync).First(&job).Error)
	assert.Nil(t, job.LockedAt, "lock must be released after the job completes")
}

// newImmichJobLockDB migrates the models the job-lock tests touch.
func newImmichJobLockDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(
		&models.User{}, &models.Contact{}, &models.ImmichConfig{},
		&models.ExternalIdentity{}, &models.ExternalActivity{},
		&models.JobExecution{},
	))
	return db
}
