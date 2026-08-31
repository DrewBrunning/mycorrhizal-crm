package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/semanticequal"
	"mycorrhizal/models"
	"mycorrhizal/vcard4"

	"github.com/emersion/go-webdav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Real-server CardDAV integration suite (issue #496 item 3): the same full
// sync lifecycle + TEST-02 fixture round trip the fake suite drives, but
// against a REAL CardDAV server (Radicale in Docker). A fake encodes our own
// understanding of the protocol — this suite exists because only a real
// server can contradict us.
//
// Gating: the test SKIPS unless MYCORRHIZAL_RADICALE_URL is set, so it never
// runs as part of the normal Go suite. The carddav-e2e.yml workflow starts
// Radicale, sets the env, and runs `go test -run TestCardDAVRadicale`. It is
// scheduled (nightly) and path-gated via .github/filters.yaml (carddav
// filter), not run per-PR — the fake suite stays the fast deterministic gate
// (see the workflow's comments for the rationale).
// ---------------------------------------------------------------------------

// radicaleEnv reads the Radicale connection info from the environment.
func radicaleEnv(t *testing.T) (baseURL, user, password string) {
	t.Helper()
	baseURL = strings.TrimRight(os.Getenv("MYCORRHIZAL_RADICALE_URL"), "/")
	if baseURL == "" {
		t.Skip("MYCORRHIZAL_RADICALE_URL not set — this suite runs against a real Radicale via .github/workflows/carddav-e2e.yml")
	}
	user = os.Getenv("MYCORRHIZAL_RADICALE_USER")
	if user == "" {
		user = "syncuser"
	}
	password = os.Getenv("MYCORRHIZAL_RADICALE_PASSWORD")
	if password == "" {
		password = "syncsecret"
	}
	return baseURL, user, password
}

// radicaleCollectionURL is the address book path for the sync user.
func radicaleCollectionURL(baseURL, user string) string {
	return baseURL + "/" + url.PathEscape(user) + "/contacts/"
}

// radicaleCardPath is the href Radicale reports for a card in its multistatus
// responses (absolute URL paths, not full URLs) — the value the sync service
// stores as ContactSyncLink.Href.
func radicaleCardPath(user, uid string) string {
	return "/" + url.PathEscape(user) + "/contacts/" + uid + ".vcf"
}

// radicaleRequest does an authenticated HTTP request against Radicale,
// returning the response (caller closes the body). rawURL is a full URL.
func radicaleRequest(t *testing.T, user, password, method, rawURL, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, rawURL, strings.NewReader(body))
	require.NoError(t, err)
	switch method {
	case http.MethodPut:
		req.Header.Set("Content-Type", "text/vcard; charset=utf-8")
	case "MKCOL":
		req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	}
	req.SetBasicAuth(user, password)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "%s %s must reach the server", method, rawURL)
	return resp
}

// radicalePut stages one vCard on Radicale and returns the server's ETag
// (unquoted), so the test can assert it landed in the real etag column.
// fullURL is the complete resource URL.
func radicalePut(t *testing.T, user, password, fullURL, raw string) string {
	t.Helper()
	resp := radicaleRequest(t, user, password, http.MethodPut, fullURL, raw)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, []int{http.StatusCreated, http.StatusNoContent}, resp.StatusCode,
		"PUT %s failed: %s (%s)", fullURL, resp.Status, body)
	return strings.Trim(resp.Header.Get("ETag"), `"`)
}

// radicaleEnsureCollection idempotently creates the address book collection
// (a fresh Radicale container has neither, and Radicale does not discover it
// for us). The user's principal is created implicitly by Radicale on first
// access and is deliberately NOT mkcol'd — the principal resource itself is
// protected (403), which is correct.
func radicaleEnsureCollection(t *testing.T, user, password, collectionPath string) {
	t.Helper()
	mkcol := `<?xml version="1.0" encoding="utf-8" ?>
<D:mkcol xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:carddav">
  <D:set><D:prop>
    <D:resourcetype><D:collection/><C:addressbook/></D:resourcetype>
  </D:prop></D:set>
</D:mkcol>`
	resp := radicaleRequest(t, user, password, "MKCOL", collectionPath, mkcol)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Contains(t, []int{http.StatusCreated, http.StatusMethodNotAllowed, http.StatusConflict}, resp.StatusCode,
		"MKCOL %s failed: %s (%s)", collectionPath, resp.Status, body)
}

// radicaleDeleteAll empties the collection so each run starts clean regardless
// of what a previous (possibly failed) run left behind. It lists via PROPFIND
// (ReadDir) rather than addressbook-query, which Radicale rejects for
// filter-less queries.
func radicaleDeleteAll(t *testing.T, baseURL, collectionPath, user, password string) {
	t.Helper()
	parsed, err := url.Parse(collectionPath)
	require.NoError(t, err)
	client, err := webdav.NewClient(webdav.HTTPClientWithBasicAuth(http.DefaultClient, user, password), collectionPath)
	require.NoError(t, err)
	infos, err := client.ReadDir(context.Background(), parsed.Path, false)
	if err != nil {
		t.Logf("radicale: initial listing of an empty/fresh collection (expected to error): %v", err)
		return
	}
	for _, info := range infos {
		// ReadDir includes the collection itself; deleting that would take the
		// address book down mid-test.
		if info.Path == parsed.Path || info.IsDir {
			continue
		}
		resp := radicaleRequest(t, user, password, http.MethodDelete, baseURL+info.Path, "")
		resp.Body.Close()
		assert.Contains(t, []int{http.StatusOK, http.StatusNoContent, http.StatusNotFound}, resp.StatusCode,
			"DELETE %s failed: %s", info.Path, resp.Status)
	}
}

// radicaleRejectsVCard reports whether Radicale refused to store raw at fullURL
// and, if so, returns the HTTP status and response body.
func radicaleRejectsVCard(t *testing.T, user, password, fullURL, raw string) (bool, int, string) {
	t.Helper()
	resp := radicaleRequest(t, user, password, http.MethodPut, fullURL, raw)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusNoContent {
		return false, resp.StatusCode, ""
	}
	return true, resp.StatusCode, string(body)
}

// sortedDifferenceConcepts returns the divergent concept_ids of a report in
// stable order (for ElementsMatch-style divergence-register pins).
func sortedDifferenceConcepts(report semanticequal.Report) []string {
	out := make([]string, 0, len(report.Differences))
	for _, d := range report.Differences {
		out = append(out, d.Concept)
	}
	sort.Strings(out)
	return out
}

// TestCardDAVRadicale_RoundTrip is the load-bearing real-server test: the
// TEST-02 fixture round-trips through a REAL CardDAV server and is compared
// semantically (issue #496 items 3 + 5), and the incremental lifecycle
// (unchanged = no re-fetch, remote update refetches, remote delete archives)
// is asserted against the server's real ETags and sync-tokens.
func TestCardDAVRadicale_RoundTrip(t *testing.T) {
	baseURL, user, password := radicaleEnv(t)
	collectionPath := radicaleCollectionURL(baseURL, user)
	radicaleEnsureCollection(t, user, password, collectionPath)
	radicaleDeleteAll(t, baseURL, collectionPath, user, password)

	db, cfg, syncUser := setupCardDAVLifecycle(t)

	// The divergence register (issue #496 item 5): fixture contacts a REAL
	// Radicale server does not round-trip faithfully, and why. The suite
	// asserts reality matches this register exactly, so a change on either
	// side — our exporter or Radicale — flips a pin into a failure that names
	// the change instead of silently carrying a stale expectation:
	//
	//   - celine is REJECTED at PUT: its exported vCard carries two N
	//     properties (RFC 9554 §3.3 alternative-name ALTID — legal, and our
	//     exporter emits it), and Radicale's vobject refuses >1 N.
	//   - bob is ACCEPTED but comes back silently altered: vobject
	//     re-serializes the card on output, truncating the inline data: PHOTO
	//     at the first ';' and dropping the ADR components beyond the seven
	//     RFC 6350 slots (apartment, floor).
	//
	// These are exactly the "a real implementation's escaping and folding will
	// disagree with ours" divergences the issue exists to surface — the fake
	// (byte-verbatim) suite cannot see them.
	type radicaleDivergence struct {
		name     string
		concepts string // ";"-joined expected divergent concepts; "" = rejected at PUT
		reason   string
	}
	divergenceRegister := []radicaleDivergence{
		{name: "celine", reason: "rejected by Radicale: exported card carries two N properties (RFC 9554 §3.3 alternative-name ALTID); vobject refuses >1 N"},
		{name: "bob", concepts: "adr;photo", reason: "vobject re-serializes on output: inline data: PHOTO truncated at the first ';' and ADR components beyond the seven RFC 6350 slots (apartment, floor) dropped"},
	}

	// --- stage the TEST-02 fixture on the real server ------------------------
	m, err := canonicalfixture.Read()
	require.NoError(t, err)

	type fixtureEntry struct {
		name string
		rec  *contactmodel.Record
		uid  string
	}
	var entries []fixtureEntry
	var rejected []string
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

		href := collectionPath + rec.Card.UID + ".vcf"
		if refused, status, body := radicaleRejectsVCard(t, user, password, href, string(raw)); refused {
			t.Logf("radicale refused to store %s (%d: %s)", entry.Name, status, body)
			rejected = append(rejected, entry.Name)
			continue
		}
		etag := radicalePut(t, user, password, href, string(raw))
		require.NotEmpty(t, etag, "Radicale must return an ETag on PUT")
		entries = append(entries, fixtureEntry{name: entry.Name, rec: rec, uid: rec.Card.UID})
	}
	require.NotEmpty(t, entries, "fixture must yield live contacts")

	// Pin the REJECTED half of the register.
	var expectedRejected []string
	for _, d := range divergenceRegister {
		if d.concepts == "" {
			expectedRejected = append(expectedRejected, d.name)
		}
	}
	assert.Equal(t, expectedRejected, rejected,
		"the set of fixture cards a real CardDAV server refuses to store changed — investigate and update the divergence register")

	// --- initial pull ---------------------------------------------------------
	sub := newContactTestSubscription(t, db, cfg, syncUser.ID, collectionPath, user, password)
	service := NewContactSyncService(false)
	stats, err := service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err, "initial pull from Radicale")
	require.Equal(t, len(entries), stats.Created,
		"every staged fixture contact must be pulled from the real server (created=%d)", stats.Created)

	// Every card pulled in must be semantically equal to what was staged —
	// the re-serialize/re-parse cycle through a real implementation must not
	// lose anything (issue #496 item 5) — EXCEPT the documented divergences,
	// which are pinned to be exactly what the register says they are.
	registerByContact := make(map[string]radicaleDivergence, len(divergenceRegister))
	for _, d := range divergenceRegister {
		registerByContact[d.name] = d
	}
	for _, entry := range entries {
		entry := entry
		t.Run(entry.name, func(t *testing.T) {
			var link models.ContactSyncLink
			require.NoError(t, db.Where("subscription_id = ? AND href = ?", sub.ID, radicaleCardPath(user, entry.uid)).First(&link).Error)
			var contact models.Contact
			require.NoError(t, db.First(&contact, link.ContactID).Error)

			pulled := models.RecordForContact(&contact, cfg.ProfilePhotoDir, db)
			report := semanticequal.Compare(entry.rec, pulled)
			divergent := sortedDifferenceConcepts(report)

			reg, documented := registerByContact[entry.name]
			switch {
			case documented:
				require.NotEmpty(t, divergent,
					"fixture contact %q is in the divergence register (%s) but now round-trips CLEANLY — the divergence is fixed; update the register",
					entry.name, reg.reason)
				assert.ElementsMatch(t, strings.Split(reg.concepts, ";"), divergent,
					"fixture contact %q diverges in different concepts than the register documents (%s):\n%s",
					entry.name, reg.reason, report.DiffText())
			case len(divergent) > 0:
				t.Errorf("fixture contact %q did not survive the round trip through a REAL CardDAV server (not in the divergence register):\n%s",
					entry.name, report.DiffText())
			}

			require.NotEmpty(t, link.ETag, "the link must carry the server's real ETag")
		})
	}

	// --- unchanged server -> no re-fetch, no reconcile ------------------------
	stats, err = service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)
	assert.Equal(t, ContactSyncStats{}, stats, "an unchanged real server must reconcile nothing")

	// --- remote update refetches and updates ----------------------------------
	target := entries[0]
	newEmail := "updated." + target.uid + "@example.com"
	newETag := radicalePut(t, user, password, collectionPath+target.uid+".vcf",
		fmt.Sprintf(carddavTestVCard, target.uid, "Updated", "Person", "Person", "Updated", newEmail))

	stats, err = service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err, "incremental update pull from Radicale")
	assert.Equal(t, ContactSyncStats{Updated: 1}, stats,
		"the remote update must be reconciled exactly once (created=%d updated=%d archived=%d skipped=%d)",
		stats.Created, stats.Updated, stats.Archived, stats.Skipped)

	var updated models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", syncUser.ID, target.uid).First(&updated).Error)
	assert.Equal(t, newEmail, updated.Email, "the remote edit must land")

	var updatedLink models.ContactSyncLink
	require.NoError(t, db.Where("subscription_id = ? AND href = ?", sub.ID, radicaleCardPath(user, target.uid)).First(&updatedLink).Error)
	assert.Equal(t, newETag, updatedLink.ETag, "the etag column must track Radicale's new ETag")

	// --- remote delete archives ------------------------------------------------
	deleted := entries[1]
	resp := radicaleRequest(t, user, password, http.MethodDelete, collectionPath+deleted.uid+".vcf", "")
	resp.Body.Close()
	assert.Contains(t, []int{http.StatusOK, http.StatusNoContent}, resp.StatusCode, "DELETE %s failed", deleted.uid)

	stats, err = service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err, "incremental delete pull from Radicale")
	assert.Equal(t, ContactSyncStats{Archived: 1}, stats,
		"the remote delete must archive exactly one contact (created=%d updated=%d archived=%d skipped=%d)",
		stats.Created, stats.Updated, stats.Archived, stats.Skipped)

	var archived models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", syncUser.ID, deleted.uid).First(&archived).Error)
	assert.True(t, archived.Archived, "a real-server delete must archive (soft delete), not hard-delete")

	// --- subscription bookkeeping ----------------------------------------------
	require.NoError(t, db.First(sub, sub.ID).Error)
	assert.Equal(t, models.ContactSyncStatusSuccess, sub.LastSyncStatus)
	assert.NotEmpty(t, sub.SyncToken, "the subscription must carry Radicale's sync-token")

	// Nothing left behind: the idempotent second run must stay clean.
	stats, err = service.SyncSubscription(context.Background(), db, cfg, sub)
	require.NoError(t, err)
	assert.Equal(t, ContactSyncStats{}, stats, "a quiescent real server must stay quiescent")
}
