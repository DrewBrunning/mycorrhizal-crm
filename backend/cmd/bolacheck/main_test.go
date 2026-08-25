package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		status int
		wantOK bool
	}{
		{http.StatusNotFound, true},
		{http.StatusForbidden, true},
		{http.StatusOK, false},
		{http.StatusCreated, false},
		{http.StatusInternalServerError, false},
		{http.StatusBadGateway, false},
		{http.StatusUnauthorized, false},
		{http.StatusBadRequest, false},
		{http.StatusMethodNotAllowed, false},
	}
	for _, c := range cases {
		verdict, ok := classify(c.status)
		if ok != c.wantOK {
			t.Errorf("classify(%d) ok = %v, want %v", c.status, ok, c.wantOK)
		}
		if c.wantOK && verdict != "" {
			t.Errorf("classify(%d) verdict = %q, want empty on ok", c.status, verdict)
		}
		if !c.wantOK && verdict == "" {
			t.Errorf("classify(%d) verdict = empty, want a reason", c.status)
		}
	}
}

func TestExtractID_NumberAndString(t *testing.T) {
	num := []byte(`{"note":{"id":42,"content":"x"}}`)
	if id, err := extractID(num, "note"); err != nil || id != "42" {
		t.Errorf("extractID(number) = %q, %v; want 42, nil", id, err)
	}

	str := []byte(`{"circle":{"id":"7c5e6a0e-1b2c-4d3e-8f4a-9b0c1d2e3f4a","name":"x"}}`)
	if id, err := extractID(str, "circle"); err != nil || id != "7c5e6a0e-1b2c-4d3e-8f4a-9b0c1d2e3f4a" {
		t.Errorf("extractID(string) = %q, %v", id, err)
	}
}

func TestExtractID_Missing(t *testing.T) {
	if _, err := extractID([]byte(`{"other":{}}`), "note"); err == nil {
		t.Error("extractID(missing key) = nil, want error")
	}
	if _, err := extractID([]byte(`{"note":{"no_id":1}}`), "note"); err == nil {
		t.Error("extractID(missing id field) = nil, want error")
	}
	if _, err := extractID([]byte(`not json`), "note"); err == nil {
		t.Error("extractID(bad json) = nil, want error")
	}
	if _, err := extractID([]byte(`{"note":{"id":""}}`), "note"); err == nil {
		t.Error("extractID(empty string id) = nil, want error")
	}
	if _, err := extractID([]byte(`{"note":{"id":null}}`), "note"); err == nil {
		t.Error("extractID(null id) = nil, want error")
	}
}

func TestLoadConfig(t *testing.T) {
	if cfg := loadConfig(func(string) string { return "" }); cfg.baseURL != "http://localhost:7300" {
		t.Errorf("default baseURL = %q", cfg.baseURL)
	}
	if cfg := loadConfig(func(k string) string {
		if k == "BOLACHECK_BASE_URL" {
			return "http://example.com/"
		}
		return ""
	}); cfg.baseURL != "http://example.com" {
		t.Errorf("baseURL = %q, want trailing slash trimmed", cfg.baseURL)
	}
}

// stubCRMServer fakes just enough of the app's HTTP surface for run() to
// exercise its full register -> create -> cross-account-access cycle. Creates
// return a wrapped `{<key>: {id: <n>}}` (plus `uid` for contacts); every
// access returns the configured accessStatus (404 by default). leakCollection,
// when non-empty, makes GET on that collection return 200 instead — simulating
// a handler that forgot its user_id scope.
func stubCRMServer(t *testing.T, accessStatus int, leakCollection string) *httptest.Server {
	t.Helper()
	var nextID int64

	createKeys := map[string]string{
		"/api/v1/notes":               "note",
		"/api/v1/activities":          "activity",
		"/api/v1/circles":             "circle",
		"/api/v1/households":          "household",
		"/api/v1/tags":                "tag",
		"/api/v1/life-events":         "life_event",
		"/api/v1/gifts":               "gift",
		"/api/v1/preferences":         "preference",
		"/api/v1/cadence-policies":    "cadence_policy",
		"/api/v1/conversation-agenda": "conversation_agenda",
		"/api/v1/relationship-edges":  "relationship_edge",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/register", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	})
	mux.HandleFunc("/api/v1/login", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	})
	mux.HandleFunc("/api/v1/contacts", func(w http.ResponseWriter, r *http.Request) {
		id := atomic.AddInt64(&nextID, 1)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"contact": map[string]any{"id": id, "uid": fmt.Sprintf("uid-%d", id)},
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Reminder create is POST /api/v1/contacts/{id}/reminders.
		if r.Method == http.MethodPost && len(r.URL.Path) > len("/api/v1/contacts/") && hasSuffix(r.URL.Path, "/reminders") {
			id := atomic.AddInt64(&nextID, 1)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"reminder": map[string]any{"id": id}})
			return
		}
		if r.Method == http.MethodPost {
			if key, ok := createKeys[r.URL.Path]; ok {
				id := atomic.AddInt64(&nextID, 1)
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]any{key: map[string]any{"id": id}})
				return
			}
		}
		// Access probes (GET/PUT/DELETE on any resource path). A leak simulates
		// a missing scope on that collection's GET.
		if leakCollection != "" && r.Method == http.MethodGet && hasPrefix(r.URL.Path, "/api/v1/"+leakCollection+"/") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"leaked":true}`))
			return
		}
		w.WriteHeader(accessStatus)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})
	return httptest.NewServer(mux)
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func bolaEnv(baseURL string) func(string) string {
	return func(k string) string {
		if k == "BOLACHECK_BASE_URL" {
			return baseURL
		}
		return ""
	}
}

func TestRun_AllScopedPasses(t *testing.T) {
	server := stubCRMServer(t, http.StatusNotFound, "")
	defer server.Close()

	if err := run(bolaEnv(server.URL)); err != nil {
		t.Fatalf("run() = %v, want nil (every cross-account access 404)", err)
	}
}

func TestRun_DetectsIDORLeak(t *testing.T) {
	server := stubCRMServer(t, http.StatusNotFound, "notes")
	defer server.Close()

	err := run(bolaEnv(server.URL))
	if err == nil {
		t.Fatal("run() = nil, want error for the leaked notes collection")
	}
	if !strings.Contains(err.Error(), "BOLA/IDOR") {
		t.Errorf("error %q does not mention BOLA/IDOR", err.Error())
	}
	if !strings.Contains(err.Error(), "note") {
		t.Errorf("error %q does not identify the leaked entity", err.Error())
	}
}

func TestRun_DetectsServerCrash(t *testing.T) {
	server := stubCRMServer(t, http.StatusInternalServerError, "")
	defer server.Close()

	err := run(bolaEnv(server.URL))
	if err == nil {
		t.Fatal("run() = nil, want error for 500 cross-account accesses")
	}
	if !strings.Contains(err.Error(), "crashed") {
		t.Errorf("error %q does not mention a crash", err.Error())
	}
}

func TestRun_RegisterFailureFails(t *testing.T) {
	// A server that rejects registration must surface as a setup error, not a
	// silent pass with zero checks.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/register", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	if err := run(bolaEnv(server.URL)); err == nil {
		t.Fatal("run() = nil, want registration failure to fail the run")
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
	if got := truncate(long, 5); got != "aaaaa..." {
		t.Errorf("truncate(long, 5) = %q, want %q", got, "aaaaa...")
	}
}

func TestBuildProbes_CoversExpectedEntities(t *testing.T) {
	probes := buildProbes(contactRef{id: 1, uid: "uid-1"})
	if len(probes) != 13 {
		t.Fatalf("buildProbes = %d probes, want 13", len(probes))
	}
	names := map[string]bool{}
	for _, p := range probes {
		if p.create == nil || len(p.accesses) != 3 {
			t.Errorf("probe %q: create=%v accesses=%d, want create set + 3 accesses", p.name, p.create == nil, len(p.accesses))
		}
		names[p.name] = true
	}
	for _, want := range []string{"contact", "note", "activity", "circle", "household", "tag",
		"life event", "gift", "preference", "cadence policy", "conversation agenda",
		"relationship edge", "reminder"} {
		if !names[want] {
			t.Errorf("buildProbes missing entity %q", want)
		}
	}
}
