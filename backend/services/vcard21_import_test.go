package services

import (
	"strings"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSniffVCardVersion_21 asserts vCard 2.1 is detected as its own version
// rather than silently falling through to the 4.0 default (T50's
// "compounding, separate issue").
func TestSniffVCardVersion(t *testing.T) {
	tests := []struct {
		name  string
		block string
		want  string
	}{
		{"2.1 exact", "BEGIN:VCARD\r\nVERSION:2.1\r\nEND:VCARD\r\n", "2.1"},
		{"2.0", "BEGIN:VCARD\r\nVERSION:2.0\r\nEND:VCARD\r\n", "2.1"},
		{"3.0", "BEGIN:VCARD\r\nVERSION:3.0\r\nEND:VCARD\r\n", "3.0"},
		{"4.0", "BEGIN:VCARD\r\nVERSION:4.0\r\nEND:VCARD\r\n", "4.0"},
		{"missing defaults to 4.0", "BEGIN:VCARD\r\nEND:VCARD\r\n", "4.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sniffVCardVersion([]byte(tt.block)))
		})
	}
}

// TestParseVCF_VCard21_BareTokenGrammar reproduces T50's reported real-world
// shapes directly (TEL;CELL;PREF:, EMAIL;PREF;HOME:,
// PHOTO;ENCODING=BASE64;JPEG:, the last folded across two physical lines to
// also exercise standard RFC 2425 continuation) and asserts phone, email and
// photo all survive import -- the three fields the ticket reports coming
// back blank.
func TestParseVCF_VCard21_BareTokenGrammar(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	// A real, tiny (1x1 transparent pixel) PNG, folded across two physical
	// lines the way real 2.1 exports wrap long PHOTO values (a line
	// starting with a single space is a continuation, RFC 2425).
	raw := "BEGIN:VCARD\r\n" +
		"VERSION:2.1\r\n" +
		"N:Brunning;Elizabeth;;;\r\n" +
		"FN:Elizabeth Brunning\r\n" +
		"TEL;CELL;PREF:608-514-2711\r\n" +
		"EMAIL;PREF;HOME:elizabeth.brunning@gmail.com\r\n" +
		"PHOTO;ENCODING=BASE64;JPEG:iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk\r\n" +
		" +A8AAQUBAScY42YAAAAASUVORK5CYII=\r\n" +
		"END:VCARD\r\n"

	contacts, previews, stats, err := ParseVCF(strings.NewReader(raw), db, user.ID)
	require.NoError(t, err)
	require.Len(t, contacts, 1)
	require.Len(t, previews, 1)
	assert.Equal(t, 1, stats.ValidCount)
	assert.Equal(t, 0, stats.ErrorCount)

	contact := contacts[0].Contact
	assert.Equal(t, "Elizabeth", contact.Firstname)
	assert.Equal(t, "Brunning", contact.Lastname)

	// The reported bug: these three all came back blank.
	assert.Equal(t, "608-514-2711", contact.Phone, "TEL;CELL;PREF: bare-token grammar must not blank the phone")
	assert.Equal(t, "elizabeth.brunning@gmail.com", contact.Email, "EMAIL;PREF;HOME: bare-token grammar must not blank the email")

	require.NotEmpty(t, contacts[0].PhotoData, "PHOTO;ENCODING=BASE64;JPEG: bare-token grammar must not drop the photo")
	assert.Equal(t, "image/jpeg", contacts[0].PhotoMediaType)

	// PREF on TEL (RFC 2426 §3.3.2: PREF is itself a TYPE token in
	// 2.1/3.0's grammar) must also have come through, not just the value.
	require.Len(t, contact.Phones, 1)
	assert.Equal(t, "608-514-2711", contact.Phones[0].Value)
}

// TestParseVCF_VCard21_QuotedPrintable proves the QUOTED-PRINTABLE decoding
// path works, including a genuine soft line break (RFC 2045 §6.7: a
// trailing "=" with no leading whitespace on the continuation line) --
// go-vcard has no notion of that continuation mechanism on its own.
func TestParseVCF_VCard21_QuotedPrintable(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	raw := "BEGIN:VCARD\r\n" +
		"VERSION:2.1\r\n" +
		"N:Roe;Jane;;;\r\n" +
		"FN:Jane Roe\r\n" +
		"NOTE;ENCODING=QUOTED-PRINTABLE;CHARSET=UTF-8:Caf=C3=A9 owner cont=\r\n" +
		"act, second line of the note.\r\n" +
		"END:VCARD\r\n"

	contacts, previews, stats, err := ParseVCF(strings.NewReader(raw), db, user.ID)
	require.NoError(t, err)
	require.Len(t, contacts, 1)
	assert.Equal(t, 1, stats.ValidCount)

	contact := contacts[0].Contact
	require.Len(t, contact.Card.Notes, 1)
	assert.Equal(t, "Café owner contact, second line of the note.", contact.Card.Notes[0].Note)

	// A successful decode should not itself surface as a diagnostic.
	for _, d := range previews[0].Diagnostics {
		assert.NotContains(t, d.Message, "QUOTED-PRINTABLE", "a successful QP decode should not warn: %q", d.Message)
	}
}

// TestParseVCF_VCard21_MalformedQuotedPrintable_Diagnostic asserts a
// property that genuinely can't be decoded even after normalization
// surfaces as a contactmodel.Diagnostic warning (visible in the preview)
// rather than being silently dropped or aborting the whole import.
func TestParseVCF_VCard21_MalformedQuotedPrintable_Diagnostic(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	// A raw DEL byte (0x7F) unescaped in a QUOTED-PRINTABLE body is invalid
	// per RFC 2045 and mime/quotedprintable.Reader rejects it outright
	// (unlike a bad hex digit after "=", which it tolerates as a literal).
	raw := "BEGIN:VCARD\r\n" +
		"VERSION:2.1\r\n" +
		"N:Doe;John;;;\r\n" +
		"FN:John Doe\r\n" +
		"NOTE;ENCODING=QUOTED-PRINTABLE:bad\x7fbyte\r\n" +
		"END:VCARD\r\n"

	contacts, previews, stats, err := ParseVCF(strings.NewReader(raw), db, user.ID)
	require.NoError(t, err)
	require.Len(t, contacts, 1)
	require.Len(t, previews, 1)
	assert.Equal(t, 1, stats.ValidCount, "an undecodable property degrades gracefully, it doesn't fail the whole card")

	found := false
	for _, d := range previews[0].Diagnostics {
		if strings.Contains(d.Message, "QUOTED-PRINTABLE") {
			found = true
		}
	}
	assert.True(t, found, "expected a QUOTED-PRINTABLE diagnostic, got %v", previews[0].Diagnostics)

	// The contact itself must still have been produced (name recovered),
	// not lost entirely.
	assert.Equal(t, "John", contacts[0].Contact.Firstname)
}

// TestParseVCF_VCard21_RoutesThroughVCard3Adapter asserts a 2.1 block is
// routed through the vcard3 adapter after normalization, not vcard4:
// vcard3's importMediaURI already tolerates 2.1-style ENCODING=BASE64,
// vcard4 only understands native "data:" URIs (see ParseVCF's routing
// comment). This is exercised indirectly by the photo test above already
// working at all -- this test pins the same expectation for a
// non-PREF/plain TEL, guarding against a future regression that routes 2.1
// to vcard4 instead.
func TestParseVCF_VCard21_RoutesThroughVCard3Adapter(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	raw := "BEGIN:VCARD\r\n" +
		"VERSION:2.1\r\n" +
		"N:Smith;Sam;;;\r\n" +
		"FN:Sam Smith\r\n" +
		"TEL;WORK;VOICE:212-555-0100\r\n" +
		"END:VCARD\r\n"

	contacts, _, stats, err := ParseVCF(strings.NewReader(raw), db, user.ID)
	require.NoError(t, err)
	require.Len(t, contacts, 1)
	assert.Equal(t, 1, stats.ValidCount)
	assert.Equal(t, "212-555-0100", contacts[0].Contact.Phone)
}
