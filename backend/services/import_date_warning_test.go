package services

import (
	"context"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issues #2631 / #2653, maintainer decision "option C" (2026-11-17): an
// impossible or unparseable birthday/anniversary is NOT a row error and is not
// silently dropped. The contact imports without that date and the row carries
// a warn diagnostic "<field> not imported: invalid value" (the existing
// per-row `diagnostics` shape; no new field, no new status).

func dateWarnings(p models.ImportRowPreview) []contactmodel.Diagnostic {
	var out []contactmodel.Diagnostic
	for _, d := range p.Diagnostics {
		if d.Concept == "birthday" || d.Concept == "anniversary" {
			out = append(out, d)
		}
	}
	return out
}

func csvDatePreview(t *testing.T, birthday, anniversary string) (models.ImportRowPreview, ImportStats) {
	t.Helper()
	db := setupImportSessionTestDB(t)
	headers := []string{"First Name", "Last Name", "Birthday", "Anniversary"}
	mappings := []models.ColumnMapping{
		{CSVColumn: "First Name", ContactField: "firstname"},
		{CSVColumn: "Last Name", ContactField: "lastname"},
		{CSVColumn: "Birthday", ContactField: "birthday"},
		{CSVColumn: "Anniversary", ContactField: "anniversary"},
	}
	_, previews, stats := GenerateCSVPreview(db, 1, [][]string{{"Ada", "Lovelace", birthday, anniversary}}, headers, mappings)
	require.Len(t, previews, 1)
	return previews[0], stats
}

func TestCSVPreview_InvalidDateIsAWarningNotAnError(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{"impossible day", "1991-02-30"},
		{"feb 29 in a non-leap year", "1991-02-29"},
		{"month 13", "1991-13-01"},
		{"impossible legacy format", "30.02.1991"},
		{"not a date", "notadate"},
		{"unsupported slash format", "05/12/1990"},
		{"day without year, impossible", "--04-31"},
	} {
		t.Run("birthday "+tc.name, func(t *testing.T) {
			p, stats := csvDatePreview(t, tc.value, "")
			assert.Empty(t, p.ValidationErrors, "an invalid birthday is not a row error")
			assert.Equal(t, "add", p.SuggestedAction)
			assert.Equal(t, 1, stats.ValidCount)
			assert.Zero(t, stats.ErrorCount)
			assert.Equal(t, "", GetStringField(p.ParsedContact, "birthday"), "the contact imports without the date")
			assert.Equal(t, "Ada", GetStringField(p.ParsedContact, "firstname"))
			w := dateWarnings(p)
			require.Len(t, w, 1)
			assert.Equal(t, contactmodel.Diagnostic{Severity: "warn", Concept: "birthday", Message: "birthday not imported: invalid value"}, w[0])
		})
	}

	t.Run("anniversary", func(t *testing.T) {
		p, _ := csvDatePreview(t, "", "1990-02-31")
		assert.Empty(t, p.ValidationErrors)
		assert.Equal(t, "", GetStringField(p.ParsedContact, "anniversary"))
		w := dateWarnings(p)
		require.Len(t, w, 1)
		assert.Equal(t, contactmodel.Diagnostic{Severity: "warn", Concept: "anniversary", Message: "anniversary not imported: invalid value"}, w[0])
	})
}

func TestCSVPreview_ValidDatesAreKeptWithoutWarning(t *testing.T) {
	for _, v := range []string{"1991-02-28", "1992-02-29", "--02-29", "--12-31", "29.06.1958"} {
		p, _ := csvDatePreview(t, v, "")
		assert.Empty(t, p.ValidationErrors, v)
		assert.Empty(t, dateWarnings(p), v)
		assert.NotEmpty(t, GetStringField(p.ParsedContact, "birthday"), v)
	}
}

func TestConfirmCSV_InvalidBirthdayContactImportsWithoutIt(t *testing.T) {
	db := setupImportSessionTestDB(t)
	m := NewImportSessionManager()
	id := m.CreateCSVSession(1, []string{"First Name", "Last Name", "Email", "Birthday"}, [][]string{{"Ada", "Lovelace", "ada@example.com", "1991-02-30"}})
	prev, appErr := m.PreviewCSV(db, 1, models.ImportPreviewRequest{SessionID: id, Mappings: append(csvMappings(), models.ColumnMapping{CSVColumn: "Birthday", ContactField: "birthday"})})
	require.Nil(t, appErr)
	require.Len(t, dateWarnings(prev.Rows[0]), 1)

	result, appErr := m.Confirm(db, 1, models.ImportConfirmRequest{SessionID: id, Actions: []models.RowImportAction{{RowIndex: 0, Action: "add"}}}, testImportLogger())
	require.Nil(t, appErr)
	assert.Equal(t, 1, result.Created)
	assert.Empty(t, result.Errors)

	var c models.Contact
	require.NoError(t, db.Where("email = ?", "ada@example.com").First(&c).Error)
	assert.Equal(t, "Lovelace", c.Lastname)
	assert.Equal(t, "", c.Birthday)
}

func parseOneVCF(t *testing.T, card string) (VCFContactData, models.ImportRowPreview, ImportStats) {
	t.Helper()
	db := setupImportSessionTestDB(t)
	contacts, previews, stats, err := ParseVCF(strings.NewReader(card), db, 1)
	require.NoError(t, err)
	require.Len(t, contacts, 1)
	return contacts[0], previews[0], stats
}

func TestParseVCF_InvalidBdayAndAnniversaryAreWarnings(t *testing.T) {
	v4 := func(lines ...string) string {
		return "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Ada Lovelace\r\nN:Lovelace;Ada;;;\r\n" + strings.Join(lines, "\r\n") + "\r\nEND:VCARD\r\n"
	}
	v3 := func(lines ...string) string {
		return "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Ada Lovelace\r\nN:Lovelace;Ada;;;\r\n" + strings.Join(lines, "\r\n") + "\r\nEND:VCARD\r\n"
	}
	for _, tc := range []struct {
		name, card, concept string
	}{
		{"v4 BDAY garbage", v4("BDAY:notadate"), "birthday"},
		{"v4 BDAY impossible day", v4("BDAY:1991-02-30"), "birthday"},
		{"v4 BDAY compact impossible", v4("BDAY:19910230"), "birthday"},
		{"v4 BDAY unparseable timestamp", v4("BDAY:TOMORROW"), "birthday"},
		{"v4 ANNIVERSARY garbage", v4("ANNIVERSARY:notadate"), "anniversary"},
		{"v3 BDAY garbage", v3("BDAY:notadate"), "birthday"},
		{"v3 BDAY impossible day", v3("BDAY:1991-02-30"), "birthday"},
		{"v3 X-ANNIVERSARY impossible", v3("X-ANNIVERSARY:1990-04-31"), "anniversary"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, p, stats := parseOneVCF(t, tc.card)
			assert.Empty(t, p.ValidationErrors)
			assert.Equal(t, "add", p.SuggestedAction)
			assert.Equal(t, 1, stats.ValidCount)
			assert.Equal(t, "", c.Contact.Birthday)
			assert.Equal(t, "", c.Contact.Anniversary)
			for _, a := range c.Contact.Card.Anniversaries {
				assert.NotContains(t, []string{"birth", "wedding"}, a.Kind, "the invalid date must not be carried in the stored Card either")
			}
			w := dateWarnings(p)
			require.Len(t, w, 1)
			assert.Equal(t, "warn", w[0].Severity)
			assert.Equal(t, tc.concept, w[0].Concept)
			assert.Equal(t, tc.concept+" not imported: invalid value", w[0].Message)
			assert.Equal(t, "Ada", c.Contact.Firstname)
		})
	}
}

func TestParseVCF_ValidBdayKeptAndInvalidAnniversaryDoesNotCostIt(t *testing.T) {
	c, p, _ := parseOneVCF(t, "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Ada Lovelace\r\nN:Lovelace;Ada;;;\r\nBDAY:1990-05-01\r\nANNIVERSARY:notadate\r\nEND:VCARD\r\n")
	assert.Equal(t, "1990-05-01", c.Contact.Birthday)
	w := dateWarnings(p)
	require.Len(t, w, 1)
	assert.Equal(t, "anniversary", w[0].Concept)

	c, p, _ = parseOneVCF(t, "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Ada Lovelace\r\nN:Lovelace;Ada;;;\r\nBDAY:--02-29\r\nEND:VCARD\r\n")
	assert.Equal(t, "--02-29", c.Contact.Birthday)
	assert.Empty(t, dateWarnings(p))
}

func TestConfirmVCF_InvalidBdayContactImportsWithoutIt(t *testing.T) {
	db := setupImportSessionTestDB(t)
	m := NewImportSessionManager()
	contacts, previews, _, err := ParseVCF(strings.NewReader("BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Ada Lovelace\r\nN:Lovelace;Ada;;;\r\nEMAIL:ada.bday@example.com\r\nBDAY:notadate\r\nEND:VCARD\r\n"), db, 1)
	require.NoError(t, err)
	id := m.CreateVCFSession(1, contacts, previews)
	result, appErr := m.ConfirmVCF(context.Background(), db, 1, models.ImportConfirmRequest{SessionID: id, Actions: []models.RowImportAction{{RowIndex: 0, Action: "add"}}}, testImportConfig(t), testImportLogger())
	require.Nil(t, appErr)
	require.Equal(t, 1, result.Created)

	var c models.Contact
	require.NoError(t, db.Where("email = ?", "ada.bday@example.com").First(&c).Error)
	assert.Equal(t, "", c.Birthday)
	for _, a := range c.Card.Anniversaries {
		assert.NotEqual(t, "birth", a.Kind)
	}
}

func TestParseJSContact_InvalidBirthdayIsAWarning(t *testing.T) {
	db := setupImportSessionTestDB(t)
	card := `{"@type":"Card","version":"1.0","uid":"js-bday","name":{"full":"Ada Lovelace","components":[{"kind":"given","value":"Ada"},{"kind":"surname","value":"Lovelace"}]},
	"anniversaries":{"A1":{"@type":"Anniversary","kind":"birth","date":{"@type":"PartialDate","year":1991,"month":2,"day":30}}}}`
	contacts, previews, stats, err := ParseJSContact(strings.NewReader(card), db, 1)
	require.NoError(t, err)
	require.Len(t, contacts, 1)
	assert.Equal(t, 1, stats.ValidCount)
	assert.Empty(t, previews[0].ValidationErrors)
	assert.Equal(t, "", contacts[0].Contact.Birthday)
	assert.Equal(t, []contactmodel.Diagnostic{{Severity: "warn", Concept: "birthday", Message: "birthday not imported: invalid value"}}, dateWarnings(previews[0]))
}

func TestDropInvalidRecordDates(t *testing.T) {
	ts := func(s string) *string { return &s }
	rec := func(a ...contactmodel.Anniversary) *contactmodel.Record {
		return &contactmodel.Record{Card: contactmodel.Card{Anniversaries: a}}
	}
	partial := func(kind string, y, m, d *int) contactmodel.Anniversary {
		return contactmodel.Anniversary{Kind: kind, Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Year: y, Month: m, Day: d}}}
	}
	valid := []contactmodel.Anniversary{
		partial("birth", intp(1992), intp(2), intp(29)), // leap day
		partial("birth", nil, intp(2), intp(29)),        // year-less leap day
		partial("birth", intp(1991), nil, nil),          // year only
		partial("birth", nil, intp(12), nil),            // month only
		partial("wedding", nil, nil, intp(31)),          // day only
		{Kind: "birth", Date: contactmodel.AnniversaryDate{Timestamp: ts("1990-05-01T10:00:00Z")}},
		{Kind: "birth", Date: contactmodel.AnniversaryDate{}}, // empty: nothing to judge
		partial("death", intp(1991), intp(2), intp(30)),       // death dates are not imported into flat fields: untouched
	}
	for i, a := range valid {
		r := rec(a)
		assert.Empty(t, DropInvalidRecordDates(r), "valid case %d", i)
		assert.Len(t, r.Card.Anniversaries, 1, "valid case %d kept", i)
	}

	invalid := []contactmodel.Anniversary{
		partial("birth", intp(1900), intp(2), intp(29)), // 1900 is not a leap year
		partial("birth", intp(1991), intp(4), intp(31)),
		partial("birth", intp(1991), intp(0), intp(5)),
		partial("birth", intp(1991), intp(13), intp(5)),
		partial("birth", intp(1991), intp(5), intp(0)),
		partial("birth", nil, nil, intp(32)),
		partial("birth", intp(-1), nil, nil),
		partial("birth", nil, nil, nil),
		{Kind: "birth", Date: contactmodel.AnniversaryDate{Timestamp: ts("tomorrow")}},
	}
	for i, a := range invalid {
		r := rec(a)
		d := DropInvalidRecordDates(r)
		require.Len(t, d, 1, "invalid case %d", i)
		assert.Equal(t, "birthday", d[0].Concept)
		assert.Empty(t, r.Card.Anniversaries, "invalid case %d dropped", i)
	}

	// Two bad birthdays on one card warn once; the good wedding survives.
	r := rec(partial("birth", intp(1991), intp(2), intp(30)), partial("birth", nil, nil, nil), partial("wedding", intp(2010), intp(6), intp(1)))
	d := DropInvalidRecordDates(r)
	assert.Len(t, d, 1)
	require.Len(t, r.Card.Anniversaries, 1)
	assert.Equal(t, "wedding", r.Card.Anniversaries[0].Kind)

	assert.Nil(t, DropInvalidRecordDates(nil))
	assert.Nil(t, DropInvalidRecordDates(&contactmodel.Record{}))
}

func TestIsValidCalendarDate(t *testing.T) {
	for _, s := range []string{"1991-02-28", "1992-02-29", "2000-02-29", "--02-29", "--12-31", "0001-01-01"} {
		assert.True(t, isValidCalendarDate(s), s)
	}
	for _, s := range []string{"", "1991-02-30", "1900-02-29", "--02-30", "--13-01", "--00-10", "1991-00-10", "1991-1-1", "notadate", "1991-02-30 "} {
		assert.False(t, isValidCalendarDate(s), s)
	}
}
