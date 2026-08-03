package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mycorrhizal/httputil"
	"net"
	"net/http"
	"net/url"
	"sort"
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
			blockedTransport = &http.Transport{
				DialContext: immichPrivateBlockingDialContext,
			}
		})
		return blockedTransport
	}
	sharedTransportOnce.Do(func() {
		sharedTransport = &http.Transport{}
	})
	return sharedTransport
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
)

const (
	immichRequestTimeout = 30 * time.Second
	maxImmichBodyBytes   = 5 * 1024 * 1024
)

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

// ImmichAsset is the slice of the Immich AssetResponseDto this client relies
// on for "latest appearance".
type ImmichAsset struct {
	ID string `json:"id"`
	// FileCreatedAt is when the photo was taken; CreatedAt when it was
	// uploaded. Latest *appearance* is best approximated by the newer of the
	// two (a backfilled old photo uploads later).
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
// mapping auth/not-found responses to sentinel errors.
func (c *ImmichClient) do(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, ErrImmichInvalidURL
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrImmichUnreachable, err)
	}
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
		resp.Body.Close()
		return nil, fmt.Errorf("%w: Immich returned %s", ErrImmichUnreachable, resp.Status)
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
func (c *ImmichClient) ListPeople() ([]ImmichPerson, error) {
	var people []ImmichPerson
	page := 1
	for {
		var envelope struct {
			People struct {
				Items       []ImmichPerson `json:"items"`
				HasNextPage bool           `json:"hasNextPage"`
			} `json:"people"`
		}
		resp, err := c.do(fmt.Sprintf("/api/people?withHidden=false&size=500&page=%d", page))
		if err != nil {
			return nil, err
		}
		if err := decodeJSON(resp, &envelope); err != nil {
			return nil, err
		}
		people = append(people, envelope.People.Items...)
		if !envelope.People.HasNextPage {
			break
		}
		page++
		if page > 100 {
			return nil, fmt.Errorf("%w: pagination did not terminate", ErrImmichInvalidData)
		}
	}
	return people, nil
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

// RecentAssets returns a person's most recent assets, newest first, from
// GET /api/people/:id/assets. "Most recent" = newest fileCreatedAt (or
// createdAt fallback). Bounded to limit items; a person with no photos
// returns an empty slice, never an error.
func (c *ImmichClient) RecentAssets(personID string, limit int) ([]ImmichAsset, error) {
	var envelope struct {
		Items []ImmichAsset `json:"items"`
	}
	resp, err := c.do("/api/people/" + url.PathEscape(personID) + "/assets?size=200")
	if err != nil {
		return nil, err
	}
	if err := decodeJSON(resp, &envelope); err != nil {
		return nil, err
	}

	assets := envelope.Items
	sort.SliceStable(assets, func(i, j int) bool {
		return assetOccurredAt(&assets[i]).After(assetOccurredAt(&assets[j]))
	})
	if len(assets) > limit {
		assets = assets[:limit]
	}
	return assets, nil
}

// Thumbnail fetches a person's thumbnail image, returning the bytes and the
// Content-Type from the server (raster only — SVG is rejected by the proxy
// handler that serves this to browsers, mirroring ProxyImage's hardening).
func (c *ImmichClient) Thumbnail(personID string) ([]byte, string, error) {
	resp, err := c.do("/api/people/" + url.PathEscape(personID) + "/thumbnail")
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImmichBodyBytes))
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrImmichInvalidData, err)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
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
