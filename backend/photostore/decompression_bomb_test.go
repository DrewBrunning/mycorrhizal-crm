package photostore

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bombPNG builds a PNG with a valid signature + IHDR chunk declaring
// width x height and nothing else -- no IDAT, no pixel data. A real decode
// would fail on the truncated stream, but image.DecodeConfig only needs to
// read IHDR to report the (attacker-controlled) declared dimensions, which
// is exactly the shape of a decompression bomb: tiny on the wire, huge once
// a naive decoder allocates the full raster. No compression trickery (e.g.
// an actually-valid, highly-compressible solid-color image) is needed to
// prove CheckImageDimensions refuses it -- the vulnerable code path never
// gets far enough to care whether IDAT exists.
func bombPNG(width, height uint32) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	writeChunk := func(typ string, data []byte) {
		var lenBuf [4]byte
		binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
		buf.Write(lenBuf[:])
		buf.WriteString(typ)
		buf.Write(data)
		crc := crc32.NewIEEE()
		crc.Write([]byte(typ))
		crc.Write(data)
		var crcBuf [4]byte
		binary.BigEndian.PutUint32(crcBuf[:], crc.Sum32())
		buf.Write(crcBuf[:])
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8] = 8 // bit depth
	ihdr[9] = 2 // color type: truecolor
	// compression/filter/interlace already zero, which is the only valid value
	writeChunk("IHDR", ihdr)
	writeChunk("IEND", nil)
	return buf.Bytes()
}

// TestCheckImageDimensionsRefusesBomb pins the guard directly: a PNG
// declaring 60000x60000 (3.6 billion pixels) is refused before any Decode
// call would allocate a raster for it.
func TestCheckImageDimensionsRefusesBomb(t *testing.T) {
	bomb := bombPNG(60000, 60000)
	if len(bomb) > 100 {
		t.Fatalf("sanity: expected a tiny file, got %d bytes", len(bomb))
	}
	if err := CheckImageDimensions(bytes.NewReader(bomb)); err == nil {
		t.Fatal("expected CheckImageDimensions to refuse a 60000x60000 image, got nil error")
	}
}

// TestCheckImageDimensionsAllowsOrdinaryPhoto pins the non-regression side:
// a normal photo-sized image is not caught by the guard.
func TestCheckImageDimensionsAllowsOrdinaryPhoto(t *testing.T) {
	ordinary := bombPNG(4000, 3000) // 12MP, a typical phone photo
	if err := CheckImageDimensions(bytes.NewReader(ordinary)); err != nil {
		t.Fatalf("expected an ordinary 4000x3000 image to pass, got: %v", err)
	}
}

// TestSaveContactPhoto_DecompressionBombRefused is the E2E case for this
// package's own entry point (used by the CardDAV/VCF import and JSContact
// reverse-apply paths, per photostore.go's package doc comment): a bomb
// image must be refused by SaveContactPhoto itself -- proving the guard is
// actually wired into the real call, not just the helper it calls -- and
// must not leave a file behind.
func TestSaveContactPhoto_DecompressionBombRefused(t *testing.T) {
	dir := t.TempDir()
	bomb := bombPNG(60000, 60000)

	_, _, err := SaveContactPhoto(bomb, "image/png", dir)
	if err == nil {
		t.Fatal("expected SaveContactPhoto to refuse a decompression-bomb image, got nil error")
	}
	// This bomb has no IDAT chunk, so a real Decode would *also* fail (a
	// different, unrelated error: a truncated/malformed stream) even
	// without the dimension guard — that would make a bare "err != nil"
	// assertion pass for the wrong reason. Pin the guard's own error text
	// specifically, so this test actually fails if CheckImageDimensions is
	// ever removed from this call site (hand-verified: commenting out the
	// guard call changes the error to a png "unexpected EOF"/format error,
	// which does not contain this substring).
	if !strings.Contains(err.Error(), "dimensions too large") {
		t.Fatalf("expected the dimension-guard error, got: %v", err)
	}

	entries, readErr := os.ReadDir(dir)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("failed to read photo dir: %v", readErr)
	}
	for _, e := range entries {
		t.Errorf("rejected bomb image must not leave a file on disk, found %s", filepath.Join(dir, e.Name()))
	}
}
