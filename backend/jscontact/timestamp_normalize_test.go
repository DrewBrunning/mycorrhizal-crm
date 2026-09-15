package jscontact

import (
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/rfctest"
)

// Issue #969: contactmodel.Timestamp.UTC is documented as RFC3339 but not
// guaranteed to already be UTC ("Z") — an upstream adapter could put an
// offset-bearing value there. This is the defense-in-depth boundary check:
// every JSContact `utc`/UTCDateTime wire value this package emits or reads
// must actually be UTC, converting a non-Z offset rather than passing it
// through unconverted (which a conformant consumer would misread as if the
// offset were zero).

func TestExport_CreatedNormalizesNonUTCOffset(t *testing.T) {
	t.Parallel()
	rec := &contactmodel.Record{Card: contactmodel.Card{
		UID:     "created-offset-example",
		Created: &contactmodel.Timestamp{UTC: "1985-04-12T10:00:00-05:00"},
	}}
	out, _, err := Adapter{}.Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertJSONPointer(t, out, "/created", "1985-04-12T15:00:00Z")
}

func TestImport_CreatedNormalizesNonUTCOffset(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"@type":"Card","version":"1.0","uid":"created-offset-example","created":"1985-04-12T10:00:00-05:00"}`)
	rec, _, err := Adapter{}.Import(raw)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if rec.Card.Created == nil {
		t.Fatalf("Created = nil")
	}
	if want := "1985-04-12T15:00:00Z"; rec.Card.Created.UTC != want {
		t.Errorf("Created.UTC = %q, want %q", rec.Card.Created.UTC, want)
	}
}

func TestExport_AnniversaryTimestampNormalizesNonUTCOffset(t *testing.T) {
	t.Parallel()
	ts := "1985-04-12T10:00:00-05:00"
	rec := &contactmodel.Record{Card: contactmodel.Card{
		UID: "anniversary-offset-example",
		Anniversaries: []contactmodel.Anniversary{
			{ID: "A1", Kind: "birth", Date: contactmodel.AnniversaryDate{Timestamp: &ts}},
		},
	}}
	out, _, err := Adapter{}.Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertJSONPointer(t, out, "/anniversaries/A1/date/utc", "1985-04-12T15:00:00Z")
}

func TestImport_AnniversaryTimestampNormalizesNonUTCOffset(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"@type":"Card","version":"1.0","uid":"anniversary-offset-example","anniversaries":{"A1":{"@type":"Anniversary","kind":"birth","date":{"@type":"Timestamp","utc":"1985-04-12T10:00:00-05:00"}}}}`)
	rec, _, err := Adapter{}.Import(raw)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(rec.Card.Anniversaries) != 1 {
		t.Fatalf("Anniversaries = %+v, want exactly one", rec.Card.Anniversaries)
	}
	date := rec.Card.Anniversaries[0].Date
	if date.Timestamp == nil {
		t.Fatalf("Date.Timestamp = nil")
	}
	if want := "1985-04-12T15:00:00Z"; *date.Timestamp != want {
		t.Errorf("Date.Timestamp = %q, want %q", *date.Timestamp, want)
	}
}
