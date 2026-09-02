package models

import (
	"strings"

	"mycorrhizal/contactmodel"
)

// Derived-data rebuild + consistency support (issue #497).
//
// A contact carries several denormalized columns that are a projection of its
// authoritative data, kept live by BeforeSave:
//
//   - sort_name          <- DeriveSortName(lastname, firstname)
//   - addresses_flat      <- FlattenAddresses(addresses JSON)
//   - phones_normalized   <- FlattenPhones(phones JSON)
//   - address             <- FormatAddress(addresses[0])
//   - firstname/lastname/email/phone/birthday/fn/org
//                        <- contactmodel.DeriveProjection(Card)
//
// A write that bypasses the GORM hooks — a raw-SQL migration that touches
// `card` or a base column (000021, 000022 both do), a bulk import that
// INSERTs rows directly, a restore from a backup taken mid-write — can leave
// any of these stale. RecomputeDerivedColumns is the one read-only place the
// rebuild (services.RebuildDerivedContactColumns) and the INV-A5 consistency
// probe (services.checkDerivedContactColumns) both go to ask "what should
// these columns be, given the canonical data, and which ones are wrong".
//
// Canonical-authoritative, not the write-path merge: BeforeSave reconciles
// flat edits back onto a loaded Card (the T75 merge in
// contact_card_merge.go), letting a flat field win where it can express the
// change. A *rebuild* is the opposite question — Card is the source of truth,
// the flat columns are its shadow — so this recomputes the flat scalars
// straight from Card's projection, and would correct (never entrench) a flat
// column that a raw `card` write left disagreeing.

// DerivedContactColumns is the set of column names RecomputeDerivedColumns
// compares and RebuildDerivedContactColumns writes back.
var DerivedContactColumns = []string{
	"firstname", "lastname", "email", "phone", "address", "birthday",
	"fn", "org", "addresses_flat", "phones_normalized", "sort_name",
}

// DerivedColumnFix is one denormalized column whose stored value disagrees
// with the value re-derived from canonical data.
type DerivedColumnFix struct {
	Column string
	Want   string
}

// RecomputeDerivedColumns returns, for every denormalized column whose stored
// value on the receiver disagrees with the value re-derived from the
// contact's canonical data, the column name and the value it should hold. A
// faithful row — one last written through the ordinary GORM path — returns
// nil (the projection is a fixpoint of a re-save; ADR 0012 INV-A5 / INV-D8).
// The receiver is never mutated.
//
// The five Card-projection scalars (firstname/lastname/email/phone/birthday)
// are only reported when the projection yields a non-empty value that differs
// — mirroring BeforeSave's "only overwrite when there's something to sync"
// rule, so a pre-migration row that only ever had the scalar set (and whose
// Card is still the zero value) is not spuriously flagged.
//
// sort_name is compared case-insensitively: migration 000021 backfilled it
// with SQLite's ASCII-only lower(), while DeriveSortName folds Unicode, so a
// non-ASCII name that predates the migration and was never re-saved differs
// only in the case of its non-ASCII letters — a cosmetic ordering nuance the
// migration documents as harmless. A stale key (wrong name entirely) still
// shows.
func (c *Contact) RecomputeDerivedColumns() []DerivedColumnFix {
	var fixes []DerivedColumnFix

	proj := contactmodel.DeriveProjection(&contactmodel.Record{
		Card: c.Card, Envelope: c.CRM, Passthrough: c.Passthrough,
	})
	// Only-overwrite-when-non-empty scalars.
	for _, s := range []struct{ col, want, have string }{
		{"firstname", proj.Firstname, c.Firstname},
		{"lastname", proj.Lastname, c.Lastname},
		{"email", proj.PrimaryEmail, c.Email},
		{"phone", proj.PrimaryPhone, c.Phone},
		{"birthday", proj.Birthday, c.Birthday},
	} {
		if s.want != "" && s.want != s.have {
			fixes = append(fixes, DerivedColumnFix{Column: s.col, Want: s.want})
		}
	}
	// The name-sort key derives from the FINAL first/last (BeforeSave computes
	// it after the projection may have replaced them), so resolve those first.
	effFirst, effLast := c.Firstname, c.Lastname
	if proj.Firstname != "" {
		effFirst = proj.Firstname
	}
	if proj.Lastname != "" {
		effLast = proj.Lastname
	}
	// Always-assigned scalars.
	if proj.FN != c.FN {
		fixes = append(fixes, DerivedColumnFix{Column: "fn", Want: proj.FN})
	}
	if proj.Org != c.Org {
		fixes = append(fixes, DerivedColumnFix{Column: "org", Want: proj.Org})
	}

	// Pure functions of the flat arrays / name columns.
	wantAddress := ""
	if len(c.Addresses) > 0 {
		wantAddress = FormatAddress(c.Addresses[0])
	}
	if len(c.Addresses) > 0 && wantAddress != c.Address {
		fixes = append(fixes, DerivedColumnFix{Column: "address", Want: wantAddress})
	}
	if want := FlattenAddresses(c.Addresses); want != c.AddressesFlat {
		fixes = append(fixes, DerivedColumnFix{Column: "addresses_flat", Want: want})
	}
	if want := FlattenPhones(c.Phones); want != c.PhonesNormalized {
		fixes = append(fixes, DerivedColumnFix{Column: "phones_normalized", Want: want})
	}
	if want := DeriveSortName(effLast, effFirst); !strings.EqualFold(want, c.SortName) {
		fixes = append(fixes, DerivedColumnFix{Column: "sort_name", Want: want})
	}

	return fixes
}
