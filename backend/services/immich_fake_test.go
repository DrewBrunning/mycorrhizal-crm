package services

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
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
	// PageSize, when > 0, makes /api/search/metadata paginate this person's
	// assets that many per page and advertise a nextPage token (T70),
	// overriding the request's `size` so a test can force a server that pages
	// *smaller* than requested — which is what makes the client's early-stop
	// observable (T83). Zero honors the requested size (single page when the
	// person fits), the behavior every other test assumes.
	PageSize int
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
	// SearchPages records the resolved `page` of every /api/search/metadata
	// request, in order, so a test can assert the pagination loop actually
	// walked pages 1, 2, 3 rather than looping on one (T70) — and that it
	// stopped early once a limit was met (T83).
	SearchPages []int
	// SearchSizes records the `size` of every /api/search/metadata request,
	// so a test can pin that the caller's limit is passed down as the page
	// size (T83).
	SearchSizes []int
	// SearchOrders records the `order` of every /api/search/metadata request,
	// pinning that the client requests newest-first explicitly rather than
	// relying on the server default (T83).
	SearchOrders []string
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

// handleSearchMetadata mimics POST /api/search/metadata, including the three
// behaviors the real endpoint has and this integration relies on:
//
//   - `page` is validated as a NUMBER.  A JSON string is rejected 400 with
//     Immich's real validation body.  This is the bug T70 fixed: the client
//     used to echo the string-typed nextPage straight back.
//   - assets.nextPage is serialized as a JSON *string* on the way out, the
//     way v3.x does, so the round-trip asymmetry is reproduced faithfully.
//   - `size` and `order` are honored, and items are returned ordered by
//     fileCreatedAt DESC (id DESC as tiebreak) — the ordering SearchRepository
//     applies — so a size-limited request yields the newest-taken assets, and
//     a limit-1 request returns exactly the one the client's early-stop
//     expects (T83).
//
// Pagination activates when PageSize is set on the fake person: it overrides
// the requested size so a test can force a server that pages *smaller* than
// requested, which is what makes the early-stop observable. Otherwise every
// page returns up to the requested size, preserving the single-page behavior
// every pre-existing test relies on.
func (f *fakeImmichServer) handleSearchMetadata(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
	var req struct {
		PersonIDs []string `json:"personIds"`
		Page      any      `json:"page"`
		Size      *int     `json:"size"`
		Order     string   `json:"order"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.PersonIDs) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// The real DTO validates size as an int in [1, 1000] and order as
	// "asc"/"desc"; reject what Immich would reject so a client regression
	// (e.g. size 0, or an order we don't request) is caught rather than
	// silently served. A size that is simply absent keeps the original
	// single-page "return everything" behavior.
	if req.Size != nil && (*req.Size < 1 || *req.Size > 1000) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.Order != "" && req.Order != "asc" && req.Order != "desc" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	page := 1
	switch raw := req.Page.(type) {
	case string:
		// Exactly what the real instance returned for T70: a string page.
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{
			"message": "Validation failed",
			"errors": []map[string]any{{
				"expected": "number",
				"code":     "invalid_type",
				"path":     []string{"page"},
				"message":  "Invalid input: expected number, received string",
			}},
		})
		return
	case float64:
		if raw < 1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		page = int(raw)
	}

	personID := req.PersonIDs[0]
	p, ok := f.People[personID]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Real Immich answers ordered by fileCreatedAt DESC, so the fake must too
	// for a size-limited response to be meaningful (T83).
	items := sortedAssetsForSearch(p.Assets)
	nextPage := ""
	pageSize := len(items)
	if req.Size != nil {
		pageSize = *req.Size
	}
	if p.PageSize > 0 {
		pageSize = p.PageSize
	}
	start := (page - 1) * pageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	items = items[start:end]
	if end < len(p.Assets) {
		nextPage = strconv.Itoa(page + 1) // string, as v3.x sends it
	}

	f.SearchPages = append(f.SearchPages, page)
	if req.Size != nil {
		f.SearchSizes = append(f.SearchSizes, *req.Size)
	}
	f.SearchOrders = append(f.SearchOrders, req.Order)
	writeJSON(w, map[string]any{
		"assets": map[string]any{
			"items":    items,
			"total":    len(p.Assets),
			"count":    len(items),
			"nextPage": nextPage,
		},
	})
}

// sortedAssetsForSearch returns the person's assets in the order real Immich's
// /api/search/metadata returns them: fileCreatedAt DESC, id DESC as the
// tiebreak (SearchRepository.searchMetadata's ORDER BY). RFC3339 strings sort
// lexicographically in timestamp order, which is exact for the UTC-form
// timestamps these tests use.
func sortedAssetsForSearch(assets []fakeImmichAsset) []fakeImmichAsset {
	out := make([]fakeImmichAsset, len(assets))
	copy(out, assets)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].FileCreatedAt != out[j].FileCreatedAt {
			return out[i].FileCreatedAt > out[j].FileCreatedAt
		}
		return out[i].ID > out[j].ID
	})
	return out
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
