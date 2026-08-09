package services

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeImmichAsset mirrors the Immich asset DTO slice this integration reads.
type fakeImmichAsset struct {
	ID            string `json:"id"`
	FileCreatedAt string `json:"fileCreatedAt"`
	CreatedAt     string `json:"createdAt"`
}

// fakeImmichPerson is the server-side state for one Immich person.
type fakeImmichPerson struct {
	Name       string
	PhotoCount int
	Assets     []fakeImmichAsset
	Thumbnail  []byte
}

// fakeImmichMe is the server-side state for GET /users/me (Test Connection's
// auth check).
type fakeImmichMe struct {
	Email string
	Name  string
}

// fakeImmichServer is a permanent, real-protocol test double for the Immich
// REST API (the oidc fake-IdP precedent, T16 "Done when"). It serves the same
// wire shapes the real Immich does for the endpoints this integration relies
// on — /api/people, /api/people/:id/statistics, /api/people/:id/assets, and
// /api/people/:id/thumbnail — so the client is exercised against the real
// HTTP protocol, not a mocked boundary.
type fakeImmichServer struct {
	t *testing.T

	Server *httptest.Server
	// APIKey, when non-empty, makes the server reject requests without a
	// matching x-api-key header (401) — the expired-key failure path.
	APIKey string
	People map[string]*fakeImmichPerson
	// AssetThumbnails serves GET /api/assets/:id/thumbnail (the contact-photo
	// picker's per-photo fetch), keyed by asset ID — independent of People
	// since a picker browses assets directly, not through a person lookup.
	AssetThumbnails map[string][]byte
	// Me is the identity returned by GET /users/me. Defaults to a fixed test
	// account when unset.
	Me *fakeImmichMe
	// FailWithStatus forces a whole-instance failure when non-zero.
	FailWithStatus int
	// LastAPIKey records the last x-api-key header seen, for asserting the
	// client sends the right credentials.
	LastAPIKey string
}

// newFakeImmichServer builds a started fake Immich instance.
func newFakeImmichServer(t *testing.T, apiKey string) *fakeImmichServer {
	f := &fakeImmichServer{
		t:               t,
		APIKey:          apiKey,
		People:          make(map[string]*fakeImmichPerson),
		AssetThumbnails: make(map[string][]byte),
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

// Close shuts the fake server down.
func (f *fakeImmichServer) Close() {
	f.Server.Close()
}

// URL returns the fake instance's base URL.
func (f *fakeImmichServer) URL() string {
	return f.Server.URL
}

func (f *fakeImmichServer) handle(w http.ResponseWriter, r *http.Request) {
	f.t.Helper()

	if f.FailWithStatus != 0 {
		w.WriteHeader(f.FailWithStatus)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api")

	// Real Immich's ping is unauthenticated — it must stay reachable
	// regardless of the API key, so Test Connection's reachability stage is
	// genuinely independent of the auth stage. Every other endpoint here
	// requires the key.
	if path == "/server/ping" {
		f.handlePing(w)
		return
	}

	key := r.Header.Get("x-api-key")
	f.LastAPIKey = key
	if f.APIKey != "" && key != f.APIKey {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Invalid API key"}`))
		return
	}

	switch {
	case path == "/users/me":
		f.handleMe(w)
	case path == "/people":
		f.handlePeople(w)
	case r.Method == http.MethodPost && path == "/search/metadata":
		f.handleSearchMetadata(w, r)
	// Checked before the generic person "/thumbnail" suffix case below —
	// both end in "/thumbnail", and this one is asset-scoped, not
	// person-scoped (the contact-photo picker's per-photo fetch).
	case strings.HasPrefix(path, "/assets/") && strings.HasSuffix(path, "/thumbnail"):
		f.handleAssetThumbnail(w, assetIDFromPath(path))
	case strings.HasSuffix(path, "/statistics"):
		f.handleStatistics(w, personIDFromPath(path, "/statistics"))
	case strings.HasSuffix(path, "/thumbnail"):
		f.handleThumbnail(w, personIDFromPath(path, "/thumbnail"))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// personIDFromPath extracts the person ID between "/people/" and suffix.
func personIDFromPath(path, suffix string) string {
	start := strings.Index(path, "/people/") + len("/people/")
	return path[start : len(path)-len(suffix)]
}

// assetIDFromPath extracts the asset ID from "/assets/:id/thumbnail".
func assetIDFromPath(path string) string {
	start := strings.Index(path, "/assets/") + len("/assets/")
	return path[start : len(path)-len("/thumbnail")]
}

func (f *fakeImmichServer) handlePing(w http.ResponseWriter) {
	writeJSON(w, map[string]any{"res": "pong"})
}

func (f *fakeImmichServer) handleMe(w http.ResponseWriter) {
	me := f.Me
	if me == nil {
		me = &fakeImmichMe{Email: "test@example.com", Name: "Test User"}
	}
	writeJSON(w, map[string]any{"email": me.Email, "name": me.Name})
}

func (f *fakeImmichServer) handlePeople(w http.ResponseWriter) {
	items := make([]map[string]any, 0, len(f.People))
	for id, p := range f.People {
		items = append(items, map[string]any{"id": id, "name": p.Name})
	}
	writeJSON(w, map[string]any{
		"people": map[string]any{"items": items, "hasNextPage": false},
		"total":  len(items),
	})
}

func (f *fakeImmichServer) handleStatistics(w http.ResponseWriter, personID string) {
	p, ok := f.People[personID]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{"assets": p.PhotoCount})
}

func (f *fakeImmichServer) handleSearchMetadata(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, map[string]any{
		"assets": map[string]any{
			"items": p.Assets,
			"total": len(p.Assets),
			"count": len(p.Assets),
		},
	})
}

func (f *fakeImmichServer) handleThumbnail(w http.ResponseWriter, personID string) {
	p, ok := f.People[personID]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(p.Thumbnail)
}

func (f *fakeImmichServer) handleAssetThumbnail(w http.ResponseWriter, assetID string) {
	body, ok := f.AssetThumbnails[assetID]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(body)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// addPerson registers a person with the given assets and thumbnail.
func (f *fakeImmichServer) addPerson(id, name string, photoCount int, assets []fakeImmichAsset, thumbnail []byte) {
	f.People[id] = &fakeImmichPerson{Name: name, PhotoCount: photoCount, Assets: assets, Thumbnail: thumbnail}
}

// addAssetThumbnail registers an asset's thumbnail bytes (the contact-photo
// picker's per-photo fetch, independent of any person).
func (f *fakeImmichServer) addAssetThumbnail(assetID string, thumbnail []byte) {
	f.AssetThumbnails[assetID] = thumbnail
}
