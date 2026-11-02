package imaging

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func encodeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 40, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

func TestProcess_ReencodesAndCapsLongEdge(t *testing.T) {
	in := encodeJPEG(t, 4000, 1000)
	res, err := Process(bytes.NewReader(in), Limits{MaxPixels: 50_000_000})
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", res.MIME)
	assert.Equal(t, "jpg", res.Ext)
	assert.Equal(t, 4000, res.Width)
	assert.Equal(t, 1000, res.Height)
	full, err := jpeg.Decode(bytes.NewReader(res.Full))
	require.NoError(t, err)
	assert.LessOrEqual(t, full.Bounds().Dx(), MaxLongEdge)
	thumb, err := jpeg.Decode(bytes.NewReader(res.Thumb))
	require.NoError(t, err)
	assert.LessOrEqual(t, thumb.Bounds().Dx(), ThumbLongEdge)
	assert.NotZero(t, res.DHash)
}

func TestProcess_PreservesTransparencyAsPNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	img.Set(0, 0, color.RGBA{R: 255, A: 0})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	res, err := Process(bytes.NewReader(buf.Bytes()), DefaultLimits)
	require.NoError(t, err)
	assert.Equal(t, "image/png", res.MIME)
	assert.Equal(t, "png", res.Ext)
}

func TestProcess_RejectsUnsupportedAndOversized(t *testing.T) {
	_, err := Process(bytes.NewReader([]byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>")), DefaultLimits)
	assert.ErrorIs(t, err, ErrUnsupportedFormat)

	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 500, 500))))
	_, err = Process(bytes.NewReader(buf.Bytes()), Limits{MaxPixels: 1000})
	assert.ErrorIs(t, err, ErrPixelsExceeded)

	_, err = Process(bytes.NewReader([]byte("not an image")), DefaultLimits)
	assert.True(t, errors.Is(err, ErrUnsupportedFormat) || errors.Is(err, ErrCorrupt))
}

func TestProcess_RejectsHEIC(t *testing.T) {
	data := make([]byte, 16)
	copy(data[4:8], "ftyp")
	copy(data[8:12], "heic")
	_, err := Process(bytes.NewReader(data), DefaultLimits)
	assert.ErrorIs(t, err, ErrHEICUnsupported)
}

func TestStore_RejectsTraversal(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	_, err := store.StoredPath(1, "../escape.jpg")
	assert.Error(t, err)
	_, err = store.StoredPath(1, "/etc/passwd")
	assert.Error(t, err)
}

func TestStore_SaveAndRemove(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	full, thumb, err := store.Save(7, []byte("full"), []byte("thumb"), "jpg", "jpg")
	require.NoError(t, err)
	got, err := store.Read(7, full)
	require.NoError(t, err)
	assert.Equal(t, []byte("full"), got)
	require.NoError(t, store.Remove(7, full))
	require.NoError(t, store.Remove(7, thumb))
	_, err = store.Read(7, full)
	assert.Error(t, err)
}

func TestOrient_AllEightTransforms(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			src.Set(x, y, color.RGBA{R: uint8(x + 1), G: uint8(y + 1), B: 1, A: 255})
		}
	}
	for orientation := 1; orientation <= 8; orientation++ {
		got := orient(src, orientation)
		b := got.Bounds()
		if orientation >= 5 {
			assert.Equal(t, 3, b.Dx())
			assert.Equal(t, 2, b.Dy())
		} else {
			assert.Equal(t, 2, b.Dx())
			assert.Equal(t, 3, b.Dy())
		}
	}
	assert.Same(t, src, orient(src, 0), "out-of-range orientation is a no-op")
}

func TestProcess_GIFAndDefaultPixels(t *testing.T) {
	img := image.NewPaletted(image.Rect(0, 0, 4, 4), []color.Color{color.Black, color.White})
	var buf bytes.Buffer
	require.NoError(t, gif.Encode(&buf, img, nil))
	res, err := Process(bytes.NewReader(buf.Bytes()), Limits{})
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", res.MIME)
}

func TestProcess_CorruptAndBusy(t *testing.T) {
	_, err := Process(bytes.NewReader([]byte{0xFF, 0xD8, 0xFF, 0xD9}), DefaultLimits)
	assert.ErrorIs(t, err, ErrCorrupt)

	SetConcurrency(1)
	t.Cleanup(func() { SetConcurrency(0) })
	processSem <- struct{}{}
	done := make(chan error, 1)
	go func() {
		_, procErr := Process(bytes.NewReader(encodeJPEG(t, 8, 8)), DefaultLimits)
		done <- procErr
	}()
	select {
	case procErr := <-done:
		assert.ErrorIs(t, procErr, ErrBusy)
	case <-time.After(7 * time.Second):
		t.Fatal("Process did not return while the only decode slot was held")
	}
	<-processSem
}

func TestIsMarkupAndHEIC(t *testing.T) {
	assert.True(t, isMarkup([]byte("  <svg></svg>"), "text/plain"))
	assert.True(t, isMarkup(nil, "image/svg+xml"))
	assert.False(t, isMarkup([]byte("not markup"), "image/jpeg"))
	for _, brand := range []string{"heic", "heix", "hevc", "hevx", "mif1", "msf1"} {
		data := make([]byte, 12)
		copy(data[4:8], "ftyp")
		copy(data[8:12], brand)
		assert.True(t, isHEIC(data))
	}
	assert.False(t, isHEIC([]byte("nope")))
}

type plainImage struct{ img image.Image }

func (p plainImage) ColorModel() color.Model { return p.img.ColorModel() }
func (p plainImage) Bounds() image.Rectangle { return p.img.Bounds() }
func (p plainImage) At(x, y int) color.Color { return p.img.At(x, y) }

func TestHasTransparency_FallbackPath(t *testing.T) {
	opaqueRGBA := image.NewRGBA(image.Rect(0, 0, 1, 1))
	opaqueRGBA.Set(0, 0, color.RGBA{A: 255})
	assert.False(t, hasTransparency(plainImage{img: opaqueRGBA}))
	transparent := image.NewRGBA(image.Rect(0, 0, 1, 1))
	transparent.Set(0, 0, color.RGBA{A: 0})
	assert.True(t, hasTransparency(plainImage{img: transparent}))
}

func TestStore_ErrorPaths(t *testing.T) {
	_, _, err := (Store{}).Save(1, []byte("a"), []byte("b"), "jpg", "jpg")
	assert.Error(t, err)
	_, err = (Store{}).Read(1, "a.jpg")
	assert.Error(t, err)
	err = (Store{}).Remove(1, "a.jpg")
	assert.Error(t, err)

	notDir := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(notDir, []byte("x"), 0o600))
	_, _, err = (Store{Dir: notDir}).Save(1, []byte("a"), []byte("b"), "jpg", "jpg")
	assert.Error(t, err)

	dir := t.TempDir()
	_, err = (Store{Dir: dir}).Read(1, "../escape")
	assert.Error(t, err)
	assert.NoError(t, (Store{Dir: dir}).Remove(1, "missing.jpg"))
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestProcess_ReadErrorIsCorrupt(t *testing.T) {
	_, err := Process(errorReader{}, DefaultLimits)
	assert.ErrorIs(t, err, ErrCorrupt)
}

func TestProcess_AppliesEXIFOrientationAndCaptureTime(t *testing.T) {
	plain := encodeJPEG(t, 20, 10)
	tiff := buildTIFF(binary.LittleEndian, 6, "2024:07:03 10:11:12", "+02:00")
	app1 := bytes.NewBuffer([]byte{0xFF, 0xE1})
	_ = binary.Write(app1, binary.BigEndian, uint16(2+len([]byte("Exif\x00\x00"))+len(tiff)))
	app1.Write([]byte("Exif\x00\x00"))
	app1.Write(tiff)
	withExif := append([]byte{}, plain[:2]...)
	withExif = append(withExif, app1.Bytes()...)
	withExif = append(withExif, plain[2:]...)

	res, err := Process(bytes.NewReader(withExif), DefaultLimits)
	require.NoError(t, err)
	assert.Equal(t, 10, res.Width)
	assert.Equal(t, 20, res.Height)
	require.NotNil(t, res.TakenDate)
	assert.Equal(t, "2024-07-03", *res.TakenDate)
}

func TestProcess_PortraitLongEdge(t *testing.T) {
	res, err := Process(bytes.NewReader(encodeJPEG(t, 1000, 4000)), DefaultLimits)
	require.NoError(t, err)
	full, err := jpeg.Decode(bytes.NewReader(res.Full))
	require.NoError(t, err)
	assert.LessOrEqual(t, full.Bounds().Dy(), MaxLongEdge)
	assert.Greater(t, full.Bounds().Dy(), full.Bounds().Dx())
}

func TestStore_WriteAndRemoveFailures(t *testing.T) {
	dir := t.TempDir()
	userDir := filepath.Join(dir, "u1")
	require.NoError(t, os.MkdirAll(userDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(userDir, 0o700) })
	_, _, err := (Store{Dir: dir}).Save(1, []byte("a"), []byte("b"), "jpg", "jpg")
	assert.Error(t, err)

	// Removing a directory entry that is a directory fails with a non-IsNotExist error.
	require.NoError(t, os.Chmod(userDir, 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(userDir, "dir.jpg"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(userDir, "dir.jpg", "child"), []byte("x"), 0o600))
	assert.Error(t, (Store{Dir: dir}).Remove(1, "dir.jpg"))
}

func TestApplyCaptureTime_NoOffsetAndNegativeOffset(t *testing.T) {
	meta := imageMetadata{}
	applyCaptureTime(&meta, "2024:07:03 10:11:12", "")
	require.NotNil(t, meta.TakenDate)
	assert.Nil(t, meta.TakenAt)

	meta = imageMetadata{}
	applyCaptureTime(&meta, "2024:07:03 10:11:12", "-02:00")
	require.NotNil(t, meta.TakenAt)
	assert.Equal(t, time.Date(2024, 7, 3, 12, 11, 12, 0, time.UTC), *meta.TakenAt)
}

func TestParseIFD_UnknownTypeDoesNotPanic(t *testing.T) {
	tiff := buildTIFF(binary.LittleEndian, 3, "2024:07:03 10:11:12", "+00:00")
	// Rewrite the orientation entry's type to an unknown value (99).
	tiff[10] = 99
	meta := imageMetadata{Orientation: 1}
	parseTIFF(tiff, &meta)
	assert.Equal(t, 1, meta.Orientation)
}

func TestIsHEIC_UnknownFTYPBrand(t *testing.T) {
	data := make([]byte, 12)
	copy(data[4:8], "ftyp")
	copy(data[8:12], "avif")
	assert.False(t, isHEIC(data))
}

func TestStore_SaveFailureCleanupAndRenameFailure(t *testing.T) {
	origWrite, origRename := writeFile, renameFile
	t.Cleanup(func() { writeFile, renameFile = origWrite, origRename })

	// The full variant writes, the thumbnail write fails: the full file must
	// be removed again.
	calls := 0
	writeFile = func(name string, data []byte, perm os.FileMode) error {
		calls++
		if calls == 2 {
			return errors.New("thumb write failed")
		}
		return os.WriteFile(name, data, perm)
	}
	store := Store{Dir: t.TempDir()}
	_, _, err := store.Save(1, []byte("full"), []byte("thumb"), "jpg", "jpg")
	assert.Error(t, err)
	entries, readErr := os.ReadDir(filepath.Join(store.Dir, "u1"))
	require.NoError(t, readErr)
	assert.Empty(t, entries)

	// Rename failure removes the temp file.
	writeFile = os.WriteFile
	renameFile = func(_, _ string) error { return errors.New("rename failed") }
	_, _, err = store.Save(1, []byte("full"), []byte("thumb"), "jpg", "jpg")
	assert.Error(t, err)
	entries, readErr = os.ReadDir(filepath.Join(store.Dir, "u1"))
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}

func TestProcess_ResizesLargeTransparentPNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1000, 600))
	img.Set(0, 0, color.RGBA{A: 0})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	res, err := Process(bytes.NewReader(buf.Bytes()), DefaultLimits)
	require.NoError(t, err)
	assert.Equal(t, "image/png", res.MIME)
	out, err := png.Decode(bytes.NewReader(res.Full))
	require.NoError(t, err)
	assert.LessOrEqual(t, out.Bounds().Dx(), MaxLongEdge)
}

func TestParseIFD_ZeroSizeTypeIsSkipped(t *testing.T) {
	tiff := buildTIFF(binary.LittleEndian, 3, "2024:07:03 10:11:12", "+00:00")
	// Change DateTimeOriginal's type to 0 (unknown / zero-width).
	tiff[10+12+2] = 0
	meta := imageMetadata{Orientation: 1}
	parseTIFF(tiff, &meta)
	assert.Equal(t, 3, meta.Orientation)
}

func TestProcess_WebPAndTruncatedDecode(t *testing.T) {
	webp := []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P', 'V', 'P', '8', ' '}
	_, err := Process(bytes.NewReader(webp), DefaultLimits)
	assert.ErrorIs(t, err, ErrUnsupportedFormat)

	full := encodeJPEG(t, 200, 200)
	_, err = Process(bytes.NewReader(full[:len(full)/4]), DefaultLimits)
	assert.ErrorIs(t, err, ErrCorrupt)
}

func TestAcquireSlot_WaitsForAFreedSlot(t *testing.T) {
	SetConcurrency(1)
	t.Cleanup(func() { SetConcurrency(0) })
	processSem <- struct{}{}
	done := make(chan error, 1)
	go func() {
		_, err := Process(bytes.NewReader(encodeJPEG(t, 8, 8)), DefaultLimits)
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	<-processSem
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("Process did not take the freed decode slot")
	}
}

func TestProcess_EmptyInputIsCorrupt(t *testing.T) {
	_, err := Process(bytes.NewReader(nil), DefaultLimits)
	assert.ErrorIs(t, err, ErrCorrupt)
}

func TestEncodeVariant_ZeroSize(t *testing.T) {
	data, mime, ext := encodeVariant(image.NewRGBA(image.Rect(0, 0, 0, 0)), 100, 85)
	assert.Nil(t, data)
	assert.Empty(t, mime)
	assert.Empty(t, ext)
}
