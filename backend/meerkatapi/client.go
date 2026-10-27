// Package meerkatapi reads a running Meerkat CRM instance over its REST API
// (issue #2013) and adapts the responses into the same meerkat.Snapshot the
// SQLite-file importer produces, so the mapping, preview, loss report and
// apply engine are shared with the upload path (ADR 0007).
//
// Everything Meerkat returns is untrusted input. Credentials are held in
// memory by the Client only; they are never logged, never echoed in an error
// and dropped by Close.
package meerkatapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"mycorrhizal/httputil"

	"golang.org/x/time/rate"
)

// Sentinel errors; services/meerkat_import_api.go maps these to user-facing
// apperrors and log-safe messages that never carry credentials.
var (
	ErrUnauthorized   = errors.New("meerkat: authentication failed")
	ErrLocked         = errors.New("meerkat: account temporarily locked")
	ErrUnreachable    = errors.New("meerkat: host could not be reached")
	ErrInvalidURL     = errors.New("meerkat: base URL is invalid")
	ErrPrivateAddress = errors.New("meerkat: URL resolves to a private address")
	ErrInvalidData    = errors.New("meerkat: response could not be parsed")
	ErrNotMeerkat     = errors.New("meerkat: the server does not look like a Meerkat instance")
	ErrNotFound       = errors.New("meerkat: not found")
)

const (
	requestTimeout = 30 * time.Second
	maxRetries     = 3
	maxRetryAfter  = 90 * time.Second
	// maxResponseBytes bounds one JSON page. A page of 100 contacts with all
	// preloaded relations is well under this.
	maxResponseBytes = 16 << 20
	// MaxPhotoBytes bounds one downloaded profile picture (photostore applies
	// its own validation and re-encoding on top).
	MaxPhotoBytes = 10 << 20
	pageSize      = 100 // Meerkat caps `limit` at 100
	// maxPages bounds the paging loop if a server keeps returning full pages
	// while ignoring the page parameter.
	maxPages = 1000
)

// Client is a rate-limited, read-only Meerkat API client.
type Client struct {
	baseURL *url.URL
	http    *http.Client
	limiter *rate.Limiter

	// mu guards bearer/loggedIn: Close (from session teardown) can race with
	// in-flight requests when a session is cancelled.
	mu sync.RWMutex
	// bearer is either a Meerkat API token (preferred) or the session JWT
	// obtained by logging in with a username and password.
	bearer string
	// loggedIn records that bearer came from /login, so Close can end it.
	loggedIn bool
}

// newLimiter builds the client-side throttle; swapped out by
// DisableRateLimitForTesting so suites do not wait on it.
var newLimiter = func() *rate.Limiter { return rate.NewLimiter(rate.Every(100*time.Millisecond), 5) }

// DisableRateLimitForTesting removes client-side throttling for all clients
// created afterwards. Test helper only.
func DisableRateLimitForTesting() {
	newLimiter = func() *rate.Limiter { return rate.NewLimiter(rate.Inf, 0) }
}

// NewClient validates and normalizes the instance URL (a trailing "/api" or
// "/api/v1" is tolerated and stripped; the client adds it itself). When
// blockPrivate is set, connections to private/loopback addresses are refused
// by the shared httputil.SafeDialContext guard — opt-in, because a
// self-hosted Meerkat usually lives on a LAN (the same trade-off
// MONICA_BLOCK_PRIVATE_URLS makes).
func NewClient(baseURL string, blockPrivate bool) (*Client, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, ErrInvalidURL
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.Path = strings.TrimSuffix(u.Path, "/v1")
	u.Path = strings.TrimSuffix(u.Path, "/api")
	u.RawQuery = ""
	u.Fragment = ""

	transport := &http.Transport{}
	if blockPrivate {
		transport.DialContext = httputil.SafeDialContext(
			fmt.Errorf("%w: could not resolve host", ErrUnreachable),
			ErrPrivateAddress,
		)
	}
	return &Client{
		baseURL: u,
		http: &http.Client{
			Timeout:   requestTimeout,
			Transport: transport,
			// Never follow redirects: the Authorization header must not be
			// replayed to another host, and a redirect is also how a hostile
			// server would steer the guarded dialer at an internal address.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		limiter: newLimiter(),
	}, nil
}

// InsecureRemote reports whether credentials would travel in clear text to a
// non-local host (plain http:// to anything but loopback or a private
// address literal).
func (c *Client) InsecureRemote() bool {
	if c.baseURL.Scheme != "http" {
		return false
	}
	host := c.baseURL.Hostname()
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || !strings.Contains(host, ".") {
		return false // single-label names are LAN / Docker service names
	}
	if ip := net.ParseIP(host); ip != nil {
		return httputil.IsPublicIP(ip)
	}
	return !strings.HasSuffix(host, ".local") && !strings.HasSuffix(host, ".lan") && !strings.HasSuffix(host, ".internal")
}

// SetToken authenticates with a Meerkat API token.
func (c *Client) SetToken(token string) {
	c.mu.Lock()
	c.bearer = token
	c.mu.Unlock()
}

func (c *Client) token() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.bearer
}

// Login exchanges a username/email and password for a session token. The
// password is not retained.
func (c *Client) Login(ctx context.Context, identifier, password string) error {
	body, err := json.Marshal(map[string]string{"identifier": identifier, "password": password})
	if err != nil { // # pragma: no cover — marshalling two strings cannot fail
		return err
	}
	resp, err := c.do(ctx, http.MethodPost, "/api/v1/login", nil, bytes.NewReader(body), false)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return ErrLocked
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusBadRequest:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotMeerkat
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("%w: login status %d", ErrInvalidData, resp.StatusCode)
	}
	for _, ck := range resp.Cookies() {
		if ck.Name == "auth_token" && ck.Value != "" {
			c.mu.Lock()
			c.bearer, c.loggedIn = ck.Value, true
			c.mu.Unlock()
			return nil
		}
	}
	return fmt.Errorf("%w: login returned no session", ErrInvalidData)
}

// Close ends a password-login session on the Meerkat side (best effort) and
// wipes the credential from memory. API tokens are the user's own and are
// not revoked. Safe to call more than once.
func (c *Client) Close(ctx context.Context) {
	c.mu.RLock()
	end := c.loggedIn && c.bearer != ""
	c.mu.RUnlock()
	if end {
		if resp, err := c.do(ctx, http.MethodPost, "/api/v1/logout", nil, nil, true); err == nil {
			resp.Body.Close()
		}
	}
	c.mu.Lock()
	c.bearer, c.loggedIn = "", false
	c.mu.Unlock()
	c.http.CloseIdleConnections()
}

// do performs one rate-limited request with retries on 429/5xx. The caller
// owns the returned body.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body io.Reader, auth bool) (*http.Response, error) {
	u := *c.baseURL
	u.Path = c.baseURL.Path + path
	if query != nil {
		u.RawQuery = query.Encode()
	}
	var payload []byte
	if body != nil {
		var err error
		if payload, err = io.ReadAll(body); err != nil {
			return nil, err
		}
	}
	for attempt := 0; ; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, u.String(), rdr)
		if err != nil {
			return nil, ErrInvalidURL
		}
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if tok := c.token(); auth && tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if errors.Is(err, ErrPrivateAddress) {
				return nil, ErrPrivateAddress
			}
			// Do not wrap err: *url.Error would carry the URL, which is fine,
			// but never the headers — still, keep the message generic.
			return nil, ErrUnreachable
		}
		retryable := resp.StatusCode >= 500 || (resp.StatusCode == http.StatusTooManyRequests && path != "/api/v1/login")
		if !retryable || attempt >= maxRetries {
			return resp, nil
		}
		delay := retryDelay(resp)
		resp.Body.Close()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
}

func retryDelay(resp *http.Response) time.Duration {
	delay := 2 * time.Second
	if s := resp.Header.Get("Retry-After"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil && secs >= 0 {
			delay = time.Duration(secs) * time.Second
		}
	}
	if delay > maxRetryAfter {
		delay = maxRetryAfter
	}
	return delay
}

// getJSON GETs an authenticated API path and decodes the JSON body into out.
func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	resp, err := c.do(ctx, http.MethodGet, path, query, nil, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		return fmt.Errorf("%w: rate limited", ErrUnreachable)
	case resp.StatusCode >= 500:
		return fmt.Errorf("%w: server error %d", ErrUnreachable, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("%w: unexpected status %d", ErrInvalidData, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return ErrUnreachable
	}
	if len(data) > maxResponseBytes {
		return fmt.Errorf("%w: response too large", ErrInvalidData)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%w: not valid JSON", ErrInvalidData)
	}
	return nil
}

// FetchProfilePicture downloads a contact's full-size profile picture. The
// bytes are untrusted: callers must validate and re-encode them (photostore).
// ErrNotFound means the contact has no picture.
func (c *Client) FetchProfilePicture(ctx context.Context, contactID int64) (data []byte, mediaType string, err error) {
	resp, err := c.do(ctx, http.MethodGet, "/api/v1/contacts/"+strconv.FormatInt(contactID, 10)+"/profile_picture", nil, nil, true)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, "", ErrNotFound
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, "", ErrUnauthorized
	case resp.StatusCode != http.StatusOK:
		return nil, "", fmt.Errorf("%w: picture status %d", ErrUnreachable, resp.StatusCode)
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, MaxPhotoBytes+1))
	if err != nil {
		return nil, "", ErrUnreachable
	}
	if len(data) > MaxPhotoBytes {
		return nil, "", fmt.Errorf("%w: picture too large", ErrInvalidData)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func limitBody(resp *http.Response) io.Reader { return io.LimitReader(resp.Body, 1<<20) }
