package main

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
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
		"LOADSMOKE_USERS":        "7",
		"LOADSMOKE_DB_PATH":      "/data/mycorrhizal.db",
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
	if cfg.users != 7 {
		t.Errorf("users = %d, want 7", cfg.users)
	}
	if cfg.dbPath != "/data/mycorrhizal.db" {
		t.Errorf("dbPath = %q, want /data/mycorrhizal.db", cfg.dbPath)
	}
}

func TestLoadConfigFromEnv_InvalidOverridesFallBackToDefault(t *testing.T) {
	env := map[string]string{
		"LOADSMOKE_WORKERS":      "not-a-number",
		"LOADSMOKE_DURATION":     "not-a-duration",
		"LOADSMOKE_MIN_REQUESTS": "-5",
		"LOADSMOKE_USERS":        "0",
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
	if cfg.users != defaultLoadConfig.users {
		t.Errorf("non-positive LOADSMOKE_USERS must fall back to default (1), got %d", cfg.users)
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

// stubAppServer is a self-contained fake of the endpoints loadsmoke touches:
// /register, /login, and the create/update/delete contact cycle. Used to
// drive run() end to end.
func stubAppServer(t *testing.T) *httptest.Server {
	t.Helper()
	var nextID int64
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/register", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"message":"created"}`))
	})
	mux.HandleFunc("/api/v1/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "auth_token", Value: "stub"})
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"ok"}`))
	})
	mux.HandleFunc("/api/v1/contacts", func(w http.ResponseWriter, _ *http.Request) {
		id := atomic.AddInt64(&nextID, 1)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"contact": map[string]any{"id": id}})
	})
	mux.HandleFunc("/api/v1/contacts/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"ok"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRunLoad_AllSucceed(t *testing.T) {
	server := stubContactsServer(t, 0, 0, "")
	defer server.Close()

	result := runLoad([]*http.Client{http.DefaultClient}, server.URL, 4, 200*time.Millisecond)

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

	result := runLoad([]*http.Client{http.DefaultClient}, server.URL, 4, 300*time.Millisecond)

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

	result := runLoad([]*http.Client{http.DefaultClient}, server.URL, 4, 300*time.Millisecond)

	if result.rateLimited == 0 {
		t.Fatal("expected runLoad to detect and count the injected 429 responses")
	}
}

// TestRunLoad_SpreadsWorkersAcrossMultipleClients exercises the issue #498
// many-users path: several independent sessions, workers bound round-robin.
func TestRunLoad_SpreadsWorkersAcrossMultipleClients(t *testing.T) {
	server := stubContactsServer(t, 0, 0, "")
	defer server.Close()

	clients := make([]*http.Client, 3)
	for i := range clients {
		jar, err := cookiejar.New(nil)
		if err != nil {
			t.Fatalf("cookiejar: %v", err)
		}
		clients[i] = &http.Client{Jar: jar, Timeout: 5 * time.Second}
	}

	result := runLoad(clients, server.URL, 6, 200*time.Millisecond)
	if result.total == 0 {
		t.Fatal("expected requests to complete across the multi-client fan-out")
	}
	if result.busy != 0 || result.rateLimited != 0 {
		t.Errorf("unexpected busy=%d rateLimited=%d", result.busy, result.rateLimited)
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

// TestRegisterSessions_ErrorsWhenAuthUnavailable exercises the multi-user
// setup path: stubContactsServer implements /api/v1/contacts but not
// /register or /login, so registration fails and the error is surfaced. Also
// covers the n < 1 clamp (0 users still tries once).
func TestRegisterSessions_ErrorsWhenAuthUnavailable(t *testing.T) {
	server := stubContactsServer(t, 0, 0, "")
	defer server.Close()

	if _, err := registerSessions(server.URL, 0); err == nil {
		t.Fatal("registerSessions with no auth endpoints should error")
	}
	if _, err := registerSessions(server.URL, 3); err == nil {
		t.Fatal("registerSessions(3) with no auth endpoints should error")
	}
}

func TestVerifyNoCorruption(t *testing.T) {
	// Empty path is a no-op (the docker e2e job checks integrity its own way).
	if err := verifyNoCorruption(""); err != nil {
		t.Fatalf("verifyNoCorruption(\"\") = %v, want nil", err)
	}

	// A missing file is an error, not a silent pass.
	if err := verifyNoCorruption(filepath.Join(t.TempDir(), "nope.db")); err == nil {
		t.Fatal("verifyNoCorruption on a missing file should error")
	}

	// A real migrated database passes.
	path := filepath.Join(t.TempDir(), "app.db")
	dbtest.NewAt(t, path)
	if err := verifyNoCorruption(path); err != nil {
		t.Fatalf("verifyNoCorruption on a healthy database = %v, want nil", err)
	}
}

func TestRun_ConfigErrorsSurface(t *testing.T) {
	// run() reads real env; point it at a dead address so registerSessions
	// fails fast and run returns that error (covers the early-return path).
	t.Setenv("LOADSMOKE_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("LOADSMOKE_USERS", "1")
	t.Setenv("LOADSMOKE_DURATION", "1s")
	if err := run(); err == nil {
		t.Fatal("run() against a dead address should return an error")
	} else if !strings.Contains(err.Error(), "register/login") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRun_HappyPathIncludingIntegrityCheck drives run() end to end against a
// stub server, with LOADSMOKE_DB_PATH pointed at a real migrated database, so
// the post-load PRAGMA integrity_check (issue #498) runs and passes.
func TestRun_HappyPathIncludingIntegrityCheck(t *testing.T) {
	srv := stubAppServer(t)
	dbPath := filepath.Join(t.TempDir(), "app.db")
	dbtest.NewAt(t, dbPath)

	t.Setenv("LOADSMOKE_BASE_URL", srv.URL)
	t.Setenv("LOADSMOKE_USERS", "3")
	t.Setenv("LOADSMOKE_WORKERS", "6")
	t.Setenv("LOADSMOKE_DURATION", "300ms")
	t.Setenv("LOADSMOKE_MIN_REQUESTS", "1")
	t.Setenv("LOADSMOKE_DB_PATH", dbPath)

	if err := run(); err != nil {
		t.Fatalf("run() against a healthy stub = %v, want nil", err)
	}
}
