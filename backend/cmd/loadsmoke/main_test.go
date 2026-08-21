package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   classification
	}{
		{"201 created", http.StatusCreated, "", classifyOK},
		{"200 ok", http.StatusOK, "", classifyOK},
		{"429 rate limited", http.StatusTooManyRequests, `{"error":"rate limit exceeded"}`, classifyRateLimited},
		{"500 database is locked", http.StatusInternalServerError, `{"error":{"message":"database is locked"}}`, classifyBusy},
		{"500 SQLITE_BUSY", http.StatusInternalServerError, `SQLITE_BUSY: database is locked`, classifyBusy},
		{"500 unrelated", http.StatusInternalServerError, `{"error":"disk full"}`, classifyOther},
		{"404 not found", http.StatusNotFound, `{"error":"not found"}`, classifyOther},
		{"422 validation", http.StatusUnprocessableEntity, `{"error":"validation failed"}`, classifyOther},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classify(c.status, []byte(c.body))
			if got != c.want {
				t.Errorf("classify(%d, %q) = %v, want %v", c.status, c.body, got, c.want)
			}
		})
	}
}

func TestLoadConfigFromEnv_Defaults(t *testing.T) {
	cfg := loadConfigFromEnv(func(string) string { return "" })
	if cfg != defaultLoadConfig {
		t.Errorf("loadConfigFromEnv with no overrides = %+v, want %+v", cfg, defaultLoadConfig)
	}
}

func TestLoadConfigFromEnv_Overrides(t *testing.T) {
	env := map[string]string{
		"LOADSMOKE_BASE_URL":     "http://example.com/",
		"LOADSMOKE_WORKERS":      "5",
		"LOADSMOKE_DURATION":     "2s",
		"LOADSMOKE_MIN_REQUESTS": "10",
	}
	cfg := loadConfigFromEnv(func(k string) string { return env[k] })
	if cfg.baseURL != "http://example.com" {
		t.Errorf("baseURL = %q, want trailing slash trimmed", cfg.baseURL)
	}
	if cfg.workers != 5 {
		t.Errorf("workers = %d, want 5", cfg.workers)
	}
	if cfg.duration != 2*time.Second {
		t.Errorf("duration = %v, want 2s", cfg.duration)
	}
	if cfg.minRequests != 10 {
		t.Errorf("minRequests = %d, want 10", cfg.minRequests)
	}
}

func TestLoadConfigFromEnv_InvalidOverridesFallBackToDefault(t *testing.T) {
	env := map[string]string{
		"LOADSMOKE_WORKERS":      "not-a-number",
		"LOADSMOKE_DURATION":     "not-a-duration",
		"LOADSMOKE_MIN_REQUESTS": "-5",
	}
	cfg := loadConfigFromEnv(func(k string) string { return env[k] })
	if cfg.workers != defaultLoadConfig.workers {
		t.Errorf("invalid LOADSMOKE_WORKERS must fall back to default, got %d", cfg.workers)
	}
	if cfg.duration != defaultLoadConfig.duration {
		t.Errorf("invalid LOADSMOKE_DURATION must fall back to default, got %v", cfg.duration)
	}
	if cfg.minRequests != defaultLoadConfig.minRequests {
		t.Errorf("negative LOADSMOKE_MIN_REQUESTS must fall back to default, got %d", cfg.minRequests)
	}
}

// stubContactsServer fakes just enough of POST/PUT/DELETE
// /api/v1/contacts[/:id] for runLoad to exercise its full create-update-
// delete cycle against a real *http.Server, without any of the real app's
// auth/persistence. failEvery, when > 0, makes every Nth create return the
// given status/body instead of succeeding — used to prove runLoad's
// classification and aggregation actually wire together end-to-end.
func stubContactsServer(t *testing.T, failEvery int, failStatus int, failBody string) *httptest.Server {
	t.Helper()
	var nextID int64
	var createCount int64

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/contacts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		n := atomic.AddInt64(&createCount, 1)
		if failEvery > 0 && n%int64(failEvery) == 0 {
			w.WriteHeader(failStatus)
			w.Write([]byte(failBody))
			return
		}
		id := atomic.AddInt64(&nextID, 1)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"contact": map[string]interface{}{"id": id},
		})
	})
	mux.HandleFunc("/api/v1/contacts/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut, http.MethodDelete:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"message":"ok"}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	return httptest.NewServer(mux)
}

func TestRunLoad_AllSucceed(t *testing.T) {
	server := stubContactsServer(t, 0, 0, "")
	defer server.Close()

	result := runLoad(http.DefaultClient, server.URL, 4, 200*time.Millisecond)

	if result.total == 0 {
		t.Fatal("expected at least some requests to complete")
	}
	if result.busy != 0 {
		t.Errorf("busy = %d, want 0: %v", result.busy, result.busySamples)
	}
	if result.rateLimited != 0 {
		t.Errorf("rateLimited = %d, want 0: %v", result.rateLimited, result.rateLimitedSamples)
	}
}

func TestRunLoad_DetectsBusyErrors(t *testing.T) {
	server := stubContactsServer(t, 3, http.StatusInternalServerError, `{"error":{"message":"database is locked"}}`)
	defer server.Close()

	result := runLoad(http.DefaultClient, server.URL, 4, 300*time.Millisecond)

	if result.busy == 0 {
		t.Fatal("expected runLoad to detect and count the injected \"database is locked\" responses")
	}
	if len(result.busySamples) == 0 {
		t.Error("expected at least one busy sample to be recorded")
	}
	if len(result.busySamples) > maxSamples {
		t.Errorf("busySamples exceeded the cap: got %d, want <= %d", len(result.busySamples), maxSamples)
	}
}

func TestRunLoad_DetectsRateLimiting(t *testing.T) {
	server := stubContactsServer(t, 3, http.StatusTooManyRequests, `{"error":"rate limit exceeded"}`)
	defer server.Close()

	result := runLoad(http.DefaultClient, server.URL, 4, 300*time.Millisecond)

	if result.rateLimited == 0 {
		t.Fatal("expected runLoad to detect and count the injected 429 responses")
	}
}

func TestClassification_StringerForDebugging(t *testing.T) {
	// Not a real Stringer — just pins that the four classification values
	// stay distinct, so a future refactor that collapses two of them (e.g.
	// merging classifyOther into classifyOK) is caught here rather than
	// silently changing runLoad's pass/fail behavior.
	values := []classification{classifyOK, classifyBusy, classifyRateLimited, classifyOther}
	seen := map[classification]bool{}
	for _, v := range values {
		if seen[v] {
			t.Fatalf("classification value %d reused", v)
		}
		seen[v] = true
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate([]byte("short"), 10); got != "short" {
		t.Errorf("truncate(short) = %q, want unchanged", got)
	}
	long := make([]byte, 20)
	for i := range long {
		long[i] = 'a'
	}
	got := truncate(long, 5)
	if got != "aaaaa..." {
		t.Errorf("truncate(long, 5) = %q, want %q", got, "aaaaa...")
	}
}
