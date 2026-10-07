// Package realrelease is the issue #1489 "data written by the REAL old
// release" harness.
//
// Every other upgrade fixture in this repo is synthetic: internal/schemafixture
// loads a schema-only dump and TRANSPLANTS rows written by the CURRENT models
// into it, so the data in an "old" database was never produced by that
// release's code. Production data is: flat columns derived by the old
// BeforeSave, rows from old buggy write paths, soft-deleted rows, audit hash
// chains computed by older code, encrypted columns, 2FA secrets and API tokens
// as the old release stored them.
//
// This package closes that gap with two pieces that share one HTTP client:
//
//   - Seed drives a RUNNING instance of a published release (a docker image of
//     tag vX.Y.Z) through its public API only — register, contacts, notes,
//     activities, circles, tags, relationship edges, an attachment, soft
//     delete, merge, audit undo, an API token, 2FA enrollment — and Capture
//     reads the whole thing back into a Snapshot.
//   - Verify takes the data directory that release left behind, upgrades it in
//     place with the CURRENT code (database.InitDB, the real boot path), boots
//     the current router over it, logs in as the same user (password, then the
//     recovery-code and TOTP second factors; the old API token as a bearer),
//     captures again, and requires Compare(pre, post) to be clean plus the
//     data-integrity checker (services.RunDataIntegrityChecks) to find nothing.
//
// The client is transport-agnostic (an http.RoundTripper), so the same code
// drives a docker container over TCP (cmd/realrelease seed) and the current
// router in-process (Verify, and this package's own tests).
package realrelease

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
)

// Client is a cookie-jar HTTP client bound to one base URL.
type Client struct {
	base string
	hc   *http.Client
}

// NewClient returns a Client for base (e.g. "http://127.0.0.1:7390", no
// trailing slash needed). rt nil means http.DefaultTransport; pass a
// handlerTransport to drive a router in-process.
func NewClient(base string, rt http.RoundTripper) *Client {
	jar, _ := cookiejar.New(nil) // never errors with nil options
	if rt == nil {
		rt = http.DefaultTransport
	}
	return &Client{
		base: strings.TrimRight(base, "/"),
		hc:   &http.Client{Jar: jar, Transport: rt},
	}
}

// HandlerTransport serves requests straight from an http.Handler, no socket.
func HandlerTransport(h http.Handler) http.RoundTripper { return handlerTransport{h} }

type handlerTransport struct{ h http.Handler }

func (t handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	r := req.Clone(req.Context())
	r.RemoteAddr = "127.0.0.1:40000"
	r.RequestURI = req.URL.RequestURI()
	t.h.ServeHTTP(rec, r)
	resp := rec.Result()
	resp.Request = req
	return resp, nil
}

// APIError is a non-2xx response.
type APIError struct {
	Method, Path string
	Status       int
	Body         string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.Path, e.Status, truncate(e.Body, 400))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// raw performs a request and returns the status, body and headers without
// treating a non-2xx status as an error.
func (c *Client) raw(ctx context.Context, method, path string, body any, header map[string]string) (int, []byte, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil { // # pragma: no cover — every body here is a literal map/struct
			return 0, nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/api/v1"+path, rdr)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, b, err
}

// call performs a request, requires a 2xx status and decodes the JSON body
// into out (when non-nil).
func (c *Client) call(ctx context.Context, method, path string, body, out any) error {
	status, b, err := c.raw(ctx, method, path, body, nil)
	if err != nil {
		return err
	}
	if status < 200 || status > 299 {
		return &APIError{Method: method, Path: path, Status: status, Body: string(b)}
	}
	if out != nil && len(b) > 0 {
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("%s %s: decoding response: %w", method, path, err)
		}
	}
	return nil
}
