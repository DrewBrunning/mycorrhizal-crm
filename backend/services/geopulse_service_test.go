package services

import (
	"context"
	"errors"
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/faults"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// geopulseTestJWTSecret is deliberately low-entropy filler: it only has to be a
// non-empty string credential_crypto.go can derive a key from, and a
// secret-shaped literal would (rightly) trip the local gitleaks scan.
const geopulseTestJWTSecret = "geopulse-test-filler-geopulse-test-filler"

func geopulseTestConfig() config.Config {
	return config.Config{JWTSecretKey: geopulseTestJWTSecret}
}

func seedGeoPulseUser(t *testing.T, db *gorm.DB, name string) models.User {
	t.Helper()
	user := models.User{Username: name, Password: "password123!A", Email: name + "@example.com"}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func connectGeoPulseForUser(t *testing.T, db *gorm.DB, userID uint, baseURL, key string) {
	t.Helper()
	if key == "" {
		key = "test-key"
	}
	_, err := UpsertGeoPulseConfig(db, geopulseTestConfig().JWTSecretKey, userID, models.GeoPulseConfigInput{BaseURL: baseURL, APIKey: key})
	require.NoError(t, err)
}

func TestNormalizeGeoPulseBaseURL(t *testing.T) {
	cases := []struct {
		name, in, want string
		wantErr        bool
	}{
		{name: "plain https", in: "https://geopulse.example", want: "https://geopulse.example"},
		{name: "trailing slash stripped", in: "https://geopulse.example/", want: "https://geopulse.example"},
		{name: "whitespace trimmed", in: "  http://10.0.0.5:8080  ", want: "http://10.0.0.5:8080"},
		{name: "path preserved", in: "https://example.com/geopulse", want: "https://example.com/geopulse"},
		{name: "scheme-less rejected", in: "geopulse.example", wantErr: true},
		{name: "ftp rejected", in: "ftp://geopulse.example", wantErr: true},
		{name: "empty rejected", in: "", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeGeoPulseBaseURL(tc.in)
			if tc.wantErr {
				assert.ErrorIs(t, err, ErrGeoPulseInvalidURL)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestUpsertGeoPulseConfig_CreateUpdateKeepKey(t *testing.T) {
	db := dbtest.New(t)
	user := seedGeoPulseUser(t, db, "gp-upsert")
	secret := geopulseTestConfig().JWTSecretKey

	got, err := GetGeoPulseConfigForUser(db, user.ID)
	require.NoError(t, err)
	assert.Nil(t, got, "no config until one is saved")

	_, err = UpsertGeoPulseConfig(db, secret, user.ID, models.GeoPulseConfigInput{BaseURL: "https://gp.example"})
	require.Error(t, err, "first connect without a key must fail")
	_, err = UpsertGeoPulseConfig(db, secret, user.ID, models.GeoPulseConfigInput{BaseURL: "gp.example", APIKey: "k"})
	assert.ErrorIs(t, err, ErrGeoPulseInvalidURL)

	created, err := UpsertGeoPulseConfig(db, secret, user.ID, models.GeoPulseConfigInput{BaseURL: "https://gp.example/", APIKey: "key-1"})
	require.NoError(t, err)
	assert.Equal(t, "https://gp.example", created.BaseURL)
	assert.True(t, created.HasAPIKey())
	assert.NotContains(t, created.APIKeyEncrypted, "key-1", "the key is stored encrypted")
	plain, err := DecryptCredential(secret, created.APIKeyEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "key-1", plain)

	// Empty key on a same-origin (path-only) update keeps the stored one.
	kept, err := UpsertGeoPulseConfig(db, secret, user.ID, models.GeoPulseConfigInput{BaseURL: "https://GP.example:443/geopulse"})
	require.NoError(t, err)
	assert.Equal(t, "https://GP.example:443/geopulse", kept.BaseURL)
	plain, _ = DecryptCredential(secret, kept.APIKeyEncrypted)
	assert.Equal(t, "key-1", plain)

	// Moving to a different origin without re-entering the token is refused and
	// changes nothing (credential-exfiltration guard); a new key makes it legal.
	for _, other := range []string{"https://gp2.example", "http://gp.example", "https://gp.example:8443", "https://gp.example.evil.test"} {
		_, err = UpsertGeoPulseConfig(db, secret, user.ID, models.GeoPulseConfigInput{BaseURL: other})
		assert.ErrorIs(t, err, ErrGeoPulseTokenRequired, other)
	}
	still, _ := GetGeoPulseConfigForUser(db, user.ID)
	assert.Equal(t, "https://GP.example:443/geopulse", still.BaseURL)

	replaced, err := UpsertGeoPulseConfig(db, secret, user.ID, models.GeoPulseConfigInput{BaseURL: "https://gp2.example", APIKey: "key-2"})
	require.NoError(t, err)
	plain, _ = DecryptCredential(secret, replaced.APIKeyEncrypted)
	assert.Equal(t, "key-2", plain)

	// An update with a bad URL is rejected and changes nothing.
	_, err = UpsertGeoPulseConfig(db, secret, user.ID, models.GeoPulseConfigInput{BaseURL: "nope"})
	assert.ErrorIs(t, err, ErrGeoPulseInvalidURL)
	again, _ := GetGeoPulseConfigForUser(db, user.ID)
	assert.Equal(t, "https://gp2.example", again.BaseURL)
}

func TestUpsertGeoPulseConfig_DatabaseFailures(t *testing.T) {
	secret := geopulseTestConfig().JWTSecretKey

	t.Run("lookup fails", func(t *testing.T) {
		db := dbtest.New(t)
		user := seedGeoPulseUser(t, db, "gp-up-lookup")
		dbtest.HideTable(t, db, "geopulse_configs")
		_, err := UpsertGeoPulseConfig(db, secret, user.ID, models.GeoPulseConfigInput{BaseURL: "https://gp.example", APIKey: "k"})
		require.Error(t, err)
	})
	t.Run("update fails", func(t *testing.T) {
		db := dbtest.New(t)
		user := seedGeoPulseUser(t, db, "gp-up-update")
		connectGeoPulseForUser(t, db, user.ID, "https://gp.example", "k")
		require.NoError(t, db.Exec(`CREATE TRIGGER gp_block_update BEFORE UPDATE ON geopulse_configs BEGIN SELECT RAISE(ABORT, 'blocked'); END`).Error)
		_, err := UpsertGeoPulseConfig(db, secret, user.ID, models.GeoPulseConfigInput{BaseURL: "https://gp.example/x"})
		require.Error(t, err)
	})
	t.Run("insert fails", func(t *testing.T) {
		db := dbtest.New(t)
		user := seedGeoPulseUser(t, db, "gp-up-insert")
		require.NoError(t, db.Exec(`CREATE TRIGGER gp_block_insert BEFORE INSERT ON geopulse_configs BEGIN SELECT RAISE(ABORT, 'blocked'); END`).Error)
		_, err := UpsertGeoPulseConfig(db, secret, user.ID, models.GeoPulseConfigInput{BaseURL: "https://gp.example", APIKey: "k"})
		require.Error(t, err)
	})
}

func TestDeleteGeoPulseConfig_SoftDeletesAndAllowsReconnect(t *testing.T) {
	db := dbtest.New(t)
	user := seedGeoPulseUser(t, db, "gp-del")
	other := seedGeoPulseUser(t, db, "gp-del-other")
	connectGeoPulseForUser(t, db, user.ID, "https://gp.example", "k")
	connectGeoPulseForUser(t, db, other.ID, "https://other.example", "k")

	require.NoError(t, DeleteGeoPulseConfig(db, user.ID))
	got, err := GetGeoPulseConfigForUser(db, user.ID)
	require.NoError(t, err)
	assert.Nil(t, got)

	var rows int64
	require.NoError(t, db.Unscoped().Model(&models.GeoPulseConfig{}).Where("user_id = ?", user.ID).Count(&rows).Error)
	assert.EqualValues(t, 1, rows, "geopulse_configs soft-deletes (the row is the user's undo)")

	otherCfg, _ := GetGeoPulseConfigForUser(db, other.ID)
	require.NotNil(t, otherCfg, "deleting one user's config must not touch another's")

	// The partial unique index lets the user connect again.
	connectGeoPulseForUser(t, db, user.ID, "https://gp.example", "k2")
}

func TestBuildGeoPulseClient_NoConfigIsUnauthorized(t *testing.T) {
	db := dbtest.New(t)
	user := seedGeoPulseUser(t, db, "gp-noconf")
	_, err := buildGeoPulseClient(db, geopulseTestConfig(), user.ID)
	assert.ErrorIs(t, err, ErrGeoPulseUnauthorized)
}

func TestBuildGeoPulseClient_UndecryptableKey(t *testing.T) {
	db := dbtest.New(t)
	user := seedGeoPulseUser(t, db, "gp-badkey")
	connectGeoPulseForUser(t, db, user.ID, "https://gp.example", "k")
	_, err := buildGeoPulseClient(db, config.Config{JWTSecretKey: geopulseTestJWTSecret + "-rotated"}, user.ID)
	assert.ErrorIs(t, err, ErrGeoPulseKeyUndecryptable, "a rotated JWT_SECRET_KEY makes the stored key undecryptable")
}

func TestBuildGeoPulseClient_UnreadableConfig(t *testing.T) {
	db := dbtest.New(t)
	user := seedGeoPulseUser(t, db, "gp-unreadable")
	dbtest.HideTable(t, db, "geopulse_configs")
	_, err := buildGeoPulseClient(db, geopulseTestConfig(), user.ID)
	assert.ErrorIs(t, err, ErrGeoPulseConfigUnreadable)
	_, err = GeoPulseSuggestionsForDate(context.Background(), db, geopulseTestConfig(), user.ID, "2026-09-20", "")
	assert.ErrorIs(t, err, ErrGeoPulseConfigUnreadable)
}

func TestGeoPulseSuggestionsForDate_ExistingActivityLookupFailure(t *testing.T) {
	db := dbtest.New(t)
	user := seedGeoPulseUser(t, db, "gp-actfail")
	f := newFakeGeoPulseServer(t, "")
	f.addStay(1, "2026-09-20T10:00:00Z", "A", "Leeds", 53.1, -1.1, 60)
	connectGeoPulseForUser(t, db, user.ID, f.URL(), "k")
	dbtest.HideTable(t, db, "activities")
	_, err := GeoPulseSuggestionsForDate(context.Background(), db, geopulseTestConfig(), user.ID, "2026-09-20", "")
	assert.ErrorIs(t, err, ErrGeoPulseConfigUnreadable, "a failed already-logged lookup is a local fault, reported as such")
}

func TestTestGeoPulseConnection_Stages(t *testing.T) {
	db := dbtest.New(t)
	user := seedGeoPulseUser(t, db, "gp-test")
	cfg := geopulseTestConfig()

	_, err := TestGeoPulseConnection(context.Background(), db, cfg, user.ID)
	assert.ErrorIs(t, err, ErrGeoPulseUnauthorized, "no connection: the check itself cannot run")

	f := newFakeGeoPulseServer(t, "good")
	connectGeoPulseForUser(t, db, user.ID, f.URL(), "good")
	res, err := TestGeoPulseConnection(context.Background(), db, cfg, user.ID)
	require.NoError(t, err)
	assert.True(t, res.OK)
	assert.Equal(t, "ok", res.Stage)
	assert.Contains(t, res.Message, "Test User")

	// No full name: the message falls back to the GeoPulse user id.
	f.UserID = "abc-123"
	f.FullName = ""
	res, err = TestGeoPulseConnection(context.Background(), db, cfg, user.ID)
	require.NoError(t, err)
	assert.True(t, res.OK)
	assert.Contains(t, res.Message, "abc-123")

	// Wrong key → auth stage.
	f.Key = "good"
	_, err = UpsertGeoPulseConfig(db, cfg.JWTSecretKey, user.ID, models.GeoPulseConfigInput{BaseURL: f.URL(), APIKey: "bad"})
	require.NoError(t, err)
	res, err = TestGeoPulseConnection(context.Background(), db, cfg, user.ID)
	require.NoError(t, err)
	assert.False(t, res.OK)
	assert.Equal(t, "auth", res.Stage)
	assert.Contains(t, res.Message, "rejected the API token")

	// Server error → reachability stage, reachable-but-failed wording.
	f.Key = ""
	f.FailMe = http.StatusInternalServerError
	res, err = TestGeoPulseConnection(context.Background(), db, cfg, user.ID)
	require.NoError(t, err)
	assert.False(t, res.OK)
	assert.Equal(t, "reachability", res.Stage)
	assert.Contains(t, res.Message, "500")

	// Unreachable host.
	dead := newFakeGeoPulseServer(t, "")
	deadURL := dead.URL()
	dead.Server.Close()
	_, err = UpsertGeoPulseConfig(db, cfg.JWTSecretKey, user.ID, models.GeoPulseConfigInput{BaseURL: deadURL, APIKey: "k"})
	require.NoError(t, err)
	res, err = TestGeoPulseConnection(context.Background(), db, cfg, user.ID)
	require.NoError(t, err)
	assert.False(t, res.OK)
	assert.Equal(t, "reachability", res.Stage)
	assert.Contains(t, res.Message, "Could not reach")

	// SSRF guard on: its own message, naming the env var.
	live := newFakeGeoPulseServer(t, "")
	_, err = UpsertGeoPulseConfig(db, cfg.JWTSecretKey, user.ID, models.GeoPulseConfigInput{BaseURL: live.URL(), APIKey: "k"})
	require.NoError(t, err)
	blocking := cfg
	blocking.GeoPulseBlockPrivateURLs = true
	res, err = TestGeoPulseConnection(context.Background(), db, blocking, user.ID)
	require.NoError(t, err)
	assert.False(t, res.OK)
	assert.Contains(t, res.Message, "GEOPULSE_BLOCK_PRIVATE_URLS")
}

func TestDiagnoseGeoPulseConnectionFailure_Messages(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{ErrGeoPulseNotFound, "did not recognise the request"},
		{ErrGeoPulseInvalidData, "could not be parsed"},
		{ErrGeoPulseRequestFailed, "an unexpected status"}, // bare sentinel: no *GeoPulseRequestError to read a status from
		{&GeoPulseRequestError{StatusCode: 502, Status: "502 Bad Gateway"}, "502 Bad Gateway"},
		{fmt.Errorf("something else entirely"), "something else entirely"},
	}
	for _, tc := range cases {
		res := diagnoseGeoPulseConnectionFailure("reachability", tc.err)
		assert.False(t, res.OK)
		assert.Equal(t, "reachability", res.Stage)
		assert.Contains(t, res.Message, tc.want, "%v", tc.err)
	}
}

func TestGeoPulseDayBounds(t *testing.T) {
	s, e, err := geopulseDayBounds("2026-09-20", "")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC), s)
	assert.Equal(t, time.Date(2026, 9, 20, 23, 59, 59, 0, time.UTC), e)

	// A timezone shifts the day's UTC instants.
	s, e, err = geopulseDayBounds("2026-09-20", "America/New_York")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC), s.UTC())
	assert.Equal(t, time.Date(2026, 9, 21, 3, 59, 59, 0, time.UTC), e.UTC())

	// DST transition days are 23/25 hours; the end is still "next local midnight - 1s".
	s, e, err = geopulseDayBounds("2026-03-08", "America/New_York")
	require.NoError(t, err)
	assert.Equal(t, 23*time.Hour-time.Second, e.Sub(s))

	for _, bad := range [][2]string{{"", ""}, {"2026-13-40", ""}, {"20-09-2026", ""}, {"2026-09-20", "Not/AZone"}} {
		_, _, err := geopulseDayBounds(bad[0], bad[1])
		assert.ErrorIs(t, err, ErrGeoPulseInvalidDate, "%v", bad)
	}
}

func TestGeoPulseSuggestionsForDate_HappyPath(t *testing.T) {
	db := dbtest.New(t)
	user := seedGeoPulseUser(t, db, "gp-sug")
	f := newFakeGeoPulseServer(t, "k")
	f.addStay(7, "2026-09-20T14:00:00Z", "Cafe Nero", "Leeds", 53.8, -1.55, 3600)
	f.addStay(8, "2026-09-20T18:00:00Z", "", "York", 53.96, -1.08, 600) // no place name: falls back to city
	f.addPhoto(53.8, "p1", "IMG_1.jpg", "2026-09-20T14:05:00Z")
	f.addPhoto(53.8, "p2", "IMG_2.jpg", "2026-09-20T14:20:00Z")
	connectGeoPulseForUser(t, db, user.ID, f.URL(), "k")

	res, err := GeoPulseSuggestionsForDate(context.Background(), db, geopulseTestConfig(), user.ID, "2026-09-20", "")
	require.NoError(t, err)
	assert.Equal(t, "2026-09-20", res.Date)
	require.Len(t, res.Suggestions, 2)

	a, b := res.Suggestions[0], res.Suggestions[1]
	assert.Equal(t, int64(7), a.StayID)
	assert.Equal(t, "geopulse:stay:7", a.ExternalRef)
	assert.Equal(t, "Cafe Nero", a.Location)
	assert.Equal(t, "Leeds", a.City)
	assert.Equal(t, time.Date(2026, 9, 20, 14, 0, 0, 0, time.UTC), a.Timestamp)
	assert.EqualValues(t, 3600, a.DurationSeconds)
	assert.False(t, a.PhotosUnavailable)
	assert.Equal(t, []GeoPulsePhotoSuggestion{
		{ID: "p1", FileName: "IMG_1.jpg", TakenAt: "2026-09-20T14:05:00Z"},
		{ID: "p2", FileName: "IMG_2.jpg", TakenAt: "2026-09-20T14:20:00Z"},
	}, a.Photos)
	assert.Nil(t, a.ExistingActivityID)

	assert.Equal(t, "York", b.Location, "a stay with no place name falls back to its city")
	assert.NotNil(t, b.Photos, "photos is always a (possibly empty) array, never null")
	assert.Empty(t, b.Photos)
	assert.False(t, b.PhotosUnavailable, "an empty successful lookup is 'no photos', not 'unavailable'")

	// One identity call, one timeline call, one photo search per stay, radius = the named constant.
	calls := f.calls()
	require.Len(t, calls, 1+1+2)
	assert.True(t, strings.HasPrefix(calls[0], "/api/streaming-timeline"))
	assert.True(t, strings.HasPrefix(calls[1], "/api/users/me"))
	assert.Contains(t, calls[2], "/api/users/"+f.UserID+"/immich/photos/search")
	assert.Contains(t, calls[2], "radiusMeters=200")

	// Nothing was persisted.
	var activities int64
	require.NoError(t, db.Model(&models.Activity{}).Where("user_id = ?", user.ID).Count(&activities).Error)
	assert.Zero(t, activities, "suggestions are ephemeral: nothing is written until the user confirms")
}

func TestGeoPulseSuggestionsForDate_FlagsAlreadyLoggedStays(t *testing.T) {
	db := dbtest.New(t)
	user := seedGeoPulseUser(t, db, "gp-dup")
	other := seedGeoPulseUser(t, db, "gp-dup-other")
	f := newFakeGeoPulseServer(t, "")
	f.addStay(7, "2026-09-20T14:00:00Z", "Cafe", "Leeds", 53.8, -1.55, 60)
	f.addStay(8, "2026-09-20T15:00:00Z", "Pub", "Leeds", 53.81, -1.56, 60)
	connectGeoPulseForUser(t, db, user.ID, f.URL(), "k")

	mine := models.Activity{UserID: user.ID, Title: "Coffee", Date: time.Now(), ExternalRef: "geopulse:stay:7"}
	require.NoError(t, db.Create(&mine).Error)
	// Another user's activity with the same ref must not count.
	require.NoError(t, db.Create(&models.Activity{UserID: other.ID, Title: "x", Date: time.Now(), ExternalRef: "geopulse:stay:8"}).Error)

	res, err := GeoPulseSuggestionsForDate(context.Background(), db, geopulseTestConfig(), user.ID, "2026-09-20", "")
	require.NoError(t, err)
	require.Len(t, res.Suggestions, 2)
	require.NotNil(t, res.Suggestions[0].ExistingActivityID)
	assert.Equal(t, mine.ID, *res.Suggestions[0].ExistingActivityID)
	assert.Nil(t, res.Suggestions[1].ExistingActivityID, "ownership scoping: another user's activity is invisible")
}

func TestGeoPulseSuggestions_PhotoFailureDegradesNotFails(t *testing.T) {
	db := dbtest.New(t)
	user := seedGeoPulseUser(t, db, "gp-degrade")
	f := newFakeGeoPulseServer(t, "")
	f.addStay(1, "2026-09-20T10:00:00Z", "A", "Leeds", 53.1, -1.1, 60)
	f.addStay(2, "2026-09-20T11:00:00Z", "B", "Leeds", 53.2, -1.2, 60)
	f.FailPhotos = http.StatusInternalServerError // e.g. Immich not configured inside GeoPulse
	connectGeoPulseForUser(t, db, user.ID, f.URL(), "k")

	res, err := GeoPulseSuggestionsForDate(context.Background(), db, geopulseTestConfig(), user.ID, "2026-09-20", "")
	require.NoError(t, err, "a failed photo call must not fail the suggestion list")
	require.Len(t, res.Suggestions, 2)
	for _, s := range res.Suggestions {
		assert.True(t, s.PhotosUnavailable)
		assert.Empty(t, s.Photos)
	}

	photoCalls := 0
	for _, c := range f.calls() {
		if strings.Contains(c, "/immich/photos/search") {
			photoCalls++
		}
	}
	assert.Equal(t, 1, photoCalls, "after the first photo failure no further photo calls are made")
}

// TestGeoPulseSuggestionsForDate_LogsNoCoordinates pins the logging half of
// issue #1445: the Warn line emitted when a photo search fails on a transport
// error must not carry the coordinate-bearing request URL.
func TestGeoPulseSuggestionsForDate_LogsNoCoordinates(t *testing.T) {
	buf := captureLoggerOutput(t)
	faults.Reset()
	t.Cleanup(faults.Reset)

	db := dbtest.New(t)
	user := seedGeoPulseUser(t, db, "gp-log-redact")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/streaming-timeline":
			writeGeoPulseEnvelope(w, map[string]any{"stays": []map[string]any{{
				"id": 1, "timestamp": "2026-09-20T10:00:00Z", "locationName": "A", "city": "Leeds",
				"country": "Testland", "latitude": 53.8, "longitude": -1.55, "stayDuration": 60,
			}}})
		case r.URL.Path == "/api/users/me":
			// Arm the request seam *before* answering identity, so the very next
			// call — the photo search carrying the coordinates — fails at the
			// transport layer, exactly where the URL would otherwise leak.
			faults.ArmError(faultGeoPulseRequest, errors.New("injected upstream failure"))
			writeGeoPulseEnvelope(w, map[string]any{"userId": "u1", "fullName": "U"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	connectGeoPulseForUser(t, db, user.ID, srv.URL, "k")

	res, err := GeoPulseSuggestionsForDate(context.Background(), db, geopulseTestConfig(), user.ID, "2026-09-20", "")
	require.NoError(t, err)
	require.Len(t, res.Suggestions, 1)
	assert.True(t, res.Suggestions[0].PhotosUnavailable)

	logs := buf.String()
	assert.Contains(t, logs, "GeoPulse photo lookup failed")
	assert.NotContains(t, logs, "53.8")
	assert.NotContains(t, logs, "-1.55")
	assert.NotContains(t, logs, "latitude=")
	assert.NotContains(t, logs, "longitude=")
}

func TestGeoPulseSuggestions_IdentityFailureSkipsPhotos(t *testing.T) {
	db := dbtest.New(t)
	user := seedGeoPulseUser(t, db, "gp-noid")
	f := newFakeGeoPulseServer(t, "")
	f.addStay(1, "2026-09-20T10:00:00Z", "A", "Leeds", 53.1, -1.1, 60)
	f.FailMe = http.StatusInternalServerError
	connectGeoPulseForUser(t, db, user.ID, f.URL(), "k")

	res, err := GeoPulseSuggestionsForDate(context.Background(), db, geopulseTestConfig(), user.ID, "2026-09-20", "")
	require.NoError(t, err)
	require.Len(t, res.Suggestions, 1)
	assert.True(t, res.Suggestions[0].PhotosUnavailable)
	for _, c := range f.calls() {
		assert.NotContains(t, c, "/immich/photos/search", "without the GeoPulse user id there is nothing valid to call")
	}
}

func TestGeoPulseSuggestions_PhotoLookupsAreCapped(t *testing.T) {
	db := dbtest.New(t)
	user := seedGeoPulseUser(t, db, "gp-cap")
	f := newFakeGeoPulseServer(t, "")
	const stays = geopulseMaxStayLookups + 5
	for i := 0; i < stays; i++ {
		f.addStay(int64(i+1), "2026-09-20T10:00:00Z", "Place", "Leeds", 50+float64(i)/100, -1, 60)
	}
	connectGeoPulseForUser(t, db, user.ID, f.URL(), "k")

	res, err := GeoPulseSuggestionsForDate(context.Background(), db, geopulseTestConfig(), user.ID, "2026-09-20", "")
	require.NoError(t, err)
	require.Len(t, res.Suggestions, stays, "every stay is still suggested")

	photoCalls := 0
	for _, c := range f.calls() {
		if strings.Contains(c, "/immich/photos/search") {
			photoCalls++
		}
	}
	assert.Equal(t, geopulseMaxStayLookups, photoCalls)
	assert.False(t, res.Suggestions[geopulseMaxStayLookups-1].PhotosUnavailable)
	assert.True(t, res.Suggestions[geopulseMaxStayLookups].PhotosUnavailable, "stays past the cap are suggested without photos and say so")
}

func TestGeoPulseSuggestionsForDate_Errors(t *testing.T) {
	db := dbtest.New(t)
	user := seedGeoPulseUser(t, db, "gp-err")
	cfg := geopulseTestConfig()

	_, err := GeoPulseSuggestionsForDate(context.Background(), db, cfg, user.ID, "not-a-date", "")
	assert.ErrorIs(t, err, ErrGeoPulseInvalidDate, "a bad date is rejected before any connection lookup")
	_, err = GeoPulseSuggestionsForDate(context.Background(), db, cfg, user.ID, "2026-09-20", "")
	assert.ErrorIs(t, err, ErrGeoPulseUnauthorized, "no connection configured")

	f := newFakeGeoPulseServer(t, "")
	connectGeoPulseForUser(t, db, user.ID, f.URL(), "k")

	f.FailTimeline = http.StatusBadGateway
	_, err = GeoPulseSuggestionsForDate(context.Background(), db, cfg, user.ID, "2026-09-20", "")
	assert.ErrorIs(t, err, ErrGeoPulseRequestFailed, "a failed timeline call fails the request")

	f.FailTimeline = 0
	f.addStay(1, "yesterday-ish", "A", "Leeds", 53.1, -1.1, 60)
	_, err = GeoPulseSuggestionsForDate(context.Background(), db, cfg, user.ID, "2026-09-20", "")
	assert.ErrorIs(t, err, ErrGeoPulseInvalidData, "an unparseable stay timestamp is invalid data, not a silent zero time")

	f.Stays = nil
	res, err := GeoPulseSuggestionsForDate(context.Background(), db, cfg, user.ID, "2026-09-20", "")
	require.NoError(t, err)
	assert.NotNil(t, res.Suggestions)
	assert.Empty(t, res.Suggestions, "a day with no stays is an empty (non-null) list")
}

func TestFindActivityByExternalRef_DatabaseError(t *testing.T) {
	db := dbtest.New(t)
	user := seedGeoPulseUser(t, db, "gp-find-err")
	dbtest.HideTable(t, db, "activities")
	got, err := FindActivityByExternalRef(db, user.ID, "geopulse:stay:1")
	require.Error(t, err, "a real DB error is not 'no match' — treating it as one would create duplicates")
	assert.Nil(t, got)
}

func TestFindActivityByExternalRef(t *testing.T) {
	db := dbtest.New(t)
	user := seedGeoPulseUser(t, db, "gp-find")
	other := seedGeoPulseUser(t, db, "gp-find-other")
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	got, err := FindActivityByExternalRef(db, user.ID, "geopulse:stay:1")
	require.NoError(t, err)
	assert.Nil(t, got)

	first := models.Activity{UserID: user.ID, Title: "first", Date: time.Now(), ExternalRef: "geopulse:stay:1"}
	require.NoError(t, db.Create(&first).Error)
	require.NoError(t, db.Model(&first).Association("Contacts").Append(&contact))
	dupe := models.Activity{UserID: user.ID, Title: "dupe", Date: time.Now(), ExternalRef: "geopulse:stay:1"}
	require.NoError(t, db.Create(&dupe).Error)

	got, err = FindActivityByExternalRef(db, user.ID, "geopulse:stay:1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, first.ID, got.ID, "the oldest match wins, deterministically")
	require.Len(t, got.Contacts, 1, "contacts are preloaded so the caller can return the full activity")

	got, err = FindActivityByExternalRef(db, other.ID, "geopulse:stay:1")
	require.NoError(t, err)
	assert.Nil(t, got, "scoped to user_id")

	// A soft-deleted activity is the user's deletion: confirming again re-creates.
	require.NoError(t, db.Delete(&first).Error)
	require.NoError(t, db.Delete(&dupe).Error)
	got, err = FindActivityByExternalRef(db, user.ID, "geopulse:stay:1")
	require.NoError(t, err)
	assert.Nil(t, got)
}

// A slow-but-alive GeoPulse (photo endpoint stalls) must not hold the request
// past the overall budget: the call returns in time with the stays suggested
// and photos_unavailable set.
func TestGeoPulseSuggestionsForDate_BudgetBoundsSlowPhotoLookups(t *testing.T) {
	prevBudget, prevTimeout := geopulseSuggestionsBudget, geopulseRequestTimeout
	geopulseSuggestionsBudget = 300 * time.Millisecond
	geopulseRequestTimeout = 30 * time.Second
	t.Cleanup(func() { geopulseSuggestionsBudget, geopulseRequestTimeout = prevBudget, prevTimeout })

	db := dbtest.New(t)
	user := seedGeoPulseUser(t, db, "gp-budget")
	f := newFakeGeoPulseServer(t, "")
	f.PhotoDelay = 10 * time.Second
	f.addStay(1, "2026-09-20T10:00:00Z", "A", "Leeds", 53.1, -1.1, 60)
	f.addStay(2, "2026-09-20T12:00:00Z", "B", "York", 53.9, -1.0, 60)
	connectGeoPulseForUser(t, db, user.ID, f.URL(), "k")

	began := time.Now()
	res, err := GeoPulseSuggestionsForDate(context.Background(), db, geopulseTestConfig(), user.ID, "2026-09-20", "")
	elapsed := time.Since(began)
	require.NoError(t, err)
	assert.Less(t, elapsed, 5*time.Second, "must return near the budget, not wait out the 10s photo delay")
	require.Len(t, res.Suggestions, 2)
	for _, s := range res.Suggestions {
		assert.True(t, s.PhotosUnavailable)
		assert.Empty(t, s.Photos)
	}
	photoCalls := 0
	for _, c := range f.calls() {
		if strings.Contains(c, "/photos/search") {
			photoCalls++
		}
	}
	assert.Equal(t, 1, photoCalls, "after the budget is spent no further photo lookups are issued")
}

// Cancelling the caller's context (client went away) aborts the timeline call.
func TestGeoPulseSuggestionsForDate_CancelledContextIsAnError(t *testing.T) {
	db := dbtest.New(t)
	user := seedGeoPulseUser(t, db, "gp-cancel")
	f := newFakeGeoPulseServer(t, "")
	connectGeoPulseForUser(t, db, user.ID, f.URL(), "k")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GeoPulseSuggestionsForDate(ctx, db, geopulseTestConfig(), user.ID, "2026-09-20", "")
	assert.ErrorIs(t, err, ErrGeoPulseUnreachable)
}

func TestSameGeoPulseOrigin(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"https://gp.example", "https://gp.example/sub/path", true},
		{"https://gp.example", "https://GP.EXAMPLE:443", true},
		{"http://gp.example", "http://gp.example:80/x", true},
		{"http://gp.example", "https://gp.example", false},
		{"https://gp.example", "https://gp.example:8443", false},
		{"https://gp.example", "https://gp.example.evil.test", false},
		{"://bad", "https://gp.example", false},
		{"https://gp.example", "://bad", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, sameGeoPulseOrigin(tc.a, tc.b), "%s vs %s", tc.a, tc.b)
	}
}
