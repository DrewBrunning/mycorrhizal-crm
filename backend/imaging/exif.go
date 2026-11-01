package imaging

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"strings"
	"time"
)

// imageMetadata is the small EXIF subset the gallery keeps: orientation and
// capture time. Everything else is discarded by the re-encode (ADR 0040
// decision 2), which also drops GPS and serial numbers by construction.
type imageMetadata struct {
	Orientation int
	TakenAt     *time.Time
	TakenDate   *string
}

// readMetadata parses a JPEG APP1 EXIF segment. PNG/GIF carry no EXIF reader
// here; orientation 1 (no transform) and no capture time is the safe default.
func readMetadata(data []byte, contentType string) imageMetadata {
	meta := imageMetadata{Orientation: 1}
	if contentType != "image/jpeg" {
		return meta
	}
	tiff := jpegExifPayload(data)
	if tiff == nil {
		return meta
	}
	parseTIFF(tiff, &meta)
	return meta
}

// jpegExifPayload returns the TIFF payload of the first APP1 segment signed
// "Exif\x00\x00", with strict bounds. Malformed input returns nil.
func jpegExifPayload(data []byte) []byte {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil
	}
	pos := 2
	for {
		if pos+2 > len(data) || data[pos] != 0xFF {
			return nil
		}
		marker := data[pos+1]
		if marker == 0xDA || marker == 0xD9 {
			return nil
		}
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			pos += 2
			continue
		}
		if pos+4 > len(data) {
			return nil
		}
		segLen := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		if segLen < 2 || pos+2+segLen > len(data) {
			return nil
		}
		seg := data[pos+2 : pos+2+segLen]
		// seg starts with the 2-byte length, then the "Exif\x00\x00" signature.
		if marker == 0xE1 && len(seg) >= 8 && bytes.Equal(seg[2:8], []byte("Exif\x00\x00")) {
			return seg[8:]
		}
		pos += 2 + segLen
	}
}

// parseTIFF reads IFD0 and the Exif sub-IFD with strict bounds. Any malformed
// structure stops parsing and leaves the defaults in place (fail open).
func parseTIFF(tiff []byte, meta *imageMetadata) {
	if len(tiff) < 8 {
		return
	}
	var order binary.ByteOrder
	switch string(tiff[0:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return
	}
	ifd0 := int(order.Uint32(tiff[4:8]))
	exifIFD := 0
	parseIFD(tiff, order, ifd0, func(tag uint16, typ uint16, count uint32, value []byte) {
		switch tag {
		case 0x0112: // Orientation
			if typ == 3 && count == 1 && len(value) >= 2 {
				meta.Orientation = int(order.Uint16(value[0:2]))
				if meta.Orientation < 1 || meta.Orientation > 8 {
					meta.Orientation = 1
				}
			}
		case 0x8769: // Exif IFD pointer
			if typ == 4 && count == 1 && len(value) >= 4 {
				exifIFD = int(order.Uint32(value[0:4]))
			}
		}
	})
	if exifIFD == 0 {
		return
	}
	var date, offset string
	parseIFD(tiff, order, exifIFD, func(tag uint16, typ uint16, count uint32, value []byte) {
		if typ != 2 || count == 0 || len(value) < int(count) {
			return
		}
		s := strings.TrimRight(string(value[:count]), "\x00")
		switch tag {
		case 0x9003: // DateTimeOriginal
			date = s
		case 0x9011: // OffsetTimeOriginal
			offset = s
		}
	})
	applyCaptureTime(meta, date, offset)
}

func parseIFD(tiff []byte, order binary.ByteOrder, offset int, fn func(tag, typ uint16, count uint32, value []byte)) {
	if offset < 0 || offset+2 > len(tiff) {
		return
	}
	n := int(order.Uint16(tiff[offset : offset+2]))
	pos := offset + 2
	for i := 0; i < n; i++ {
		if pos+12 > len(tiff) {
			return
		}
		entry := tiff[pos : pos+12]
		tag := order.Uint16(entry[0:2])
		typ := order.Uint16(entry[2:4])
		count := order.Uint32(entry[4:8])
		size := typeSize(typ) * int(count)
		if size < 0 {
			return
		}
		var value []byte
		if size <= 4 {
			value = entry[8 : 8+size]
		} else {
			valOff := int(order.Uint32(entry[8:12]))
			if valOff < 0 || valOff+size > len(tiff) {
				return
			}
			value = tiff[valOff : valOff+size]
		}
		fn(tag, typ, count, value)
		pos += 12
	}
}

func typeSize(typ uint16) int {
	switch typ {
	case 1, 2, 6, 7:
		return 1
	case 3, 8:
		return 2
	case 4, 9, 11:
		return 4
	case 5, 10, 12:
		return 8
	default:
		return 0
	}
}

func applyCaptureTime(meta *imageMetadata, date, offset string) {
	const exifLayout = "2006:01:02 15:04:05"
	t, err := time.Parse(exifLayout, strings.TrimSpace(date))
	if err != nil {
		return
	}
	day := t.Format("2006-01-02")
	meta.TakenDate = &day

	offset = strings.TrimSpace(offset)
	if offset == "" || (offset[0] != '+' && offset[0] != '-') {
		return
	}
	// OffsetTimeOriginal is "±HH:MM"; accept only a real numeric offset so a
	// corrupt value cannot shift the day.
	sign := 1
	if strings.HasPrefix(offset, "-") {
		sign = -1
	}
	parts := strings.Split(strings.TrimLeft(offset, "+-"), ":")
	if len(parts) != 2 {
		return
	}
	hh, err1 := strconv.Atoi(parts[0])
	mm, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || hh > 23 || mm > 59 {
		return
	}
	loc := time.FixedZone("", sign*(hh*3600+mm*60))
	withZone, err := time.ParseInLocation(exifLayout, strings.TrimSpace(date), loc)
	if err != nil {
		return
	}
	utc := withZone.UTC()
	meta.TakenAt = &utc
}
