package services

import (
	"bytes"
	"strings"
	"testing"
)

// importNormalizerFuzzSeeds exercise the normalizers' documented input space:
// ISO and legacy birthday forms (import_service.go's supported list), the
// gender spellings in NormalizeGender's table, circle separator variants
// (comma, semicolon, the ":::" alias and the "*"-ignore prefix), the three
// source timestamp layouts parseSourceTime understands, and CRLF/CR line
// endings normalizeVCardLineEndings folds to LF.
var importNormalizerFuzzSeeds = [][]byte{
	[]byte("1958-06-29"),
	[]byte("--04-20"),
	[]byte("29.06.1958"),
	[]byte("29.06."),
	[]byte(" 1958-06-29 "),
	[]byte("not a date"),
	[]byte(""),
	[]byte("male"),
	[]byte("M"),
	[]byte("männlich"),
	[]byte("non-binary"),
	[]byte("prefer not to say"),
	[]byte("Family,Book Club"),
	[]byte("Family;Book Club;*ignored"),
	[]byte("Family:::Book Club"),
	[]byte("2026-03-01T10:00:00Z"),
	[]byte("2026-03-01 10:00:00"),
	[]byte("2026-03-01"),
	[]byte("BEGIN:VCARD\r\nFN:Ada\r\nEND:VCARD\r"),
	[]byte("a\r\nb\rc\nd"),
}

// FuzzImportNormalizers covers issue #1625's shared import-normalizer target:
// these helpers run on every CSV/vCard/source-import row, so a malformed scalar
// reaches them from an untrusted file. NormalizeBirthday and ParseCircles feed
// the create/update paths, NormalizeGender the neutral model, parseSourceTime
// every source timestamp (Meerkat/Monica/CSV), and normalizeVCardLineEndings
// every vCard upload.
//
// Beyond not panicking, the harness asserts the invariants the doc comments
// promise: NormalizeBirthday, NormalizeGender and normalizeVCardLineEndings are
// idempotent (a normalizer re-applied to its own output is a no-op — the
// property the import engine relies on when a row is re-processed); the line
// normalizer never grows its input; ParseCircles emits only non-empty,
// non-"*"-prefixed tokens; and parseSourceTime never returns a non-zero time
// alongside an error.
func FuzzImportNormalizers(f *testing.F) {
	for _, seed := range importNormalizerFuzzSeeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		s := string(data)

		birthday := NormalizeBirthday(s)
		if NormalizeBirthday(birthday) != birthday {
			t.Fatalf("NormalizeBirthday not idempotent: %q -> %q -> %q", s, birthday, NormalizeBirthday(birthday))
		}

		gender := NormalizeGender(s)
		if NormalizeGender(gender) != gender {
			t.Fatalf("NormalizeGender not idempotent: %q -> %q -> %q", s, gender, NormalizeGender(gender))
		}

		for _, c := range ParseCircles(s) {
			if strings.TrimSpace(c) == "" {
				t.Fatalf("ParseCircles emitted an empty token from %q", s)
			}
			if strings.HasPrefix(c, "*") {
				t.Fatalf("ParseCircles emitted a *-prefixed token %q from %q", c, s)
			}
		}

		if tm, err := parseSourceTime(s); err != nil && !tm.IsZero() {
			t.Fatalf("parseSourceTime returned non-zero time %v alongside error %v for %q", tm, err, s)
		}

		normalized := normalizeVCardLineEndings(data)
		if len(normalized) > len(data) {
			t.Fatalf("normalizeVCardLineEndings grew %d bytes to %d", len(data), len(normalized))
		}
		if !bytes.Equal(normalizeVCardLineEndings(normalized), normalized) {
			t.Fatalf("normalizeVCardLineEndings not idempotent for %q", data)
		}
	})
}
