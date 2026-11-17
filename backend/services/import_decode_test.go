package services

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeImportText(t *testing.T) {
	tests := []struct {
		name     string
		in       []byte
		want     string
		wantWarn bool
	}{
		{"empty file", nil, "", false},
		{"BOM only", []byte("\xef\xbb\xbf"), "", false},
		{"plain ascii", []byte("a,b\n1,2\n"), "a,b\n1,2\n", false},
		{"valid UTF-8 is untouched", []byte("José,Müller\n"), "José,Müller\n", false},
		{"BOM stripped once", []byte("\xef\xbb\xbfName\n"), "Name\n", false},
		{"only one BOM stripped", []byte("\xef\xbb\xbf\xef\xbb\xbfName\n"), "\ufeffName\n", false},
		{"latin1 bytes read as windows-1252", []byte("Jos\xe9,M\xfcller\n"), "José,Müller\n", true},
		{"windows-1252 specials (euro, curly quote)", []byte("\x80 \x93hi\x94"), "€ “hi”", true},
		{"BOM then latin1", []byte("\xef\xbb\xbfJos\xe9"), "José", true},
		// A file that is mostly UTF-8 with one stray byte is read as one
		// encoding, deterministically: Windows-1252 for the whole file.
		{"mixed encodings", []byte("Jos\xc3\xa9,M\xfcller"), "JosÃ©,Müller", true},
		{"truncated multibyte sequence at EOF", []byte("caf\xc3"), "cafÃ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, diags := DecodeImportText(tt.in)
			assert.Equal(t, tt.want, string(got))
			assert.True(t, utf8.Valid(got), "output must always be valid UTF-8")
			if tt.wantWarn {
				require.Len(t, diags, 1)
				assert.Equal(t, "warn", diags[0].Severity)
				assert.Equal(t, "encoding", diags[0].Concept)
				assert.Equal(t, ImportEncodingFallbackMessage, diags[0].Message)
			} else {
				assert.Empty(t, diags)
			}
		})
	}
}

func TestDecodeImportText_HugeLine(t *testing.T) {
	line := bytes.Repeat([]byte("x\xe9"), 4<<20) // 8 MiB, one line, every other byte invalid UTF-8
	got, diags := DecodeImportText(line)
	require.Len(t, diags, 1)
	assert.Equal(t, 4<<20*3, len(got), "each é is two bytes of UTF-8 after the x")
	assert.True(t, strings.HasPrefix(string(got), "xéx"))
}

func TestParseCSV_StripsBOMAndDecodesWindows1252(t *testing.T) {
	headers, rows, err := ParseCSV(strings.NewReader("\xef\xbb\xbfFirst Name,Last Name\nJos\xe9,M\xfcller\n"))
	require.NoError(t, err)
	assert.Equal(t, []string{"First Name", "Last Name"}, headers)
	assert.Equal(t, [][]string{{"José", "Müller"}}, rows)
}

func TestParseCSVWithDiagnostics_ReportsFallback(t *testing.T) {
	_, _, diags, err := ParseCSVWithDiagnostics(strings.NewReader("a\nJos\xe9\n"))
	require.NoError(t, err)
	require.Len(t, diags, 1)
	assert.Equal(t, ImportEncodingFallbackMessage, diags[0].Message)

	_, _, diags, err = ParseCSVWithDiagnostics(strings.NewReader("a\nJosé\n"))
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestParseCSV_BOMOnlyIsEmpty(t *testing.T) {
	_, _, err := ParseCSV(strings.NewReader("\xef\xbb\xbf"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestParseEntityCSVSections_StripsBOM(t *testing.T) {
	// A BOM in front of the first section marker would hide it from the
	// marker regex, silently dropping that whole section.
	sections, _, err := ParseEntityCSVSections(strings.NewReader("\xef\xbb\xbf" + entityCSVSampleForBOM))
	require.NoError(t, err)
	plain, _, err := ParseEntityCSVSections(strings.NewReader(entityCSVSampleForBOM))
	require.NoError(t, err)
	assert.Equal(t, plain, sections)
}

const entityCSVSampleForBOM = "=== ENTITIES ===\nid,kind,name,sensitivity,core\ne1,organization,Acme,normal,{}\n"
