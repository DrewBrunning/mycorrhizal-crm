package services

import (
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
	"strconv"
	"strings"
	"sync"
	"time"
)

// Shared transports so connection pooling works across requests — the same
// shape as the Paperless client's (per-host pooling is handled inside
// http.Transport, and the API key is a per-request header, so sharing a TCP
// connection across users is safe).
var (
	geopulseSharedTransport     *http.Transport
	geopulseSharedTransportOnce sync.Once

	geopulseBlockedTransport     *http.Transport
	geopulseBlockedTransportOnce sync.Once
)

func getGeoPulseTransport(blockPrivate bool) *http.Transport {
	if blockPrivate {
		geopulseBlockedTransportOnce.Do(func() {
			geopulseBlockedTransport = newGeoPulseTransport()
			geopulseBlockedTransport.DialContext = geopulsePrivateBlockingDialContext
		})
		return geopulseBlockedTransport
	}
	geopulseSharedTransportOnce.Do(func() {
		geopulseSharedTransport = newGeoPulseTransport()
	})
	return geopulseSharedTransport
}

// newGeoPulseTransport mirrors newPaperlessTransport: HTTP/2 disabled, idle
// connections closed after 30 s.
func newGeoPulseTransport() *http.Transport {
	return &http.Transport{
		TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),

		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConnsPerHost:   4,
	}
}

// Sentinel errors for GeoPulse client failures, mapped to API errors in the
// controller (the paperless_client pattern).
var (
	ErrGeoPulseInvalidURL     = errors.New("GeoPulse base URL is invalid")
	ErrGeoPulseUnreachable    = errors.New("GeoPulse could not be reached")
	ErrGeoPulseUnauthorized   = errors.New("GeoPulse API token is invalid or expired")
	ErrGeoPulseNotFound       = errors.New("GeoPulse resource was not found")
	ErrGeoPulseInvalidData    = errors.New("GeoPulse returned data that could not be parsed")
	ErrGeoPulsePrivateAddress = errors.New("GeoPulse URL resolves to a private or loopback address")
	ErrGeoPulseRequestFailed  = errors.New("GeoPulse responded with an unexpected status")
)

// geopulseRequestTimeout is a var (not a const) so a test can shrink it to keep
// the black-hole-host assertion fast (INT-02, issue #465).
var geopulseRequestTimeout = 30 * time.Second

const (
	maxGeoPulseBodyBytes = 5 * 1024 * 1024
	// maxGeoPulseErrorBodyBytes bounds the response body captured alongside
	// ErrGeoPulseRequestFailed for logging — diagnostic only.
	maxGeoPulseErrorBodyBytes = 2048
)

// GeoPulseRequestError carries the real HTTP status (and a bounded body
// snippet, for logging) behind ErrGeoPulseRequestFailed.
type GeoPulseRequestError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *GeoPulseRequestError) Error() string {
	return fmt.Sprintf("%s: GeoPulse returned %s", ErrGeoPulseRequestFailed, e.Status)
}

func (e *GeoPulseRequestError) Unwrap() error {
	return ErrGeoPulseRequestFailed
}

// GeoPulseUser is the slice of GET /api/users/me this client relies on: just the
// id the photo-search URL needs.
type GeoPulseUser struct {
	UserID   string `json:"userId"`
	FullName string `json:"fullName"`
}

// GeoPulseStay is the slice of GeoPulse's TimelineStayLocationDTO this client
// relies on. Only the consumed fields, so GeoPulse adding others never breaks
// parsing. Timestamp is an ISO-8601 instant.
type GeoPulseStay struct {
	ID           int64   `json:"id"`
	Timestamp    string  `json:"timestamp"`
	LocationName string  `json:"locationName"`
	City         string  `json:"city"`
	Country      string  `json:"country"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	// StayDuration is the length of the stay in seconds.
	StayDuration int64 `json:"stayDuration"`
}

// GeoPulsePhoto is the slice of GeoPulse's ImmichPhotoDto this client relies on.
// Display-only: it is never persisted (ADR 0033).
type GeoPulsePhoto struct {
	ID               string `json:"id"`
	OriginalFileName string `json:"originalFileName"`
	TakenAt          string `json:"takenAt"`
}

// GeoPulseClient is a minimal client for the GeoPulse REST API (issue #160, ADR
// 0033), verified against GeoPulse v1.39.0's source. It implements only the
// endpoints the location-history flow relies on:
//
//   - GET /api/users/me                                — identity + token check
//   - GET /api/streaming-timeline                      — a day's stays
//   - GET /api/users/{userId}/immich/photos/search     — photos near a stay
//
// Every GeoPulse response is wrapped in an {status, message, data} envelope;
// any unexpected shape maps to ErrGeoPulseInvalidData rather than a wrong value.
// GeoPulse documents no API-versioning commitment, so "pin what you rely on and
// fail gracefully" is the contract.
type GeoPulseClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewGeoPulseClient builds a GeoPulse client. When blockPrivateURLs is set,
// connections to private/loopback/link-local addresses are refused (SSRF
// protection for cloud deployments); it defaults to off because a GeoPulse base
// URL is user-supplied and typically a private self-hosted address.
func NewGeoPulseClient(baseURL, apiKey string, blockPrivateURLs bool) (*GeoPulseClient, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil, ErrGeoPulseInvalidURL
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, ErrGeoPulseInvalidURL
	}

	return &GeoPulseClient{
		baseURL: trimmed,
		apiKey:  apiKey,
		client: &http.Client{
			Timeout:   geopulseRequestTimeout,
			Transport: faultingRoundTripper{name: faultGeoPulseRequest, base: getGeoPulseTransport(blockPrivateURLs)},
		},
	}, nil
}

// geopulsePrivateBlockingDialContext refuses to connect to non-public
// addresses, pinning the resolved IP so DNS rebinding cannot redirect the dial
// inward — the shared httputil.SafeDialContext mechanism.
func geopulsePrivateBlockingDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	dial := httputil.SafeDialContext(
		fmt.Errorf("%w: could not resolve host", ErrGeoPulseUnreachable),
		ErrGeoPulsePrivateAddress,
	)
	return dial(ctx, network, addr)
}

// do performs a GET against the GeoPulse API, applying the API-key header and
// mapping auth/not-found responses to sentinel errors. Calls are logged at Debug
// (method/path/outcome, never the key — and never the query string, which carries
// the user's coordinates).
func (c *GeoPulseClient) do(path string, query url.Values) (*http.Response, error) {
	full := c.baseURL + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, full, nil)
	if err != nil {
		return nil, ErrGeoPulseInvalidURL // # pragma: no cover — NewGeoPulseClient already proved baseURL parses and every path/query here is built from constants and url.Values
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		// Never log or wrap the raw *url.Error: its text embeds the full
		// request URL, and SearchPhotos puts the user's coordinates and the
		// queried date range in the query string. Redact the URL while keeping
		// the cause (and the SSRF guard's dial sentinel) reachable through
		// errors.Is/As — see redactedGeoPulseTransportError.
		cause := redactedGeoPulseTransportError(err)
		logger.Debug().Str("path", path).Str("cause", cause.Error()).Msg("GeoPulse API request failed")
		// %w for the cause too, so a dial refused by the SSRF guard stays
		// distinguishable (errors.Is ErrGeoPulsePrivateAddress) from plain
		// unreachability, and Test connection can say which it was.
		return nil, fmt.Errorf("%w: %w", ErrGeoPulseUnreachable, cause)
	}
	logger.Debug().Str("path", path).Int("status", resp.StatusCode).Msg("GeoPulse API request")
	switch resp.StatusCode {
	case http.StatusOK:
		return resp, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		resp.Body.Close()
		return nil, ErrGeoPulseUnauthorized
	case http.StatusNotFound, http.StatusGone:
		resp.Body.Close()
		return nil, ErrGeoPulseNotFound
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxGeoPulseErrorBodyBytes))
		resp.Body.Close()
		logger.Debug().Str("path", path).Int("status", resp.StatusCode).
			Str("body", string(body)).Msg("GeoPulse API request: unexpected status (GeoPulse responded, not unreachable)")
		return nil, &GeoPulseRequestError{StatusCode: resp.StatusCode, Status: resp.Status, Body: string(body)}
	}
}

// redactedGeoPulseTransportError rewrites a transport error so its message no
// longer embeds the request URL, while preserving the wrapped chain. net/http
// returns a *url.Error whose Error() text is `Get "<full URL>": …`, and for
// SearchPhotos that URL carries the user's latitude/longitude and the queried
// date range. This is the GeoPulse counterpart of the geocoder client's
// redactedTransportError (which returns a string; an error is returned here so
// the SSRF guard's ErrGeoPulsePrivateAddress stays reachable via errors.Is).
func redactedGeoPulseTransportError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		// %w ue.Err: its text (the bare network/transport cause) omits the URL,
		// and Unwrap keeps the original chain for errors.Is/As.
		return fmt.Errorf("%w", ue.Err)
	}
	return err
}

// decodeGeoPulseData reads a bounded {status, message, data} envelope and
// unmarshals its data member into out. A non-"success" status, a missing data
// member, or any decode failure maps to ErrGeoPulseInvalidData.
func decodeGeoPulseData(resp *http.Response, out any) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxGeoPulseBodyBytes))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrGeoPulseInvalidData, err)
	}
	var envelope struct {
		Status string          `json:"status"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("%w: %v", ErrGeoPulseInvalidData, err)
	}
	if envelope.Status != "success" {
		return fmt.Errorf("%w: unexpected envelope status %q", ErrGeoPulseInvalidData, envelope.Status)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return fmt.Errorf("%w: response has no data", ErrGeoPulseInvalidData)
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("%w: %v", ErrGeoPulseInvalidData, err)
	}
	return nil
}

// GetMe resolves the token's owning account (GET /api/users/me). It doubles as
// the connection check (reachability + auth in one call) and as the source of
// the GeoPulse user id the photo-search URL needs.
func (c *GeoPulseClient) GetMe() (*GeoPulseUser, error) {
	resp, err := c.do("/api/users/me", nil)
	if err != nil {
		return nil, err
	}
	var u GeoPulseUser
	if err := decodeGeoPulseData(resp, &u); err != nil {
		return nil, err
	}
	if u.UserID == "" {
		return nil, fmt.Errorf("%w: user id missing from response", ErrGeoPulseInvalidData)
	}
	return &u, nil
}

// Ping verifies reachability and the API token in one call.
func (c *GeoPulseClient) Ping() error {
	_, err := c.GetMe()
	return err
}

// GetStays returns the stays GeoPulse recorded between start and end
// (GET /api/streaming-timeline). GeoPulse documents no range cap, so callers
// self-limit to a single day.
func (c *GeoPulseClient) GetStays(start, end time.Time) ([]GeoPulseStay, error) {
	q := url.Values{}
	q.Set("startTime", start.UTC().Format(time.RFC3339))
	q.Set("endTime", end.UTC().Format(time.RFC3339))
	resp, err := c.do("/api/streaming-timeline", q)
	if err != nil {
		return nil, err
	}
	var timeline struct {
		Stays []GeoPulseStay `json:"stays"`
	}
	if err := decodeGeoPulseData(resp, &timeline); err != nil {
		return nil, err
	}
	return timeline.Stays, nil
}

// SearchPhotos asks GeoPulse (which proxies to the Immich instance it has
// configured) for photos taken within radiusMeters of a point during
// [start, end] (GET /api/users/{userId}/immich/photos/search). userID is the
// GeoPulse user id from GetMe; it is path-escaped.
func (c *GeoPulseClient) SearchPhotos(userID string, lat, lon, radiusMeters float64, start, end time.Time, limit int) ([]GeoPulsePhoto, error) {
	q := url.Values{}
	q.Set("startDate", start.Format(time.RFC3339))
	q.Set("endDate", end.Format(time.RFC3339))
	q.Set("latitude", strconv.FormatFloat(lat, 'f', -1, 64))
	q.Set("longitude", strconv.FormatFloat(lon, 'f', -1, 64))
	q.Set("radiusMeters", strconv.FormatFloat(radiusMeters, 'f', -1, 64))
	q.Set("limit", strconv.Itoa(limit))
	resp, err := c.do("/api/users/"+url.PathEscape(userID)+"/immich/photos/search", q)
	if err != nil {
		return nil, err
	}
	var result struct {
		Photos []GeoPulsePhoto `json:"photos"`
	}
	if err := decodeGeoPulseData(resp, &result); err != nil {
		return nil, err
	}
	return result.Photos, nil
}
