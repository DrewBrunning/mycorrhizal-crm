package controllers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testAsset mirrors the Immich asset DTO slice the integration reads.
type testAsset struct {
	ID            string `json:"id"`
	FileCreatedAt string `json:"fileCreatedAt"`
	CreatedAt     string `json:"createdAt"`
}

// testPerson is the server-side state for one Immich person.
type testPerson struct {
	Name       string
	PhotoCount int
	Assets     []testAsset
	Thumbnail  []byte
}

// testMe is the server-side state for GET /users/me (Test Connection's auth
// check).
type testMe struct {
	Email string
	Name  string
}

// newImmichTestServer is the controller-package copy of the fake Immich
// server (the services package keeps its own for its tests; both exercise the
// real HTTP protocol against the same wire shapes).
type fakeImmichController struct {
	t      *testing.T
	Server *httptest.Server
	APIKey string
	People map[string]*testPerson
	// AssetThumbnails serves GET /api/assets/:id/thumbnail (the contact-photo
	// picker's per-photo fetch), keyed by asset ID — independent of People.
	AssetThumbnails map[string][]byte
	// Me is the identity returned by GET /users/me. Defaults to a fixed test
	// account when unset.
	Me             *testMe
	FailWithStatus int
	LastAPIKey     string
}

// newImmichTestServer builds a started fake Immich instance.
func newImmichTestServer(t *testing.T, apiKey string) *fakeImmichController {
	f := &fakeImmichController{
		t:               t,
		APIKey:          apiKey,
		People:          make(map[string]*testPerson),
		AssetThumbnails: make(map[string][]byte),
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

// Close shuts the fake server down.
func (f *fakeImmichController) Close() {
	f.Server.Close()
}

// URL returns the fake instance's base URL.
func (f *fakeImmichController) URL() string {
	return f.Server.URL
}

// addTestPerson registers a person.
func (f *fakeImmichController) addTestPerson(id, name string, photoCount int, assets []testAsset, thumbnail []byte) {
	f.People[id] = &testPerson{Name: name, PhotoCount: photoCount, Assets: assets, Thumbnail: thumbnail}
}

// addTestAssetThumbnail registers an asset's thumbnail bytes.
func (f *fakeImmichController) addTestAssetThumbnail(assetID string, thumbnail []byte) {
	f.AssetThumbnails[assetID] = thumbnail
}

func (f *fakeImmichController) handle(w http.ResponseWriter, r *http.Request) {
	f.t.Helper()

	if f.FailWithStatus != 0 {
		w.WriteHeader(f.FailWithStatus)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api")

	// Real Immich's ping is unauthenticated — see the services-package fake's
	// identical comment for why this must stay outside the key check.
	if path == "/server/ping" {
		writeControllerJSON(w, map[string]any{"res": "pong"})
		return
	}

	key := r.Header.Get("x-api-key")
	f.LastAPIKey = key
	if f.APIKey != "" && key != f.APIKey {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	switch {
	case path == "/users/me":
		me := f.Me
		if me == nil {
			me = &testMe{Email: "test@example.com", Name: "Test User"}
		}
		writeControllerJSON(w, map[string]any{"email": me.Email, "name": me.Name})
	case path == "/people":
		items := make([]map[string]any, 0, len(f.People))
		for id, p := range f.People {
			items = append(items, map[string]any{"id": id, "name": p.Name})
		}
		writeControllerJSON(w, map[string]any{"people": map[string]any{"items": items, "hasNextPage": false}, "total": len(items)})
	case r.Method == http.MethodPost && path == "/search/metadata":
		f.handleSearchMetadata(w, r)
	// Checked before the generic person "/thumbnail" suffix case below —
	// both end in "/thumbnail", and this one is asset-scoped, not
	// person-scoped (the contact-photo picker's per-photo fetch).
	case strings.HasPrefix(path, "/assets/") && strings.HasSuffix(path, "/thumbnail"):
		id := testAssetIDFromPath(path)
		body, ok := f.AssetThumbnails[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(body)
	case strings.HasSuffix(path, "/statistics"):
		id := testPersonIDFromPath(path, "/statistics")
		p, ok := f.People[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeControllerJSON(w, map[string]any{"assets": p.PhotoCount})
	case strings.HasSuffix(path, "/thumbnail"):
		id := testPersonIDFromPath(path, "/thumbnail")
		p, ok := f.People[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(p.Thumbnail)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func testPersonIDFromPath(path, suffix string) string {
	start := strings.Index(path, "/people/") + len("/people/")
	return path[start : len(path)-len(suffix)]
}

// testAssetIDFromPath extracts the asset ID from "/assets/:id/thumbnail".
func testAssetIDFromPath(path string) string {
	start := strings.Index(path, "/assets/") + len("/assets/")
	return path[start : len(path)-len("/thumbnail")]
}

func writeControllerJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeImmichController) handleSearchMetadata(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
	var req struct {
		PersonIDs []string `json:"personIds"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.PersonIDs) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	personID := req.PersonIDs[0]
	p, ok := f.People[personID]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeControllerJSON(w, map[string]any{
		"assets": map[string]any{
			"items": p.Assets,
			"total": len(p.Assets),
			"count": len(p.Assets),
		},
	})
}
