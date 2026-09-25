package contactmodel

// TemporalRange is a CRM-only start/end period for a value that is true over
// an interval — "lived at X 2019–2024", "worked at Y for three years". It is
// deliberately NOT part of the standardized Card: RFC 9553 defines no date on
// Address/Organization/Title, RFC 9554 adds no date-range property, and RFC
// 9555 therefore has no correspondence row, so ADR 0002 forbids inventing one
// (docs/adrs/0025-temporal-periods.md).
//
// Endpoints are PartialDate (calendar dates with one or more components
// deliberately unknown), never instants: a period is a calendar fact, has no
// time and no zone, and is never zone-converted (docs/adrs/0015-temporal-semantics.md).
// Either endpoint may be nil (open-ended); both nil asserts nothing and is
// rejected at the API boundary. Duration is never stored — it is derived from
// the endpoints so it cannot drift.
type TemporalRange struct {
	Start *PartialDate `json:"start,omitempty"`
	End   *PartialDate `json:"end,omitempty"`
}

// EntryPeriod attaches one TemporalRange to one entry within the Card. The
// reference is by the element's neutral ID (the JSContact map key / vCard
// PROP-ID), not by array position: keying by position would re-associate
// periods with the wrong entry the first time a row is deleted
// (docs/adrs/0025-temporal-periods.md).
type EntryPeriod struct {
	// Kind names which Card collection EntryID indexes — JSContact Id keys
	// are unique only within a collection. Not hard-enumerated in the type,
	// but a Kind with no Card resolver (see CardEntryIDs) is rejected as an
	// invalid reference at the API boundary, because a period that cannot
	// point at an entry is dead data.
	Kind string `json:"kind"`
	// EntryID is the referenced Card element's ID. An entry must carry an ID
	// to carry a period; a reference that does not resolve is a caller error.
	EntryID string        `json:"entry_id"`
	Range   TemporalRange `json:"range"`
}

// Period kinds for EntryPeriod.Kind. Only these three have neutral entries
// today; the set is extensible without a schema change.
const (
	PeriodKindAddress      = "address"
	PeriodKindOrganization = "organization"
	PeriodKindTitle        = "title"
)

// IsEmpty reports whether the range asserts nothing (both endpoints absent).
// An empty range is invalid and must not be persisted.
func (r TemporalRange) IsEmpty() bool {
	return r.Start == nil && r.End == nil
}

// Reversed reports whether the range's end precedes its start, compared at
// the coarsest precision the two share (ADR 0015 Rule 3: partial dates have no
// total order, so a comparison only exists at shared precision). A range whose
// endpoints cannot be compared (e.g. one has no year) is never "reversed" —
// non-comparability is not a caller error. A range that is empty or open on
// either side is never reversed.
func (r TemporalRange) Reversed() bool {
	cmp, ok := ComparePartialDates(r.Start, r.End)
	return ok && cmp > 0
}

// ComparePartialDates compares two partial dates at the coarsest precision
// they share, returning (comparison, comparable). comparison is -1, 0, or 1.
// Not comparable (either side year-less, or nil) returns ok=false. Exported so
// period-ordering callers (e.g. the life-event suggestion rules, issue #1233,
// deciding whether one address period succeeds another) use the same
// comparison the model's own Reversed() guard does, rather than a second one.
func ComparePartialDates(a, b *PartialDate) (int, bool) {
	if a == nil || b == nil || a.Year == nil || b.Year == nil {
		return 0, false
	}
	if *a.Year != *b.Year {
		return cmpInt(*a.Year, *b.Year), true
	}
	if a.Month == nil || b.Month == nil {
		return 0, true
	}
	if *a.Month != *b.Month {
		return cmpInt(*a.Month, *b.Month), true
	}
	if a.Day == nil || b.Day == nil {
		return 0, true
	}
	return cmpInt(*a.Day, *b.Day), true
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// Period returns the range attached to (kind, entryID) on the envelope, if
// one exists. The first match wins; callers that need all of them use
// Periods directly.
func (e CRMEnvelope) Period(kind, entryID string) (TemporalRange, bool) {
	for _, p := range e.Periods {
		if p.Kind == kind && p.EntryID == entryID {
			return p.Range, true
		}
	}
	return TemporalRange{}, false
}

// CardEntryIDs returns the set of element IDs present in the named Card
// collection, or nil for a kind the Card does not hold. It is the resolver
// behind period-reference validation.
func (c Card) CardEntryIDs(kind string) map[string]struct{} {
	ids := map[string]struct{}{}
	add := func(id string) {
		if id != "" {
			ids[id] = struct{}{}
		}
	}
	switch kind {
	case PeriodKindAddress:
		for _, a := range c.Addresses {
			add(a.ID)
		}
	case PeriodKindOrganization:
		for _, o := range c.Organizations {
			add(o.ID)
		}
	case PeriodKindTitle:
		for _, t := range c.Titles {
			add(t.ID)
		}
	default:
		return nil
	}
	return ids
}

// InvalidPeriods returns every period on the envelope that is not storable:
// an empty range (both endpoints absent), a missing Kind or EntryID, or an
// EntryID that does not resolve to an entry of that Kind on the Card. Callers
// turn a non-empty result into a 400 — never a silent drop (ADR 0002's
// degradation policy is for formats, not for user input).
func (e CRMEnvelope) InvalidPeriods(c Card) []EntryPeriod {
	var bad []EntryPeriod
	for _, p := range e.Periods {
		if p.Range.IsEmpty() || p.Range.Reversed() || p.Kind == "" || p.EntryID == "" {
			bad = append(bad, p)
			continue
		}
		ids := c.CardEntryIDs(p.Kind)
		if ids == nil {
			bad = append(bad, p)
			continue
		}
		if _, ok := ids[p.EntryID]; !ok {
			bad = append(bad, p)
		}
	}
	return bad
}

// PruneOrphanPeriods returns the envelope's periods whose (Kind, EntryID)
// still resolves on the Card. It is the merge-time counterpart of
// InvalidPeriods: when a legacy flat save rebuilds the Card from flat fields
// and drops an entry the period referred to, the period goes with it rather
// than dangling.
func (e CRMEnvelope) PruneOrphanPeriods(c Card) []EntryPeriod {
	if len(e.Periods) == 0 {
		return e.Periods
	}
	kept := make([]EntryPeriod, 0, len(e.Periods))
	for _, p := range e.Periods {
		ids := c.CardEntryIDs(p.Kind)
		if ids == nil {
			continue
		}
		if _, ok := ids[p.EntryID]; ok {
			kept = append(kept, p)
		}
	}
	return kept
}
