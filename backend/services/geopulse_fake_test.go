package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGeoPulseServer is a permanent, real-protocol test double for the slice of
// the GeoPulse API (verified against v1.39.0) this integration relies on:
// GET /api/users/me, GET /api/streaming-timeline and
// GET /api/users/{userId}/immich/photos/search. It serves the same
// {status, message, data} envelope the real server does, so the client is
// exercised against the real wire shape, not a mocked boundary.
type fakeGeoPulseServer struct {
	t      *testing.T
	Server *httptest.Server

	// Key, when non-empty, makes the server reject requests without a matching
	// X-API-Key header (401).
	Key string
	// UserID / FullName are what GET /api/users/me reports.
	UserID   string
	FullName string
	// Stays is the raw stay objects the timeline returns (so a test can also
	// inject malformed ones).
	Stays []map[string]any
	// Photos maps a stay latitude (strconv 'f',-1) to the photos found near it.
	Photos map[string][]map[string]any

	// FailMe / FailTimeline / FailPhotos force a status for that endpoint.
	FailMe       int
	FailTimeline int
	FailPhotos   int
	// PhotoDelay delays every photo-search response (slow-GeoPulse tests).
	PhotoDelay time.Duration
	// RedirectTo, when non-empty, answers every request with a 302 to it.
	RedirectTo string
	// RawBody, when non-empty, replaces every 200 response body (malformed-data tests).
	RawBody string

	mu sync.Mutex
	// Calls records "<path>?<query>" for every request, for assertions.
	Calls []string
	// LastKey records the last X-API-Key seen.
	LastKey string
}

func newFakeGeoPulseServer(t *testing.T, key string) *fakeGeoPulseServer {
	f := &fakeGeoPulseServer{
		t:        t,
		Key:      key,
		UserID:   "11111111-2222-3333-4444-555555555555",
		FullName: "Test User",
		Photos:   map[string][]map[string]any{},
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.Server.Close)
	return f
}

func (f *fakeGeoPulseServer) URL() string { return f.Server.URL }

// calls returns a copy of the recorded request lines.
func (f *fakeGeoPulseServer) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.Calls...)
}

// addStay appends a stay in the real DTO's JSON shape.
func (f *fakeGeoPulseServer) addStay(id int64, timestamp, name, city string, lat, lon float64, durationSeconds int64) {
	f.Stays = append(f.Stays, map[string]any{
		"id": id, "timestamp": timestamp, "locationName": name, "city": city, "country": "Testland",
		"latitude": lat, "longitude": lon, "stayDuration": durationSeconds,
	})
}

func (f *fakeGeoPulseServer) addPhoto(stayLat float64, id, fileName, takenAt string) {
	k := strconv.FormatFloat(stayLat, 'f', -1, 64)
	f.Photos[k] = append(f.Photos[k], map[string]any{
		"id": id, "originalFileName": fileName, "takenAt": takenAt,
		"thumbnailUrl": "/api/users/x/immich/photos/" + id + "/thumbnail",
	})
}

func (f *fakeGeoPulseServer) handle(w http.ResponseWriter, r *http.Request) {
	f.t.Helper()

	f.mu.Lock()
	f.Calls = append(f.Calls, r.URL.EscapedPath()+"?"+r.URL.RawQuery)
	f.LastKey = r.Header.Get("X-API-Key")
	f.mu.Unlock()

	if f.RedirectTo != "" {
		http.Redirect(w, r, f.RedirectTo, http.StatusFound)
		return
	}

	if f.Key != "" && r.Header.Get("X-API-Key") != f.Key {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if f.RawBody != "" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(f.RawBody))
		return
	}

	switch {
	case r.URL.Path == "/api/users/me":
		if f.FailMe != 0 {
			w.WriteHeader(f.FailMe)
			return
		}
		writeGeoPulseEnvelope(w, map[string]any{"userId": f.UserID, "fullName": f.FullName, "email": "t@example.com"})
	case r.URL.Path == "/api/streaming-timeline":
		if f.FailTimeline != 0 {
			w.WriteHeader(f.FailTimeline)
			return
		}
		stays := f.Stays
		if stays == nil {
			stays = []map[string]any{}
		}
		writeGeoPulseEnvelope(w, map[string]any{"stays": stays, "trips": []any{}, "dataGaps": []any{}})
	case strings.HasPrefix(r.URL.Path, "/api/users/") && strings.HasSuffix(r.URL.Path, "/immich/photos/search"):
		if f.PhotoDelay > 0 {
			select {
			case <-time.After(f.PhotoDelay):
			case <-r.Context().Done():
				return
			}
		}
		if f.FailPhotos != 0 {
			w.WriteHeader(f.FailPhotos)
			return
		}
		photos := f.Photos[r.URL.Query().Get("latitude")]
		if photos == nil {
			photos = []map[string]any{}
		}
		writeGeoPulseEnvelope(w, map[string]any{"photos": photos, "totalCount": len(photos)})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func writeGeoPulseEnvelope(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": data})
}
