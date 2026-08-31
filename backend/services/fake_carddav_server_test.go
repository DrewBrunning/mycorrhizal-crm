package services

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeCardDAVServer is a real-protocol, in-memory test double for the CardDAV
// surface the contact sync client actually speaks (issue #496). Unlike the
// hand-rolled httptest handlers in contact_sync_service_test.go (which only
// serve a fixed full-refetch response), it implements the full RFC 6578 sync
// lifecycle so the incremental-pull and ETag/CTag semantics the real
// integration depends on can be exercised deterministically on every PR:
//
//   - REPORT sync-collection (delta since a token, RFC 6578)
//   - REPORT addressbook-query (full membership) and addressbook-multiget
//     (bodies for a path list) — the two-step fetchChanges pattern
//   - GET / PUT / DELETE on address objects, with per-resource ETags and a
//     monotonic collection sync-token that bumps on every mutation
//   - PROPFIND exposing the collection's sync-token (a getctag stand-in)
//
// It deliberately stores the raw vCard bytes it was PUT (or seeded with) and
// returns them verbatim — a real server stores what a client writes, and
// byte-verbatim is exactly what makes the fake "a real server that cannot
// disagree with us" for the round-trip assertions, while the ETag/token
// bookkeeping is where a server CAN disagree and therefore where the
// deterministic suite concentrates its assertions.
//
// The ETag/token degradation knobs exist so the "missing or malformed ETag /
// token must degrade to a defined behavior" half of issue #496 item 2 is
// exercised explicitly instead of assumed:
//
//   - suppressETags   — omit getetag from REPORT/GET responses
//   - malformedETags  — emit an unquoted, unparseable getetag
//   - emptySyncToken  — omit the sync-token from sync-collection responses
type fakeCardDAVServer struct {
	t *testing.T

	Server   *httptest.Server
	bookPath string
	mu       sync.Mutex

	cards map[string]*fakeCardDAVCard // keyed by URL path, e.g. /addressbooks/u/contacts/x.vcf

	// changeLog records every mutation (create/update/delete) in token order;
	// sync-collection deltas are computed by replaying it. nextToken is the
	// collection's current sync-token value (monotonic).
	nextToken int
	changeLog []fakeCardDAVChange

	suppressETags  bool
	malformedETags bool
	emptySyncToken bool
	// failRequests makes every request return 503 — the "server is down"
	// mode the offline-window/reconnection test toggles.
	failRequests bool
	// failQuery makes only addressbook-query REPORTs fail (503) — the
	// "sync-collection works but the full refetch does not" degradation mode.
	failQuery bool

	counts fakeCardDAVCounts
}

type fakeCardDAVCard struct {
	raw     string
	etag    string
	modTime time.Time
}

type fakeCardDAVChange struct {
	token   int
	href    string
	deleted bool
}

// fakeCardDAVCounts tallies requests by kind so a test can assert that an
// unchanged sync-token produced NO re-fetch of card bodies (the load-bearing
// incremental-sync property), not just that the stats line read zero.
type fakeCardDAVCounts struct {
	SyncCollection int
	MultiGet       int
	Query          int
	Put            int
	Delete         int
	Get            int
	Propfind       int
}

func (c *fakeCardDAVCounts) total() int {
	return c.SyncCollection + c.MultiGet + c.Query + c.Put + c.Delete + c.Get + c.Propfind
}

func newFakeCardDAVServer(t *testing.T, bookPath string) *fakeCardDAVServer {
	t.Helper()
	if bookPath == "" {
		bookPath = "/addressbooks/test/contacts/"
	}
	if !strings.HasSuffix(bookPath, "/") {
		bookPath += "/"
	}
	f := &fakeCardDAVServer{
		t:        t,
		bookPath: bookPath,
		cards:    make(map[string]*fakeCardDAVCard),
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

// URL returns the base URL of the test server.
func (f *fakeCardDAVServer) URL() string { return f.Server.URL }

// bookURL returns the full address-book collection URL (what a subscription
// would point at).
func (f *fakeCardDAVServer) bookURL() string { return f.Server.URL + f.bookPath }

func (f *fakeCardDAVServer) Close() { f.Server.Close() }

func (f *fakeCardDAVServer) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t.Helper()

	if f.failRequests {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		f.counts.Get++
		f.serveGet(w, r)
	case http.MethodPut:
		f.counts.Put++
		f.servePut(w, r)
	case http.MethodDelete:
		f.counts.Delete++
		f.serveDelete(w, r)
	case "REPORT":
		f.serveReport(w, r)
	case "PROPFIND":
		f.counts.Propfind++
		f.servePropfind(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// --- helpers ----------------------------------------------------------------

func (f *fakeCardDAVServer) currentToken() string {
	return fmt.Sprintf("fake-sync-token-%06d", f.nextToken)
}

// nextETag returns a fresh, unique, UNQUOTED etag value. The wire form is
// always the quoted string (RFC 7232 strong ETag); cardResponse/serveGet emit
// the quotes, and the go-webdav client unquotes on read — so the value stored
// on the app's contact_sync_links.etag column equals the value stored here,
// which is what the lifecycle assertions compare against.
func (f *fakeCardDAVServer) nextETag() string {
	return fmt.Sprintf("etag-%d-%d", time.Now().UnixNano(), f.nextToken)
}

func (f *fakeCardDAVServer) recordChange(href string, deleted bool) {
	f.nextToken++
	f.changeLog = append(f.changeLog, fakeCardDAVChange{token: f.nextToken, href: href, deleted: deleted})
}

// parseTokenValue extracts the opaque token integer from a token string the
// way the fake stores them. Unparseable tokens (empty, garbage) mean "I don't
// know this server state" and are treated as a first sync.
func parseFakeToken(token string) (int, bool) {
	if !strings.HasPrefix(token, "fake-sync-token-") {
		return 0, false
	}
	var n int
	if _, err := fmt.Sscanf(strings.TrimPrefix(token, "fake-sync-token-"), "%d", &n); err != nil {
		return 0, false
	}
	return n, true
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func (f *fakeCardDAVServer) cardResponse(sb *strings.Builder, href string, card *fakeCardDAVCard) {
	etag := ""
	if !f.suppressETags {
		if f.malformedETags {
			etag = "<d:getetag>etag-without-quotes</d:getetag>"
		} else {
			etag = "<d:getetag>" + fmt.Sprintf("%q", card.etag) + "</d:getetag>"
		}
	}
	modified := "<d:getlastmodified>" + card.modTime.UTC().Format(http.TimeFormat) + "</d:getlastmodified>"
	fmt.Fprintf(sb, `  <d:response>
    <d:href>%s</d:href>
    <d:propstat>
      <d:prop>
        <card:address-data>%s</card:address-data>
        <d:getcontentlength>%d</d:getcontentlength>
        %s
        %s
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
`, href, xmlEscape(card.raw), len(card.raw), modified, etag)
}

func (f *fakeCardDAVServer) deletedResponse(sb *strings.Builder, href string) {
	fmt.Fprintf(sb, `  <d:response>
    <d:href>%s</d:href>
    <d:status>HTTP/1.1 404 Not Found</d:status>
  </d:response>
`, href)
}

func (f *fakeCardDAVServer) writeMultistatus(w http.ResponseWriter, sb *strings.Builder, includeToken bool) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	var out strings.Builder
	out.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	out.WriteString(`<d:multistatus xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">` + "\n")
	if includeToken && !f.emptySyncToken {
		fmt.Fprintf(&out, "  <d:sync-token>%s</d:sync-token>\n", f.currentToken())
	}
	out.WriteString(sb.String())
	out.WriteString(`</d:multistatus>`)
	_, _ = w.Write([]byte(out.String()))
}

// --- per-method handlers ----------------------------------------------------

func (f *fakeCardDAVServer) serveGet(w http.ResponseWriter, r *http.Request) {
	card, ok := f.cards[strings.TrimRight(r.URL.Path, "/")]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
	w.Header().Set("ETag", fmt.Sprintf("%q", card.etag))
	w.Header().Set("Last-Modified", card.modTime.UTC().Format(http.TimeFormat))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(card.raw))
}

func (f *fakeCardDAVServer) servePut(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	href := strings.TrimRight(r.URL.Path, "/")
	_, had := f.cards[href]
	etag := f.nextETag()
	f.cards[href] = &fakeCardDAVCard{raw: string(body), etag: etag, modTime: time.Now().UTC()}
	f.recordChange(href, false)
	if !had {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
	w.Header().Set("ETag", fmt.Sprintf("%q", etag))
}

func (f *fakeCardDAVServer) serveDelete(w http.ResponseWriter, r *http.Request) {
	href := strings.TrimRight(r.URL.Path, "/")
	if _, ok := f.cards[href]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	delete(f.cards, href)
	f.recordChange(href, true)
	w.WriteHeader(http.StatusNoContent)
}

// seedCard inserts a card directly into the fake's store without going through
// PUT, recording the change so a later incremental sync sees it as a remote
// create. This is the staging primitive the lifecycle suite uses to set up
// server state before the app's first sync.
func (f *fakeCardDAVServer) seedCard(href, raw string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cards[strings.TrimRight(href, "/")] = &fakeCardDAVCard{
		raw:     raw,
		etag:    f.nextETag(),
		modTime: time.Now().UTC(),
	}
	f.recordChange(strings.TrimRight(href, "/"), false)
}

func (f *fakeCardDAVServer) deleteCard(href string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	href = strings.TrimRight(href, "/")
	delete(f.cards, href)
	f.recordChange(href, true)
}

func (f *fakeCardDAVServer) serveReport(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var root struct {
		XMLName   xml.Name
		SyncToken string   `xml:"sync-token"`
		Hrefs     []string `xml:"href"`
	}
	if err := xml.Unmarshal(body, &root); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var sb strings.Builder
	switch root.XMLName.Local {
	case "sync-collection":
		f.counts.SyncCollection++
		f.serveSyncCollection(&sb, root.SyncToken)
	case "addressbook-query":
		f.counts.Query++
		if f.failQuery {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		f.serveFullListing(&sb)
	case "addressbook-multiget":
		f.counts.MultiGet++
		f.serveMultiGet(&sb, root.Hrefs)
	default:
		w.WriteHeader(http.StatusNotImplemented)
		return
	}
	f.writeMultistatus(w, &sb, true)
}

func (f *fakeCardDAVServer) serveSyncCollection(sb *strings.Builder, reqToken string) {
	// A delta since the requested token. Unparseable/empty token = the client
	// has no usable server state = first sync = send the entire current
	// membership as created (deletions can't be known without a baseline).
	reqN, ok := parseFakeToken(reqToken)
	if !ok {
		for _, href := range sortedCardKeys(f.cards) {
			f.cardResponse(sb, href, f.cards[href])
		}
		return
	}
	if reqN >= f.nextToken {
		return // no changes since the token — the "unchanged CTag" case
	}

	// Replay the change log since reqN, keeping the LAST change per href (a
	// href may have been updated then deleted, or deleted then recreated).
	latest := make(map[string]fakeCardDAVChange)
	for _, ch := range f.changeLog {
		if ch.token > reqN {
			latest[ch.href] = ch
		}
	}
	hrefs := make([]string, 0, len(latest))
	for href := range latest {
		hrefs = append(hrefs, href)
	}
	sort.Strings(hrefs)
	for _, href := range hrefs {
		if card, present := f.cards[href]; present {
			f.cardResponse(sb, href, card)
		} else {
			f.deletedResponse(sb, href)
		}
	}
}

func (f *fakeCardDAVServer) serveFullListing(sb *strings.Builder) {
	for _, href := range sortedCardKeys(f.cards) {
		f.cardResponse(sb, href, f.cards[href])
	}
}

func (f *fakeCardDAVServer) serveMultiGet(sb *strings.Builder, hrefs []string) {
	for _, href := range hrefs {
		href = strings.TrimRight(href, "/")
		if card, ok := f.cards[href]; ok {
			f.cardResponse(sb, href, card)
		} else {
			f.deletedResponse(sb, href)
		}
	}
}

func (f *fakeCardDAVServer) servePropfind(w http.ResponseWriter, r *http.Request) {
	var sb strings.Builder
	fmt.Fprintf(&sb, `  <d:response>
    <d:href>%s</d:href>
    <d:propstat>
      <d:prop>
        <cs:getctag>%s</cs:getctag>
        <d:sync-token>%s</d:sync-token>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
`, f.bookPath, f.currentToken(), f.currentToken())
	f.writeMultistatus(w, &sb, false)
}

func sortedCardKeys(cards map[string]*fakeCardDAVCard) []string {
	keys := make([]string, 0, len(cards))
	for k := range cards {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
