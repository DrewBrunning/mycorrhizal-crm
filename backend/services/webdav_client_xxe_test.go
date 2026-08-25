package services

// Issue #416: XXE non-applicability pinning test.
//
// webdav_client.go's propfind (called by the exported Ping/ListDir) is the
// one call site in this codebase where xml.Unmarshal parses a body that is
// genuinely externally-controlled: the PROPFIND multistatus response
// returned by whatever remote Nextcloud/ownCloud server the user configured
// (research for this issue found the only other direct encoding/xml usage,
// cmd/schemagate, parses a locally-generated CI report, not request input;
// CalDAV/CardDAV server-side WebDAV XML is handled by the third-party
// go-webdav dependency, not first-party code).
//
// Go's encoding/xml has no support for external entities or DTD-based
// entity expansion at all -- xml.Decoder/xml.Unmarshal tokenizes a DOCTYPE
// declaration but never fetches an external SYSTEM/PUBLIC entity and never
// expands a general entity beyond the five predefined XML entities, so
// classic XXE (file disclosure, SSRF-via-entity, billion-laughs expansion)
// is not applicable here by construction. This test proves that against the
// real code path rather than resting on stdlib documentation alone: an
// internal-subset entity referencing a local file is rejected as a parse
// error (Go's decoder doesn't define arbitrary entities without an explicit
// Decoder.Entity map, which this codebase never sets), and an external DTD
// reference is never fetched.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebDAVClient_Propfind_InternalEntityXXE_RejectedNotExpanded(t *testing.T) {
	hostile := `<?xml version="1.0"?>
<!DOCTYPE d:multistatus [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/foo</d:href>
    <d:propstat>
      <d:prop><d:displayname>&xxe;</d:displayname></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(hostile))
	}))
	defer server.Close()

	client, err := NewWebDAVClient(server.URL, "testuser", "app-password", false)
	require.NoError(t, err)

	pingErr := client.Ping()
	require.Error(t, pingErr, "an undefined entity reference must not silently resolve to file content")
	assert.True(t, errors.Is(pingErr, ErrWebDAVInvalidData), "must surface as a parse failure, not succeed with expanded/leaked data: %v", pingErr)
	assert.NotContains(t, pingErr.Error(), "root:", "no /etc/passwd content must ever appear, even in the error text")
}

func TestWebDAVClient_Propfind_ExternalDTDReference_NeverFetched(t *testing.T) {
	var dtdServerHit bool
	dtdServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dtdServerHit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer dtdServer.Close()

	hostile := `<?xml version="1.0"?>
<!DOCTYPE d:multistatus SYSTEM "` + dtdServer.URL + `/evil.dtd">
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/foo</d:href>
    <d:propstat>
      <d:prop><d:displayname>ordinary value</d:displayname></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(hostile))
	}))
	defer server.Close()

	client, err := NewWebDAVClient(server.URL, "testuser", "app-password", false)
	require.NoError(t, err)

	// Whether this parses successfully (Go tolerates the DOCTYPE token
	// without acting on it) or errors doesn't matter for this test -- what
	// matters is that the external DTD URL is never requested.
	_ = client.Ping()
	assert.False(t, dtdServerHit, "an external DTD reference must never be fetched")
}
