package realserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	envImmichURL = "MYCORRHIZAL_RS_IMMICH_URL"

	immichAdminEmail = "contract-admin@example.com"
	immichAdminPass  = "contract-admin-pass"
)

// immichAPI is a deliberately tiny, independent Immich client used only to
// seed state (admin account, API key, asset, person, face). It must not share
// code with services.ImmichClient, or a shared misreading of the API would
// cancel out.
type immichAPI struct {
	t     *testing.T
	base  string
	token string // bearer access token (seeding)
}

func (a *immichAPI) do(method, path, contentType string, body io.Reader, out any) int {
	a.t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, a.base+"/api"+path, body)
	require.NoError(a.t, err)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(a.t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode/100 == 2 {
		require.NoError(a.t, json.Unmarshal(raw, out), "decoding %s %s: %s", method, path, raw)
	} else if resp.StatusCode/100 != 2 && out != nil {
		a.t.Fatalf("%s %s: %d %s", method, path, resp.StatusCode, raw)
	}
	return resp.StatusCode
}

func (a *immichAPI) postJSON(path string, in, out any) int {
	a.t.Helper()
	raw, err := json.Marshal(in)
	require.NoError(a.t, err)
	return a.do(http.MethodPost, path, "application/json", bytes.NewReader(raw), out)
}

// testJPEG renders a small, valid, non-trivial JPEG.
func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 255 / w), G: uint8(y * 255 / h), B: 160, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

func (a *immichAPI) upload(jpg []byte, takenAt time.Time) string {
	a.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	stamp := takenAt.UTC().Format("2006-01-02T15:04:05.000Z")
	id := fmt.Sprintf("contract-%d", time.Now().UnixNano())
	for k, v := range map[string]string{"deviceAssetId": id, "deviceId": "mycorrhizal-contract", "fileCreatedAt": stamp, "fileModifiedAt": stamp} {
		require.NoError(a.t, mw.WriteField(k, v))
	}
	fw, err := mw.CreateFormFile("assetData", id+".jpg")
	require.NoError(a.t, err)
	_, err = fw.Write(jpg)
	require.NoError(a.t, err)
	require.NoError(a.t, mw.Close())
	var res struct {
		ID string `json:"id"`
	}
	a.do(http.MethodPost, "/assets", mw.FormDataContentType(), &buf, &res)
	require.NotEmpty(a.t, res.ID)
	return res.ID
}

// immichFixture is the seeded state a test runs against.
type immichFixture struct {
	client   *services.ImmichClient
	base     string
	apiKey   string
	personID string
	assetID  string
	takenAt  time.Time
}

// seedImmich boots the server's first admin, mints an API key (what a user
// pastes into Mycorrhizal), uploads one real JPEG and attaches a named person
// to it through the API (no ML container: a manual face is how the suite gets
// a person with an asset).
func seedImmich(t *testing.T) *immichFixture {
	t.Helper()
	base := serverURL(t, envImmichURL)
	waitReady(t, base+"/api/server/ping", 300*time.Second)

	a := &immichAPI{t: t, base: base}
	// Idempotent across tests in one run: the first sign-up wins, later ones 400.
	a.postJSON("/auth/admin-sign-up", map[string]string{"email": immichAdminEmail, "password": immichAdminPass, "name": "Contract Admin"}, nil)
	var login struct {
		AccessToken string `json:"accessToken"`
	}
	a.postJSON("/auth/login", map[string]string{"email": immichAdminEmail, "password": immichAdminPass}, &login)
	require.NotEmpty(t, login.AccessToken)
	a.token = login.AccessToken

	var key struct {
		Secret string `json:"secret"`
	}
	a.postJSON("/api-keys", map[string]any{"name": fmt.Sprintf("contract-%d", time.Now().UnixNano()), "permissions": []string{"all"}}, &key)
	require.NotEmpty(t, key.Secret)

	taken := time.Date(2024, 6, 15, 12, 30, 0, 0, time.UTC)
	assetID := a.upload(testJPEG(t, 320, 240), taken)

	var person struct {
		ID string `json:"id"`
	}
	a.postJSON("/people", map[string]string{"name": fmt.Sprintf("Contract Person %d", time.Now().UnixNano())}, &person)
	require.NotEmpty(t, person.ID)

	// Face creation needs the asset's metadata job to have run; retry briefly.
	deadline := time.Now().Add(120 * time.Second)
	for {
		code := a.postJSON("/faces", map[string]any{
			"assetId": assetID, "personId": person.ID,
			"imageWidth": 320, "imageHeight": 240, "x": 80, "y": 60, "width": 120, "height": 120,
		}, nil)
		if code/100 == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("POST /faces never succeeded (last status %d)", code)
		}
		time.Sleep(2 * time.Second)
	}

	c, err := services.NewImmichClient(base, key.Secret, false)
	require.NoError(t, err)
	return &immichFixture{client: c, base: base, apiKey: key.Secret, personID: person.ID, assetID: assetID, takenAt: taken}
}

// TestImmich_ConnectionAndPickerFlowAgainstRealServer exercises every call the
// integration makes — Test Connection (Ping, GetMyUser), the person picker
// (ListPeople, GetStatistics), "latest appearance" (RecentAssets, via
// POST /api/search/metadata) and both image fetches — against a real Immich.
// This is the test that would have caught Immich's API changing between
// minors (/api/people/:id/assets was removed in v3): the fake only ever
// answers the shapes we already believe in.
func TestImmich_ConnectionAndPickerFlowAgainstRealServer(t *testing.T) {
	f := seedImmich(t)

	require.NoError(t, f.client.Ping(), "Test Connection stage 1")
	me, err := f.client.GetMyUser()
	require.NoError(t, err, "Test Connection stage 2")
	assert.Equal(t, immichAdminEmail, me.Email)
	assert.Equal(t, "Contract Admin", me.Name)

	people, err := f.client.ListPeople()
	require.NoError(t, err)
	var found bool
	for _, p := range people {
		found = found || p.ID == f.personID
	}
	assert.True(t, found, "the seeded person must be in the picker list (got %d people)", len(people))

	// Metadata/face indexing is asynchronous inside Immich: poll the statistic
	// our "photo count" display depends on.
	var count int
	for i := 0; i < 60 && count < 1; i++ {
		count, err = f.client.GetStatistics(f.personID)
		require.NoError(t, err)
		if count < 1 {
			time.Sleep(2 * time.Second)
		}
	}
	assert.Equal(t, 1, count, "GetStatistics")

	assets, err := f.client.RecentAssets(f.personID, 5)
	require.NoError(t, err)
	require.Len(t, assets, 1, "RecentAssets via POST /api/search/metadata")
	assert.Equal(t, f.assetID, assets[0].ID)
	taken, err := time.Parse(time.RFC3339, assets[0].FileCreatedAt)
	require.NoError(t, err, "fileCreatedAt must parse as RFC 3339, got %q", assets[0].FileCreatedAt)
	assert.True(t, taken.Equal(f.takenAt), "fileCreatedAt %s != uploaded %s", taken, f.takenAt)

	// Thumbnails are produced by background jobs; poll.
	var body []byte
	var ctype string
	for i := 0; i < 60; i++ {
		body, ctype, err = f.client.AssetThumbnail(f.assetID)
		if err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	require.NoError(t, err, "AssetThumbnail")
	assert.True(t, strings.HasPrefix(ctype, "image/"), ctype)
	assert.True(t, looksLikeRaster(body), "the thumbnail bytes must be a JPEG/WebP/PNG image, got %d bytes starting % x", len(body), body[:min(len(body), 12)])

	for i := 0; i < 60; i++ {
		body, ctype, err = f.client.Thumbnail(f.personID)
		if err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	require.NoError(t, err, "person Thumbnail")
	assert.True(t, strings.HasPrefix(ctype, "image/"), ctype)
	assert.NotEmpty(t, body)
}

// looksLikeRaster sniffs the magic bytes of the formats Immich serves as
// thumbnails (WebP by default, JPEG optionally; stdlib cannot decode WebP, so
// decoding is not the check).
func looksLikeRaster(b []byte) bool {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF: // JPEG
		return true
	case len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return true
	case len(b) >= 8 && string(b[1:4]) == "PNG":
		return true
	}
	return false
}

// TestImmich_ErrorsMapToSentinels pins the real server's failure statuses: a
// wrong API key is ErrImmichUnauthorized. A person id that does not exist is,
// on real Immich, a 400 (its access check reports "not found or no access" as
// Bad Request, not 404), so it surfaces as ErrImmichRequestFailed carrying 400
// — pinned here so a change to that mapping (Immich moving to 404, or us
// reclassifying) is a deliberate edit, not a silent drift.
func TestImmich_ErrorsMapToSentinels(t *testing.T) {
	f := seedImmich(t)

	_, err := f.client.GetStatistics("00000000-0000-4000-8000-000000000000")
	require.ErrorIs(t, err, services.ErrImmichRequestFailed)
	var reqErr *services.ImmichRequestError
	require.ErrorAs(t, err, &reqErr)
	assert.Equal(t, http.StatusBadRequest, reqErr.StatusCode)

	bad, err := services.NewImmichClient(f.base, "definitely-not-an-api-key", false)
	require.NoError(t, err)
	_, err = bad.GetMyUser()
	require.ErrorIs(t, err, services.ErrImmichUnauthorized)
}
