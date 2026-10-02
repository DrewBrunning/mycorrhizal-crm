package services

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"mycorrhizal/buildinfo"
	"mycorrhizal/config"
	"mycorrhizal/httputil"
	"mycorrhizal/logger"
)

// Geocoder client (ADR 0031, issue #694). One explicit, per-address lookup
// against the operator-selected provider (GEOCODER_PROVIDER). It is the only
// place address text leaves the instance for geocoding, so every property that
// matters is enforced here rather than at the call site:
//
//   - every connection is dialed through httputil.SafeDialContext (the same
//     choke point Immich/Paperless/Seafile use), unconditionally — the provider
//     endpoints are fixed public hosts, so there is no "trusted LAN" reason to
//     allow a private address;
//   - the nominatim provider self-throttles to its 1 request/second usage
//     policy, because the public instance has no server-side rate limit that
//     would protect the operator;
//   - the MapTiler API key rides in the query string, so it is stripped from
//     every error before it can reach a log line or an API response.

// Sentinel errors for geocoder failures, mapped to API errors in the controller
// (the immich_client / paperless_client pattern).
var (
	ErrGeocoderDisabled      = errors.New("geocoding is not enabled on this server")
	ErrGeocoderUnreachable   = errors.New("the geocoder could not be reached")
	ErrGeocoderUnauthorized  = errors.New("the geocoder rejected the configured credentials")
	ErrGeocoderNotFound      = errors.New("the geocoder endpoint was not found")
	ErrGeocoderInvalidData   = errors.New("the geocoder returned data that could not be parsed")
	ErrGeocoderPrivateAddr   = errors.New("the geocoder resolves to a private or loopback address")
	ErrGeocoderRequestFailed = errors.New("the geocoder responded with an unexpected status")
	ErrGeocoderRateLimited   = errors.New("the geocoder is rate limiting requests")
	// ErrGeocoderNoResult is a successful lookup with no match: the provider
	// answered, it just has no coordinate for that text.
	ErrGeocoderNoResult = errors.New("the geocoder found no match for that address")
)

// geocoderRequestTimeout is a var (not a const) so a test can shrink it to
// keep the black-hole-host assertion fast (INT-02, issue #465).
var geocoderRequestTimeout = 15 * time.Second

const (
	maxGeocoderBodyBytes      = 1 << 20 // a one-result lookup is a few hundred bytes
	maxGeocoderErrorBodyBytes = 2048
	maxGeocoderQueryRunes     = 500 // ContactAddress fields are capped well below this combined; a hard stop for the URL

	nominatimBaseURL = "https://nominatim.openstreetmap.org"
	maptilerBaseURL  = "https://api.maptiler.com"

	// nominatimMinInterval is the public instance's usage policy: an absolute
	// maximum of one request per second.
	nominatimMinInterval = time.Second
)

// GeocodeResult is one resolved coordinate (WGS-84 degrees).
type GeocodeResult struct {
	Lat float64
	Lon float64
}

// GeocoderClient looks one address up at the configured provider.
type GeocoderClient struct {
	provider string
	apiKey   string
	baseURL  string
	client   *http.Client
	gate     *rateGate // nil for providers with no client-side ceiling
}

var (
	geocoderTransportOnce sync.Once
	geocoderTransport     *http.Transport

	// nominatimGate is process-wide: the 1 req/s policy binds the instance's
	// egress IP, not any one client value, and the controller may build more
	// than one client over the process lifetime (config reload in tests).
	nominatimGate = newRateGate(nominatimMinInterval)
)

func getGeocoderTransport() *http.Transport {
	geocoderTransportOnce.Do(func() {
		geocoderTransport = newGeocoderTransport()
		geocoderTransport.DialContext = geocoderPrivateBlockingDialContext
	})
	return geocoderTransport
}

// newGeocoderTransport mirrors newPaperlessTransport: HTTP/2 off (forced
// HTTP/1.1), idle connections closed after 30 s.
func newGeocoderTransport() *http.Transport {
	return &http.Transport{
		TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),

		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConnsPerHost:   2,
	}
}

// geocoderPrivateBlockingDialContext refuses non-public addresses and pins the
// resolved IP so DNS rebinding cannot redirect the dial inward.
func geocoderPrivateBlockingDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	dial := httputil.SafeDialContext(
		fmt.Errorf("%w: could not resolve host", ErrGeocoderUnreachable),
		ErrGeocoderPrivateAddr,
	)
	return dial(ctx, network, addr)
}

// NewGeocoderClient builds the client for provider ("nominatim" or "maptiler").
// It never accepts "none": a disabled geocoder is ErrGeocoderDisabled, decided
// by NewGeocoder before any client exists.
func NewGeocoderClient(provider, apiKey string) (*GeocoderClient, error) {
	switch provider {
	case config.GeocoderProviderNominatim:
		return newGeocoderClient(provider, "", nominatimBaseURL, getGeocoderTransport(), nominatimGate), nil
	case config.GeocoderProviderMapTiler:
		if strings.TrimSpace(apiKey) == "" {
			return nil, ErrGeocoderUnauthorized
		}
		return newGeocoderClient(provider, apiKey, maptilerBaseURL, getGeocoderTransport(), nil), nil
	default:
		return nil, ErrGeocoderDisabled
	}
}

// newGeocoderClient is the shared constructor; tests pass an httptest base URL,
// a plain transport (the fixed provider hosts are public, an httptest server is
// not) and their own gate.
func newGeocoderClient(provider, apiKey, baseURL string, transport http.RoundTripper, gate *rateGate) *GeocoderClient {
	return &GeocoderClient{
		provider: provider,
		apiKey:   apiKey,
		baseURL:  strings.TrimRight(baseURL, "/"),
		gate:     gate,
		client: &http.Client{
			Timeout:   geocoderRequestTimeout,
			Transport: faultingRoundTripper{name: faultGeocoderRequest, base: transport},
			// A provider answers a search directly; following a redirect would
			// only ever be an attempt to bounce the (key-bearing) request.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// Provider returns the provider this client talks to.
func (c *GeocoderClient) Provider() string { return c.provider }

// Geocode resolves query (free-form address text) to a coordinate. ctx bounds
// both the throttle wait and the request.
func (c *GeocoderClient) Geocode(ctx context.Context, query string) (GeocodeResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return GeocodeResult{}, ErrGeocoderNoResult
	}
	if r := []rune(query); len(r) > maxGeocoderQueryRunes {
		query = string(r[:maxGeocoderQueryRunes])
	}

	if c.gate != nil {
		if err := c.gate.Wait(ctx); err != nil {
			return GeocodeResult{}, fmt.Errorf("%w: %v", ErrGeocoderUnreachable, err)
		}
	}

	req, err := c.buildRequest(ctx, query)
	if err != nil { // # pragma: no cover — buildRequest only fails on the unreachable branches it marks
		return GeocodeResult{}, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		// Never wrap the *url.Error: its text embeds the full request URL,
		// including the MapTiler key.
		cause := redactedTransportError(err)
		logger.Debug().Str("provider", c.provider).Str("cause", cause).Msg("Geocoder request failed")
		return GeocodeResult{}, fmt.Errorf("%w: %s", ErrGeocoderUnreachable, cause)
	}
	defer resp.Body.Close()
	logger.Debug().Str("provider", c.provider).Int("status", resp.StatusCode).Msg("Geocoder request")

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return GeocodeResult{}, ErrGeocoderUnauthorized
	case http.StatusNotFound, http.StatusGone:
		return GeocodeResult{}, ErrGeocoderNotFound
	case http.StatusTooManyRequests:
		return GeocodeResult{}, ErrGeocoderRateLimited
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxGeocoderErrorBodyBytes))
		logger.Debug().Str("provider", c.provider).Int("status", resp.StatusCode).
			Str("body", string(body)).Msg("Geocoder responded with an unexpected status")
		return GeocodeResult{}, &GeocoderRequestError{StatusCode: resp.StatusCode, Status: resp.Status}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxGeocoderBodyBytes))
	if err != nil {
		return GeocodeResult{}, fmt.Errorf("%w: %v", ErrGeocoderInvalidData, redactedTransportError(err))
	}
	return c.parse(body)
}

// GeocoderRequestError carries the real HTTP status behind
// ErrGeocoderRequestFailed, mirroring PaperlessRequestError (no body: the
// diagnostic copy is Debug-logged, never returned).
type GeocoderRequestError struct {
	StatusCode int
	Status     string
}

func (e *GeocoderRequestError) Error() string {
	return fmt.Sprintf("%s: geocoder returned %s", ErrGeocoderRequestFailed, e.Status)
}

func (e *GeocoderRequestError) Unwrap() error { return ErrGeocoderRequestFailed }

func (c *GeocoderClient) buildRequest(ctx context.Context, query string) (*http.Request, error) {
	var target string
	switch c.provider {
	case config.GeocoderProviderNominatim:
		q := url.Values{}
		q.Set("q", query)
		q.Set("format", "jsonv2")
		q.Set("limit", "1")
		target = c.baseURL + "/search?" + q.Encode()
	case config.GeocoderProviderMapTiler:
		q := url.Values{}
		q.Set("key", c.apiKey)
		q.Set("limit", "1")
		target = c.baseURL + "/geocoding/" + url.PathEscape(query) + ".json?" + q.Encode()
	default: // # pragma: no cover — newGeocoderClient is only reached with a known provider
		return nil, ErrGeocoderDisabled
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil { // # pragma: no cover — the URL is assembled from escaped parts
		return nil, fmt.Errorf("%w: %s", ErrGeocoderInvalidData, "could not build request")
	}
	req.Header.Set("Accept", "application/json")
	// Nominatim's policy requires an identifying User-Agent; it is harmless for
	// MapTiler, so both get it.
	req.Header.Set("User-Agent", "Mycorrhizal-CRM/"+buildinfo.Get().Version+" (self-hosted geocoding)")
	return req, nil
}

func (c *GeocoderClient) parse(body []byte) (GeocodeResult, error) {
	switch c.provider {
	case config.GeocoderProviderNominatim:
		var hits []struct {
			Lat string `json:"lat"`
			Lon string `json:"lon"`
		}
		if err := json.Unmarshal(body, &hits); err != nil {
			return GeocodeResult{}, fmt.Errorf("%w: %v", ErrGeocoderInvalidData, err)
		}
		if len(hits) == 0 {
			return GeocodeResult{}, ErrGeocoderNoResult
		}
		return newGeocodeResult(hits[0].Lat, hits[0].Lon)
	default: // maptiler: GeoJSON FeatureCollection, coordinates are [lon, lat]
		var fc struct {
			Features []struct {
				Geometry struct {
					Type        string    `json:"type"`
					Coordinates []float64 `json:"coordinates"`
				} `json:"geometry"`
				Center []float64 `json:"center"`
			} `json:"features"`
		}
		if err := json.Unmarshal(body, &fc); err != nil {
			return GeocodeResult{}, fmt.Errorf("%w: %v", ErrGeocoderInvalidData, err)
		}
		if len(fc.Features) == 0 {
			return GeocodeResult{}, ErrGeocoderNoResult
		}
		f := fc.Features[0]
		coords := f.Center
		if len(coords) < 2 && f.Geometry.Type == "Point" {
			coords = f.Geometry.Coordinates
		}
		if len(coords) < 2 {
			return GeocodeResult{}, fmt.Errorf("%w: feature has no point", ErrGeocoderInvalidData)
		}
		return validGeocodeResult(coords[1], coords[0])
	}
}

func newGeocodeResult(latStr, lonStr string) (GeocodeResult, error) {
	var lat, lon float64
	if _, err := fmt.Sscanf(strings.TrimSpace(latStr), "%g", &lat); err != nil {
		return GeocodeResult{}, fmt.Errorf("%w: latitude %q", ErrGeocoderInvalidData, latStr)
	}
	if _, err := fmt.Sscanf(strings.TrimSpace(lonStr), "%g", &lon); err != nil {
		return GeocodeResult{}, fmt.Errorf("%w: longitude %q", ErrGeocoderInvalidData, lonStr)
	}
	return validGeocodeResult(lat, lon)
}

// validGeocodeResult range-checks a provider's answer: a hostile or buggy
// upstream must not be able to store an out-of-range coordinate that the map
// would then choke on.
func validGeocodeResult(lat, lon float64) (GeocodeResult, error) {
	if !(lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180) { // also false for NaN
		return GeocodeResult{}, fmt.Errorf("%w: coordinate out of range", ErrGeocoderInvalidData)
	}
	return GeocodeResult{Lat: lat, Lon: lon}, nil
}

// redactedTransportError returns an error's message with the request URL (and
// therefore any API key in it) removed.
func redactedTransportError(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err.Error()
	}
	return err.Error()
}

// rateGate spaces calls at least interval apart, serializing waiters. The
// clock and sleep are fields so the throttle is testable without real waits.
type rateGate struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
	now      func() time.Time
	sleep    func(ctx context.Context, d time.Duration) error
}

func newRateGate(interval time.Duration) *rateGate {
	return &rateGate{interval: interval, now: time.Now, sleep: sleepContext}
}

// Wait blocks until this caller's slot, or returns ctx's error. Slots are
// reserved under the lock, so concurrent callers queue one interval apart. A
// caller whose context ends still consumed its slot, which only ever makes the
// gate more conservative than the provider's policy requires.
func (g *rateGate) Wait(ctx context.Context) error {
	g.mu.Lock()
	now := g.now()
	slot := g.next
	if slot.Before(now) {
		slot = now
	}
	g.next = slot.Add(g.interval)
	g.mu.Unlock()

	if d := slot.Sub(now); d > 0 {
		return g.sleep(ctx, d)
	}
	return ctx.Err()
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
