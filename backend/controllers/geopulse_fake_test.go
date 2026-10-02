package controllers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeGeoPulseController is a real-protocol test double for the slice of the
// GeoPulse API (v1.39.0) the integration uses — the controller-package copy of
// services/geopulse_fake_test.go, serving the same {status, data} envelope.
type fakeGeoPulseController struct {
	Server *httptest.Server
	// Key, when non-empty, makes the server 401 any request without that X-API-Key.
	Key    string
	UserID string
	Stays  []map[string]any
	// Photos is returned for every photo search.
	Photos []map[string]any
	// FailTimeline / FailPhotos force a status for that endpoint.
	FailTimeline int
	FailPhotos   int
	// RawBody replaces every 200 body (malformed-data tests).
	RawBody string
}

func newGeoPulseTestServer(t *testing.T, key string) *fakeGeoPulseController {
	f := &fakeGeoPulseController{Key: key, UserID: "11111111-2222-3333-4444-555555555555"}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.Server.Close)
	return f
}

func (f *fakeGeoPulseController) URL() string { return f.Server.URL }

func (f *fakeGeoPulseController) addStay(id int64, timestamp, name, city string, lat, lon float64) {
	f.Stays = append(f.Stays, map[string]any{
		"id": id, "timestamp": timestamp, "locationName": name, "city": city, "country": "Testland",
		"latitude": lat, "longitude": lon, "stayDuration": 1800,
	})
}

func (f *fakeGeoPulseController) handle(w http.ResponseWriter, r *http.Request) {
	if f.Key != "" && r.Header.Get("X-API-Key") != f.Key {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if f.RawBody != "" {
		_, _ = w.Write([]byte(f.RawBody))
		return
	}
	envelope := func(data any) {
		writeControllerJSON(w, map[string]any{"status": "success", "data": data})
	}
	switch {
	case r.URL.Path == "/api/users/me":
		envelope(map[string]any{"userId": f.UserID, "fullName": "Test User"})
	case r.URL.Path == "/api/streaming-timeline":
		if f.FailTimeline != 0 {
			w.WriteHeader(f.FailTimeline)
			return
		}
		stays := f.Stays
		if stays == nil {
			stays = []map[string]any{}
		}
		envelope(map[string]any{"stays": stays, "trips": []any{}, "dataGaps": []any{}})
	case strings.HasPrefix(r.URL.Path, "/api/users/") && strings.HasSuffix(r.URL.Path, "/immich/photos/search"):
		if f.FailPhotos != 0 {
			w.WriteHeader(f.FailPhotos)
			return
		}
		photos := f.Photos
		if photos == nil {
			photos = []map[string]any{}
		}
		envelope(map[string]any{"photos": photos, "totalCount": len(photos)})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}
