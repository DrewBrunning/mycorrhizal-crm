package imaging

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// exifOrientedJPEG re-wraps a plain JPEG with an EXIF block carrying the given
// orientation (same construction TestProcess_AppliesEXIFOrientationAndCaptureTime uses).
func exifOrientedJPEG(tb testing.TB, plain []byte, orientation uint16) []byte {
	tb.Helper()
	tiff := buildTIFF(binary.LittleEndian, orientation, "2024:07:03 10:11:12", "+02:00")
	app1 := bytes.NewBuffer([]byte{0xFF, 0xE1})
	_ = binary.Write(app1, binary.BigEndian, uint16(2+len("Exif\x00\x00")+len(tiff)))
	app1.WriteString("Exif\x00\x00")
	app1.Write(tiff)
	out := append([]byte{}, plain[:2]...)
	out = append(out, app1.Bytes()...)
	return append(out, plain[2:]...)
}

// texturedJPEG is a deterministic, asymmetric, noisy image so every
// orientation and every dHash bit is distinguishable.
func texturedJPEG(tb testing.TB, w, h int) []byte {
	tb.Helper()
	rng := rand.New(rand.NewSource(42)) // #nosec G404 -- deterministic test fixture
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x*255/w + rng.Intn(40)) % 256),
				G: uint8((y*255/h + rng.Intn(40)) % 256),
				B: uint8(((x ^ y) * 3) % 256),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	require.NoError(tb, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92}))
	return buf.Bytes()
}

// Process's stored dHash must equal the dHash of the decoded, fully oriented
// raster for every EXIF orientation (issue #2313: stored hashes must not drift
// when orient moves after the resize), and the stored dimensions are the
// oriented full-resolution ones.
func TestProcess_DHashAndDimensionsMatchFullOrientedRaster(t *testing.T) {
	const w, h = 300, 200
	plain := texturedJPEG(t, w, h)
	decoded, err := jpeg.Decode(bytes.NewReader(plain))
	require.NoError(t, err)
	for o := uint16(1); o <= 8; o++ {
		res, err := Process(bytes.NewReader(exifOrientedJPEG(t, plain, o)), DefaultLimits)
		require.NoError(t, err, "orientation %d", o)
		want := differenceHash(orient(decoded, int(o)))
		assert.Equal(t, want, res.DHash, "dHash drifted for orientation %d", o)
		if o >= 5 {
			assert.Equal(t, h, res.Width, "orientation %d width", o)
			assert.Equal(t, w, res.Height, "orientation %d height", o)
		} else {
			assert.Equal(t, w, res.Width)
			assert.Equal(t, h, res.Height)
		}
	}
}

// Stored variants must come out oriented and capped on the oriented long
// edge, including for orientations that swap axes and for images large enough
// to be resized.
func TestProcess_OrientedVariantsDimensions(t *testing.T) {
	plain := texturedJPEG(t, 3000, 1500)
	for o := uint16(1); o <= 8; o++ {
		res, err := Process(bytes.NewReader(exifOrientedJPEG(t, plain, o)), DefaultLimits)
		require.NoError(t, err)
		full, err := jpeg.Decode(bytes.NewReader(res.Full))
		require.NoError(t, err)
		thumb, err := jpeg.Decode(bytes.NewReader(res.Thumb))
		require.NoError(t, err)
		if o >= 5 { // portrait after orientation
			assert.Equal(t, MaxLongEdge, full.Bounds().Dy(), "o=%d", o)
			assert.Equal(t, 1024, full.Bounds().Dx(), "o=%d", o)
			assert.Equal(t, ThumbLongEdge, thumb.Bounds().Dy(), "o=%d", o)
			assert.Equal(t, 160, thumb.Bounds().Dx(), "o=%d", o)
		} else {
			assert.Equal(t, MaxLongEdge, full.Bounds().Dx(), "o=%d", o)
			assert.Equal(t, 1024, full.Bounds().Dy(), "o=%d", o)
			assert.Equal(t, ThumbLongEdge, thumb.Bounds().Dx(), "o=%d", o)
			assert.Equal(t, 160, thumb.Bounds().Dy(), "o=%d", o)
		}
	}
}

// Pixel content: orientation 6 rotates the picture 90 degrees clockwise, so
// the source top-left lands at the oriented top-right.
func TestProcess_Orientation6PixelsAreRotated(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			c := color.RGBA{R: 20, G: 20, B: 20, A: 255}
			if x < 10 && y < 10 { // top-left red block
				c = color.RGBA{R: 255, A: 255}
			}
			src.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, src, &jpeg.Options{Quality: 100}))
	res, err := Process(bytes.NewReader(exifOrientedJPEG(t, buf.Bytes(), 6)), DefaultLimits)
	require.NoError(t, err)
	out, err := jpeg.Decode(bytes.NewReader(res.Full))
	require.NoError(t, err)
	assert.Equal(t, 20, out.Bounds().Dx())
	assert.Equal(t, 40, out.Bounds().Dy())
	r, g, _, _ := out.At(15, 4).RGBA() // top-right of the 20x40 result
	assert.Greater(t, r>>8, uint32(200))
	assert.Less(t, g>>8, uint32(80))
	r, _, _, _ = out.At(4, 4).RGBA()
	assert.Less(t, r>>8, uint32(80))
}

// BenchmarkProcess_Orientation6_6000x4000 is the #2313 probe: a 24 MP phone
// photo with EXIF orientation 6, the case that used to copy the whole raster
// pixel-by-pixel through img.At/dst.Set before resizing.
func BenchmarkProcess_Orientation6_6000x4000(b *testing.B) {
	data := exifOrientedJPEG(b, encodeJPEG(b, 6000, 4000), 6)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Process(bytes.NewReader(data), DefaultLimits); err != nil {
			b.Fatal(err)
		}
	}
}

// An oriented photo must not pay a per-pixel copy of the full raster: Process
// on a 2400x1600 (3.8 MP) orientation-6 JPEG used to make ~7.7M allocations
// (a boxed color.Color for every At and Set in orient). The oriented dHash
// still reads each pixel once through a lazy view (~1 boxed At per pixel), so
// the budget sits between the two.
func TestProcess_OrientedPhotoDoesNotCopyFullRaster(t *testing.T) {
	data := exifOrientedJPEG(t, encodeJPEG(t, 2400, 1600), 6)
	allocs := testing.AllocsPerRun(2, func() {
		_, err := Process(bytes.NewReader(data), DefaultLimits)
		require.NoError(t, err)
	})
	assert.Less(t, allocs, 5_500_000.0, "orient must run on the resized outputs, not the decoded raster")
}
