package models

import (
	"testing"

	"mycorrhizal/contactmodel"
)

// ADR 0025: a plain flat-path save must not destroy periods it cannot express.
// The untouched address survives the merge (with its ID), so its period is
// preserved; a period whose entry no longer exists is pruned rather than left
// dangling.
func TestMergeRecordFromFlat_PreservesPeriods(t *testing.T) {
	loadedAddr := AddressFromContactAddress(ContactAddress{Street: "1 Main St"})
	loadedAddr.ID = "a1"

	period := contactmodel.TemporalRange{Start: &contactmodel.PartialDate{Year: intPtr(2019)}}
	loaded := contactmodel.Record{
		Card: contactmodel.Card{Addresses: []contactmodel.Address{loadedAddr}},
		Envelope: contactmodel.CRMEnvelope{Periods: []contactmodel.EntryPeriod{
			{Kind: contactmodel.PeriodKindAddress, EntryID: "a1", Range: period},
			{Kind: contactmodel.PeriodKindAddress, EntryID: "gone", Range: period},
		}},
	}
	// The flat derivation re-projects the same address (no components changed).
	fresh := contactmodel.Record{Card: contactmodel.Card{
		Addresses: []contactmodel.Address{AddressFromContactAddress(ContactAddress{Street: "1 Main St"})},
	}}

	merged := mergeRecordFromFlat(loaded, fresh)

	if len(merged.Card.Addresses) != 1 || merged.Card.Addresses[0].ID != "a1" {
		t.Fatalf("the untouched address (and its ID) must survive: %+v", merged.Card.Addresses)
	}
	if len(merged.Envelope.Periods) != 1 || merged.Envelope.Periods[0].EntryID != "a1" {
		t.Fatalf("expected only the resolvable period to survive, got %+v", merged.Envelope.Periods)
	}
}

// A flat edit that replaces the address drops its ID (the flat shape has no
// ID), so the old period is pruned — the caller said so — and no orphan is
// stored.
func TestMergeRecordFromFlat_PrunesPeriodWhenEntryReplaced(t *testing.T) {
	loadedAddr := AddressFromContactAddress(ContactAddress{Street: "1 Main St"})
	loadedAddr.ID = "a1"
	loaded := contactmodel.Record{
		Card: contactmodel.Card{Addresses: []contactmodel.Address{loadedAddr}},
		Envelope: contactmodel.CRMEnvelope{Periods: []contactmodel.EntryPeriod{
			{Kind: contactmodel.PeriodKindAddress, EntryID: "a1", Range: contactmodel.TemporalRange{
				Start: &contactmodel.PartialDate{Year: intPtr(2019)},
			}},
		}},
	}
	fresh := contactmodel.Record{Card: contactmodel.Card{
		Addresses: []contactmodel.Address{AddressFromContactAddress(ContactAddress{Street: "2 Changed Rd"})},
	}}

	merged := mergeRecordFromFlat(loaded, fresh)

	if len(merged.Envelope.Periods) != 0 {
		t.Errorf("a replaced entry's period must be pruned, got %+v", merged.Envelope.Periods)
	}
}
