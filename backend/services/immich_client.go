package services

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mycorrhizal/httputil"
	"mycorrhizal/logger"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Shared transports so connection pooling works across requests. Each
// ImmichClient created with the same blockPrivateURLs setting shares the same
// transport; per-host pooling is handled internally by http.Transport, so
// different users' different Immich hosts pool independently. The Immich API
// key is per-request (x-api-key header), not per-connection, so pooling a TCP
// connection across different user requests is safe.
var (
	sharedTransport     *http.Transport
	sharedTransportOnce sync.Once

	blockedTransport     *http.Transport
	blockedTransportOnce sync.Once
)

func getSharedTransport(blockPrivate bool) *http.Transport {
	if blockPrivate {
		blockedTransportOnce.Do(func() {
			blockedTransport = newImmichTransport()
			blockedTransport.DialContext = immichPrivateBlockingDialContext
		})
		return blockedTransport
	}
	sharedTransportOnce.Do(func() {
		sharedTransport = newImmichTransport()
	})
	return sharedTransport
}

// newImmichTransport builds an http.Transport tuned for the Immich integration:
// HTTP/2 is disabled (forced HTTP/1.1) to avoid session-reuse failures that
// surfaced as spurious "Could not reach Immich" errors when an HTTP/2
// connection opened by one request went stale before the next request reused
// it (Caddy at the edge closes idle HTTP/2 sessions before Go notices).  For a
// low-traffic CRM the multiplexing benefit is negligible; reliability matters
// more.  Idle connections are closed after 30 s so dead connections from a
// pooled transport are never handed to a caller.
func newImmichTransport() *http.Transport {
	return &http.Transport{
		// Force HTTP/1.1: clear the ALPN protocol map so Go never upgrades
		// to HTTP/2.  An empty map still allows the TLS handshake to
		// negotiate any protocol the server offers; clearing "h2" is what
		// prevents Go from choosing it.  (TLSNextProto is the canonical
		// escape hatch — x/net/http2 registers "h2" here at init time.)
		TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),

		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConnsPerHost:   4,
	}
}

// Sentinel errors for Immich client failures, mapped to API errors in the
// controller (the calendar_sync_service pattern).
var (
	ErrImmichInvalidURL     = errors.New("Immich base URL is invalid")
	ErrImmichUnreachable    = errors.New("Immich could not be reached")
	ErrImmichUnauthorized   = errors.New("Immich API key is invalid or expired")
	ErrImmichNotFound       = errors.New("Immich person was not found")
	ErrImmichInvalidData    = errors.New("Immich returned data that could not be parsed")
	ErrImmichPrivateAddress = errors.New("Immich URL resolves to a private or loopback address")

	// ErrImmichRequestFailed is a real HTTP response from Immich carrying a
	// status this client has no dedicated sentinel for (anything other than
	// 200/401/403/404/410) — a rejected query parameter, a proxy 5xx, or any
	// other application-level failure from a live, reachable, correctly-keyed
	// instance. T42: this used to collapse into ErrImmichUnreachable, which
	// told users to go check whether their Immich instance was up when it
	// plainly was (Test Connection had just proved it). Never returned for a
	// genuine transport failure — see ImmichRequestError for the real status.
	ErrImmichRequestFailed = errors.New("Immich responded with an unexpected status")
)

const (
	immichRequestTimeout = 30 * time.Second
	maxImmichBodyBytes   = 5 * 1024 * 1024
	// maxImmichErrorBodyBytes bounds the response body captured alongside
	// ErrImmichRequestFailed for logging — small, since it's diagnostic only,
	// never the payload a caller actually consumes.
	maxImmichErrorBodyBytes = 2048
	// maxImmichSearchPages bounds RecentAssets' pagination loop. It is a pure
	// non-termination guard, not a correctness knob (T83): the loop requests
	// size=limit and stops as soon as limit assets are in hand, so a
	// well-behaved server is walked at most two pages. Only a server that
	// pages smaller than requested while still advertising a nextPage ever
	// approaches this cap.
	maxImmichSearchPages = 100
)

// ImmichRequestError carries the real HTTP status (and a bounded response
// body snippet, for logging) behind ErrImmichRequestFailed. Unwrap makes
// errors.Is(err, ErrImmichRequestFailed) work for callers that only care that
// it happened; errors.As(err, &ImmichRequestError{}) reaches the status/body
// for callers that want to report it.
type ImmichRequestError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *ImmichRequestError) Error() string {
	return fmt.Sprintf("%s: Immich returned %s", ErrImmichRequestFailed, e.Status)
}

func (e *ImmichRequestError) Unwrap() error {
	return ErrImmichRequestFailed
}

// ImmichPerson is the slice of the Immich PeopleResponse DTO this client
// relies on (GET /api/people). Kept minimal and defensive: only the fields
// this integration consumes, so Immich adding/removing other fields never
// breaks parsing.
type ImmichPerson struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ImmichPersonStatistics is the GET /api/people/:id/statistics DTO.
type ImmichPersonStatistics struct {
	Assets int `json:"assets"`
}

// ImmichUser is the slice of GET /api/users/me this client relies on —
// used only by "Test connection" (L1) to confirm an API key resolves to a
// real account. Nothing else in this integration needs the caller's own
// identity.
type ImmichUser struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// ImmichAsset is the slice of the Immich AssetResponseDto this client relies
// on for "latest appearance".
type ImmichAsset struct {
	ID string `json:"id"`
	// FileCreatedAt is when the photo was taken; CreatedAt when it was
	// uploaded. assetOccurredAt prefers the taken date and falls back to the
	// upload date only when the taken date is unparseable (a photo with no
	// EXIF) — so "latest appearance" tracks newest *taken* for normal photos.
	FileCreatedAt string `json:"fileCreatedAt"`
	CreatedAt     string `json:"createdAt"`
}

// ImmichClient is a minimal, version-pinned client for the Immich REST API
// (ticket T15/T16). It intentionally implements only the endpoints this
// integration relies on:
//
//   - GET /api/people                 — browse/search persons to link (L1)
//   - GET /api/people/:id/statistics  — photo count (L1 display)
//   - GET /api/people/:id/assets      — latest appearance (L1 display, L2)
//   - GET /api/people/:id/thumbnail   — person thumbnail (L1 display)
//   - GET /api/assets/:id/thumbnail   — one photo's image (contact-photo picker)
//
// "Pin what you rely on and fail gracefully" (T16 trap): every parse is
// defensive, and any unexpected response shape maps to ErrImmichInvalidData
// rather than a panic or a wrong value.
type ImmichClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewImmichClient builds an Immich client. When blockPrivateURLs is set,
// connections to private/loopback/link-local addresses are refused (SSRF
// protection for cloud deployments). The Immich base URL is user-supplied and
// typically a *private* self-hosted address, so this defaults to off — the
// exact trade-off WEBHOOK_BLOCK_PRIVATE_URLS / CALDAV_BLOCK_PRIVATE_URLS
// already make configurable. Documented in .env.example.
//
// The underlying http.Client shares a transport across all ImmichClient
// instances with the same blockPrivateURLs setting, so TCP connections to
// the same Immich host are pooled across requests. The Immich API key is
// sent per-request (x-api-key header), never per-connection, so pooling
// across different users' requests is safe.
func NewImmichClient(baseURL, apiKey string, blockPrivateURLs bool) (*ImmichClient, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil, ErrImmichInvalidURL
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, ErrImmichInvalidURL
	}

	return &ImmichClient{
		baseURL: trimmed,
		apiKey:  apiKey,
		client: &http.Client{
			Timeout:   immichRequestTimeout,
			Transport: getSharedTransport(blockPrivateURLs),
		},
	}, nil
}

// immichPrivateBlockingDialContext refuses to connect to non-public
// addresses, pinning the resolved IP so DNS rebinding cannot redirect the
// dial inward — the shared httputil.SafeDialContext mechanism.
func immichPrivateBlockingDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	dial := httputil.SafeDialContext(
		fmt.Errorf("%w: could not resolve host", ErrImmichUnreachable),
		ErrImmichPrivateAddress,
	)
	return dial(ctx, network, addr)
}

// do performs a GET against the Immich API, applying the x-api-key header and
// mapping auth/not-found responses to sentinel errors. Every call is logged
// at Debug (method/path/outcome, never the API key) so LOG_LEVEL=debug shows
// exactly what's being called and what Immich returned — the "add debugging"
// half of the fix for the swallowed-error bug users hit when a connection
// silently doesn't work.
func (c *ImmichClient) do(path string) (*http.Response, error) {
	return c.doRequest(http.MethodGet, path, nil)
}

// doPost performs a POST against the Immich API with a JSON body. The
// status-code mapping is the same as do().
func (c *ImmichClient) doPost(path string, body any) (*http.Response, error) {
	return c.doRequest(http.MethodPost, path, body)
}

func (c *ImmichClient) doRequest(method, path string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrImmichInvalidURL, err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, ErrImmichInvalidURL
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		logger.Debug().Err(err).Str("method", method).Str("url", c.baseURL+path).Msg("Immich API request failed")
		return nil, fmt.Errorf("%w: %v", ErrImmichUnreachable, err)
	}
	logger.Debug().Str("method", method).Str("url", c.baseURL+path).Int("status", resp.StatusCode).Msg("Immich API request")
	switch resp.StatusCode {
	case http.StatusOK:
		return resp, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		resp.Body.Close()
		return nil, ErrImmichUnauthorized
	case http.StatusNotFound, http.StatusGone:
		resp.Body.Close()
		return nil, ErrImmichNotFound
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxImmichErrorBodyBytes))
		resp.Body.Close()
		logger.Debug().Str("method", method).Str("url", c.baseURL+path).Int("status", resp.StatusCode).
			Str("body", string(body)).Msg("Immich API request: unexpected status (Immich responded, not unreachable)")
		return nil, &ImmichRequestError{StatusCode: resp.StatusCode, Status: resp.Status, Body: string(body)}
	}
}

// decodeJSON reads a bounded JSON body into out, mapping any failure to
// ErrImmichInvalidData.
func decodeJSON(resp *http.Response, out any) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImmichBodyBytes))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrImmichInvalidData, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: %v", ErrImmichInvalidData, err)
	}
	return nil
}

// ListPeople returns every person in the Immich instance (paginated client-
// side until exhausted). The frontend filters/search matches by name — Immich
// has no stable name-search endpoint across versions, so this pins the stable
// GET /api/people pagination instead.
//
// Immich's /api/people response shape varies by version:
//
//	Newer: {"people": {"items": [...], "hasNextPage": bool, "total": N}}
//	Older:  {"people": [...], "total": N}
//
// decodeJSON is not used here because the two shapes need different structs.
func (c *ImmichClient) ListPeople() ([]ImmichPerson, error) {
	var people []ImmichPerson
	page := 1
	for {
		resp, err := c.do(fmt.Sprintf("/api/people?withHidden=false&size=500&page=%d", page))
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxImmichBodyBytes))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrImmichInvalidData, err)
		}

		// Try the newer object-wrapped shape first.
		var paginated struct {
			People struct {
				Items       []ImmichPerson `json:"items"`
				HasNextPage bool           `json:"hasNextPage"`
			} `json:"people"`
		}
		if json.Unmarshal(body, &paginated) == nil && len(paginated.People.Items) > 0 {
			people = append(people, paginated.People.Items...)
			if !paginated.People.HasNextPage {
				break
			}
		} else {
			// Fall back to the older flat-array shape.
			var flat struct {
				People []ImmichPerson `json:"people"`
				Total  int            `json:"total"`
			}
			if err := json.Unmarshal(body, &flat); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrImmichInvalidData, err)
			}
			people = append(people, flat.People...)
			if flat.Total > 0 && len(people) >= flat.Total {
				break
			}
			if len(flat.People) < 500 {
				break
			}
		}
		page++
		if page > 100 {
			return nil, fmt.Errorf("%w: pagination did not terminate", ErrImmichInvalidData)
		}
	}
	return people, nil
}

// Ping checks basic reachability of the Immich server (GET /api/server/ping),
// independent of API key validity — this endpoint requires no auth, so it
// isolates "is the URL even right and reachable" from "is the key valid"
// (Test Connection's first stage).
func (c *ImmichClient) Ping() error {
	resp, err := c.do("/api/server/ping")
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// GetMyUser resolves the API key's owning account (GET /api/users/me) — used
// only to validate a key (Test Connection's second stage).
func (c *ImmichClient) GetMyUser() (*ImmichUser, error) {
	resp, err := c.do("/api/users/me")
	if err != nil {
		return nil, err
	}
	var u ImmichUser
	if err := decodeJSON(resp, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetStatistics returns a person's photo count (GET /api/people/:id/statistics).
func (c *ImmichClient) GetStatistics(personID string) (int, error) {
	var stats ImmichPersonStatistics
	resp, err := c.do("/api/people/" + url.PathEscape(personID) + "/statistics")
	if err != nil {
		return 0, err
	}
	if err := decodeJSON(resp, &stats); err != nil {
		return 0, err
	}
	if stats.Assets < 0 {
		return 0, ErrImmichInvalidData
	}
	return stats.Assets, nil
}

// parseImmichPage reads Immich's assets.nextPage value, which is a JSON
// string ("2") on v3.x but is permitted here to be a bare number too, and
// reports the next page to request.  ok is false for every "stop paginating"
// shape: absent, null, "", 0, or anything non-numeric.
//
// A non-numeric token deliberately stops the loop rather than erroring: the
// endpoint requires a numeric `page` (T70), so a token that isn't a number is
// one this client cannot send back under any encoding.  Stopping yields the
// assets gathered so far; erroring would fail the whole person's sync.
func parseImmichPage(raw json.RawMessage) (int, bool) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return 0, false
	}
	if unquoted, err := strconv.Unquote(s); err == nil {
		s = unquoted
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// RecentAssets returns a person's most recent assets, newest first, from
// POST /api/search/metadata (Immich v3.x removed GET /api/people/:id/assets).
// "Most recent" = newest fileCreatedAt (or createdAt fallback).  Bounded to
// limit items; a person with no photos returns an empty slice, never an error.
//
// Pagination: the first request omits the page parameter and subsequent
// requests send the page number carried in the response's assets.nextPage
// field.  The loop stops as soon as limit assets are in hand, or when there
// is no next page, or at the maxImmichSearchPages non-termination cap.
//
// T70: nextPage must be sent back as a NUMBER, not echoed verbatim.  Immich
// v3.x serializes assets.nextPage as a JSON *string* ("2"), but
// /api/search/metadata's request validator requires `page` to be a number --
// sending the token back as received is rejected with
// `{"expected":"number","code":"invalid_type","path":["page"]}`.  Since the
// first page omits the parameter entirely, this only ever fired for a person
// with more than one page of assets (>200), which is why it survived T59's
// end-to-end verification and had no test coverage.  parseImmichPage accepts
// either JSON shape on the way in; the request always sends a number out.
//
// T83: why early-stop is safe.  The caller's limit is requested as the page
// size and the sort order is requested explicitly, so a single request
// answers the call whenever the person has at least limit assets.  That
// trusts /api/search/metadata's ordering, which is genuinely newest-first:
// in Immich v3.1.0 (the version this integration pins) SearchRepository
// orders by `asset.fileCreatedAt` DESC by default and the HTTP DTO accepts an
// explicit `order` ("desc"/"asc"), with the field fixed server-side to
// fileCreatedAt — there is no way to request createdAt ordering.  The client
// still re-sorts what it fetched (assetOccurredAt) as a cheap safety net, but
// that sort no longer drives the walk.
//
// Approximation accepted and documented here so it is a decision, not a
// surprise: "latest appearance" means newest by fileCreatedAt, the same field
// the server orders by.  assetOccurredAt only diverges from the server's
// order for an asset whose taken date is unparseable (no EXIF): the server
// pages it by its DB fileCreatedAt, while the client's fallback to createdAt
// can rank it higher than its page position — such an asset deeper than the
// fetched window is missed.  Real Immich photos always carry fileCreatedAt,
// so in practice the two orders agree and the fetched window is the correct
// answer.  Full correctness for no-EXIF assets requires the entire walk this
// early-stop exists to eliminate.
func (c *ImmichClient) RecentAssets(personID string, limit int) ([]ImmichAsset, error) {
	if limit < 1 {
		limit = 1
	}
	var all []ImmichAsset
	// The first request IS page 1, but sends no `page` parameter (Immich
	// defaults to it). Tracking that as pageNum=1 rather than 0 is what lets
	// the advance check below catch a server that answers the first request
	// with nextPage=1 — which is a non-advancement, not a second page.
	pageNum := 1
	sendPage := false
	for p := 1; p <= maxImmichSearchPages; p++ {
		reqBody := map[string]any{
			"personIds": []string{personID},
			// size = the caller's limit (T83): requesting 200 to satisfy
			// limit:1 is wasteful even on a single page. The endpoint's size
			// cap is 1000, which every current caller (1/25/30) is far under.
			"size": limit,
			// order made explicit so the early-stop never depends on the
			// server's default changing under us.
			"order": "desc",
		}
		if sendPage {
			reqBody["page"] = pageNum
		}
		resp, err := c.doPost("/api/search/metadata", reqBody)
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(io.LimitReader(resp.Body, maxImmichBodyBytes))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrImmichInvalidData, err)
		}

		var envelope struct {
			Assets struct {
				Items []ImmichAsset `json:"items"`
				// RawMessage, not string: tolerate both the JSON-string form
				// v3.x sends and a bare number, rather than failing the whole
				// sync with ErrImmichInvalidData on a version difference.
				NextPage json.RawMessage `json:"nextPage"`
			} `json:"assets"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrImmichInvalidData, err)
		}
		all = append(all, envelope.Assets.Items...)
		// Early stop (T83): the server answers newest-first, so once limit
		// assets are in hand every remaining page is necessarily older. No
		// need to ask for it. The safety-net sort below only ever reorders
		// what this loop has collected.
		if len(all) >= limit {
			break
		}
		next, ok := parseImmichPage(envelope.Assets.NextPage)
		// Must advance: a server that echoes the current page (or an earlier
		// one) would otherwise refetch it until the cap, silently
		// accumulating duplicate assets rather than erroring. The cap alone
		// bounds the requests but not the duplication.
		if !ok || next <= pageNum {
			break
		}
		pageNum = next
		sendPage = true
	}

	sort.SliceStable(all, func(i, j int) bool {
		return assetOccurredAt(&all[i]).After(assetOccurredAt(&all[j]))
	})
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// Thumbnail fetches a person's thumbnail image, returning the bytes and the
// Content-Type from the server (raster only — SVG is rejected by the proxy
// handler that serves this to browsers, mirroring ProxyImage's hardening).
func (c *ImmichClient) Thumbnail(personID string) ([]byte, string, error) {
	return c.fetchImage("/api/people/" + url.PathEscape(personID) + "/thumbnail")
}

// AssetThumbnail fetches one asset's thumbnail image (the contact-photo
// picker's "load this candidate photo" step, L1: browse a linked person's
// recent photos and pick one). Same hardening as Thumbnail.
func (c *ImmichClient) AssetThumbnail(assetID string) ([]byte, string, error) {
	return c.fetchImage("/api/assets/" + url.PathEscape(assetID) + "/thumbnail")
}

// fetchImage performs the shared image-fetch-and-validate behind Thumbnail
// and AssetThumbnail: non-image content types are rejected at this layer
// before the body reaches the caller, matching httputil.FetchImageFromURL's
// own validation.
func (c *ImmichClient) fetchImage(path string) ([]byte, string, error) {
	resp, err := c.do(path)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return nil, "", fmt.Errorf("%w: non-image content type %q", ErrImmichInvalidData, contentType)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImmichBodyBytes))
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrImmichInvalidData, err)
	}
	return body, contentType, nil
}

// assetOccurredAt parses an asset's occurrence timestamp (fileCreatedAt, else
// createdAt, else now) into a time.Time. A failed parse degrades to time.Now
// so ordering still works rather than comparing zero times.
func assetOccurredAt(a *ImmichAsset) time.Time {
	for _, raw := range []string{a.FileCreatedAt, a.CreatedAt} {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return t
		}
	}
	return time.Now()
}
