// Package imaging is the one decode/encode pipeline for gallery images
// (ADR 0040 decision 2). It accepts JPEG, PNG and GIF, re-encodes without
// metadata, applies EXIF orientation, and produces a bounded full variant plus
// a thumbnail. It knows nothing about owners, database rows or HTTP.
package imaging

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/nfnt/resize"
)

// Sentinel errors, mapped to reason codes by the controller.
var (
	ErrUnsupportedFormat = errors.New("imaging: unsupported image format")
	ErrHEICUnsupported   = errors.New("imaging: HEIC is not supported for gallery images")
	ErrPixelsExceeded    = errors.New("imaging: image pixel count exceeds the limit")
	ErrCorrupt           = errors.New("imaging: image is corrupt or truncated")
	ErrTooLarge          = errors.New("imaging: image exceeds the byte limit")
	ErrBusy              = errors.New("imaging: image processing is busy")
)

const (
	// MaxLongEdge is the stored full variant's long edge. Images are never
	// enlarged (ADR 0040 decision 2).
	MaxLongEdge = 2048
	// ThumbLongEdge is the stored thumbnail's long edge.
	ThumbLongEdge = 320

	jpegFullQuality  = 85
	jpegThumbQuality = 80

	// processWait bounds how long a request waits for a decode slot before
	// returning ErrBusy (the controller answers 503 + Retry-After).
	processWait = 5 * time.Second
)

// Limits are the per-request pipeline bounds. MaxPixels is checked from the
// header before any full decode allocates a raster.
type Limits struct {
	MaxPixels int
}

// DefaultLimits is the ADR 0040 default (50 megapixels).
var DefaultLimits = Limits{MaxPixels: 50_000_000}

// Result is the processed output and the facts stored on the image row.
type Result struct {
	Full      []byte
	Thumb     []byte
	MIME      string
	ThumbMIME string
	Ext       string
	Width     int
	Height    int
	DHash     int64
	TakenAt   *time.Time
	TakenDate *string
}

// processSem bounds concurrent decode-through-encode work process-wide.
// Configure concurrency once at startup with SetConcurrency.
var processSem = make(chan struct{}, defaultConcurrency())

func defaultConcurrency() int {
	if runtime.GOMAXPROCS(0) < 2 {
		return runtime.GOMAXPROCS(0)
	}
	return 2
}

// SetConcurrency replaces the decode semaphore with n slots. n <= 0 restores
// the default. It must be called during startup, before requests are served.
func SetConcurrency(n int) {
	if n <= 0 {
		n = defaultConcurrency()
	}
	processSem = make(chan struct{}, n)
}

func acquireSlot() bool {
	select {
	case processSem <- struct{}{}:
		return true
	default:
	}
	timer := time.NewTimer(processWait)
	defer timer.Stop()
	select {
	case processSem <- struct{}{}:
		return true
	case <-timer.C:
		return false
	}
}

func releaseSlot() { <-processSem }

// Process reads one image, validates it, applies EXIF orientation, and returns
// re-encoded full and thumbnail variants. The caller owns the bytes.
func Process(r io.Reader, lim Limits) (*Result, error) {
	if lim.MaxPixels <= 0 {
		lim = DefaultLimits
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if len(data) == 0 {
		return nil, ErrCorrupt
	}

	contentType := http.DetectContentType(data)
	if isMarkup(data, contentType) {
		return nil, ErrUnsupportedFormat
	}
	if isHEIC(data) {
		return nil, ErrHEICUnsupported
	}

	var decode func(io.Reader) (image.Image, error)
	switch contentType {
	case "image/jpeg":
		decode = jpeg.Decode
	case "image/png":
		decode = png.Decode
	case "image/gif":
		decode = gif.Decode
	case "image/webp":
		return nil, ErrUnsupportedFormat
	default:
		return nil, ErrUnsupportedFormat
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, ErrCorrupt
	}
	if int64(cfg.Width)*int64(cfg.Height) > int64(lim.MaxPixels) {
		return nil, ErrPixelsExceeded
	}

	meta := readMetadata(data, contentType)

	if !acquireSlot() {
		return nil, ErrBusy
	}
	defer releaseSlot()

	img, err := decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	img = orient(img, meta.Orientation)

	full, fullMIME, ext := encodeVariant(img, MaxLongEdge, jpegFullQuality)
	thumb, thumbMIME, _ := encodeVariant(img, ThumbLongEdge, jpegThumbQuality)
	if full == nil || thumb == nil {
		return nil, ErrCorrupt
	}

	bounds := img.Bounds()
	return &Result{
		Full:      full,
		Thumb:     thumb,
		MIME:      fullMIME,
		ThumbMIME: thumbMIME,
		Ext:       ext,
		Width:     bounds.Dx(),
		Height:    bounds.Dy(),
		DHash:     differenceHash(img),
		TakenAt:   meta.TakenAt,
		TakenDate: meta.TakenDate,
	}, nil
}

// encodeVariant resizes to at most longEdge (never enlarging) and encodes. It
// returns nil bytes only for an impossible zero-sized image.
func encodeVariant(img image.Image, longEdge, quality int) ([]byte, string, string) {
	out := img
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, "", ""
	}
	if w > longEdge || h > longEdge {
		if w >= h {
			out = resize.Resize(uint(longEdge), 0, img, resize.Lanczos3)
		} else {
			out = resize.Resize(0, uint(longEdge), img, resize.Lanczos3)
		}
	}

	var buf bytes.Buffer
	if hasTransparency(out) {
		if err := png.Encode(&buf, out); err != nil {
			return nil, "", ""
		}
		return buf.Bytes(), "image/png", "png"
	}
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: quality}); err != nil {
		return nil, "", ""
	}
	return buf.Bytes(), "image/jpeg", "jpg"
}

func hasTransparency(img image.Image) bool {
	if op, ok := img.(interface{ Opaque() bool }); ok {
		return !op.Opaque()
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a != 0xffff {
				return true
			}
		}
	}
	return false
}

func isMarkup(data []byte, contentType string) bool {
	lower := strings.ToLower(contentType)
	if strings.Contains(lower, "svg") || strings.Contains(lower, "html") ||
		strings.HasPrefix(lower, "text/xml") || strings.HasPrefix(lower, "application/xml") {
		return true
	}
	trimmed := bytes.TrimLeft(data, " \t\r\n\xef\xbb\xbf")
	s := strings.ToLower(string(trimmed))
	for _, prefix := range []string{"<svg", "<html", "<!doctype", "<script", "<style"} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func isHEIC(data []byte) bool {
	if len(data) < 12 || string(data[4:8]) != "ftyp" {
		return false
	}
	switch string(data[8:12]) {
	case "heic", "heix", "hevc", "hevx", "mif1", "msf1":
		return true
	}
	return false
}

// orient applies EXIF orientation 1-8.
func orient(img image.Image, orientation int) image.Image {
	if orientation < 2 || orientation > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	var dstW, dstH int
	if orientation >= 5 {
		dstW, dstH = h, w
	} else {
		dstW, dstH = w, h
	}
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var nx, ny int
			switch orientation {
			case 2:
				nx, ny = w-1-x, y
			case 3:
				nx, ny = w-1-x, h-1-y
			case 4:
				nx, ny = x, h-1-y
			case 5:
				nx, ny = y, x
			case 6:
				nx, ny = h-1-y, x
			case 7:
				nx, ny = h-1-y, w-1-x
			case 8:
				nx, ny = y, w-1-x
			default:
				nx, ny = x, y
			}
			dst.Set(nx, ny, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// differenceHash computes a 64-bit dHash: 9x8 grayscale, adjacent-column
// comparison (ADR 0040 decision 9).
func differenceHash(img image.Image) int64 {
	resized := resize.Resize(9, 8, img, resize.NearestNeighbor)
	rgba, ok := resized.(*image.RGBA)
	if !ok {
		b := resized.Bounds()
		rgba = image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				rgba.Set(x-b.Min.X, y-b.Min.Y, resized.At(x, y))
			}
		}
	}
	var hash int64
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			l := color.GrayModel.Convert(rgba.At(x, y)).(color.Gray).Y
			r := color.GrayModel.Convert(rgba.At(x+1, y)).(color.Gray).Y
			if l > r {
				hash |= 1 << (y*8 + x)
			}
		}
	}
	return hash
}
