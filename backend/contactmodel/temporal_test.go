package contactmodel

import (
	"encoding/json"
	"testing"
)

func intPtr(v int) *int { return &v }

func date(y, m, d int) *PartialDate {
	return &PartialDate{Year: intPtr(y), Month: intPtr(m), Day: intPtr(d)}
}

func dateRange(start, end *PartialDate) TemporalRange {
	return TemporalRange{Start: start, End: end}
}

// periodCard returns a Card with one address/org/title, each carrying an ID,
// so period references have something to resolve against.
func periodCard() Card {
	return Card{
		Addresses:     []Address{{ID: "a1", Full: "1 Main St"}},
		Organizations: []Organization{{ID: "o1", Name: "Acme"}},
		Titles:        []Title{{ID: "t1", Name: "Engineer", Kind: "title"}},
	}
}

func TestTemporalRangeIsEmpty(t *testing.T) {
	if !(TemporalRange{}).IsEmpty() {
		t.Error("zero TemporalRange should be empty")
	}
	if (TemporalRange{Start: date(2019, 1, 1)}).IsEmpty() {
		t.Error("a start-only range is not empty")
	}
	if (TemporalRange{End: date(2019, 1, 1)}).IsEmpty() {
		t.Error("an end-only range is not empty")
	}
}

func TestTemporalRangeReversed(t *testing.T) {
	year := func(y int) *PartialDate { return &PartialDate{Year: intPtr(y)} }
	tests := []struct {
		name string
		r    TemporalRange
		want bool
	}{
		{"open start", TemporalRange{End: date(2024, 1, 1)}, false},
		{"open end", TemporalRange{Start: date(2019, 1, 1)}, false},
		{"empty", TemporalRange{}, false},
		{"ordered", dateRange(date(2019, 1, 1), date(2024, 1, 1)), false},
		{"equal", dateRange(date(2019, 1, 1), date(2019, 1, 1)), false},
		{"reversed", dateRange(date(2024, 1, 1), date(2019, 1, 1)), true},
		{"coarse year span ordered", dateRange(year(2019), year(2024)), false},
		{"coarse year span reversed", dateRange(year(2024), year(2019)), true},
		{"month within same year reversed", dateRange(&PartialDate{Year: intPtr(2019), Month: intPtr(6)}, &PartialDate{Year: intPtr(2019), Month: intPtr(3)}), true},
		{"year-only vs full comparable at year", dateRange(year(2024), date(2019, 5, 1)), true},
		{"year-less not comparable", dateRange(&PartialDate{Month: intPtr(12)}, &PartialDate{Month: intPtr(1)}), false},
	}
	for _, tt := range tests {
		if got := tt.r.Reversed(); got != tt.want {
			t.Errorf("%s: Reversed = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestEnvelopePeriodLookup(t *testing.T) {
	env := CRMEnvelope{Periods: []EntryPeriod{
		{Kind: PeriodKindAddress, EntryID: "a1", Range: dateRange(date(2019, 1, 1), date(2024, 1, 1))},
		{Kind: PeriodKindOrganization, EntryID: "o1", Range: dateRange(nil, nil)},
	}}

	got, ok := env.Period(PeriodKindAddress, "a1")
	if !ok || got.Start == nil || *got.Start.Year != 2019 {
		t.Errorf("Period(address,a1) = %+v, %v; want the stored start", got, ok)
	}
	if _, ok := env.Period(PeriodKindAddress, "missing"); ok {
		t.Error("Period for an absent reference should report false")
	}
	// Same EntryID under a different Kind must not match.
	if _, ok := env.Period(PeriodKindOrganization, "a1"); ok {
		t.Error("Period must match on Kind as well as EntryID")
	}
}

func TestCardEntryIDs(t *testing.T) {
	c := periodCard()
	tests := []struct {
		kind string
		want []string
	}{
		{PeriodKindAddress, []string{"a1"}},
		{PeriodKindOrganization, []string{"o1"}},
		{PeriodKindTitle, []string{"t1"}},
	}
	for _, tt := range tests {
		ids := c.CardEntryIDs(tt.kind)
		if len(ids) != len(tt.want) {
			t.Fatalf("CardEntryIDs(%q) = %v, want %v", tt.kind, ids, tt.want)
		}
		for _, w := range tt.want {
			if _, ok := ids[w]; !ok {
				t.Errorf("CardEntryIDs(%q) missing %q", tt.kind, w)
			}
		}
	}
	if ids := c.CardEntryIDs("bogus"); ids != nil {
		t.Errorf("CardEntryIDs(unknown kind) = %v, want nil", ids)
	}

	// An empty ID is not a resolvable reference.
	blank := Card{Addresses: []Address{{ID: "", Full: "x"}}}
	if len(blank.CardEntryIDs(PeriodKindAddress)) != 0 {
		t.Error("blank element IDs must not be resolvable")
	}
}

func TestInvalidPeriods(t *testing.T) {
	card := periodCard()
	tests := []struct {
		name string
		p    EntryPeriod
		bad  bool
	}{
		{"resolves", EntryPeriod{Kind: PeriodKindAddress, EntryID: "a1", Range: dateRange(date(2020, 1, 1), nil)}, false},
		{"empty range", EntryPeriod{Kind: PeriodKindAddress, EntryID: "a1", Range: dateRange(nil, nil)}, true},
		{"missing kind", EntryPeriod{Kind: "", EntryID: "a1", Range: dateRange(date(2020, 1, 1), nil)}, true},
		{"missing entry id", EntryPeriod{Kind: PeriodKindAddress, EntryID: "", Range: dateRange(date(2020, 1, 1), nil)}, true},
		{"unresolved entry id", EntryPeriod{Kind: PeriodKindAddress, EntryID: "nope", Range: dateRange(date(2020, 1, 1), nil)}, true},
		{"reversed", EntryPeriod{Kind: PeriodKindAddress, EntryID: "a1", Range: dateRange(date(2024, 1, 1), date(2020, 1, 1))}, true},
		{"unknown kind", EntryPeriod{Kind: "phone", EntryID: "p1", Range: dateRange(date(2020, 1, 1), nil)}, true},
	}
	for _, tt := range tests {
		env := CRMEnvelope{Periods: []EntryPeriod{tt.p}}
		got := env.InvalidPeriods(card)
		if (len(got) > 0) != tt.bad {
			t.Errorf("%s: InvalidPeriods = %d entries, want bad=%v", tt.name, len(got), tt.bad)
		}
	}
}

func TestPruneOrphanPeriods(t *testing.T) {
	card := periodCard()
	env := CRMEnvelope{Periods: []EntryPeriod{
		{Kind: PeriodKindAddress, EntryID: "a1", Range: dateRange(date(2019, 1, 1), date(2024, 1, 1))},
		{Kind: PeriodKindAddress, EntryID: "gone", Range: dateRange(date(2019, 1, 1), nil)},
		{Kind: "phone", EntryID: "p1", Range: dateRange(date(2019, 1, 1), nil)},
		{Kind: PeriodKindTitle, EntryID: "t1", Range: dateRange(nil, date(2024, 1, 1))},
	}}
	kept := env.PruneOrphanPeriods(card)
	if len(kept) != 2 {
		t.Fatalf("PruneOrphanPeriods kept %d, want 2: %+v", len(kept), kept)
	}
	if kept[0].EntryID != "a1" || kept[1].EntryID != "t1" {
		t.Errorf("kept the wrong periods: %+v", kept)
	}
	if env2 := (CRMEnvelope{}); len(env2.PruneOrphanPeriods(card)) != 0 {
		t.Error("no periods in, no periods out")
	}
}

// TestEntryPeriodJSONRoundTrip pins the wire/storage shape: the ID must
// serialize (ADR 0001 — persistence identity depends on it) and a partial
// endpoint must survive.
func TestEntryPeriodJSONRoundTrip(t *testing.T) {
	in := CRMEnvelope{Periods: []EntryPeriod{{
		Kind:    PeriodKindAddress,
		EntryID: "a1",
		Range:   dateRange(date(2019, 5, 1), &PartialDate{Year: intPtr(2024)}),
	}}}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out CRMEnvelope
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Periods) != 1 || out.Periods[0].EntryID != "a1" {
		t.Fatalf("round trip lost the reference: %+v", out.Periods)
	}
	if out.Periods[0].Range.End == nil || out.Periods[0].Range.End.Month != nil {
		t.Errorf("year-only end did not survive: %+v", out.Periods[0].Range.End)
	}
}
