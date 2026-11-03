package photostore

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emersion/go-vcard"
)

// testPNGBase64 is a 1x1 transparent PNG, small enough to embed inline in
// tests. Same fixture used by services/contact_sync_service_test.go's
// testCardWithPhoto for the same reason.
const testPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

func decodeTestPNG(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(testPNGBase64)
	if err != nil {
		t.Fatalf("failed to decode test fixture PNG: %v", err)
	}
	return data
}

// --- SaveContactPhoto ---

func TestSaveContactPhoto(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	data := decodeTestPNG(t)

	photoPath, thumbnail, err := SaveContactPhoto(data, "image/png", dir)
	if err != nil {
		t.Fatalf("SaveContactPhoto returned error: %v", err)
	}
	if photoPath == "" {
		t.Fatal("expected a non-empty photo path")
	}
	if !strings.HasSuffix(photoPath, "_photo.jpg") {
		t.Errorf("photoPath = %q, want a _photo.jpg suffix", photoPath)
	}
	if !strings.HasPrefix(thumbnail, "data:image/jpeg;base64,") {
		t.Errorf("thumbnail = %q, want a data:image/jpeg;base64, prefix", thumbnail)
	}

	// The photo must actually be written to disk under photoDir, and must be
	// a valid re-encoded JPEG (SaveContactPhoto always re-encodes to JPEG
	// regardless of the input format).
	fullPath := filepath.Join(dir, photoPath)
	fileBytes, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("photo was not written to disk at %q: %v", fullPath, err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(fileBytes)); err != nil {
		t.Errorf("saved photo is not a valid JPEG: %v", err)
	}

	// The thumbnail payload must itself decode to a valid JPEG.
	thumbData := strings.TrimPrefix(thumbnail, "data:image/jpeg;base64,")
	decodedThumb, err := base64.StdEncoding.DecodeString(thumbData)
	if err != nil {
		t.Fatalf("thumbnail is not valid base64: %v", err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(decodedThumb)); err != nil {
		t.Errorf("thumbnail is not a valid JPEG: %v", err)
	}
}

func TestSaveContactPhoto_EmptyInputIsNoop(t *testing.T) {
	t.Parallel()
	photoPath, thumbnail, err := SaveContactPhoto(nil, "", t.TempDir())
	if err != nil {
		t.Fatalf("SaveContactPhoto(nil) returned error: %v", err)
	}
	if photoPath != "" || thumbnail != "" {
		t.Errorf("SaveContactPhoto(nil) = (%q, %q), want empty strings", photoPath, thumbnail)
	}
}

func TestSaveContactPhoto_InvalidImageDataErrors(t *testing.T) {
	t.Parallel()
	_, _, err := SaveContactPhoto([]byte("this is not an image"), "", t.TempDir())
	if err == nil {
		t.Error("expected an error decoding non-image bytes, got nil")
	}
}

// --- ReadContactPhoto / ReadContactPhotoDataURI ---

func TestReadContactPhotoRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	data := decodeTestPNG(t)

	photoPath, _, err := SaveContactPhoto(data, "image/png", dir)
	if err != nil {
		t.Fatalf("SaveContactPhoto returned error: %v", err)
	}

	onDisk, err := os.ReadFile(filepath.Join(dir, photoPath))
	if err != nil {
		t.Fatalf("failed to read saved photo back directly: %v", err)
	}

	got, mediaType := ReadContactPhoto(photoPath, "", dir)
	if mediaType != "image/jpeg" {
		t.Errorf("mediaType = %q, want image/jpeg", mediaType)
	}
	decoded, err := base64.StdEncoding.DecodeString(got)
	if err != nil {
		t.Fatalf("ReadContactPhoto returned invalid base64: %v", err)
	}
	if !bytes.Equal(decoded, onDisk) {
		t.Error("ReadContactPhoto bytes do not match the bytes actually on disk")
	}
}

func TestReadContactPhoto_FallsBackToThumbnailWhenFileMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	thumbnail := "data:image/jpeg;base64,Zm9vYmFy" // "foobar"

	got, mediaType := ReadContactPhoto("does-not-exist_photo.jpg", thumbnail, dir)
	if mediaType != "image/jpeg" {
		t.Errorf("mediaType = %q, want image/jpeg", mediaType)
	}
	if got != "Zm9vYmFy" {
		t.Errorf("got = %q, want raw base64 payload from thumbnail", got)
	}
}

func TestReadContactPhoto_NoPhotoAvailable(t *testing.T) {
	t.Parallel()
	got, mediaType := ReadContactPhoto("", "", "")
	if got != "" || mediaType != "" {
		t.Errorf("ReadContactPhoto with nothing set = (%q, %q), want empty strings", got, mediaType)
	}
}

func TestReadContactPhotoDataURI(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	data := decodeTestPNG(t)
	photoPath, _, err := SaveContactPhoto(data, "image/png", dir)
	if err != nil {
		t.Fatalf("SaveContactPhoto returned error: %v", err)
	}

	uri, mediaType := ReadContactPhotoDataURI(photoPath, "", dir)
	if mediaType != "image/jpeg" {
		t.Errorf("mediaType = %q, want image/jpeg", mediaType)
	}
	if !strings.HasPrefix(uri, "data:image/jpeg;base64,") {
		t.Errorf("uri = %q, want a data:image/jpeg;base64, prefix", uri)
	}
}

func TestReadContactPhotoDataURI_NoPhotoAvailable(t *testing.T) {
	t.Parallel()
	uri, mediaType := ReadContactPhotoDataURI("", "", "")
	if uri != "" || mediaType != "" {
		t.Errorf("ReadContactPhotoDataURI with nothing set = (%q, %q), want empty strings", uri, mediaType)
	}
}

// --- ExtractPhotoData ---

func TestExtractPhotoData_Nil(t *testing.T) {
	t.Parallel()
	data, mediaType, photoURL := ExtractPhotoData(nil)
	if data != nil || mediaType != "" || photoURL != "" {
		t.Errorf("ExtractPhotoData(nil) = (%v, %q, %q), want all zero values", data, mediaType, photoURL)
	}
}

func TestExtractPhotoData_InlineBase64WithTypeParam(t *testing.T) {
	t.Parallel()
	data := decodeTestPNG(t)
	field := &vcard.Field{
		Value:  base64.StdEncoding.EncodeToString(data),
		Params: vcard.Params{"TYPE": []string{"PNG"}},
	}

	got, mediaType, photoURL := ExtractPhotoData(field)
	if photoURL != "" {
		t.Errorf("photoURL = %q, want empty for inline data", photoURL)
	}
	if mediaType != "image/png" {
		t.Errorf("mediaType = %q, want image/png (derived from TYPE=PNG)", mediaType)
	}
	if !bytes.Equal(got, data) {
		t.Error("decoded photo bytes do not match the original inline base64 payload")
	}
}

func TestExtractPhotoData_DataURIForm(t *testing.T) {
	t.Parallel()
	data := decodeTestPNG(t)
	field := &vcard.Field{
		Value: "data:image/png;base64," + base64.StdEncoding.EncodeToString(data),
	}

	got, mediaType, photoURL := ExtractPhotoData(field)
	if photoURL != "" {
		t.Errorf("photoURL = %q, want empty for a data: URI", photoURL)
	}
	if mediaType != "image/png" {
		t.Errorf("mediaType = %q, want image/png (parsed out of the data: URI)", mediaType)
	}
	if !bytes.Equal(got, data) {
		t.Error("decoded photo bytes do not match the original data: URI payload")
	}
}

func TestExtractPhotoData_ReferencedURI(t *testing.T) {
	t.Parallel()
	field := &vcard.Field{Value: "https://example.com/photo.jpg"}

	data, _, photoURL := ExtractPhotoData(field)
	if data != nil {
		t.Errorf("expected nil data for a referenced (non-embedded) photo URL, got %v", data)
	}
	if photoURL != "https://example.com/photo.jpg" {
		t.Errorf("photoURL = %q, want the original https URL", photoURL)
	}
}

// --- FetchPhotoFromURL: SSRF protection ---
//
// FetchPhotoFromURL delegates to httputil.FetchImageFromURL, which blocks
// loopback/private/link-local targets both by hostname (a fixed blocklist
// checked before any DNS lookup) and by resolved IP (net.LookupIP on an IP
// literal parses it directly, no real network access needed) — the same
// SSRF-protection shape services/calendar_sync_service.go's
// privateBlockingDialContext documents for the CalDAV sync client. These
// cases are all resolved without any real DNS/network call, so they run
// deterministically in this test.

func TestFetchPhotoFromURL_BlocksLocalhostHostname(t *testing.T) {
	t.Parallel()
	if _, _, err := FetchPhotoFromURL(context.Background(), "http://localhost/photo.jpg"); err == nil {
		t.Error("expected fetching from localhost to be rejected as an SSRF target")
	}
}

func TestFetchPhotoFromURL_BlocksLoopbackIPv4Literal(t *testing.T) {
	t.Parallel()
	if _, _, err := FetchPhotoFromURL(context.Background(), "http://127.0.0.1/photo.jpg"); err == nil {
		t.Error("expected fetching from 127.0.0.1 to be rejected as an SSRF target")
	}
}

func TestFetchPhotoFromURL_BlocksLoopbackIPv6Literal(t *testing.T) {
	t.Parallel()
	if _, _, err := FetchPhotoFromURL(context.Background(), "http://[::1]/photo.jpg"); err == nil {
		t.Error("expected fetching from [::1] to be rejected as an SSRF target")
	}
}

func TestFetchPhotoFromURL_BlocksPrivateLANAddress(t *testing.T) {
	t.Parallel()
	// 192.168.0.0/16 is not in the hardcoded hostname blocklist at all -- this
	// proves the resolved-IP check (ip.IsPrivate()), not just the hostname
	// string list, actually rejects it.
	if _, _, err := FetchPhotoFromURL(context.Background(), "http://192.168.1.1/photo.jpg"); err == nil {
		t.Error("expected fetching from a private LAN address (192.168.1.1) to be rejected")
	}
}

func TestFetchPhotoFromURL_BlocksLinkLocalMetadataAddress(t *testing.T) {
	t.Parallel()
	// 169.254.169.254 is the classic cloud-metadata SSRF target (AWS/GCP/Azure
	// instance metadata service); ip.IsLinkLocalUnicast() must catch it.
	if _, _, err := FetchPhotoFromURL(context.Background(), "http://169.254.169.254/latest/meta-data/"); err == nil {
		t.Error("expected fetching from the link-local metadata address to be rejected")
	}
}

func TestFetchPhotoFromURL_RejectsNonHTTPScheme(t *testing.T) {
	t.Parallel()
	if _, _, err := FetchPhotoFromURL(context.Background(), "file:///etc/passwd"); err == nil {
		t.Error("expected a non-http(s) scheme to be rejected")
	}
}

func TestFetchPhotoFromURL_RejectsUnparsableURL(t *testing.T) {
	t.Parallel()
	// A URL containing a raw control character fails url.Parse itself.
	_, err := url.Parse("http://\x7f")
	if err == nil {
		t.Skip("test environment's url.Parse unexpectedly accepted the malformed URL")
	}
	if _, _, err := FetchPhotoFromURL(context.Background(), "http://\x7f"); err == nil {
		t.Error("expected an unparsable URL to be rejected")
	}
}

// --- DecodePhotoURI ---

func TestDecodePhotoURI_EmptyValue(t *testing.T) {
	t.Parallel()
	data, mediaType, url := DecodePhotoURI("", "")
	if data != nil || mediaType != "" || url != "" {
		t.Errorf("DecodePhotoURI(\"\") = (%v, %q, %q), want all zero", data, mediaType, url)
	}
}

func TestDecodePhotoURI_HTTPURLWithMediaTypeHint(t *testing.T) {
	t.Parallel()
	data, mediaType, photoURL := DecodePhotoURI("https://example.com/photo.jpg", "image/jpeg")
	if data != nil {
		t.Error("expected nil data for an HTTP URL")
	}
	if photoURL != "https://example.com/photo.jpg" {
		t.Errorf("photoURL = %q, want https://example.com/photo.jpg", photoURL)
	}
	if mediaType != "image/jpeg" {
		t.Errorf("mediaType = %q, want image/jpeg (media type hint passed through)", mediaType)
	}
}

func TestDecodePhotoURI_HTTPURLWithWhitespaceStripping(t *testing.T) {
	t.Parallel()
	// Simulates Google VCF format with embedded whitespace.
	dirty := "https://example.com/pho\r\nto.jpg"
	data, _, photoURL := DecodePhotoURI(dirty, "")
	if data != nil {
		t.Error("expected nil data for a whitespace-cleaned HTTP URL")
	}
	if photoURL != "https://example.com/photo.jpg" {
		t.Errorf("photoURL = %q, want whitespace-stripped https://example.com/photo.jpg", photoURL)
	}
}

func TestDecodePhotoURI_RawBase64WithMediaTypeHint(t *testing.T) {
	t.Parallel()
	pngData := decodeTestPNG(t)
	b64 := base64.StdEncoding.EncodeToString(pngData)

	data, mediaType, photoURL := DecodePhotoURI(b64, "image/png")
	if photoURL != "" {
		t.Errorf("photoURL = %q, want empty for raw base64", photoURL)
	}
	if mediaType != "image/png" {
		t.Errorf("mediaType = %q, want image/png from mediaTypeHint", mediaType)
	}
	if !bytes.Equal(data, pngData) {
		t.Error("decoded data does not match the original PNG bytes")
	}
}

func TestDecodePhotoURI_DataURIWithMediaTypeExtraction(t *testing.T) {
	t.Parallel()
	pngData := decodeTestPNG(t)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngData)

	data, mediaType, photoURL := DecodePhotoURI(uri, "")
	if photoURL != "" {
		t.Errorf("photoURL = %q, want empty for data: URI", photoURL)
	}
	if mediaType != "image/png" {
		t.Errorf("mediaType = %q, want image/png from data: URI prefix", mediaType)
	}
	if !bytes.Equal(data, pngData) {
		t.Error("decoded data does not match original bytes embedded in data: URI")
	}
}

func TestDecodePhotoURI_DataURIPrefersMediaTypeHintOverEmbedded(t *testing.T) {
	t.Parallel()
	pngData := decodeTestPNG(t)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngData)

	// When mediaTypeHint is provided, it should take precedence over the
	// data URI's embedded media type.
	data, mediaType, _ := DecodePhotoURI(uri, "image/jpeg")
	if mediaType != "image/jpeg" {
		t.Errorf("mediaType = %q, want image/jpeg from hint (takes precedence)", mediaType)
	}
	if !bytes.Equal(data, pngData) {
		t.Error("decoded data does not match original bytes")
	}
}

func TestDecodePhotoURI_InvalidBase64(t *testing.T) {
	t.Parallel()
	// Raw string that is not valid base64 — must return nil data, not an error.
	data, mediaType, photoURL := DecodePhotoURI("!!!not-valid-base64!!!", "image/png")
	if data != nil {
		t.Errorf("expected nil data for invalid base64, got %v", data)
	}
	if mediaType != "" || photoURL != "" {
		t.Errorf("expected empty mediaType/photoURL for invalid base64, got (%q, %q)", mediaType, photoURL)
	}
}

func TestDecodePhotoURI_MalformedDataURI(t *testing.T) {
	t.Parallel()
	// A data: URI with no comma separator — SplitN returns one part.
	data, _, _ := DecodePhotoURI("data:bare-no-comma", "")
	if data != nil {
		t.Error("expected nil data for a data URI with no comma separator")
	}
}

// --- cropToSquare ---

func TestCropToSquare_AlreadySquare(t *testing.T) {
	t.Parallel()
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	result := cropToSquare(img)
	bounds := result.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 100 {
		t.Errorf("cropToSquare(square) = %dx%d, want 100x100", bounds.Dx(), bounds.Dy())
	}
}

func TestCropToSquare_WiderThanTall(t *testing.T) {
	t.Parallel()
	// 200x100 — should crop to 100x100 centered horizontally.
	img := image.NewRGBA(image.Rect(0, 0, 200, 100))
	result := cropToSquare(img)
	bounds := result.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 100 {
		t.Errorf("cropToSquare(200x100) = %dx%d, want 100x100", bounds.Dx(), bounds.Dy())
	}
}

func TestCropToSquare_TallerThanWide(t *testing.T) {
	t.Parallel()
	// 100x200 — should crop to 100x100 centered vertically.
	img := image.NewRGBA(image.Rect(0, 0, 100, 200))
	result := cropToSquare(img)
	bounds := result.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 100 {
		t.Errorf("cropToSquare(100x200) = %dx%d, want 100x100", bounds.Dx(), bounds.Dy())
	}
}

// --- SaveContactPhoto: JPEG input and downscale path ---

func TestSaveContactPhoto_JPEGInput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("failed to encode test JPEG: %v", err)
	}

	photoPath, thumbnail, err := SaveContactPhoto(buf.Bytes(), "image/jpeg", dir)
	if err != nil {
		t.Fatalf("SaveContactPhoto(jpeg) returned error: %v", err)
	}
	if photoPath == "" {
		t.Fatal("expected a non-empty photo path")
	}
	if !strings.HasPrefix(thumbnail, "data:image/jpeg;base64,") {
		t.Errorf("thumbnail = %q, want a data:image/jpeg;base64, prefix", thumbnail)
	}
}

func TestSaveContactPhoto_DownscaleWhenLargerThanMax(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	img := image.NewRGBA(image.Rect(0, 0, 500, 500))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("failed to encode test JPEG: %v", err)
	}

	photoPath, _, err := SaveContactPhoto(buf.Bytes(), "image/jpeg", dir)
	if err != nil {
		t.Fatalf("SaveContactPhoto(large) returned error: %v", err)
	}

	fullPath := filepath.Join(dir, photoPath)
	fileBytes, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("photo was not written to disk: %v", err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(fileBytes))
	if err != nil {
		t.Fatalf("saved photo is not a valid JPEG: %v", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() > 400 || bounds.Dy() > 400 {
		t.Errorf("downscaled photo is %dx%d, want at most 400 on each side", bounds.Dx(), bounds.Dy())
	}
}

// --- ExtractPhotoData: edge cases ---

func TestExtractPhotoData_EmptyValue(t *testing.T) {
	t.Parallel()
	field := &vcard.Field{Value: ""}
	data, mediaType, photoURL := ExtractPhotoData(field)
	if data != nil || mediaType != "" || photoURL != "" {
		t.Errorf("ExtractPhotoData with empty Value = (%v, %q, %q), want all zero", data, mediaType, photoURL)
	}
}

func TestExtractPhotoData_MEDIATYPEParam(t *testing.T) {
	t.Parallel()
	data := decodeTestPNG(t)
	field := &vcard.Field{
		Value:  base64.StdEncoding.EncodeToString(data),
		Params: vcard.Params{"MEDIATYPE": []string{"image/png"}},
	}

	got, mediaType, _ := ExtractPhotoData(field)
	if mediaType != "image/png" {
		t.Errorf("mediaType = %q, want image/png (from MEDIATYPE vCard 4 param)", mediaType)
	}
	if !bytes.Equal(got, data) {
		t.Error("decoded photo bytes do not match original PNG data")
	}
}

func TestRemoveContactPhoto(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.jpg")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := RemoveContactPhoto(dir, "p.jpg"); err != nil {
		t.Fatalf("remove existing: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file should be gone, stat err = %v", err)
	}
	if err := RemoveContactPhoto(dir, "p.jpg"); err != nil {
		t.Fatalf("missing file must not be an error: %v", err)
	}
}

func TestRemoveContactPhoto_RejectsNonBasenames(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "photos")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"", "../keep.txt", "sub/p.jpg"} {
		if err := RemoveContactPhoto(dir, name); err != nil {
			t.Fatalf("name %q: %v", name, err)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("path traversal deleted a file outside photoDir: %v", err)
	}
}

func TestRemoveContactPhoto_PropagatesRemoveError(t *testing.T) {
	dir := t.TempDir()
	// A non-empty directory can't be os.Remove'd and isn't "not exist".
	sub := filepath.Join(dir, "d")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "f"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveContactPhoto(dir, "d"); err == nil {
		t.Fatal("expected error removing a non-empty directory")
	}
}

// --- applyCrop / SaveContactPhotoCropped ---

func TestApplyCrop_SubImagePath(t *testing.T) {
	t.Parallel()
	img := image.NewRGBA(image.Rect(0, 0, 100, 200))
	got := applyCrop(img, CropRect{X: 0.1, Y: 0.25, W: 0.5, H: 0.5})
	b := got.Bounds()
	if b.Dx() != 50 || b.Dy() != 100 {
		t.Errorf("applyCrop = %dx%d, want 50x100", b.Dx(), b.Dy())
	}
	if b.Min.X != 10 || b.Min.Y != 50 {
		t.Errorf("applyCrop origin = (%d,%d), want (10,50)", b.Min.X, b.Min.Y)
	}
}

func TestApplyCrop_ClampsToBounds(t *testing.T) {
	t.Parallel()
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	got := applyCrop(img, CropRect{X: 0.5, Y: 0.5, W: 1, H: 1})
	b := got.Bounds()
	if b.Dx() != 50 || b.Dy() != 50 {
		t.Errorf("applyCrop clamped = %dx%d, want 50x50", b.Dx(), b.Dy())
	}
}

func TestApplyCrop_DegenerateLeavesImageUntouched(t *testing.T) {
	t.Parallel()
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for _, c := range []CropRect{{W: 0, H: 0.5}, {W: 0.5, H: 0}, {X: 2, Y: 2, W: 0.5, H: 0.5}} {
		if got := applyCrop(img, c); got.Bounds() != img.Bounds() {
			t.Errorf("applyCrop(%+v) = %v, want unchanged %v", c, got.Bounds(), img.Bounds())
		}
	}
}

// nonSubImager has no SubImage method, forcing the pixel-copy fallback.
type nonSubImager struct{ image.Image }

func TestApplyCrop_FallbackCopyPath(t *testing.T) {
	t.Parallel()
	src := image.NewRGBA(image.Rect(0, 0, 40, 40))
	got := applyCrop(nonSubImager{src}, CropRect{X: 0.25, Y: 0.25, W: 0.5, H: 0.5})
	b := got.Bounds()
	if b.Dx() != 20 || b.Dy() != 20 || b.Min.X != 0 || b.Min.Y != 0 {
		t.Errorf("applyCrop fallback bounds = %v, want 20x20 at origin", b)
	}
}

func TestSaveContactPhotoCropped_AppliesCrop(t *testing.T) {
	t.Parallel()
	// Left half red, right half blue. Cropping the left half must yield a red
	// avatar; without the crop the centered square would be mixed.
	src := image.NewRGBA(image.Rect(0, 0, 300, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 300; x++ {
			if x < 150 {
				src.Set(x, y, color.RGBA{R: 255, A: 255})
			} else {
				src.Set(x, y, color.RGBA{B: 255, A: 255})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path, thumb, err := SaveContactPhotoCropped(buf.Bytes(), "image/png", &CropRect{X: 0, Y: 0, W: 0.5, H: 1}, dir)
	if err != nil {
		t.Fatalf("SaveContactPhotoCropped: %v", err)
	}
	if thumb == "" {
		t.Error("expected a thumbnail")
	}
	data, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		t.Fatalf("photo file missing: %v", err)
	}
	out, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode saved photo: %v", err)
	}
	b := out.Bounds()
	for _, pt := range []image.Point{{b.Min.X + 2, b.Min.Y + 2}, {b.Max.X - 3, b.Max.Y - 3}, {b.Min.X + b.Dx()/2, b.Min.Y + b.Dy()/2}} {
		r, _, bl, _ := out.At(pt.X, pt.Y).RGBA()
		if r>>8 < 200 || bl>>8 > 60 {
			t.Errorf("pixel %v = r%d b%d, want red (crop not applied)", pt, r>>8, bl>>8)
		}
	}
}
