package controllers

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #948: the HTTP layer between the client/nginx and the Go backend was
// unverified against request-smuggling (CL.TE/TE.CL and friends) and
// CRLF-header-injection classes. Go/net-http defaults make a live desync
// unlikely, but "unlikely" was never pinned by a test. These tests exercise
// the REAL net/http server (httptest.NewServer, a genuine TCP listener) with
// hand-crafted, byte-exact request framing -- not httptest.NewRecorder,
// which never round-trips through net/http's header/body serialization and
// so cannot observe either class of bug.

// dialRouter starts router on a real TCP listener and returns its address.
func dialRouter(t *testing.T, router http.Handler) string {
	t.Helper()
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().String()
}

// TestConflictingFramingRejectedNotSmuggled pins how the backend resolves
// ambiguous request framing -- the exact ambiguity a request-smuggling
// attack exploits when a front-end (nginx) and the backend disagree about
// where a request ends. A CL.TE-style desync needs two disagreeing hops; this
// test can't reproduce nginx's half, but it pins the backend's half: this
// server must never silently prefer a claimed Content-Length over a
// malformed/ambiguous Transfer-Encoding, and must reject (not guess at)
// outright conflicting framing headers, closing the connection rather than
// leaving it in a state where a later request could be misparsed.
func TestConflictingFramingRejectedNotSmuggled(t *testing.T) {
	_, router, _, _ := setupAttachmentRouter(t)
	addr := dialRouter(t, router)

	tests := []struct {
		name       string
		rawTmpl    string
		wantStatus int
	}{
		{
			// CL.CL: two different Content-Length values for the same
			// request. Some proxies use the first, some the last -- exactly
			// the disagreement request smuggling relies on. The backend must
			// refuse to guess.
			name:       "duplicate_conflicting_content_length",
			rawTmpl:    "POST /nope HTTP/1.1\r\nHost: %s\r\nContent-Length: 4\r\nContent-Length: 5\r\n\r\nABCDE",
			wantStatus: http.StatusBadRequest,
		},
		{
			// TE.TE: a duplicated/obfuscated Transfer-Encoding header, a
			// documented smuggling technique for slipping a chunked framing
			// past a front-end that only inspects the first occurrence.
			name:       "duplicate_transfer_encoding",
			rawTmpl:    "POST /nope HTTP/1.1\r\nHost: %s\r\nContent-Length: 4\r\nTransfer-Encoding: chunked\r\nTransfer-Encoding: identity\r\n\r\nABCD",
			wantStatus: http.StatusNotImplemented,
		},
		{
			// A Transfer-Encoding value that is not exactly "chunked"
			// (obfuscation like "xchunked", "chunked ", etc. is another
			// documented smuggling technique) must not be silently treated
			// as either chunked or absent.
			name:       "bogus_transfer_encoding_value",
			rawTmpl:    "POST /nope HTTP/1.1\r\nHost: %s\r\nContent-Length: 4\r\nTransfer-Encoding: xchunked\r\n\r\nABCD",
			wantStatus: http.StatusNotImplemented,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conn, err := net.Dial("tcp", addr)
			require.NoError(t, err)
			defer conn.Close()
			require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))

			raw := fmt.Sprintf(tc.rawTmpl, addr)
			_, err = conn.Write([]byte(raw))
			require.NoError(t, err)

			resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, tc.wantStatus, resp.StatusCode, "ambiguous framing must be rejected, not guessed at")
			// http.ReadResponse consumes the hop-by-hop "Connection" header
			// out of resp.Header and records its meaning in resp.Close.
			assert.True(t, resp.Close,
				"a rejected, ambiguously-framed request must close the connection -- leaving it open risks the next bytes on the wire being misparsed as a new request")
		})
	}
}

// TestChunkedFramingIgnoresConflictingContentLength is the classic CL.TE
// probe: Content-Length claims a 6-byte body (long enough to swallow the
// pipelined request that follows), but Transfer-Encoding is chunked with a
// body that terminates immediately ("0\r\n\r\n"). RFC 7230 says
// Transfer-Encoding must win when both are present. If this backend ever
// preferred Content-Length instead, the trailing bytes below would be
// silently consumed as part of this request's body instead of being parsed
// as the next request on the connection -- exactly the seam a front-end/
// back-end desync exploits to hide a smuggled request inside another one. A
// merged/smuggled body would leave only ONE response on the wire, so the
// key assertion is that a second, independently-parseable HTTP response
// exists at all.
func TestChunkedFramingIgnoresConflictingContentLength(t *testing.T) {
	_, router, _, _ := setupAttachmentRouter(t)
	addr := dialRouter(t, router)

	raw := "POST /nope-first HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Content-Length: 6\r\n" +
		"Transfer-Encoding: chunked\r\n" +
		"\r\n" +
		"0\r\n\r\n" +
		"GET /nope-second HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"\r\n"

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
	_, err = conn.Write([]byte(raw))
	require.NoError(t, err)

	reader := bufio.NewReader(conn)

	first, err := http.ReadResponse(reader, nil)
	require.NoError(t, err)
	first.Body.Close()
	assert.Equal(t, http.StatusNotFound, first.StatusCode,
		"the chunked body terminates at 0\\r\\n\\r\\n; a Content-Length that claims more must not extend it")

	second, err := http.ReadResponse(reader, nil)
	require.NoError(t, err,
		"the trailing bytes must parse as their own well-formed request on the same connection, not be swallowed into the first request's body")
	defer second.Body.Close()
	assert.Equal(t, http.StatusNotFound, second.StatusCode,
		"the pipelined GET /nope-second must be served as a real, independent request, proving no bytes were smuggled inside the first request")
}

// TestDownloadCRLFFilenameNeutralizedOnWire pins the other half of issue
// #948: attachment_controller.go's DownloadAttachment builds the
// Content-Disposition header from the user-supplied OriginalName, escaping
// only quotes -- filename safety against embedded CR/LF (response-splitting
// / header injection) relies entirely on net/http.Header.Write's own
// newline-to-space scrubbing at serialization time, which had no test. A
// test built on httptest.NewRecorder cannot see this: gin's c.Header just
// stores the raw string in a map, and NewRecorder never serializes it, so a
// regression that removed or bypassed the stdlib scrubbing would still pass
// there. This test goes over a real net/http.Server (httptest.NewServer) and
// a real HTTP client so the header is actually written to the wire.
//
// The malicious name is written directly to the database rather than sent
// through the multipart upload path: this is a defense-in-depth check of
// the download response itself (the same posture the code comment at
// attachment_controller.go's DownloadAttachment claims), independent of
// whatever the upload path does or doesn't filter.
func TestDownloadCRLFFilenameNeutralizedOnWire(t *testing.T) {
	db, router, user, _ := setupAttachmentRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	rec := uploadFile(t, router, itoa2(contact.ID), "report.txt", "text/plain", []byte("hello"))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	att := decodeAttachment(t, rec)

	const injectedHeader = "X-Injected"
	malicious := "evil.txt\r\n" + injectedHeader + ": pwned\r\nSet-Cookie: session=attacker"
	require.NoError(t, db.Model(&models.Attachment{}).Where("id = ?", att.ID).
		Update("original_name", malicious).Error)

	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, err := http.Get(fmt.Sprintf("%s/attachments/%s/download", srv.URL, itoa2(att.ID)))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// The attacker-controlled header names must never appear as real,
	// separate headers on the response.
	assert.Empty(t, resp.Header.Values(injectedHeader), "a CRLF-embedded filename must not inject a new response header")
	assert.Empty(t, resp.Header.Values("Set-Cookie"), "a CRLF-embedded filename must not inject a Set-Cookie header")

	disposition := resp.Header.Get("Content-Disposition")
	require.NotEmpty(t, disposition)
	assert.NotContains(t, disposition, "\r", "the raw CR must be scrubbed before the header reaches the wire")
	assert.NotContains(t, disposition, "\n", "the raw LF must be scrubbed before the header reaches the wire")
	// The scrubbed value stays on one physical header line, so the injected
	// text (if any leaked through unscrubbed) shows up folded into the
	// Content-Disposition value itself rather than as its own header.
	assert.True(t, strings.HasPrefix(disposition, "attachment;"), "download disposition must be preserved")
}
