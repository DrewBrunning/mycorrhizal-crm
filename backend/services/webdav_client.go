package services

import (
	"context"
	"crypto/tls"
	"encoding/xml"
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
// WebDAVClient created with the same blockPrivateURLs setting shares the same
// transport. The Nextcloud/ownCloud app password is sent per-request (HTTP
// Basic), not per-connection, so pooling across different users' requests is
// safe.
var (
	webdavSharedTransport     *http.Transport
	webdavSharedTransportOnce sync.Once

	webdavBlockedTransport     *http.Transport
	webdavBlockedTransportOnce sync.Once
)

func getWebDAVTransport(blockPrivate bool) *http.Transport {
	if blockPrivate {
		webdavBlockedTransportOnce.Do(func() {
			webdavBlockedTransport = newWebDAVTransport()
			webdavBlockedTransport.DialContext = webdavPrivateBlockingDialContext
		})
		return webdavBlockedTransport
	}
	webdavSharedTransportOnce.Do(func() {
		webdavSharedTransport = newWebDAVTransport()
	})
	return webdavSharedTransport
}

// newWebDAVTransport mirrors newImmichTransport: HTTP/2 disabled (forced
// HTTP/1.1) to avoid session-reuse failures, idle connections closed after
// 30 s.
func newWebDAVTransport() *http.Transport {
	return &http.Transport{
		TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),

		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConnsPerHost:   4,
	}
}

// Sentinel errors for the WebDAV client failures, mapped to API errors in the
// controller (the immich_client pattern).
var (
	ErrWebDAVInvalidURL     = errors.New("Nextcloud base URL is invalid")
	ErrWebDAVUnreachable    = errors.New("Nextcloud could not be reached")
	ErrWebDAVUnauthorized   = errors.New("Nextcloud app password is invalid or expired")
	ErrWebDAVNotFound       = errors.New("Nextcloud file or folder was not found")
	ErrWebDAVInvalidData    = errors.New("Nextcloud returned data that could not be parsed")
	ErrWebDAVPrivateAddress = errors.New("Nextcloud URL resolves to a private or loopback address")
	ErrWebDAVRequestFailed  = errors.New("Nextcloud responded with an unexpected status")
)

const (
	webdavRequestTimeout = 30 * time.Second
	// maxWebDAVBodyBytes bounds a PROPFIND response body.
	maxWebDAVBodyBytes      = 8 * 1024 * 1024
	maxWebDAVErrorBodyBytes = 2048
)

// WebDAVRequestError carries the real HTTP status (and a bounded response
// body snippet, for logging) behind ErrWebDAVRequestFailed.
type WebDAVRequestError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *WebDAVRequestError) Error() string {
	return fmt.Sprintf("%s: Nextcloud returned %s", ErrWebDAVRequestFailed, e.Status)
}

func (e *WebDAVRequestError) Unwrap() error {
	return ErrWebDAVRequestFailed
}

// WebDAVItem is one entry of a PROPFIND Depth:1 listing — a file or folder.
type WebDAVItem struct {
	// Name is the display name (decoded).
	Name string `json:"name"`
	// Path is the WebDAV path relative to the user's dav root, decoded,
	// starting with "/" — the stored external_id.
	Path string `json:"path"`
	// Type is "file" or "dir".
	Type string `json:"type"`
	Size int64  `json:"size,omitempty"`
	// ModifiedAt is the last-modified HTTP date normalized to RFC3339 UTC.
	ModifiedAt string `json:"modified_at,omitempty"`
	// FileID is the server's stable file id (ownCloud namespace), when the
	// server exposes it — used to deep-link a file in the files app.
	FileID string `json:"file_id,omitempty"`
}

// WebDAVClient is a minimal WebDAV client for Nextcloud / ownCloud (issue
// #157). It speaks standard WebDAV (PROPFIND) against the user's dav root —
// both Nextcloud and ownCloud serve files at /remote.php/dav/files/<user>/,
// which is why one shared client suffices for the fork pair. It intentionally
// implements only what this integration relies on:
//
//   - PROPFIND (Depth: 1) on the dav root   — reachability + auth (Test Connection)
//   - PROPFIND (Depth: 1) on a directory     — browse files/folders to link (L1)
//
// Only an app password is accepted (never the account password); it is sent as
// HTTP Basic with the configured username.
type WebDAVClient struct {
	baseURL     string
	davRootPath string
	username    string
	password    string
	client      *http.Client
}

// NewWebDAVClient builds a WebDAV client. baseURL is the Nextcloud/ownCloud
// server root. The dav root is derived as /remote.php/dav/files/<username>/,
// the standard location both Nextcloud and ownCloud serve user files at. When
// blockPrivateURLs is set, connections to private/loopback/link-local
// addresses are refused (SSRF protection for cloud deployments); it defaults
// to off, mirroring IMMICH_BLOCK_PRIVATE_URLS. Documented in .env.example.
func NewWebDAVClient(baseURL, username, appPassword string, blockPrivateURLs bool) (*WebDAVClient, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil, ErrWebDAVInvalidURL
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, ErrWebDAVInvalidURL
	}
	if strings.TrimSpace(username) == "" {
		return nil, ErrWebDAVInvalidURL
	}

	davRoot := "/remote.php/dav/files/" + username + "/"
	return &WebDAVClient{
		baseURL:     trimmed,
		davRootPath: davRoot,
		username:    username,
		password:    appPassword,
		client: &http.Client{
			Timeout:   webdavRequestTimeout,
			Transport: getWebDAVTransport(blockPrivateURLs),
		},
	}, nil
}

// webdavPrivateBlockingDialContext refuses to connect to non-public addresses,
// pinning the resolved IP so DNS rebinding cannot redirect the dial inward —
// the shared httputil.SafeDialContext mechanism.
func webdavPrivateBlockingDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	dial := httputil.SafeDialContext(
		fmt.Errorf("%w: could not resolve host", ErrWebDAVUnreachable),
		ErrWebDAVPrivateAddress,
	)
	return dial(ctx, network, addr)
}

// propfindXML is the request body for a Depth:1 PROPFIND requesting exactly
// the properties this integration consumes.
const propfindXML = `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns">
  <d:prop>
    <d:resourcetype/>
    <d:displayname/>
    <d:getcontentlength/>
    <d:getlastmodified/>
    <d:getetag/>
    <oc:fileid/>
  </d:prop>
</d:propfind>`

// davRequestURL resolves a dav-root-relative path into the full request URL.
// The base URL was validated at save time and the dav root is derived from the
// stored username, so the path is the only user-supplied component. It is
// therefore guarded against URL structure tricks (see isSafeWebDAVPath), and
// the resolved URL is re-checked so a request can never be sent anywhere other
// than the user's configured Nextcloud host — url.Parse accepts a scheme or
// authority embedded in the reference, which ResolveReference would otherwise
// honor.
func (c *WebDAVClient) davRequestURL(relPath string) (*url.URL, error) {
	path := relPath
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = normalizeWebDAVPath(path)
	if !isSafeWebDAVPath(path) {
		return nil, ErrWebDAVInvalidURL
	}

	base, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, ErrWebDAVInvalidURL
	}
	ref, err := url.Parse(c.davRootPath + strings.TrimPrefix(path, "/"))
	if err != nil {
		return nil, ErrWebDAVInvalidURL
	}
	resolved := base.ResolveReference(ref)
	if !strings.EqualFold(resolved.Scheme, base.Scheme) || !strings.EqualFold(resolved.Host, base.Host) {
		return nil, ErrWebDAVInvalidURL
	}
	return resolved, nil
}

// isSafeWebDAVPath reports whether path is a plain dav-root-relative path that
// cannot alter the request target: no backslash (URL path-separator aliasing),
// no dot-dot segments (escaping the dav root), no query/fragment delimiters
// (changing the requested resource), and no control characters.
func isSafeWebDAVPath(path string) bool {
	if strings.ContainsAny(path, "\\?#") {
		return false
	}
	for _, r := range path {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	for _, seg := range strings.Split(path, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

// propfind performs a Depth:1 PROPFIND on a path relative to the dav root,
// returning the parsed multistatus. Errors map to the sentinel set.
func (c *WebDAVClient) propfind(relPath string) (*webdavMultistatus, error) {
	reqURL, err := c.davRequestURL(relPath)
	if err != nil {
		return nil, err
	}

	// PROPFIND is not one of the stdlib http.Method constants; the method
	// string is the canonical WebDAV verb.
	req, err := http.NewRequest("PROPFIND", reqURL.String(), strings.NewReader(propfindXML))
	if err != nil {
		return nil, ErrWebDAVInvalidURL
	}
	req.Header.Set("Depth", "1")
	req.Header.Set("Accept", "application/xml")
	req.Header.Set("Content-Type", "application/xml")
	req.SetBasicAuth(c.username, c.password)

	resp, err := c.client.Do(req)
	if err != nil {
		logger.Debug().Err(err).Str("url", reqURL.String()).Msg("WebDAV PROPFIND failed")
		return nil, fmt.Errorf("%w: %v", ErrWebDAVUnreachable, err)
	}
	logger.Debug().Str("url", reqURL.String()).Int("status", resp.StatusCode).Msg("WebDAV PROPFIND")
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusMultiStatus:
		// ok
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrWebDAVUnauthorized
	case http.StatusNotFound, http.StatusGone:
		return nil, ErrWebDAVNotFound
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxWebDAVErrorBodyBytes))
		logger.Debug().Str("url", reqURL.String()).Int("status", resp.StatusCode).
			Str("body", string(body)).Msg("WebDAV PROPFIND: unexpected status (Nextcloud responded, not unreachable)")
		return nil, &WebDAVRequestError{StatusCode: resp.StatusCode, Status: resp.Status, Body: string(body)}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxWebDAVBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWebDAVInvalidData, err)
	}
	var ms webdavMultistatus
	if err := xml.Unmarshal(body, &ms); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWebDAVInvalidData, err)
	}
	return &ms, nil
}

// webdavMultistatus is the parsed PROPFIND 207 Multi-Status envelope. Go's
// encoding/xml matches element tags by local name, so the "d:"/"oc:" namespace
// prefixes are transparent.
type webdavMultistatus struct {
	XMLName   xml.Name         `xml:"multistatus"`
	Responses []webdavResponse `xml:"response"`
}

type webdavResponse struct {
	Href      string           `xml:"href"`
	Propstats []webdavPropstat `xml:"propstat"`
}

type webdavPropstat struct {
	Prop   webdavProp `xml:"prop"`
	Status string     `xml:"status"`
}

type webdavProp struct {
	DisplayName      string             `xml:"displayname"`
	GetContentLength int64              `xml:"getcontentlength"`
	GetLastModified  string             `xml:"getlastmodified"`
	ResourceType     webdavResourceType `xml:"resourcetype"`
	FileID           string             `xml:"fileid"`
}

type webdavResourceType struct {
	Collection *struct{} `xml:"collection"`
}

func (rt *webdavResourceType) IsCollection() bool {
	return rt != nil && rt.Collection != nil
}

// Ping checks basic reachability + auth of the dav root (a Depth:1 PROPFIND
// on the root) — Test Connection uses it as its only stage, since WebDAV
// cannot split "reachable" from "authenticated" without a separate
// unauthenticated request. A 401 surfaces as ErrWebDAVUnauthorized.
func (c *WebDAVClient) Ping() error {
	_, err := c.propfind("/")
	return err
}

// ListDir returns the immediate children of a directory (a Depth:1 PROPFIND),
// excluding the directory itself. relPath is relative to the dav root, "/" for
// the root itself.
func (c *WebDAVClient) ListDir(relPath string) ([]WebDAVItem, error) {
	if relPath == "" || relPath == "/" {
		relPath = "/"
	}
	ms, err := c.propfind(relPath)
	if err != nil {
		return nil, err
	}

	// The requested path, normalized, to exclude the directory's own response
	// (Depth:1 returns the target plus its children).
	target := normalizeWebDAVPath(relPath)
	target = "/remote.php/dav/files/" + c.username + target

	var items []WebDAVItem
	for _, resp := range ms.Responses {
		href := strings.TrimSpace(resp.Href)
		if href == "" {
			continue
		}
		decoded, err := url.PathUnescape(href)
		if err != nil {
			decoded = href
		}
		// Some WebDAV servers return relative hrefs; normalize to absolute.
		if !strings.HasPrefix(decoded, "/") {
			decoded = c.davRootPath + decoded
		}
		decoded = strings.TrimRight(decoded, "/")
		// Skip the directory itself. Path comparison is case-SENSITIVE (WebDAV
		// paths are case-sensitive on the underlying filesystem).
		if decoded == strings.TrimRight(target, "/") {
			continue
		}

		var prop webdavProp
		for _, ps := range resp.Propstats {
			if strings.Contains(ps.Status, "200") {
				prop = ps.Prop
				break
			}
		}

		isDir := prop.ResourceType.IsCollection()
		rel := strings.TrimPrefix(decoded, "/remote.php/dav/files/"+c.username)
		if rel == "" {
			rel = "/"
		}
		if !strings.HasPrefix(rel, "/") {
			rel = "/" + rel
		}

		item := WebDAVItem{
			Name:   prop.DisplayName,
			Path:   rel,
			Type:   "file",
			FileID: prop.FileID,
		}
		if isDir {
			item.Type = "dir"
			item.Path = strings.TrimRight(rel, "/") + "/"
		} else {
			item.Size = prop.GetContentLength
		}
		if t, err := time.Parse(time.RFC1123, prop.GetLastModified); err == nil {
			item.ModifiedAt = t.UTC().Format(time.RFC3339)
		}
		if item.Name == "" {
			// displayname was absent (some servers omit it) — fall back to the
			// final path segment.
			trimmed := strings.Trim(rel, "/")
			if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
				item.Name = trimmed[idx+1:]
			} else {
				item.Name = trimmed
			}
		}
		items = append(items, item)
	}
	return items, nil
}

// normalizeWebDAVPath collapses a user-supplied path into the canonical
// leading-slash form.
func normalizeWebDAVPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = strings.TrimRight(p, "/")
	if p == "" {
		return "/"
	}
	return p
}
