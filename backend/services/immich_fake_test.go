package services

import (
	"encoding/json"
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
	// FailWithStatus forces a whole-instance failure when non-zero.
	FailWithStatus int
	// LastAPIKey records the last x-api-key header seen, for asserting the
	// client sends the right credentials.
	LastAPIKey string
}

// newFakeImmichServer builds a started fake Immich instance.
func newFakeImmichServer(t *testing.T, apiKey string) *fakeImmichServer {
	f := &fakeImmichServer{
		t:      t,
		APIKey: apiKey,
		People: make(map[string]*fakeImmichPerson),
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

	key := r.Header.Get("x-api-key")
	f.LastAPIKey = key
	if f.APIKey != "" && key != f.APIKey {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Invalid API key"}`))
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api")
	switch {
	case path == "/people":
		f.handlePeople(w)
	case strings.HasSuffix(path, "/statistics"):
		f.handleStatistics(w, personIDFromPath(path, "/statistics"))
	case strings.HasSuffix(path, "/assets"):
		f.handleAssets(w, personIDFromPath(path, "/assets"))
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

func (f *fakeImmichServer) handleAssets(w http.ResponseWriter, personID string) {
	p, ok := f.People[personID]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{"items": p.Assets, "nextPage": ""})
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

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// addPerson registers a person with the given assets and thumbnail.
func (f *fakeImmichServer) addPerson(id, name string, photoCount int, assets []fakeImmichAsset, thumbnail []byte) {
	f.People[id] = &fakeImmichPerson{Name: name, PhotoCount: photoCount, Assets: assets, Thumbnail: thumbnail}
}
