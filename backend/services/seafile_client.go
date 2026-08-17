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
	"strings"
	"sync"
	"time"
)

// Shared transports so connection pooling works across requests. Each
// SeafileClient created with the same blockPrivateURLs setting shares the same
// transport; per-host pooling is handled internally by http.Transport. The
// Seafile API token is per-request (Authorization header), not per-connection,
// so pooling a TCP connection across different user requests is safe.
var (
	seafileSharedTransport     *http.Transport
	seafileSharedTransportOnce sync.Once

	seafileBlockedTransport     *http.Transport
	seafileBlockedTransportOnce sync.Once
)

func getSeafileTransport(blockPrivate bool) *http.Transport {
	if blockPrivate {
		seafileBlockedTransportOnce.Do(func() {
			seafileBlockedTransport = newSeafileTransport()
			seafileBlockedTransport.DialContext = seafilePrivateBlockingDialContext
		})
		return seafileBlockedTransport
	}
	seafileSharedTransportOnce.Do(func() {
		seafileSharedTransport = newSeafileTransport()
	})
	return seafileSharedTransport
}

// newSeafileTransport mirrors newImmichTransport: HTTP/2 disabled (forced
// HTTP/1.1) to avoid session-reuse failures, idle connections closed after
// 30 s.
func newSeafileTransport() *http.Transport {
	return &http.Transport{
		TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),

		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConnsPerHost:   4,
	}
}

// Sentinel errors for Seafile client failures, mapped to API errors in the
// controller (the immich_client pattern).
var (
	ErrSeafileInvalidURL     = errors.New("Seafile base URL is invalid")
	ErrSeafileUnreachable    = errors.New("Seafile could not be reached")
	ErrSeafileUnauthorized   = errors.New("Seafile API token is invalid or expired")
	ErrSeafileNotFound       = errors.New("Seafile library or file was not found")
	ErrSeafileInvalidData    = errors.New("Seafile returned data that could not be parsed")
	ErrSeafilePrivateAddress = errors.New("Seafile URL resolves to a private or loopback address")
	ErrSeafileRequestFailed  = errors.New("Seafile responded with an unexpected status")
)

const (
	seafileRequestTimeout    = 30 * time.Second
	maxSeafileBodyBytes      = 5 * 1024 * 1024
	maxSeafileErrorBodyBytes = 2048
)

// SeafileRequestError carries the real HTTP status (and a bounded response
// body snippet, for logging) behind ErrSeafileRequestFailed.
type SeafileRequestError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *SeafileRequestError) Error() string {
	return fmt.Sprintf("%s: Seafile returned %s", ErrSeafileRequestFailed, e.Status)
}

func (e *SeafileRequestError) Unwrap() error {
	return ErrSeafileRequestFailed
}

// SeafileLibrary is the slice of GET /api2/repos/ this client relies on.
type SeafileLibrary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Type is "library" or "virtual" (a read-only virtual library Seafile
	// derives, e.g. for a group).
	Type string `json:"type"`
}

// SeafileItem is one entry of GET /api2/repos/:id/dir/ — a file or subfolder
// inside a library.
type SeafileItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Type is "file" or "dir".
	Type string `json:"type"`
	// Size is the file size in bytes (0 for directories).
	Size int64 `json:"size"`
	// MTime is the Unix mtime seconds.
	MTime int64 `json:"mtime"`
	// ParentDir is the containing directory path ("/...").
	ParentDir string `json:"parent_dir"`
}

// SeafileClient is a minimal client for the Seafile Web API (issue #156). It
// intentionally implements only the endpoints this integration relies on:
//
//   - GET /api2/ping/               — reachability (Test Connection stage 1)
//   - GET /api2/auth/ping/          — token validation (Test Connection stage 2)
//   - GET /api2/repos/              — browse libraries to link (L1)
//   - GET /api2/repos/:id/dir/      — list a library folder's contents (L1)
//
// "Pin what you rely on and fail gracefully": every parse is defensive, and
// any unexpected response shape maps to ErrSeafileInvalidData.
type SeafileClient struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewSeafileClient builds a Seafile client. When blockPrivateURLs is set,
// connections to private/loopback/link-local addresses are refused (SSRF
// protection for cloud deployments). The Seafile base URL is user-supplied
// and typically a *private* self-hosted address, so this defaults to off — the
// same trade-off WEBHOOK_BLOCK_PRIVATE_URLS / IMMICH_BLOCK_PRIVATE_URLS make
// configurable. Documented in .env.example.
func NewSeafileClient(baseURL, token string, blockPrivateURLs bool) (*SeafileClient, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil, ErrSeafileInvalidURL
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, ErrSeafileInvalidURL
	}

	return &SeafileClient{
		baseURL: trimmed,
		token:   token,
		client: &http.Client{
			Timeout:   seafileRequestTimeout,
			Transport: getSeafileTransport(blockPrivateURLs),
		},
	}, nil
}

// seafilePrivateBlockingDialContext refuses to connect to non-public
// addresses, pinning the resolved IP so DNS rebinding cannot redirect the dial
// inward — the shared httputil.SafeDialContext mechanism.
func seafilePrivateBlockingDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	dial := httputil.SafeDialContext(
		fmt.Errorf("%w: could not resolve host", ErrSeafileUnreachable),
		ErrSeafilePrivateAddress,
	)
	return dial(ctx, network, addr)
}

// do performs a GET against the Seafile API, applying the token header and
// mapping auth/not-found responses to sentinel errors.
func (c *SeafileClient) do(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, ErrSeafileInvalidURL
	}
	req.Header.Set("Authorization", "Token "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		logger.Debug().Err(err).Str("url", c.baseURL+path).Msg("Seafile API request failed")
		return nil, fmt.Errorf("%w: %v", ErrSeafileUnreachable, err)
	}
	logger.Debug().Str("url", c.baseURL+path).Int("status", resp.StatusCode).Msg("Seafile API request")
	switch resp.StatusCode {
	case http.StatusOK:
		return resp, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		resp.Body.Close()
		return nil, ErrSeafileUnauthorized
	case http.StatusNotFound, http.StatusGone:
		resp.Body.Close()
		return nil, ErrSeafileNotFound
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxSeafileErrorBodyBytes))
		resp.Body.Close()
		logger.Debug().Str("url", c.baseURL+path).Int("status", resp.StatusCode).
			Str("body", string(body)).Msg("Seafile API request: unexpected status (Seafile responded, not unreachable)")
		return nil, &SeafileRequestError{StatusCode: resp.StatusCode, Status: resp.Status, Body: string(body)}
	}
}

// doUnauthenticated is do() without the token header — used only by Ping
// (GET /api2/ping/ requires no auth on real Seafile).
func (c *SeafileClient) doUnauthenticated(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, ErrSeafileInvalidURL
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSeafileUnreachable, err)
	}
	logger.Debug().Str("url", c.baseURL+path).Int("status", resp.StatusCode).Msg("Seafile API request (unauthenticated)")
	switch resp.StatusCode {
	case http.StatusOK:
		return resp, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		resp.Body.Close()
		return nil, ErrSeafileUnauthorized
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxSeafileErrorBodyBytes))
		resp.Body.Close()
		return nil, &SeafileRequestError{StatusCode: resp.StatusCode, Status: resp.Status, Body: string(body)}
	}
}

// decodeSeafileJSON reads a bounded JSON body into out, mapping any failure to
// ErrSeafileInvalidData.
func decodeSeafileJSON(resp *http.Response, out any) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSeafileBodyBytes))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSeafileInvalidData, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: %v", ErrSeafileInvalidData, err)
	}
	return nil
}

// Ping checks basic reachability of the Seafile server (GET /api2/ping/),
// independent of token validity. The endpoint answers the quoted string
// "pong".
func (c *SeafileClient) Ping() error {
	resp, err := c.doUnauthenticated("/api2/ping/")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSeafileInvalidData, err)
	}
	if !strings.Contains(string(body), "pong") {
		return fmt.Errorf("%w: ping response was %q", ErrSeafileInvalidData, strings.TrimSpace(string(body)))
	}
	return nil
}

// PingAuth validates the token (GET /api2/auth/ping/) — used only by Test
// Connection's second stage. The endpoint answers the quoted string "pong".
func (c *SeafileClient) PingAuth() error {
	resp, err := c.do("/api2/auth/ping/")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSeafileInvalidData, err)
	}
	if !strings.Contains(string(body), "pong") {
		return fmt.Errorf("%w: auth ping response was %q", ErrSeafileInvalidData, strings.TrimSpace(string(body)))
	}
	return nil
}

// ListLibraries returns every library (repo) the token can access
// (GET /api2/repos/).
func (c *SeafileClient) ListLibraries() ([]SeafileLibrary, error) {
	resp, err := c.do("/api2/repos/")
	if err != nil {
		return nil, err
	}
	var libs []SeafileLibrary
	if err := decodeSeafileJSON(resp, &libs); err != nil {
		return nil, err
	}
	return libs, nil
}

// ListDir returns the contents of a library folder (GET /api2/repos/:id/dir/).
// path must begin with "/" (root is "/").
func (c *SeafileClient) ListDir(repoID, path string) ([]SeafileItem, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	params := url.Values{}
	params.Set("p", path)
	params.Set("recursive", "0")
	resp, err := c.do("/api2/repos/" + url.PathEscape(repoID) + "/dir/?" + params.Encode())
	if err != nil {
		return nil, err
	}
	var items []SeafileItem
	if err := decodeSeafileJSON(resp, &items); err != nil {
		return nil, err
	}
	return items, nil
}
