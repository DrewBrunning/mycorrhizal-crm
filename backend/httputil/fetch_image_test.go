package httputil

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fetchImageWithClient: response contract ---
//
// fetchImageWithClient is the layer that turns an allowed URL into bytes; it
// carries no SSRF logic of its own (that lives in validateURLForSSRF and the
// client's dialer/redirect policy), so a loopback httptest server is a
// legitimate stand-in for the remote image host here — provided the client is
// a *plain* one. buildImageClient() would reject the loopback dial outright;
// that guard is exercised separately.

func plainTestClient() *http.Client {
	return &http.Client{}
}

func TestFetchImageWithClient_Success(t *testing.T) {
	t.Parallel()
	var gotUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte{0x89, 'P', 'N', 'G'})
	}))
	defer server.Close()

	data, contentType, err := fetchImageWithClient(server.URL+"/photo.png", plainTestClient())
	require.NoError(t, err)
	assert.Equal(t, []byte{0x89, 'P', 'N', 'G'}, data)
	assert.Equal(t, "image/png", contentType)
	assert.Equal(t, "Mozilla/5.0 (compatible; MycorrhizalCRM/1.0)", gotUA,
		"requests must carry the MycorrhizalCRM user agent so image hosts don't block us")
}

func TestFetchImageWithClient_Non200Status(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("nope"))
	}))
	defer server.Close()

	_, _, err := fetchImageWithClient(server.URL+"/missing.jpg", plainTestClient())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote server returned 404 Not Found")
}

func TestFetchImageWithClient_NonImageContentType(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html></html>"))
	}))
	defer server.Close()

	_, _, err := fetchImageWithClient(server.URL+"/page.html", plainTestClient())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "URL does not point to an image")
}

func TestFetchImageWithClient_ContentTypePrefixOnly(t *testing.T) {
	t.Parallel()
	// The check is a prefix match ("image/"), so an image type with parameters
	// must still be accepted.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg; charset=binary")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte{0xff, 0xd8})
	}))
	defer server.Close()

	data, contentType, err := fetchImageWithClient(server.URL+"/p.jpg", plainTestClient())
	require.NoError(t, err)
	assert.Equal(t, []byte{0xff, 0xd8}, data)
	assert.Equal(t, "image/jpeg; charset=binary", contentType)
}

func TestFetchImageWithClient_TooLarge(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		w.Write(make([]byte, maxImageSize+1))
	}))
	defer server.Close()

	_, _, err := fetchImageWithClient(server.URL+"/big.png", plainTestClient())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image is too large")
}

func TestFetchImageWithClient_ExactlyAtLimitIsAccepted(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		w.Write(make([]byte, maxImageSize))
	}))
	defer server.Close()

	data, _, err := fetchImageWithClient(server.URL+"/max.png", plainTestClient())
	require.NoError(t, err)
	assert.Len(t, data, maxImageSize, "a body exactly at the limit must be accepted")
}

// --- redirect policy ---

func TestBuildImageClient_RedirectPolicyRejectsDisallowedTarget(t *testing.T) {
	t.Parallel()
	client := buildImageClient()
	via := []*http.Request{}

	// A redirect to the cloud-metadata endpoint must be rejected by the
	// re-validation, even though it would only ever be reached via a redirect.
	req, err := http.NewRequest("GET", "http://169.254.169.254/latest/meta-data/", nil)
	require.NoError(t, err)

	err = client.CheckRedirect(req, via)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redirect to disallowed location")
}

func TestBuildImageClient_RedirectPolicyCapsHops(t *testing.T) {
	t.Parallel()
	client := buildImageClient()

	// A public target that passes validation with 3 prior hops must be
	// rejected purely on hop count.
	req, err := http.NewRequest("GET", "http://93.184.216.34/photo.jpg", nil)
	require.NoError(t, err)
	via := make([]*http.Request, 3)

	err = client.CheckRedirect(req, via)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many redirects")
}

func TestBuildImageClient_RedirectPolicyAllowsPublicTarget(t *testing.T) {
	t.Parallel()
	client := buildImageClient()
	req, err := http.NewRequest("GET", "http://93.184.216.34/photo.jpg", nil)
	require.NoError(t, err)
	via := make([]*http.Request, 1)

	assert.NoError(t, client.CheckRedirect(req, via), "a redirect to a public address within the hop limit must be allowed")
}

func TestBuildImageClient_TransportIsSSRFGuarded(t *testing.T) {
	t.Parallel()
	// The transport the fetch client dials through must be the SSRF-guarded
	// dialer — a test can't reach the real network, but it can prove the
	// dialer rejects a loopback target outright, which is exactly the guard
	// that would have to be defeated for a redirect to land inward.
	transport, ok := buildImageClient().Transport.(*http.Transport)
	require.True(t, ok, "client transport must be *http.Transport")
	assert.NotNil(t, transport.DialContext)

	// Sanity: SafeDialContext returns the private-address sentinel for a
	// loopback literal (no real network access needed).
	_, dialErr := transport.DialContext(t.Context(), "tcp", "127.0.0.1:9999")
	require.Error(t, dialErr)
	assert.Contains(t, dialErr.Error(), "internal IP addresses")
}

// --- sanitizeURL ---

func TestSanitizeURL(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "http://example.com/photo.jpg", sanitizeURL("http://example.com/photo.jpg"))
	assert.Equal(t, "http://example.com/photo.jpg", sanitizeURL(" http ://example.com/photo.jpg "))
	assert.Equal(t, "http://example.com/photo.jpg", sanitizeURL("http://example.com/pho\r\nto.jpg"))
	assert.Equal(t, "http://example.com/photo.jpg", sanitizeURL("http://example.com/photo.jpg\n"))
}

func TestSanitizeURL_EmptyInput(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", sanitizeURL(""))
}

// --- FetchImageFromURL end-to-end via a public-looking host ---
//
// FetchImageFromURL rejects loopback outright, so a plain httptest server
// cannot stand in for the image host. These cases instead prove the wiring:
// a URL that fails validation never reaches the client, and a whitespace-dirty
// URL is cleaned before validation. (The response-contract branches above are
// exercised directly via fetchImageWithClient.)

func TestFetchImageFromURL_StillRejectsLoopbackAfterRefactor(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must never be reached for a loopback URL")
	}))
	defer server.Close()

	_, _, err := FetchImageFromURL(server.URL + "/photo.jpg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal hosts")
}

func TestFetchImageFromURL_CleanedURLPassesToResponseContract(t *testing.T) {
	t.Parallel()
	// Not a real fetch: the URL fails SSRF validation, but the error must be
	// the *cleaned* URL's error (loopback rejection), proving sanitization ran
	// before validation on the full pipeline.
	_, _, err := FetchImageFromURL("http://127.0.0.1/pho\r\nto.jpg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal hosts")
}
