package services

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/charmap"
)

func encodeWindows1252(s string) ([]byte, error) {
	return charmap.Windows1252.NewEncoder().Bytes([]byte(s))
}

// Issue #2688: a binary file renamed .csv must be rejected with
// ErrImportNotText, while every legitimate text encoding the decoder accepts
// (UTF-8, UTF-8 BOM, Windows-1252/Latin-1) must pass untouched.

var sniffCSV = "First Name,Last Name,Email\nJosé,Müller,jose@example.com\n"

func TestSniffImportText_Binaries(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte("abc,def\n"), 20)...)
	zip := append([]byte("PK\x03\x04"), bytes.Repeat([]byte("a,b\n"), 20)...)
	// An .xlsx is a ZIP container.
	xlsx := append([]byte("PK\x03\x04\x14\x00\x06\x00"), []byte("[Content_Types].xml")...)
	emptyZip := []byte("PK\x05\x06" + strings.Repeat("\x00", 18))
	pdf := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("a,b\n"), 20)...)
	gif := append([]byte("GIF89a"), bytes.Repeat([]byte("a,b\n"), 20)...)
	jpeg := append([]byte("\xff\xd8\xff\xe0"), bytes.Repeat([]byte("a,b\n"), 20)...)
	gz := append([]byte("\x1f\x8b\x08"), bytes.Repeat([]byte("a,b\n"), 20)...)
	ole := append([]byte("\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"), bytes.Repeat([]byte("a,b\n"), 20)...)
	utf16le := append([]byte{0xFF, 0xFE}, []byte("F\x00i\x00r\x00s\x00t\x00,\x00N\x00\n\x00")...)
	utf16be := append([]byte{0xFE, 0xFF}, []byte("\x00F\x00i\x00r\x00s\x00t\x00,\x00N\x00\n")...)
	utf16NoBOM := []byte("F\x00i\x00r\x00s\x00t\x00,\x00N\x00\n\x00")
	nul := []byte("First,Last\nAda,Love\x00lace\n")
	// 40 control bytes in 400: 10% control, no NUL, no magic number.
	ctrl := bytes.Repeat([]byte("abcdefghi\x01"), 40)
	// Pseudo-random bytes with no NUL and no magic prefix.
	var rnd []byte
	for i := 0; i < 4096; i++ {
		b := byte((i*131 + 7) % 251)
		if b == 0 {
			b = 1
		}
		rnd = append(rnd, b)
	}

	cases := map[string][]byte{
		"png": png, "zip": zip, "xlsx": xlsx, "empty zip": emptyZip, "pdf": pdf,
		"gif": gif, "jpeg": jpeg, "gzip": gz, "ole xls": ole,
		"utf16le bom": utf16le, "utf16be bom": utf16be, "utf16 no bom": utf16NoBOM,
		"nul byte": nul, "control ratio": ctrl, "noise": rnd,
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			err := SniffImportText(data)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrImportNotText)
		})
	}
}

func TestSniffImportText_LegitimateText(t *testing.T) {
	win1252, err := encodeWindows1252(sniffCSV + "Zoë,Ångström,€ price\n")
	require.NoError(t, err)
	var longLine strings.Builder
	longLine.WriteString("Notes\n\"")
	longLine.WriteString(strings.Repeat("x", 2<<20))
	longLine.WriteString("\"\n")
	manyCtrl := strings.Repeat("a,b\r\n", 100) + "\ta\f\n" // tabs, CR, FF are text

	cases := map[string][]byte{
		"utf8":             []byte(sniffCSV),
		"utf8 bom":         append([]byte{0xEF, 0xBB, 0xBF}, []byte(sniffCSV)...),
		"windows-1252":     win1252,
		"empty":            {},
		"bom only":         {0xEF, 0xBB, 0xBF},
		"very long line":   []byte(longLine.String()),
		"tabs and crlf":    []byte(manyCtrl),
		"cjk utf8":         []byte("名前,メール\n山田,y@example.jp\n"),
		"png word in text": []byte("Name\nPNG,PK,%PDF-\n"),
		"high bytes only":  bytes.Repeat([]byte{0xE9}, 500),
		"nul after window": append(bytes.Repeat([]byte("a,b\n"), 3000), 0x00), // beyond the 8 KiB sniff window
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, SniffImportText(data))
		})
	}
}

func TestParseCSV_RejectsBinaryAndKeepsLegitimate(t *testing.T) {
	_, _, _, err := ParseCSVWithDiagnostics(bytes.NewReader(append([]byte("\x89PNG\r\n\x1a\n"), 0, 0, 0, 13)))
	require.ErrorIs(t, err, ErrImportNotText)

	win1252, encErr := encodeWindows1252(sniffCSV)
	require.NoError(t, encErr)
	headers, rows, diags, err := ParseCSVWithDiagnostics(bytes.NewReader(win1252))
	require.NoError(t, err)
	assert.Equal(t, []string{"First Name", "Last Name", "Email"}, headers)
	assert.Equal(t, "José", rows[0][0])
	require.Len(t, diags, 1)
	assert.Equal(t, ImportEncodingConcept, diags[0].Concept)

	headers, _, diags, err = ParseCSVWithDiagnostics(bytes.NewReader(append([]byte{0xEF, 0xBB, 0xBF}, []byte(sniffCSV)...)))
	require.NoError(t, err)
	assert.Equal(t, "First Name", headers[0])
	assert.Empty(t, diags)
}

func TestParseEntityCSVSections_RejectsBinary(t *testing.T) {
	_, _, err := ParseEntityCSVSections(bytes.NewReader([]byte("PK\x03\x04junk")))
	require.ErrorIs(t, err, ErrImportNotText)
	assert.True(t, IsEntityCSVRejection(err), "a binary upload is the client's fault (400), not a 500")
}
