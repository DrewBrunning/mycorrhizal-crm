package scoring

import "fmt"

// resolveClosenessAndInterval computes the Closeness facet AND the target
// interval (days) that Recency/Frequency normalize against. The interval
// prefers the contact's real CadencePolicy when one exists; otherwise it
// falls back to a default derived from the Closeness tier — a spouse with no
// cadence policy defaults to a much shorter "should hear from them" window
// than a coworker with no policy (see ADR-0023).
func resolveClosenessAndInterval(raw RawInputs, cfg Config) (intervalDays int, facet FacetResult) {
	fallbackInterval, facet := computeCloseness(raw, cfg)

	intervalDays = fallbackInterval
	if raw.HasCadencePolicy {
		intervalDays = raw.CadenceIntervalDays
	}
	// A caller-supplied interval of <1 would divide-by-zero/produce NaN in
	// computeRecency/computeFrequency, which encoding/json cannot marshal —
	// a data bug should not turn into a 500 on every read of this contact.
	// Every legitimate source (CadencePolicy.TargetIntervalDays validates
	// gt=0; every tier default in testdata/weights.json is Validate()d >0)
	// is already positive, so this only guards a caller bug, not a real case.
	if intervalDays < 1 {
		intervalDays = 1
	}
	return intervalDays, facet
}

// computeCloseness resolves the Closeness facet and the tier-derived default
// interval (used only when the contact has no real CadencePolicy).
func computeCloseness(raw RawInputs, cfg Config) (defaultIntervalDays int, facet FacetResult) {
	// Direct edge wins over hop distance: pick the highest-tier structural
	// type among every edge type found, deterministically (never "whichever
	// the DB/traversal happened to return first" — see ADR-0023's
	// conflicts_with/friend_of ambiguity fix). Any token with no entry in
	// RelationCloseness (affinity types, or an unregistered/unknown token)
	// is silently ignored here, which is exactly how affinity-only edges
	// fail to elevate closeness.
	var best *RelationCloseness
	var bestType string
	for _, t := range raw.DirectRelationTypes {
		rc, ok := cfg.RelationCloseness[t]
		if !ok {
			continue
		}
		if best == nil || rc.Weight > best.Weight {
			rcCopy := rc
			best = &rcCopy
			bestType = t
		}
	}
	if best != nil {
		return best.DefaultIntervalDays, FacetResult{
			Value:  best.Weight,
			Weight: cfg.WeightCloseness,
			Reason: fmt.Sprintf("Directly connected via a %s relationship", bestType),
		}
	}

	if raw.ReachableHops != nil {
		hops := *raw.ReachableHops
		value := clamp(cfg.ClosenessHopBaseWeight-cfg.ClosenessHopDecayPerHop*float64(hops-1), cfg.ClosenessHopFloor, 100)
		return cfg.UnknownClosenessIntervalDays, FacetResult{
			Value:  value,
			Weight: cfg.WeightCloseness,
			Reason: fmt.Sprintf("Reachable %d hops away in the relationship graph", hops),
		}
	}

	return cfg.UnknownClosenessIntervalDays, FacetResult{
		Value:  cfg.UnknownClosenessWeight,
		Weight: cfg.WeightCloseness,
		Reason: "Not connected to your self-contact in the relationship graph — closeness unknown",
	}
}

// computeRecency scores how long ago the last qualifying interaction was,
// relative to intervalDays. A contact with no qualifying interaction ever
// gets a fixed, documented default — deliberately neither 0 (which would be
// indistinguishable from "wildly overdue") nor derived from intervalDays
// (which would make a same-day-created contact coincidentally match "1 day
// overdue" — an accident, not a decision).
func computeRecency(raw RawInputs, cfg Config, intervalDays int) FacetResult {
	if !raw.HasQualifyingInteraction {
		return FacetResult{
			Value:  cfg.RecencyNoInteractionValue,
			Weight: cfg.WeightRecency,
			Reason: "No qualifying interaction recorded yet",
		}
	}
	ratio := float64(raw.DaysSinceLastInteraction) / float64(intervalDays)
	value := clamp(100*(1-ratio/cfg.RecencyOverdueRatioCap), 0, 100)
	return FacetResult{
		Value:  value,
		Weight: cfg.WeightRecency,
		Reason: fmt.Sprintf("Last qualifying interaction %d day(s) ago (target every %d days)", raw.DaysSinceLastInteraction, intervalDays),
	}
}

// computeFrequency scores the rolling count of qualifying interactions
// against how many would be "on pace" given intervalDays over the tunable
// FrequencyWindowDays window.
func computeFrequency(raw RawInputs, cfg Config, intervalDays int) FacetResult {
	expected := float64(cfg.FrequencyWindowDays) / float64(intervalDays)
	var ratio float64
	if expected > 0 {
		ratio = float64(raw.QualifyingInteractionsInWindow) / expected
	}
	value := clamp(100*ratio, 0, 100)
	return FacetResult{
		Value:  value,
		Weight: cfg.WeightFrequency,
		Reason: fmt.Sprintf("%d qualifying interaction(s) in the last %d days (expected ~%.1f at the target cadence)", raw.QualifyingInteractionsInWindow, cfg.FrequencyWindowDays, expected),
	}
}

// computeReachOut folds in the (unrelated-to-staleness) reach-out-suggestion
// engine: an un-dismissed org/title/address change is a legitimate warning
// signal ("something changed and you haven't followed up"), scored as a
// fixed low value rather than a gradient — it's a discrete event, not a
// decaying one.
func computeReachOut(raw RawInputs, cfg Config) FacetResult {
	if raw.HasPendingReachOut {
		return FacetResult{
			Value:  cfg.ReachOutPendingValue,
			Weight: cfg.WeightReachOut,
			Reason: "There's an unreviewed reach-out suggestion for this contact",
		}
	}
	return FacetResult{
		Value:  100,
		Weight: cfg.WeightReachOut,
		Reason: "No pending reach-out suggestion",
	}
}

// computeLastUpdated is deliberately the lowest-weighted, weakest-signal
// facet (record-editing activity is a poor proxy for relationship health) —
// it decays toward a floor, never to 0, so it can nudge but never dominate
// the score on its own.
func computeLastUpdated(raw RawInputs, cfg Config) FacetResult {
	ratio := float64(raw.DaysSinceUpdated) / float64(cfg.LastUpdatedWindowDays)
	value := clamp(100-(100-cfg.LastUpdatedFloor)*ratio, cfg.LastUpdatedFloor, 100)
	return FacetResult{
		Value:  value,
		Weight: cfg.WeightLastUpdated,
		Reason: fmt.Sprintf("Contact record last edited %d day(s) ago", raw.DaysSinceUpdated),
	}
}
