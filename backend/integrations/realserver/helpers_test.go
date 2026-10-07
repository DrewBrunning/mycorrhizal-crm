package realserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/citest"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Environment variables the suite reads. Each names one real server; absent in
// a developer environment means "skip", absent in CI (MYCORRHIZAL_REQUIRE_REFERENCES
// set) means "fail". Keep in step with integration-real-servers.yml and
// stack/*.compose.yml.
const (
	envNtfyURL    = "MYCORRHIZAL_RS_NTFY_URL"
	envGotifyURL  = "MYCORRHIZAL_RS_GOTIFY_URL"
	envGotifyUser = "MYCORRHIZAL_RS_GOTIFY_ADMIN_USER"
	envGotifyPass = "MYCORRHIZAL_RS_GOTIFY_ADMIN_PASSWORD"

	envOIDCIssuer = "MYCORRHIZAL_RS_OIDC_ISSUER"
)

// serverURL returns the base URL of the named real server (trailing slash
// trimmed), or skips/fails per citest.SkipOrRequire when it is not provisioned.
func serverURL(t *testing.T, env string) string {
	t.Helper()
	v := strings.TrimRight(os.Getenv(env), "/")
	if v == "" {
		citest.SkipOrRequire(t, env+" is not set (real server not provisioned; see docs/development/testing.md)")
	}
	return v
}

// requireEnv is serverURL for non-URL settings (credentials).
func requireEnv(t *testing.T, env string) string {
	t.Helper()
	v := os.Getenv(env)
	if v == "" {
		citest.SkipOrRequire(t, env+" is not set")
	}
	return v
}

// waitReady polls url until it answers 2xx, so a test does not race a server
// that is still booting between "port open" and "serving".
func waitReady(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:gosec,noctx // test-only poll of a fixed local server
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode/100 == 2 {
				return
			}
			last = resp.Status
		} else {
			last = err.Error()
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("server at %s not ready after %s (last: %s)", url, timeout, last)
}

// getJSON GETs url (optionally with basic auth) and decodes the JSON body.
func getJSON(t *testing.T, url, user, pass string, out any) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		t.Fatalf("GET %s: status %d: %s", url, resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("GET %s: decoding %q: %v", url, body, err)
	}
}

// testCfg is the minimum config the services under test read. JWTSecretKey keys
// the at-rest credential encryption (Gotify token); WebhookBlockPrivateURLs is
// deliberately off — the servers are on loopback.
func testCfg() config.Config {
	return config.Config{JWTSecretKey: "realserver-contract-test-secret-0123456789"}
}

// newUser creates a user in a fresh real-migrated database.
func newUser(t *testing.T) (*gorm.DB, models.User) {
	t.Helper()
	db := dbtest.New(t)
	u := models.User{Username: "contract", Password: "password123", Email: fmt.Sprintf("contract-%d@example.com", time.Now().UnixNano())}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return db, u
}
