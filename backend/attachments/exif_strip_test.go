package attachments

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tinyJPEG returns a minimal valid encoded JPEG.
func tinyJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 60), G: uint8(y * 60), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

// spliceJPEGAPP1 inserts an APP1 segment with the given 6-byte signature (plus
// filler payload) immediately after a JPEG's SOI marker.
func spliceJPEGAPP1(t *testing.T, jpegBytes []byte, signature string, payload []byte) []byte {
	t.Helper()
	require.True(t, len(jpegBytes) >= 2 && jpegBytes[0] == 0xFF && jpegBytes[1] == 0xD8)

	body := append([]byte(signature), payload...)
	segLen := len(body) + 2 // length field includes itself
	var seg bytes.Buffer
	seg.WriteByte(0xFF)
	seg.WriteByte(0xE1)
	require.LessOrEqual(t, segLen, 0xFFFF)
	lenBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBytes, uint16(segLen))
	seg.Write(lenBytes)
	seg.Write(body)

	out := make([]byte, 0, len(jpegBytes)+seg.Len())
	out = append(out, jpegBytes[:2]...)
	out = append(out, seg.Bytes()...)
	out = append(out, jpegBytes[2:]...)
	return out
}

func TestStripJPEGExif(t *testing.T) {
	t.Parallel()
	original := tinyJPEG(t)

	t.Run("drops an Exif-signed APP1 segment, leaving the rest byte-identical", func(t *testing.T) {
		withExif := spliceJPEGAPP1(t, original, "Exif\x00\x00", bytes.Repeat([]byte{0xAB}, 40)) // stand-in GPS IFD bytes
		require.NotEqual(t, original, withExif)

		stripped := stripJPEGExif(withExif)
		assert.Equal(t, original, stripped)
		assert.False(t, bytes.Contains(stripped, []byte("Exif")), "no Exif signature should survive")
	})

	t.Run("leaves a non-Exif APP1 (e.g. XMP) untouched", func(t *testing.T) {
		xmpSignature := "http://ns.adobe.com/xap/1.0/\x00"
		withXMP := spliceJPEGAPP1(t, original, xmpSignature, []byte("<x:xmpmeta/>"))

		stripped := stripJPEGExif(withXMP)
		assert.Equal(t, withXMP, stripped, "non-Exif APP1 must survive untouched")
	})

	t.Run("is a no-op on a JPEG with no Exif segment", func(t *testing.T) {
		assert.Equal(t, original, stripJPEGExif(original))
	})

	t.Run("fails open on truncated/malformed input", func(t *testing.T) {
		truncated := []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00}
		assert.Equal(t, truncated, stripJPEGExif(truncated))
	})

	t.Run("fails open on non-JPEG bytes", func(t *testing.T) {
		notJPEG := []byte("%PDF-1.4\n...")
		assert.Equal(t, notJPEG, stripJPEGExif(notJPEG))
	})
}

// tinyPNG returns a minimal valid encoded PNG.
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 60), G: uint8(y * 60), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// splicePNGChunk inserts a chunk of the given type right after the PNG's
// leading IHDR chunk. crc need not be valid: stripPNGExif never checks it.
func splicePNGChunk(t *testing.T, pngBytes []byte, chunkType string, payload []byte) []byte {
	t.Helper()
	const sigLen = 8
	require.True(t, len(pngBytes) > sigLen+12)
	ihdrLen := int(binary.BigEndian.Uint32(pngBytes[sigLen : sigLen+4]))
	insertAt := sigLen + 12 + ihdrLen

	var chunk bytes.Buffer
	lenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBytes, uint32(len(payload)))
	chunk.Write(lenBytes)
	chunk.WriteString(chunkType)
	chunk.Write(payload)
	chunk.Write([]byte{0, 0, 0, 0}) // placeholder CRC — never validated by stripPNGExif

	out := make([]byte, 0, len(pngBytes)+chunk.Len())
	out = append(out, pngBytes[:insertAt]...)
	out = append(out, chunk.Bytes()...)
	out = append(out, pngBytes[insertAt:]...)
	return out
}

func TestStripPNGExif(t *testing.T) {
	t.Parallel()
	original := tinyPNG(t)

	t.Run("drops an eXIf chunk, leaving the rest byte-identical", func(t *testing.T) {
		withExif := splicePNGChunk(t, original, "eXIf", bytes.Repeat([]byte{0xCD}, 30))
		require.NotEqual(t, original, withExif)

		stripped := stripPNGExif(withExif)
		assert.Equal(t, original, stripped)
	})

	t.Run("leaves other ancillary chunks (e.g. tEXt) untouched", func(t *testing.T) {
		withText := splicePNGChunk(t, original, "tEXt", []byte("Comment\x00hello"))

		stripped := stripPNGExif(withText)
		assert.Equal(t, withText, stripped)
	})

	t.Run("is a no-op on a PNG with no eXIf chunk", func(t *testing.T) {
		assert.Equal(t, original, stripPNGExif(original))
	})

	t.Run("fails open on truncated/malformed input", func(t *testing.T) {
		truncated := original[:10]
		assert.Equal(t, truncated, stripPNGExif(truncated))
	})

	t.Run("fails open on non-PNG bytes", func(t *testing.T) {
		notPNG := []byte("not a png at all")
		assert.Equal(t, notPNG, stripPNGExif(notPNG))
	})
}

func TestStripImageMetadata_Dispatch(t *testing.T) {
	t.Parallel()

	t.Run("routes image/jpeg through the JPEG stripper", func(t *testing.T) {
		original := tinyJPEG(t)
		withExif := spliceJPEGAPP1(t, original, "Exif\x00\x00", []byte{0x01, 0x02})
		assert.Equal(t, original, StripImageMetadata(withExif, "image/jpeg"))
	})

	t.Run("routes image/png through the PNG stripper", func(t *testing.T) {
		original := tinyPNG(t)
		withExif := splicePNGChunk(t, original, "eXIf", []byte{0x01, 0x02})
		assert.Equal(t, original, StripImageMetadata(withExif, "image/png"))
	})

	t.Run("passes through every other content type unchanged, including HEIC and non-images", func(t *testing.T) {
		for _, contentType := range []string{"image/heic", "image/heif", "application/pdf", "text/plain"} {
			data := []byte("arbitrary bytes for " + contentType)
			assert.Equal(t, data, StripImageMetadata(data, contentType), contentType)
		}
	})
}
