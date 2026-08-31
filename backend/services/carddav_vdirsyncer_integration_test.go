package services

import (
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/semanticequal"
	"mycorrhizal/vcard4"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Real-client CardDAV integration suite (issue #681, TEST-09 direction 1):
// a REAL third-party client consumes OUR CardDAV server. vdirsyncer is a
// mature, widely-deployed CardDAV/CalDAV sync client with its own protocol
// implementation and its own vCard parser (vobject) — the "code and tests
// consistently, confidently wrong" shape #496 warns about is exactly a server
// that only ever sees our own client and our own test doubles. Here the
// client is the oracle: if our server's discovery, ETag handling, version
// negotiation or vCard output is wrong in a way vdirsyncer's parser cares
// about, this suite fails.
//
// The fixture (TEST-02, issue #430) is staged on OUR server and pulled by
// vdirsyncer; the client's parsed view (the bytes vdirsyncer stored after
// parsing what we served) must be semantically equal (TEST-03, #431) to the
// source. Create/update/delete push through vdirsyncer and must land.
//
// Gating: the test SKIPS unless MYCORRHIZAL_OUR_CARDDAV_URL is set. The
// reference-clients-e2e.yml workflow starts our real server, pip-installs the
// hash-pinned vdirsyncer, and runs this test. Scheduled (nightly) + manual +
// path-gated, not per-PR.
// ---------------------------------------------------------------------------

// vdirsyncerEnv resolves the connection info and the vdirsyncer binary.
func vdirsyncerEnv(t *testing.T) (baseURL, cmd string) {
	t.Helper()
	baseURL = strings.TrimRight(os.Getenv("MYCORRHIZAL_OUR_CARDDAV_URL"), "/")
	if baseURL == "" {
		t.Skip("MYCORRHIZAL_OUR_CARDDAV_URL not set — this suite runs a real vdirsyncer against our server via .github/workflows/reference-clients-e2e.yml")
	}
	cmd = os.Getenv("MYCORRHIZAL_VDIRSYNCER_CMD")
	if cmd == "" {
		cmd = "vdirsyncer"
	}
	if _, err := exec.LookPath(cmd); err != nil {
		t.Skipf("vdirsyncer (%s) not installed — this suite runs against the real client via .github/workflows/reference-clients-e2e.yml", cmd)
	}
	return baseURL, cmd
}

// vdirsyncerRegisterUser provisions a throwaway user on OUR server via the
// public registration endpoint (the same path a self-host operator or e2e
// harness uses). The user's CardDAV credentials are username + password.
func vdirsyncerRegisterUser(t *testing.T, baseURL, username, password string) {
	t.Helper()
	body := fmt.Sprintf(`{"username":%q,"email":%q,"password":%q}`,
		username, username+"@example.com", password)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/register", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "registration must reach our server")
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	require.Contains(t, []int{http.StatusOK, http.StatusCreated}, resp.StatusCode,
		"registration failed: %s", respBody)
}

// vdirsyncerConfig writes a minimal vdirsyncer config syncing the server
// address book into a local filesystem storage, and returns the config path,
// status dir and local storage dir.
func vdirsyncerConfig(t *testing.T, baseURL, username, password string) (configPath, statusDir, localDir string) {
	t.Helper()
	root := t.TempDir()
	statusDir = filepath.Join(root, "status")
	localDir = filepath.Join(root, "local")
	configPath = filepath.Join(root, "config")
	require.NoError(t, os.MkdirAll(statusDir, 0o755))
	require.NoError(t, os.MkdirAll(localDir, 0o755))

	collectionURL := baseURL + "/carddav/addressbooks/" + username + "/contacts/"
	cfg := fmt.Sprintf(`[general]
status_path = %q

[pair our_server]
a = "remote"
b = "local"
collections = ["contacts"]
conflict_resolution = "a wins"

[storage remote]
type = "carddav"
url = %q
username = %q
password = %q

[storage local]
type = "filesystem"
path = %q
fileext = ".vcf"
`, statusDir, collectionURL, username, password, localDir)
	require.NoError(t, os.WriteFile(configPath, []byte(cfg), 0o600))
	return configPath, statusDir, localDir
}

// vdirsyncerRun executes a vdirsyncer subcommand, answering collection-
// creation prompts with yes. Output is returned for assertions.
func vdirsyncerRun(t *testing.T, cmd, configPath string, args ...string) string {
	t.Helper()
	fullArgs := append([]string{"-c", configPath}, args...)
	c := exec.Command(cmd, fullArgs...)
	c.Stdin = strings.NewReader("y\n")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("vdirsyncer %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

// vdirsyncerLocalCard returns the local .vcf bytes vdirsyncer stored for uid.
func vdirsyncerLocalCard(t *testing.T, localDir, uid string) string {
	t.Helper()
	path := filepath.Join(localDir, "contacts", uid+".vcf")
	b, err := os.ReadFile(path)
	require.NoError(t, err, "vdirsyncer must have stored %s (the client's parsed view)", uid)
	return string(b)
}

// TestCardDAVVdirsyncer_ClientRoundTrip is the load-bearing real-client test:
// the TEST-02 fixture staged on OUR server is pulled by a REAL vdirsyncer and
// must come back semantically equal (the client's view), and create/update/
// delete pushed by vdirsyncer must round-trip to our server.
func TestCardDAVVdirsyncer_ClientRoundTrip(t *testing.T) {
	baseURL, cmd := vdirsyncerEnv(t)

	// A fresh throwaway user keeps runs independent of server state.
	username := "vd" + randomSuffix(t)
	password := "VdirUserPass123!"
	vdirsyncerRegisterUser(t, baseURL, username, password)
	configPath, statusDir, localDir := vdirsyncerConfig(t, baseURL, username, password)
	_ = statusDir
	collectionURL := baseURL + "/carddav/addressbooks/" + username + "/contacts/"

	// --- stage the TEST-02 fixture on OUR server -----------------------------
	m, err := canonicalfixture.Read()
	require.NoError(t, err)

	type fixtureEntry struct {
		name string
		rec  *contactmodel.Record
		uid  string
	}
	var entries []fixtureEntry
	for _, entry := range m.Contacts {
		if entry.SoftDeleted {
			continue
		}
		rec := entry.Record()
		if entry.RecreatesVCardUIDOf != "" {
			for _, other := range m.Contacts {
				if other.Name == entry.RecreatesVCardUIDOf {
					rec.Card.UID = other.Card.UID
				}
			}
		}
		raw, _, exportErr := vcard4.Adapter{}.Export(rec)
		require.NoError(t, exportErr, "export %s", entry.Name)

		href := collectionURL + rec.Card.UID + ".vcf"
		if refused, status, body := referenceRejectsVCard(t, username, password, href, string(raw)); refused {
			t.Fatalf("OUR OWN server refused to store fixture contact %s (%d: %s) — that is a server bug, not a divergence to pin",
				entry.Name, status, body)
		}
		referencePut(t, username, password, href, string(raw))
		entries = append(entries, fixtureEntry{name: entry.Name, rec: rec, uid: rec.Card.UID})
	}
	require.NotEmpty(t, entries, "fixture must yield live contacts")

	// --- vdirsyncer discovers + pulls (the client's view) ---------------------
	vdirsyncerRun(t, cmd, configPath, "discover", "our_server")
	vdirsyncerRun(t, cmd, configPath, "sync")

	// Every staged contact must come back through the real client's parser
	// semantically equal — OUR server round-trips its own exports, so any
	// divergence here is a real server bug that a real client would surface.
	for _, entry := range entries {
		entry := entry
		t.Run(entry.name, func(t *testing.T) {
			local := vdirsyncerLocalCard(t, localDir, entry.uid)
			pulled, diags, importErr := vcard4.Adapter{}.Import([]byte(local))
			require.NoError(t, importErr, "the client's stored card must parse")
			for _, d := range diags {
				t.Logf("import diag: %s: %s", d.Severity, d.Message)
			}
			report := semanticequal.Compare(entry.rec, pulled)
			if len(report.Differences) > 0 {
				t.Errorf("fixture contact %q did not survive the round trip through a REAL client (vdirsyncer):\n%s", entry.name, report.DiffText())
			}
		})
	}

	// --- quiescence: a real client with stable ETags makes no second pass -----
	quiet := vdirsyncerRun(t, cmd, configPath, "sync")
	assert.NotContains(t, quiet, "Copying", "an unchanged server must not make vdirsyncer re-fetch (stable ETags)")

	// --- push path: vdirsyncer creates a card on OUR server -------------------
	createUID := "vdirsyncer-create-1"
	createCard := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:" + createUID + "\r\nFN:Created By Client\r\nN:Client;Created;;;\r\nEMAIL:created@client.example\r\nEND:VCARD\r\n"
	require.NoError(t, os.WriteFile(filepath.Join(localDir, "contacts", createUID+".vcf"), []byte(createCard), 0o644))
	vdirsyncerRun(t, cmd, configPath, "sync")
	require.Contains(t, mustDavGet(t, baseURL, username, password, createUID), "Created By Client",
		"a card created by the real client must land on our server")

	// --- push path: vdirsyncer updates a card on OUR server --------------------
	updateUID := entries[0].uid
	updatePath := filepath.Join(localDir, "contacts", updateUID+".vcf")
	orig := mustReadFile(t, updatePath)
	updated := strings.Replace(orig, entries[0].rec.Card.Emails[0].Address, "updated-by-client@example.com", 1)
	require.NotEqual(t, orig, updated, "the replacement must change the card")
	require.NoError(t, os.WriteFile(updatePath, []byte(updated), 0o644))
	vdirsyncerRun(t, cmd, configPath, "sync")
	require.Contains(t, mustDavGet(t, baseURL, username, password, updateUID), "updated-by-client@example.com",
		"a card edit made by the real client must land on our server")

	// --- push path: vdirsyncer deletes a card on OUR server --------------------
	deleteUID := entries[1].uid
	require.NoError(t, os.Remove(filepath.Join(localDir, "contacts", deleteUID+".vcf")))
	vdirsyncerRun(t, cmd, configPath, "sync")
	resp := davRequest(t, username, password, http.MethodGet, baseURL+"/carddav/addressbooks/"+username+"/contacts/"+deleteUID+".vcf")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"a card deleted by the real client must disappear from our server (soft-delete is invisible to clients)")
}

// --- helpers ---------------------------------------------------------------

func randomSuffix(t *testing.T) string {
	t.Helper()
	var id [8]byte
	_, err := rand.Read(id[:])
	require.NoError(t, err)
	return fmt.Sprintf("%x", id[:])
}

// mustDavGet fetches a card from OUR server's CardDAV endpoint.
func mustDavGet(t *testing.T, baseURL, user, password, uid string) string {
	t.Helper()
	resp := davRequest(t, user, password, http.MethodGet, baseURL+"/carddav/addressbooks/"+user+"/contacts/"+uid+".vcf")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "GET %s failed: %s", uid, resp.Status)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(b)
}

// davRequest does an authenticated request against OUR server's DAV surface.
func davRequest(t *testing.T, user, password, method, rawURL string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, rawURL, nil)
	require.NoError(t, err)
	req.SetBasicAuth(user, password)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "%s %s must reach our server", method, rawURL)
	return resp
}

// mustReadFile reads a file, failing the test on error.
func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}
