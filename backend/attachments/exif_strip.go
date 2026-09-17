package attachments

import (
	"bytes"
	"encoding/binary"
)

// jpegExifSignature is the fixed 6-byte prefix ("Exif\x00\x00") that marks a
// JPEG APP1 segment as carrying EXIF (as opposed to XMP, which also lives in
// APP1 under a different signature).
var jpegExifSignature = []byte("Exif\x00\x00")

// StripImageMetadata removes EXIF/GPS metadata from JPEG and PNG attachments
// at ingest (issue #945): a camera's GPS coordinates and body serial number
// otherwise ride along unchanged into downloads and operator backups. It is
// scoped deliberately narrow:
//
//   - JPEG: only the APP1 segment(s) signed "Exif\x00\x00" are dropped. Other
//     APPn segments (APP0/JFIF, APP2/ICC color profile, APP13/IPTC, a non-Exif
//     APP1 such as XMP) are left alone — stripping the ICC profile would
//     visibly shift color rendering, which is not this issue's concern.
//   - PNG: only the ancillary "eXIf" chunk is dropped; every other chunk
//     (including "iCCP") is kept byte-identical.
//   - Every other content type (including HEIC, and non-image attachments
//     like PDFs) passes through unchanged. HEIC/ISOBMFF metadata stripping
//     needs real box-editing that isn't implemented here — a documented gap,
//     not a silent one (see docs/security/asvs-l2.md P10).
//
// Both strippers fail open: any structure they don't recognize as well-formed
// JPEG/PNG returns the original bytes unchanged rather than risk truncating
// or corrupting the upload over a best-effort metadata scrub.
func StripImageMetadata(data []byte, contentType string) []byte {
	switch contentType {
	case "image/jpeg":
		return stripJPEGExif(data)
	case "image/png":
		return stripPNGExif(data)
	default:
		return data
	}
}

// stripJPEGExif walks JPEG segment markers from the SOI and drops any APP1
// segment signed as EXIF. See StripImageMetadata's doc comment for scope.
func stripJPEGExif(data []byte) []byte {
	const (
		markerPrefix = 0xFF
		soi          = 0xD8
		eoi          = 0xD9
		sos          = 0xDA
		app1         = 0xE1
	)

	if len(data) < 4 || data[0] != markerPrefix || data[1] != soi {
		return data // not a JPEG (or too short) — fail open
	}

	out := make([]byte, 0, len(data))
	out = append(out, data[0], data[1])
	pos := 2

	for {
		if pos+2 > len(data) {
			return data // ran off the end mid-structure — fail open on the original
		}
		if data[pos] != markerPrefix {
			return data // not a marker where one was expected — fail open
		}
		marker := data[pos+1]

		if marker == eoi {
			out = append(out, data[pos], data[pos+1])
			return out
		}
		if marker == sos {
			// Compressed scan data (and any subsequent scans/markers for a
			// progressive JPEG) follows verbatim; nothing past this point is
			// an APPn segment.
			out = append(out, data[pos:]...)
			return out
		}
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			// TEM / RSTn: standalone, no length field.
			out = append(out, data[pos], data[pos+1])
			pos += 2
			continue
		}

		if pos+4 > len(data) {
			return data // fail open
		}
		segmentLen := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		if segmentLen < 2 || pos+2+segmentLen > len(data) {
			return data // fail open
		}
		segment := data[pos : pos+2+segmentLen]

		isExifApp1 := marker == app1 &&
			segmentLen >= 2+len(jpegExifSignature) &&
			bytes.Equal(segment[4:4+len(jpegExifSignature)], jpegExifSignature)

		if !isExifApp1 {
			out = append(out, segment...)
		}
		pos += 2 + segmentLen
	}
}

// stripPNGExif walks PNG chunks after the 8-byte signature and drops any
// "eXIf" chunk. See StripImageMetadata's doc comment for scope.
func stripPNGExif(data []byte) []byte {
	pngSignature := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}
	if len(data) < len(pngSignature) || !bytes.Equal(data[:len(pngSignature)], pngSignature) {
		return data // not a PNG — fail open
	}

	out := make([]byte, 0, len(data))
	out = append(out, data[:len(pngSignature)]...)
	pos := len(pngSignature)

	for {
		if pos == len(data) {
			return out // clean end of input
		}
		// A chunk is: 4-byte length, 4-byte type, length bytes of data, 4-byte CRC.
		if pos+12 > len(data) {
			return data // fail open — truncated chunk header
		}
		chunkLen := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		if chunkLen < 0 || pos+12+chunkLen > len(data) {
			return data // fail open
		}
		chunkType := string(data[pos+4 : pos+8])
		chunkEnd := pos + 12 + chunkLen

		if chunkType != "eXIf" {
			out = append(out, data[pos:chunkEnd]...)
		}
		pos = chunkEnd

		if chunkType == "IEND" {
			// Any trailing bytes past IEND aren't valid PNG structure; keep
			// them verbatim rather than silently drop them.
			if pos < len(data) {
				out = append(out, data[pos:]...)
			}
			return out
		}
	}
}
