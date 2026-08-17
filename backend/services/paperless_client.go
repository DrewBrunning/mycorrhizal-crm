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

// Shared transports so connection pooling works across requests. Each
// PaperlessClient created with the same blockPrivateURLs setting shares the
// same transport; per-host pooling is handled internally by http.Transport, so
// different users' different Paperless hosts pool independently. The Paperless
// API token is per-request (Authorization header), not per-connection, so
// pooling a TCP connection across different user requests is safe.
var (
	paperlessSharedTransport     *http.Transport
	paperlessSharedTransportOnce sync.Once

	paperlessBlockedTransport     *http.Transport
	paperlessBlockedTransportOnce sync.Once
)

func getPaperlessTransport(blockPrivate bool) *http.Transport {
	if blockPrivate {
		paperlessBlockedTransportOnce.Do(func() {
			paperlessBlockedTransport = newPaperlessTransport()
			paperlessBlockedTransport.DialContext = paperlessPrivateBlockingDialContext
		})
		return paperlessBlockedTransport
	}
	paperlessSharedTransportOnce.Do(func() {
		paperlessSharedTransport = newPaperlessTransport()
	})
	return paperlessSharedTransport
}

// newPaperlessTransport mirrors newImmichTransport: HTTP/2 disabled (forced
// HTTP/1.1) to avoid session-reuse failures, idle connections closed after
// 30 s so dead connections from a pooled transport are never handed to a
// caller.
func newPaperlessTransport() *http.Transport {
	return &http.Transport{
		TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),

		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConnsPerHost:   4,
	}
}

// Sentinel errors for Paperless client failures, mapped to API errors in the
// controller (the immich_client pattern).
var (
	ErrPaperlessInvalidURL     = errors.New("Paperless base URL is invalid")
	ErrPaperlessUnreachable    = errors.New("Paperless could not be reached")
	ErrPaperlessUnauthorized   = errors.New("Paperless API token is invalid or expired")
	ErrPaperlessNotFound       = errors.New("Paperless document was not found")
	ErrPaperlessInvalidData    = errors.New("Paperless returned data that could not be parsed")
	ErrPaperlessPrivateAddress = errors.New("Paperless URL resolves to a private or loopback address")
	ErrPaperlessRequestFailed  = errors.New("Paperless responded with an unexpected status")
)

const (
	paperlessRequestTimeout = 30 * time.Second
	maxPaperlessBodyBytes   = 5 * 1024 * 1024
	// maxPaperlessErrorBodyBytes bounds the response body captured alongside
	// ErrPaperlessRequestFailed for logging — small, since it's diagnostic only.
	maxPaperlessErrorBodyBytes = 2048
	// maxPaperlessSearchPages is a pure non-termination guard on
	// ListDocuments' pagination loop, mirroring maxImmichSearchPages.
	maxPaperlessSearchPages = 100
)

// PaperlessRequestError carries the real HTTP status (and a bounded response
// body snippet, for logging) behind ErrPaperlessRequestFailed, mirroring
// ImmichRequestError.
type PaperlessRequestError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *PaperlessRequestError) Error() string {
	return fmt.Sprintf("%s: Paperless returned %s", ErrPaperlessRequestFailed, e.Status)
}

func (e *PaperlessRequestError) Unwrap() error {
	return ErrPaperlessRequestFailed
}

// PaperlessDocument is the slice of the Paperless DocumentSerializer this
// client relies on (GET /api/documents/). Kept minimal and defensive: only the
// fields this integration consumes, so Paperless adding/removing other fields
// never breaks parsing.
type PaperlessDocument struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	FileName string `json:"file_name"`
	// Created is the document's creation date (RFC 3339), Added the date it
	// entered the archive. Both are dates Paperless renders without time.
	Created string `json:"created"`
	Added   string `json:"added"`
}

// PaperlessUser is the slice of GET /api/auth/me/ this client relies on —
// used only by "Test connection" (L1) to confirm an API token resolves to a
// real account.
type PaperlessUser struct {
	UserName string `json:"user_name"`
	ID       int    `json:"id"`
}

// PaperlessClient is a minimal client for the Paperless-ngx REST API (issue
// #155). It intentionally implements only the endpoints this integration
// relies on:
//
//   - GET /api/               — reachability (Test Connection stage 1)
//   - GET /api/auth/me/       — token validation (Test Connection stage 2)
//   - GET /api/documents/     — browse/search documents to link (L1)
//
// "Pin what you rely on and fail gracefully": every parse is defensive, and
// any unexpected response shape maps to ErrPaperlessInvalidData rather than a
// panic or a wrong value.
type PaperlessClient struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewPaperlessClient builds a Paperless client. When blockPrivateURLs is set,
// connections to private/loopback/link-local addresses are refused (SSRF
// protection for cloud deployments). The Paperless base URL is user-supplied
// and typically a *private* self-hosted address, so this defaults to off — the
// same trade-off WEBHOOK_BLOCK_PRIVATE_URLS / IMMICH_BLOCK_PRIVATE_URLS
// already make configurable. Documented in .env.example.
func NewPaperlessClient(baseURL, token string, blockPrivateURLs bool) (*PaperlessClient, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil, ErrPaperlessInvalidURL
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, ErrPaperlessInvalidURL
	}

	return &PaperlessClient{
		baseURL: trimmed,
		token:   token,
		client: &http.Client{
			Timeout:   paperlessRequestTimeout,
			Transport: getPaperlessTransport(blockPrivateURLs),
		},
	}, nil
}

// paperlessPrivateBlockingDialContext refuses to connect to non-public
// addresses, pinning the resolved IP so DNS rebinding cannot redirect the dial
// inward — the shared httputil.SafeDialContext mechanism.
func paperlessPrivateBlockingDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	dial := httputil.SafeDialContext(
		fmt.Errorf("%w: could not resolve host", ErrPaperlessUnreachable),
		ErrPaperlessPrivateAddress,
	)
	return dial(ctx, network, addr)
}

// doRequest performs a GET against the Paperless API, applying the token
// header and mapping auth/not-found responses to sentinel errors. Every call
// is logged at Debug (method/path/outcome, never the token).
func (c *PaperlessClient) do(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, ErrPaperlessInvalidURL
	}
	req.Header.Set("Authorization", "Token "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		logger.Debug().Err(err).Str("url", c.baseURL+path).Msg("Paperless API request failed")
		return nil, fmt.Errorf("%w: %v", ErrPaperlessUnreachable, err)
	}
	logger.Debug().Str("url", c.baseURL+path).Int("status", resp.StatusCode).Msg("Paperless API request")
	switch resp.StatusCode {
	case http.StatusOK:
		return resp, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		resp.Body.Close()
		return nil, ErrPaperlessUnauthorized
	case http.StatusNotFound, http.StatusGone:
		resp.Body.Close()
		return nil, ErrPaperlessNotFound
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxPaperlessErrorBodyBytes))
		resp.Body.Close()
		logger.Debug().Str("url", c.baseURL+path).Int("status", resp.StatusCode).
			Str("body", string(body)).Msg("Paperless API request: unexpected status (Paperless responded, not unreachable)")
		return nil, &PaperlessRequestError{StatusCode: resp.StatusCode, Status: resp.Status, Body: string(body)}
	}
}

// decodePaperlessJSON reads a bounded JSON body into out, mapping any failure
// to ErrPaperlessInvalidData.
func decodePaperlessJSON(resp *http.Response, out any) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPaperlessBodyBytes))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPaperlessInvalidData, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: %v", ErrPaperlessInvalidData, err)
	}
	return nil
}

// Ping checks basic reachability of the Paperless server (GET /api/),
// independent of token validity. If a proxy or version gates the root on auth
// anyway, an unauthorized response still surfaces as ErrPaperlessUnauthorized
// so Test Connection can classify by sentinel, not by which call failed.
func (c *PaperlessClient) Ping() error {
	resp, err := c.do("/api/")
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// GetMe resolves the token's owning account (GET /api/auth/me/) — used only to
// validate a token (Test Connection's second stage).
func (c *PaperlessClient) GetMe() (*PaperlessUser, error) {
	resp, err := c.do("/api/auth/me/")
	if err != nil {
		return nil, err
	}
	var u PaperlessUser
	if err := decodePaperlessJSON(resp, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetDocument fetches one document by id (GET /api/documents/:id/). Used at
// link time so the metadata stored on the ExternalIdentity is authoritative —
// fetched from Paperless under the user's own token, never trusted from the
// client.
func (c *PaperlessClient) GetDocument(id int) (*PaperlessDocument, error) {
	resp, err := c.do("/api/documents/" + strconv.Itoa(id) + "/")
	if err != nil {
		return nil, err
	}
	var doc PaperlessDocument
	if err := decodePaperlessJSON(resp, &doc); err != nil {
		return nil, err
	}
	if doc.ID < 1 {
		return nil, fmt.Errorf("%w: document id missing from response", ErrPaperlessInvalidData)
	}
	return &doc, nil
}

// ListDocuments returns documents matching query (searched against title,
// content, and OCR fields by Paperless itself), paginated client-side until
// exhausted. An empty query lists every document. Documents are returned in
// added order (newest first), which is what the L1 picker wants.
func (c *PaperlessClient) ListDocuments(query string) ([]PaperlessDocument, error) {
	var all []PaperlessDocument
	page := 1
	for page <= maxPaperlessSearchPages {
		params := url.Values{}
		params.Set("fields", "id,title,file_name,created,added")
		params.Set("ordering", "-added")
		params.Set("page_size", "100")
		params.Set("page", strconv.Itoa(page))
		if strings.TrimSpace(query) != "" {
			params.Set("query", strings.TrimSpace(query))
		}

		resp, err := c.do("/api/documents/?" + params.Encode())
		if err != nil {
			return nil, err
		}
		var pageResult struct {
			Count   int                 `json:"count"`
			Results []PaperlessDocument `json:"results"`
		}
		if err := decodePaperlessJSON(resp, &pageResult); err != nil {
			return nil, err
		}
		all = append(all, pageResult.Results...)
		if len(pageResult.Results) == 0 || len(all) >= pageResult.Count {
			break
		}
		page++
	}
	return all, nil
}
