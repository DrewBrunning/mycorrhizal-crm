package imaging

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTIFF builds a minimal little/big-endian TIFF block with IFD0
// (orientation, Exif IFD pointer) and an Exif IFD (capture date/offset).
func buildTIFF(order binary.ByteOrder, orientation uint16, date, offset string) []byte {
	const ifd0Offset = 8
	ifd0Entries := 2
	ifd0Size := 2 + ifd0Entries*12 + 4
	exifOffset := ifd0Offset + ifd0Size
	exifEntries := 2
	exifSize := 2 + exifEntries*12 + 4
	dataStart := exifOffset + exifSize

	dateBytes := append([]byte(date), 0)
	offsetBytes := append([]byte(offset), 0)
	dateOffset := dataStart
	offsetOffset := dateOffset + len(dateBytes)
	total := offsetOffset + len(offsetBytes)
	buf := make([]byte, total)
	copy(buf[0:2], map[bool]string{true: "II", false: "MM"}[order == binary.LittleEndian])
	order.PutUint16(buf[2:4], 42)
	order.PutUint32(buf[4:8], ifd0Offset)

	pos := ifd0Offset
	order.PutUint16(buf[pos:pos+2], uint16(ifd0Entries))
	pos += 2
	order.PutUint16(buf[pos:pos+2], 0x0112)
	order.PutUint16(buf[pos+2:pos+4], 3)
	order.PutUint32(buf[pos+4:pos+8], 1)
	order.PutUint16(buf[pos+8:pos+10], orientation)
	pos += 12
	order.PutUint16(buf[pos:pos+2], 0x8769)
	order.PutUint16(buf[pos+2:pos+4], 4)
	order.PutUint32(buf[pos+4:pos+8], 1)
	order.PutUint32(buf[pos+8:pos+12], uint32(exifOffset))
	pos += 12
	order.PutUint32(buf[pos:pos+4], 0)

	pos = exifOffset
	order.PutUint16(buf[pos:pos+2], uint16(exifEntries))
	pos += 2
	order.PutUint16(buf[pos:pos+2], 0x9003)
	order.PutUint16(buf[pos+2:pos+4], 2)
	order.PutUint32(buf[pos+4:pos+8], uint32(len(dateBytes)))
	order.PutUint32(buf[pos+8:pos+12], uint32(dateOffset))
	pos += 12
	order.PutUint16(buf[pos:pos+2], 0x9011)
	order.PutUint16(buf[pos+2:pos+4], 2)
	order.PutUint32(buf[pos+4:pos+8], uint32(len(offsetBytes)))
	order.PutUint32(buf[pos+8:pos+12], uint32(offsetOffset))
	pos += 12
	order.PutUint32(buf[pos:pos+4], 0)

	copy(buf[dateOffset:], dateBytes)
	copy(buf[offsetOffset:], offsetBytes)
	return buf
}

func wrapExifJPEG(tiff []byte) []byte {
	segmentLen := 2 + len([]byte("Exif\x00\x00")) + len(tiff)
	buf := bytes.NewBuffer([]byte{0xFF, 0xD8, 0xFF, 0xE1})
	_ = binary.Write(buf, binary.BigEndian, uint16(segmentLen))
	buf.Write([]byte("Exif\x00\x00"))
	buf.Write(tiff)
	buf.Write([]byte{0xFF, 0xD9})
	return buf.Bytes()
}

func TestReadMetadata_ParsesOrientationAndCaptureTime(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		data := wrapExifJPEG(buildTIFF(order, 6, "2024:07:03 10:11:12", "+02:00"))
		meta := readMetadata(data, "image/jpeg")
		assert.Equal(t, 6, meta.Orientation)
		require.NotNil(t, meta.TakenDate)
		assert.Equal(t, "2024-07-03", *meta.TakenDate)
		require.NotNil(t, meta.TakenAt)
		assert.Equal(t, time.Date(2024, 7, 3, 8, 11, 12, 0, time.UTC), *meta.TakenAt)
	}
}

func TestApplyCaptureTime_RejectsMalformedValues(t *testing.T) {
	for _, tc := range []struct{ date, offset string }{
		{"not a date", "+01:00"},
		{"2024:07:03 10:11:12", "01:00"},
		{"2024:07:03 10:11:12", "+99:00"},
		{"2024:07:03 10:11:12", "+01:99"},
		{"2024:07:03 10:11:12", "+01"},
	} {
		meta := imageMetadata{}
		applyCaptureTime(&meta, tc.date, tc.offset)
		if tc.date == "not a date" {
			assert.Nil(t, meta.TakenDate)
		} else {
			require.NotNil(t, meta.TakenDate)
			assert.Nil(t, meta.TakenAt, "bad offset %q must not set taken_at", tc.offset)
		}
	}
}

func TestReadMetadata_MalformedInputsFailOpen(t *testing.T) {
	cases := [][]byte{
		nil,
		{0xFF},
		{0xFF, 0xD8},                         // SOI only
		{0xFF, 0xD8, 0x00, 0x00},             // not a marker
		{0xFF, 0xD8, 0xFF, 0xDA},             // SOS before EXIF
		{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x01}, // bad segment length
		{0xFF, 0xD8, 0xFF, 0xD0, 0xFF, 0xD9}, // standalone marker
		{0xFF, 0xD8, 0xFF, 0xE1},             // truncated segment header
		wrapExifJPEG(buildTIFF(binary.LittleEndian, 0, "", "")),
	}
	for _, data := range cases {
		meta := readMetadata(data, "image/jpeg")
		assert.Equal(t, 1, meta.Orientation)
	}
	assert.Equal(t, 1, readMetadata([]byte("not-jpeg"), "image/png").Orientation)
}

func TestParseTIFF_RejectsBadHeader(t *testing.T) {
	for _, data := range [][]byte{
		{},
		[]byte("XX\x00\x2a\x00\x00\x00\x08"),
		[]byte("II\x00\x00\x00\x00\x00\x08"),
		[]byte("II\x2a\x00\x00\x00\xff\xff"),
	} {
		meta := imageMetadata{Orientation: 1}
		parseTIFF(data, &meta)
		assert.Equal(t, 1, meta.Orientation)
	}
}

func TestParseIFD_RejectsBadEntries(t *testing.T) {
	// count says one entry but the body is truncated.
	data := []byte("II\x2a\x00\x08\x00\x00\x00\x01\x00")
	meta := imageMetadata{Orientation: 1}
	parseTIFF(data, &meta)
	assert.Equal(t, 1, meta.Orientation)

	// A long value whose offset is outside the block.
	tiff := buildTIFF(binary.LittleEndian, 3, "2024:07:03 10:11:12", "+00:00")
	idx := bytes.Index(tiff, []byte("2024:07:03"))
	require.Positive(t, idx)
	binary.LittleEndian.PutUint32(tiff[idx-4:idx], 0xFFFFFF)
	meta = imageMetadata{Orientation: 1}
	parseTIFF(tiff, &meta)
	assert.Equal(t, 3, meta.Orientation, "IFD0 still parsed; the bad Exif string is skipped")
}

func TestTypeSize(t *testing.T) {
	assert.Equal(t, 1, typeSize(2))
	assert.Equal(t, 2, typeSize(3))
	assert.Equal(t, 4, typeSize(4))
	assert.Equal(t, 8, typeSize(5))
	assert.Zero(t, typeSize(99))
}

func TestJpegExifPayload_TruncatedSegmentHeader(t *testing.T) {
	assert.Nil(t, jpegExifPayload([]byte{0xFF, 0xD8, 0xFF, 0xE1}))
}

func TestParseIFD_NonStringAndOutOfBoundsValue(t *testing.T) {
	// One IFD0 entry with tag 0x9003 but a numeric type: the callback must skip it.
	entry := make([]byte, 8+2+12+4)
	copy(entry[0:2], "II")
	binary.LittleEndian.PutUint16(entry[2:4], 42)
	binary.LittleEndian.PutUint32(entry[4:8], 8)
	binary.LittleEndian.PutUint16(entry[8:10], 1)
	binary.LittleEndian.PutUint16(entry[10:12], 0x9003)
	binary.LittleEndian.PutUint16(entry[12:14], 3)
	binary.LittleEndian.PutUint32(entry[14:18], 1)
	parseIFD(entry, binary.LittleEndian, 8, func(tag, typ uint16, count uint32, value []byte) {
		assert.Equal(t, uint16(0x9003), tag)
		assert.Equal(t, uint16(3), typ)
	})

	// A long ASCII value whose offset is outside the block.
	out := make([]byte, 8+2+12+4)
	copy(out[0:2], "II")
	binary.LittleEndian.PutUint16(out[2:4], 42)
	binary.LittleEndian.PutUint32(out[4:8], 8)
	binary.LittleEndian.PutUint16(out[8:10], 1)
	binary.LittleEndian.PutUint16(out[10:12], 0x9003)
	binary.LittleEndian.PutUint16(out[12:14], 2)
	binary.LittleEndian.PutUint32(out[14:18], 10)
	binary.LittleEndian.PutUint32(out[18:22], 9999)
	parseIFD(out, binary.LittleEndian, 8, func(uint16, uint16, uint32, []byte) {
		t.Fatal("out-of-bounds value must not reach the callback")
	})
}

func TestParseTIFF_SkipsNonStringExifDates(t *testing.T) {
	tiff := buildTIFF(binary.LittleEndian, 3, "2024:07:03 10:11:12", "+00:00")
	// The Exif IFD starts at 8 + (2 + 2*12 + 4) = 38; its first entry's type is
	// at byte 42. Make DateTimeOriginal numeric instead of ASCII.
	binary.LittleEndian.PutUint16(tiff[42:44], 3)
	meta := imageMetadata{Orientation: 1}
	parseTIFF(tiff, &meta)
	assert.Equal(t, 3, meta.Orientation)
	assert.Nil(t, meta.TakenDate)
}
