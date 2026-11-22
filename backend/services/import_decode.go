package services

import (
	"unicode/utf8"

	"mycorrhizal/contactmodel"

	"golang.org/x/text/encoding/charmap"
)

// ImportEncodingFallbackMessage is the file-level warning shown when an
// uploaded CSV/VCF was not valid UTF-8 and was read as Windows-1252 instead
// (issue #2669). Clients match on the diagnostic's concept ("encoding"); the
// message is the English fallback.
const ImportEncodingFallbackMessage = "file was not UTF-8; read as Windows-1252"

// ImportEncodingConcept is the Diagnostic.Concept of the encoding warning.
const ImportEncodingConcept = "encoding"

// DecodeImportText is the single decoding layer for uploaded text files (CSV,
// VCF) before any parser sees them. It does two things:
//
//  1. Strips one leading UTF-8 BOM (issues #2662): Excel's "CSV UTF-8" and
//     many Windows exporters write one, and left in place it becomes part of
//     the first header ("\ufeffFirst Name"), which then maps to nothing.
//  2. If the bytes are not valid UTF-8, reads the WHOLE file as Windows-1252
//     (issue #2669) and returns a warn Diagnostic, so Latin-1/Windows-1252
//     exports from older Windows tools import as "José" instead of storing
//     U+FFFD. Windows-1252 is a superset of printable Latin-1, and every byte
//     maps to some code point, so the output is always valid UTF-8. A file
//     that is a mix of UTF-8 and stray single bytes is read as Windows-1252 as
//     a whole: one deterministic encoding, never per-byte guessing.
//
// Valid UTF-8 input is returned untouched and with no diagnostic.
func DecodeImportText(data []byte) ([]byte, []contactmodel.Diagnostic) {
	data = stripUTF8BOM(data)
	if utf8.Valid(data) {
		return data, nil
	}
	// charmap decoding cannot fail: every byte value has a mapping.
	decoded, _ := charmap.Windows1252.NewDecoder().Bytes(data)
	return decoded, []contactmodel.Diagnostic{{
		Severity: "warn",
		Concept:  ImportEncodingConcept,
		Message:  ImportEncodingFallbackMessage,
	}}
}
