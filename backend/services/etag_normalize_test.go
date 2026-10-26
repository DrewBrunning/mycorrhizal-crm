package services

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeETagValue(t *testing.T) {
	cases := map[string]struct {
		raw  string
		want string
	}{
		"bare":                      {raw: "abc123", want: "abc123"},
		"already quoted":            {raw: `"abc123"`, want: "abc123"},
		"weak quoted":               {raw: `W/"abc123"`, want: "abc123"},
		"weak bare":                 {raw: `W/abc123`, want: "abc123"},
		"lowercase weak":            {raw: `w/"abc123"`, want: "abc123"},
		"surrounding whitespace":    {raw: `  "abc123"  `, want: "abc123"},
		"embedded backslash":        {raw: `a\b`, want: `a\b`},
		"quoted embedded backslash": {raw: `"a\b"`, want: `a\b`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			normalized := normalizeETagValue(tc.raw)
			unquoted, err := strconv.Unquote(normalized)
			require.NoError(t, err, "normalized ETag %q must be unquotable", normalized)
			assert.Equal(t, tc.want, unquoted)
		})
	}
}

func TestDecodeNormalizedETag(t *testing.T) {
	cases := map[string]struct {
		header string
		want   string
	}{
		"empty":             {header: "", want: ""},
		"bare fallback":     {header: "bare", want: "bare"},
		"quoted":            {header: `"quoted"`, want: "quoted"},
		"escaped backslash": {header: `"a\\b"`, want: `a\b`},
		"escaped quote":     {header: `"a\"b"`, want: `a"b`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, decodeNormalizedETag(tc.header))
		})
	}
}

func TestNormalizeGetETagXML(t *testing.T) {
	const body = `<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">
<d:response><d:propstat><d:prop>
<card:address-data>BEGIN:VCARD&#13;&#10;NOTE:a &amp; b&#13;&#10;END:VCARD</card:address-data>
<d:getetag><![CDATA[bare-in-cdata]]></d:getetag>
<d:getetag><![CDATA[W/"weak-in-cdata"]]></d:getetag>
<d:getetag>a&amp;b</d:getetag>
</d:prop></d:propstat></d:response>
</d:multistatus>`

	normalized := normalizeGetETagXML([]byte(body))
	require.NotEqual(t, body, normalized, "the getetag values must have been rewritten")
	assert.Contains(t, string(normalized), "NOTE:a &amp; b", "non-ETag XML escaping must be preserved")

	assert.Equal(t, []string{"bare-in-cdata", "weak-in-cdata", "a&b"}, decodedGetETags(t, normalized))
}

func TestNormalizeGetETagXMLEmptyElements(t *testing.T) {
	const body = `<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">
<d:response><d:href>/addressbooks/test/self-closing.vcf</d:href><d:propstat><d:prop>
<d:getetag/>
</d:prop></d:propstat></d:response>
<d:response><d:href>/addressbooks/test/paired-empty.vcf</d:href><d:propstat><d:prop>
<d:getetag></d:getetag>
</d:prop></d:propstat></d:response>
<d:response><d:href>/addressbooks/test/empty-cdata.vcf</d:href><d:propstat><d:prop>
<d:getetag><![CDATA[]]></d:getetag>
</d:prop></d:propstat></d:response>
<d:response><d:href>/addressbooks/test/weak.vcf</d:href><d:propstat><d:prop>
<d:getetag>W/"weak"</d:getetag>
</d:prop></d:propstat></d:response>
</d:multistatus>`

	normalized := normalizeGetETagXML([]byte(body))
	require.NotEqual(t, body, normalized, "empty getetag elements must have been rewritten")

	assert.Equal(t, []string{"", "", "", "weak"}, decodedGetETags(t, normalized),
		"empty elements must normalize to an empty quoted value the library can unquote")
	assert.NotContains(t, string(normalized), "<d:getetag/>", "a self-closing getetag must become a text element")
	for _, href := range []string{
		"/addressbooks/test/self-closing.vcf",
		"/addressbooks/test/paired-empty.vcf",
		"/addressbooks/test/empty-cdata.vcf",
		"/addressbooks/test/weak.vcf",
	} {
		assert.Contains(t, string(normalized), "<d:href>"+href+"</d:href>", "surrounding XML must be preserved")
	}
}

func TestNormalizeGetETagXMLLeavesMalformedBodyUntouched(t *testing.T) {
	body := []byte(`<d:multistatus><d:getetag>bare</d:main></d:multistatus>`)
	assert.Equal(t, body, normalizeGetETagXML(body))
}

func TestNormalizeGetETagXMLNoETagIsUnchanged(t *testing.T) {
	body := []byte(`<d:multistatus><d:response><d:href>/a.vcf</d:href></d:response></d:multistatus>`)
	assert.Equal(t, body, normalizeGetETagXML(body))
}

func TestNormalizeETagResponseXMLWithoutReportMethod(t *testing.T) {
	body := `<d:multistatus xmlns:d="DAV:"><d:response><d:propstat><d:prop><d:getetag>bare</d:getetag></d:prop></d:propstat></d:response></d:multistatus>`
	resp := &http.Response{
		StatusCode: http.StatusMultiStatus,
		Header: http.Header{
			"Content-Type": []string{"application/xml; charset=utf-8"},
			"Etag":         []string{`W/"header"`},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
	req := httptest.NewRequest(http.MethodGet, "https://remote.example/dav", nil)

	buffered, err := normalizeETagResponse(req, resp, 1024, errors.New("too large"))
	require.NoError(t, err)
	assert.True(t, buffered, "an XML body is buffered even when the request is not a REPORT")

	header, err := strconv.Unquote(resp.Header.Get("ETag"))
	require.NoError(t, err)
	assert.Equal(t, "header", header)
	assert.Equal(t, []string{"bare"}, decodedGetETags(t, mustReadAll(t, resp.Body)))
}

func TestNormalizeETagResponseNonXMLStaysStreaming(t *testing.T) {
	const body = "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/calendar; charset=utf-8"},
			"Etag":         []string{`W/"header"`},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
	req := httptest.NewRequest(http.MethodGet, "https://remote.example/cal.ics", nil)

	buffered, err := normalizeETagResponse(req, resp, 1024, errors.New("too large"))
	require.NoError(t, err)
	assert.False(t, buffered, "a non-XML GET body must not be buffered")
	assert.Equal(t, body, string(mustReadAll(t, resp.Body)), "the body must be passed through untouched")

	header, err := strconv.Unquote(resp.Header.Get("ETag"))
	require.NoError(t, err)
	assert.Equal(t, "header", header, "headers are normalized regardless of body shape")
}

func TestNormalizeETagResponseNilBody(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusNoContent,
		Header:     http.Header{"Etag": []string{"bare"}},
	}
	req := httptest.NewRequest("REPORT", "https://remote.example/dav", nil)

	buffered, err := normalizeETagResponse(req, resp, 1024, errors.New("too large"))
	require.NoError(t, err)
	assert.False(t, buffered)
	assert.Equal(t, `"bare"`, resp.Header.Get("ETag"))
}

func TestNormalizeETagResponseTooLarge(t *testing.T) {
	sizeErr := errors.New("too large")
	resp := &http.Response{
		StatusCode: http.StatusMultiStatus,
		Header:     http.Header{"Content-Type": []string{"application/xml"}},
		Body:       io.NopCloser(strings.NewReader("0123456789")),
	}
	req := httptest.NewRequest("REPORT", "https://remote.example/dav", nil)

	buffered, err := normalizeETagResponse(req, resp, 5, sizeErr)
	assert.False(t, buffered)
	assert.ErrorIs(t, err, sizeErr)
}

func mustReadAll(t *testing.T, body io.ReadCloser) []byte {
	t.Helper()
	defer body.Close()
	b, err := io.ReadAll(body)
	require.NoError(t, err)
	return b
}

type readErrorBody struct{}

func (readErrorBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (readErrorBody) Close() error             { return nil }

type closeErrorBody struct {
	io.Reader
}

func (closeErrorBody) Close() error { return errors.New("close failed") }

func TestNormalizeETagResponseReadError(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusMultiStatus,
		Header:     http.Header{"Content-Type": []string{"application/xml"}},
		Body:       readErrorBody{},
	}
	req := httptest.NewRequest("REPORT", "https://remote.example/dav", nil)

	buffered, err := normalizeETagResponse(req, resp, 1024, errors.New("too large"))
	assert.False(t, buffered)
	assert.ErrorContains(t, err, "read failed")
}

func TestNormalizeETagResponseCloseError(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusMultiStatus,
		Header:     http.Header{"Content-Type": []string{"application/xml"}},
		Body:       closeErrorBody{Reader: strings.NewReader(`<d:multistatus xmlns:d="DAV:"/>`)},
	}
	req := httptest.NewRequest("REPORT", "https://remote.example/dav", nil)

	buffered, err := normalizeETagResponse(req, resp, 1024, errors.New("too large"))
	assert.False(t, buffered)
	assert.ErrorContains(t, err, "close failed")
}

func TestNormalizeETagResponseUpdatesContentLength(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusMultiStatus,
		Header: http.Header{
			"Content-Type":   []string{"application/xml"},
			"Content-Length": []string{"999"},
		},
		Body: io.NopCloser(strings.NewReader(`<d:multistatus xmlns:d="DAV:"><d:response><d:propstat><d:prop><d:getetag>bare</d:getetag></d:prop></d:propstat></d:response></d:multistatus>`)),
	}
	req := httptest.NewRequest("REPORT", "https://remote.example/dav", nil)

	buffered, err := normalizeETagResponse(req, resp, 1024, errors.New("too large"))
	require.NoError(t, err)
	require.True(t, buffered)

	body := mustReadAll(t, resp.Body)
	assert.Equal(t, strconv.Itoa(len(body)), resp.Header.Get("Content-Length"))
	assert.Equal(t, int64(len(body)), resp.ContentLength)
}

func TestNormalizeGetETagXMLContainsNameButNoElement(t *testing.T) {
	body := []byte(`<d:multistatus xmlns:d="DAV:"><d:note>getetag</d:note></d:multistatus>`)
	assert.Equal(t, body, normalizeGetETagXML(body))
}
