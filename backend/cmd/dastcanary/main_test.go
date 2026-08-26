package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The ZAP spider job starts at the canary's site root and treats a non-200
// there as a plan warning, which makes zap.sh exit 2 and fails the workflow
// step before zapgate runs. Go's ServeMux has no root pattern by default, so
// "/" used to 404 and the DAST job could never go green.
func TestIndexHandlerServesRoot(t *testing.T) {
	rr := doRequest(t, http.MethodGet, "/")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", rr.Code, http.StatusOK)
	}
	// The spider follows links; the planted endpoints must be reachable from
	// the root, not only from the requestor job's seed URLs.
	for _, want := range []string{"/reflected?q=hello", "/idor/1", "/idor/2"} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Errorf("GET / body does not link %q", want)
		}
	}
}

// "/" is ServeMux's catch-all, so the index must not swallow unknown paths
// into a 200 — that would make the spider report phantom pages.
func TestIndexHandlerNotFoundForUnknownPath(t *testing.T) {
	rr := doRequest(t, http.MethodGet, "/no-such-path")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("GET /no-such-path status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestHealthHandler(t *testing.T) {
	rr := doRequest(t, http.MethodGet, "/health")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /health status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(rr.Body.String()); got != "ok" {
		t.Fatalf("GET /health body = %q, want %q", got, "ok")
	}
}

func TestReflectedHandlerReflectsUnescaped(t *testing.T) {
	payload := `<script>alert('xss')</script>`
	rr := doRequest(t, http.MethodGet, "/reflected?q="+urlQueryEscape(payload))

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /reflected status = %d, want %d", rr.Code, http.StatusOK)
	}

	body := rr.Body.String()
	// The planted vulnerability: the raw payload appears in the HTML body
	// verbatim (this is what ZAP's reflected-XSS rule 40012 keys on). If this
	// ever becomes escaped, the canary has been "fixed" and the DAST self-test
	// will go blind — that is a regression, not an improvement.
	if !strings.Contains(body, payload) {
		t.Fatalf("GET /reflected body did not reflect the raw payload:\n%q", body)
	}
	// A sane (non-vulnerable) server would encode the angle brackets.
	if strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("GET /reflected body appears HTML-escaped — the canary must reflect raw input:\n%q", body)
	}
}

func TestReflectedHandlerDefaultsWhenMissing(t *testing.T) {
	rr := doRequest(t, http.MethodGet, "/reflected")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /reflected (no q) status = %d, want %d", rr.Code, http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), "hello") {
		t.Fatalf("GET /reflected (no q) body = %q, want default greeting", rr.Body.String())
	}
}

func TestIDORHandlerReturnsSecretForAnyID(t *testing.T) {
	rr := doRequest(t, http.MethodGet, "/idor/42")

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /idor/42 status = %d, want %d", rr.Code, http.StatusOK)
	}

	var got struct {
		Object string `json:"object"`
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("GET /idor/42 returned non-JSON: %v\n%s", err, rr.Body.String())
	}
	if got.ID != "42" {
		t.Fatalf("GET /idor/42 id = %q, want 42", got.ID)
	}
	if got.Secret != canaryIDPrefix+"42" {
		t.Fatalf("GET /idor/42 secret = %q, want %q", got.Secret, canaryIDPrefix+"42")
	}
}

func TestIDORHandlerNoAuthorizationRequired(t *testing.T) {
	// The IDOR canary must serve secrets with no auth headers at all — the
	// whole point of the planted vulnerability (an attacker requests another
	// object's id directly).
	rr := doRequest(t, http.MethodGet, "/idor/7")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /idor/7 status = %d, want %d (no auth required)", rr.Code, http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), canaryIDPrefix+"7") {
		t.Fatalf("GET /idor/7 body = %q, want the object's secret", rr.Body.String())
	}
}

func TestAddrFromEnv(t *testing.T) {
	if got := addrFromEnv(func(string) string { return "" }); got != defaultAddr {
		t.Errorf("addrFromEnv() = %q, want default %q", got, defaultAddr)
	}
	if got := addrFromEnv(func(string) string { return ":9999" }); got != ":9999" {
		t.Errorf("addrFromEnv(override) = %q, want :9999", got)
	}
}

// doRequest runs one request through the canary mux and returns the recorder.
func doRequest(t *testing.T, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	rr := httptest.NewRecorder()
	newMux().ServeHTTP(rr, req)
	return rr
}

// urlQueryEscape escapes a value for safe embedding in a query string in tests
// (kept local so the canary server itself stays dependency-free).
func urlQueryEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "<", "%3C"), ">", "%3E")
}
