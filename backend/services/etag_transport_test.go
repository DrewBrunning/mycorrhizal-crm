package services

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cannedRoundTripper serves one fixed response, so a sync transport's
// response rewriting can be tested without a real CardDAV/CalDAV server.
type cannedRoundTripper struct {
	resp *http.Response
}

func (c *cannedRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return c.resp, nil
}

// etagMultistatus mixes the three wire forms issue #2003 has to tolerate:
// bare, weak, and already-quoted. The address-data payload proves the rewrite
// leaves the rest of the XML (including entity escaping) byte-for-byte intact.
const etagMultistatus = `<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">
<d:response><d:href>/addressbooks/test/a.vcf</d:href><d:propstat><d:prop>
<card:address-data>BEGIN:VCARD&#13;&#10;NOTE:a &amp; b&#13;&#10;END:VCARD</card:address-data>
<d:getetag>unquoted-etag</d:getetag>
</d:prop></d:propstat></d:response>
<d:response><d:href>/addressbooks/test/b.vcf</d:href><d:propstat><d:prop>
<d:getetag>W/"weak-etag"</d:getetag>
</d:prop></d:propstat></d:response>
<d:response><d:href>/addressbooks/test/c.vcf</d:href><d:propstat><d:prop>
<d:getetag>&quot;quoted-etag&quot;</d:getetag>
</d:prop></d:propstat></d:response>
</d:multistatus>`

func cannedETagResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusMultiStatus,
		Header: http.Header{
			"Content-Type": []string{"application/xml; charset=utf-8"},
			"Etag":         []string{`W/"header-etag"`},
		},
		Body: io.NopCloser(strings.NewReader(etagMultistatus)),
	}
}

func decodedGetETags(t *testing.T, body []byte) []string {
	t.Helper()
	var doc struct {
		Responses []struct {
			Propstat []struct {
				Prop []struct {
					ETags []string `xml:"getetag"`
				} `xml:"prop"`
			} `xml:"propstat"`
		} `xml:"response"`
	}
	require.NoError(t, xml.Unmarshal(body, &doc))

	var tags []string
	for _, resp := range doc.Responses {
		for _, ps := range resp.Propstat {
			for _, prop := range ps.Prop {
				for _, tag := range prop.ETags {
					// go-webdav v0.7.0's internal.ETag.UnmarshalText does
					// exactly this; a value it rejects fails the whole sync.
					unquoted, err := strconv.Unquote(tag)
					require.NoError(t, err, "getetag %q must be a quoted form the library can unquote", tag)
					tags = append(tags, unquoted)
				}
			}
		}
	}
	return tags
}

// TestSyncRoundTrippersNormalizeUnquotedAndWeakETags proves the issue #2003
// transport wrapper is in front of both the CardDAV and CalDAV sync clients:
// an unquoted or weak ETag in a multistatus body, and a weak response header,
// are rewritten to the strong quoted form go-webdav v0.7.0 expects.
func TestSyncRoundTrippersNormalizeUnquotedAndWeakETags(t *testing.T) {
	cases := map[string]func(base http.RoundTripper) http.RoundTripper{
		"contact":  func(base http.RoundTripper) http.RoundTripper { return &contactRoundTripper{base: base} },
		"calendar": func(base http.RoundTripper) http.RoundTripper { return &calendarRoundTripper{base: base} },
	}
	for name, newRT := range cases {
		t.Run(name, func(t *testing.T) {
			rt := newRT(&cannedRoundTripper{resp: cannedETagResponse()})
			req := httptest.NewRequest("REPORT", "https://remote.example/dav", nil)

			resp, err := rt.RoundTrip(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			headerETag := resp.Header.Get("ETag")
			unquotedHeader, err := strconv.Unquote(headerETag)
			require.NoError(t, err, "response ETag header %q must be a quoted form the library can unquote", headerETag)
			assert.Equal(t, "header-etag", unquotedHeader)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, []string{"unquoted-etag", "weak-etag", "quoted-etag"}, decodedGetETags(t, body))
			assert.Contains(t, string(body), "NOTE:a &amp; b",
				"non-ETag XML, including entity escaping, must be preserved")
		})
	}
}
