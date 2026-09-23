package scoring

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadConfig_Valid is the cheap per-PR structural gate for the committed
// weights.json, mirroring perfbench's TestEmbeddedBudgetsValid: the file
// parses, and validate() (weights sum to 100, thresholds ordered, every
// structural relation type from the live registry tiered) passes.
func TestLoadConfig_Valid(t *testing.T) {
	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.InDelta(t, 100, cfg.WeightRecency+cfg.WeightFrequency+cfg.WeightCloseness+cfg.WeightReachOut+cfg.WeightLastUpdated, 0.001)
	assert.Less(t, cfg.ChanterelleMin, cfg.MossMin)
}

// TestValidate_RejectsMisconfiguration proves validate() actually catches a
// bad config rather than only ever seeing the one hand-tuned committed file —
// mirrors perfbench's own "Validate must be capable of catching a bad one"
// discipline. Each case starts from a known-good config and breaks exactly
// one thing.
func TestValidate_RejectsMisconfiguration(t *testing.T) {
	good, err := LoadConfig()
	require.NoError(t, err)

	cases := map[string]func(Config) Config{
		"weights don't sum to 100": func(c Config) Config {
			c.WeightRecency += 1
			return c
		},
		"chanterelle_min >= moss_min": func(c Config) Config {
			c.ChanterelleMin = c.MossMin
			return c
		},
		"negative weight": func(c Config) Config {
			// -1 + 21 offset on Recency keeps the sum at exactly 100, so this
			// (not the sum check) is what trips.
			c.WeightFrequency = -1
			c.WeightRecency += 21
			return c
		},
		"zero unknown closeness interval": func(c Config) Config {
			c.UnknownClosenessIntervalDays = 0
			return c
		},
		"zero frequency window": func(c Config) Config {
			c.FrequencyWindowDays = 0
			return c
		},
		"hop floor above hop base": func(c Config) Config {
			c.ClosenessHopFloor = c.ClosenessHopBaseWeight + 1
			return c
		},
		"missing structural relation type": func(c Config) Config {
			delete(c.RelationCloseness, "friend_of")
			return c
		},
		"affinity type wrongly tiered": func(c Config) Config {
			c.RelationCloseness["conflicts_with"] = RelationCloseness{Weight: 50, DefaultIntervalDays: 30}
			return c
		},
		"weight over 100": func(c Config) Config {
			// +81 on Frequency, -81 offset on Recency keeps the sum at
			// exactly 100. Recency necessarily goes negative too (the other
			// four weights can't absorb an 81-point increase on their own),
			// but that only means either weight can trip this same check —
			// still proves validate rejects an out-of-[0,100] weight.
			c.WeightFrequency = 101
			c.WeightRecency -= 81
			return c
		},
		"moss_min over 100": func(c Config) Config {
			c.MossMin = 101
			return c
		},
		"unknown closeness weight negative": func(c Config) Config {
			c.UnknownClosenessWeight = -1
			return c
		},
		"unknown closeness weight over 100": func(c Config) Config {
			c.UnknownClosenessWeight = 101
			return c
		},
		"recency no_interaction_value negative": func(c Config) Config {
			c.RecencyNoInteractionValue = -1
			return c
		},
		"recency no_interaction_value over 100": func(c Config) Config {
			c.RecencyNoInteractionValue = 101
			return c
		},
		"recency overdue_ratio_cap zero": func(c Config) Config {
			c.RecencyOverdueRatioCap = 0
			return c
		},
		"negative frequency window": func(c Config) Config {
			c.FrequencyWindowDays = -1
			return c
		},
		"closeness hop_base_weight negative": func(c Config) Config {
			c.ClosenessHopBaseWeight = -1
			return c
		},
		"closeness hop_base_weight over 100": func(c Config) Config {
			c.ClosenessHopBaseWeight = 101
			return c
		},
		"closeness hop_decay_per_hop negative": func(c Config) Config {
			c.ClosenessHopDecayPerHop = -1
			return c
		},
		"closeness hop_floor negative": func(c Config) Config {
			c.ClosenessHopFloor = -1
			return c
		},
		"reach_out pending_value negative": func(c Config) Config {
			c.ReachOutPendingValue = -1
			return c
		},
		"reach_out pending_value over 100": func(c Config) Config {
			c.ReachOutPendingValue = 101
			return c
		},
		"negative last_updated window": func(c Config) Config {
			c.LastUpdatedWindowDays = -1
			return c
		},
		"last_updated floor negative": func(c Config) Config {
			c.LastUpdatedFloor = -1
			return c
		},
		"last_updated floor over 100": func(c Config) Config {
			c.LastUpdatedFloor = 101
			return c
		},
		"relation closeness tier weight negative": func(c Config) Config {
			c.RelationCloseness["friend_of"] = RelationCloseness{Weight: -1, DefaultIntervalDays: 30}
			return c
		},
		"relation closeness tier weight over 100": func(c Config) Config {
			c.RelationCloseness["friend_of"] = RelationCloseness{Weight: 101, DefaultIntervalDays: 30}
			return c
		},
		"relation closeness tier interval zero": func(c Config) Config {
			c.RelationCloseness["friend_of"] = RelationCloseness{Weight: 50, DefaultIntervalDays: 0}
			return c
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			broken := mutate(good)
			err := validate(broken)
			assert.Error(t, err, "expected validate to reject: %s", name)
		})
	}
}

// TestParseConfig_RejectsUndefinedTier proves flatten() (not just validate())
// catches a closeness_tiers entry pointing at a tier_definitions key that
// doesn't exist — a config authoring mistake, not a numeric range problem.
func TestParseConfig_RejectsUndefinedTier(t *testing.T) {
	raw := []byte(`{
		"weights": [
			{"facet":"recency","weight":100},
			{"facet":"frequency","weight":0},
			{"facet":"closeness","weight":0},
			{"facet":"reach_out","weight":0},
			{"facet":"last_updated","weight":0}
		],
		"thresholds": {"moss_min":70,"chanterelle_min":40},
		"closeness_tiers": {"friend_of": {"tier":"no-such-tier"}},
		"tier_definitions": {},
		"unknown_closeness": {"weight":50,"default_interval_days":90},
		"recency": {"no_interaction_value":45,"overdue_ratio_cap":2},
		"frequency": {"window_days":180},
		"closeness": {"hop_base_weight":60,"hop_decay_per_hop":15,"hop_floor":20},
		"reach_out": {"pending_value":30},
		"last_updated": {"window_days":180,"floor":30}
	}`)
	_, err := parseConfig(raw)
	assert.ErrorContains(t, err, "undefined tier")
}

// TestParseConfig_RejectsMalformedJSON proves parseConfig's own json.Unmarshal
// error path (distinct from a well-formed-but-invalid doc, which the other
// parseConfig/validate tests cover).
func TestParseConfig_RejectsMalformedJSON(t *testing.T) {
	_, err := parseConfig([]byte(`{not valid json`))
	assert.ErrorContains(t, err, "parsing config")
}

// TestParseConfig_RejectsUnknownWeightFacet proves flatten's own
// switch-default error path for a weights[].facet typo.
func TestParseConfig_RejectsUnknownWeightFacet(t *testing.T) {
	raw := []byte(`{
		"weights": [
			{"facet":"typo","weight":100}
		],
		"thresholds": {"moss_min":70,"chanterelle_min":40},
		"closeness_tiers": {},
		"tier_definitions": {},
		"unknown_closeness": {"weight":50,"default_interval_days":90},
		"recency": {"no_interaction_value":45,"overdue_ratio_cap":2},
		"frequency": {"window_days":180},
		"closeness": {"hop_base_weight":60,"hop_decay_per_hop":15,"hop_floor":20},
		"reach_out": {"pending_value":30},
		"last_updated": {"window_days":180,"floor":30}
	}`)
	_, err := parseConfig(raw)
	assert.ErrorContains(t, err, "unknown weight facet")
}

// TestParseConfig_PropagatesValidateError proves parseConfig's own
// validate() error-propagation branch: a doc that parses and flattens fine
// but fails a validate() range check (weights summing to 90, not 100) must
// surface that error through parseConfig, not just through calling
// validate() directly (scoring_test.go's other tests all do that).
func TestParseConfig_PropagatesValidateError(t *testing.T) {
	raw := []byte(`{
		"weights": [
			{"facet":"recency","weight":90},
			{"facet":"frequency","weight":0},
			{"facet":"closeness","weight":0},
			{"facet":"reach_out","weight":0},
			{"facet":"last_updated","weight":0}
		],
		"thresholds": {"moss_min":70,"chanterelle_min":40},
		"closeness_tiers": {},
		"tier_definitions": {},
		"unknown_closeness": {"weight":50,"default_interval_days":90},
		"recency": {"no_interaction_value":45,"overdue_ratio_cap":2},
		"frequency": {"window_days":180},
		"closeness": {"hop_base_weight":60,"hop_decay_per_hop":15,"hop_floor":20},
		"reach_out": {"pending_value":30},
		"last_updated": {"window_days":180,"floor":30}
	}`)
	_, err := parseConfig(raw)
	assert.ErrorContains(t, err, "must sum to 100")
}

// defaultConfigForTest returns the real committed config — the facet-math
// tests below exercise Compute against the actual tunables, not a synthetic
// stand-in, so a change to weights.json is honestly reflected in these
// expectations rather than tested against a config nobody ships.
func defaultConfigForTest(t *testing.T) Config {
	t.Helper()
	cfg, err := LoadConfig()
	require.NoError(t, err)
	return cfg
}

func TestCompute_WeightsSumGuaranteesScoreRange(t *testing.T) {
	cfg := defaultConfigForTest(t)

	// All-perfect inputs -> score should be exactly 100.
	perfect := RawInputs{
		HasCadencePolicy:               true,
		CadenceIntervalDays:            30,
		HasQualifyingInteraction:       true,
		DaysSinceLastInteraction:       0,
		QualifyingInteractionsInWindow: 100, // wildly over "expected" — capped at 100
		DirectRelationTypes:            []string{"spouse_of"},
		HasPendingReachOut:             false,
		DaysSinceUpdated:               0,
	}
	got := Compute(perfect, cfg)
	assert.Equal(t, 100, got.Score)
	assert.Equal(t, BandMoss, got.Band)

	// All-worst inputs -> score should be exactly 0.
	worst := RawInputs{
		HasCadencePolicy:               true,
		CadenceIntervalDays:            30,
		HasQualifyingInteraction:       true,
		DaysSinceLastInteraction:       9999,
		QualifyingInteractionsInWindow: 0,
		DirectRelationTypes:            nil,
		ReachableHops:                  nil,
		HasPendingReachOut:             true,
		DaysSinceUpdated:               9999,
	}
	// worst's Closeness/ReachOut/LastUpdated facets have documented non-zero
	// floors (unknown_closeness=50, reach_out pending=30, last_updated
	// floor=30) — this is a deliberate design property (see ADR-0023), not a
	// bug, so the "worst case" score is asserted against the real floor math
	// rather than assumed to be 0.
	gotWorst := Compute(worst, cfg)
	assert.Equal(t, BandRussula, gotWorst.Band)
	assert.Less(t, gotWorst.Score, 40)
}

// TestCompute_ThresholdBoundaries pins the exact moss/chanterelle/russula cut
// points against the real committed thresholds, using Closeness alone (held
// at 100 weight isolation isn't possible since weights are fixed — instead
// drive the total score directly via Recency, the highest-weighted facet, at
// hand-picked day counts that land exactly on each boundary given the
// committed config).
func TestCompute_ThresholdBoundaries(t *testing.T) {
	cfg := defaultConfigForTest(t)

	for _, tc := range []struct {
		name  string
		score int
		want  string
	}{
		{"exactly moss_min", int(cfg.MossMin), BandMoss},
		{"one below moss_min", int(cfg.MossMin) - 1, BandChanterelle},
		{"exactly chanterelle_min", int(cfg.ChanterelleMin), BandChanterelle},
		{"one below chanterelle_min", int(cfg.ChanterelleMin) - 1, BandRussula},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, band(float64(tc.score), cfg))
		})
	}
}

func TestCompute_RecencyNoInteractionEver_IsNeitherZeroNorIntervalDerived(t *testing.T) {
	cfg := defaultConfigForTest(t)

	withNoInteraction := Compute(RawInputs{HasQualifyingInteraction: false, DirectRelationTypes: []string{"friend_of"}}, cfg)
	assert.Equal(t, cfg.RecencyNoInteractionValue, withNoInteraction.Recency.Value)
	assert.NotZero(t, withNoInteraction.Recency.Value, "no-interaction-ever must not read as the worst possible score")
	assert.Less(t, withNoInteraction.Recency.Value, 100.0, "no-interaction-ever must not read as a healthy score either")

	// Changing the interval (via a different closeness tier) must not move
	// the no-interaction-ever value — it's a fixed constant, not derived
	// from the interval.
	withDifferentInterval := Compute(RawInputs{HasQualifyingInteraction: false, DirectRelationTypes: []string{"coworker_of"}}, cfg)
	assert.Equal(t, withNoInteraction.Recency.Value, withDifferentInterval.Recency.Value)
}

func TestCompute_RecencyRatioCurve(t *testing.T) {
	cfg := defaultConfigForTest(t)

	for _, tc := range []struct {
		name        string
		daysSince   int
		intervalDay int
		want        float64
	}{
		{"exactly on time", 0, 30, 100},
		{"halfway to due", 15, 30, 75},
		{"exactly due", 30, 30, 50},
		{"double the interval overdue", 60, 30, 0},
		{"far beyond double, clamped at 0", 999, 30, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := RawInputs{
				HasCadencePolicy:         true,
				CadenceIntervalDays:      tc.intervalDay,
				HasQualifyingInteraction: true,
				DaysSinceLastInteraction: tc.daysSince,
			}
			got := Compute(raw, cfg)
			assert.InDelta(t, tc.want, got.Recency.Value, 0.01)
		})
	}
}

func TestCompute_FrequencyRatioCurve(t *testing.T) {
	cfg := defaultConfigForTest(t)
	// windowDays=180, intervalDays=30 -> expected = 6.
	raw := func(count int) RawInputs {
		return RawInputs{
			HasCadencePolicy:               true,
			CadenceIntervalDays:            30,
			QualifyingInteractionsInWindow: count,
		}
	}

	assert.InDelta(t, 0, Compute(raw(0), cfg).Frequency.Value, 0.01)
	assert.InDelta(t, 50, Compute(raw(3), cfg).Frequency.Value, 0.01)
	assert.InDelta(t, 100, Compute(raw(6), cfg).Frequency.Value, 0.01)
	assert.InDelta(t, 100, Compute(raw(60), cfg).Frequency.Value, 0.01, "frequency must cap at 100, not reward far-over-expected contact indefinitely")
}

func TestCompute_ClosenessDirectEdge_PicksHighestTierDeterministically(t *testing.T) {
	cfg := defaultConfigForTest(t)

	// friend_of (close tier) + conflicts_with (unregistered/affinity,
	// ignored) must resolve to friend_of's tier every time, regardless of
	// slice order — the actual bug this design fixes (map-iteration-order
	// non-determinism in the naive "read TraverseGraph's resolved relation"
	// approach).
	forward := Compute(RawInputs{DirectRelationTypes: []string{"friend_of", "conflicts_with"}}, cfg)
	reversed := Compute(RawInputs{DirectRelationTypes: []string{"conflicts_with", "friend_of"}}, cfg)
	assert.Equal(t, forward.Closeness.Value, reversed.Closeness.Value)
	assert.Equal(t, cfg.RelationCloseness["friend_of"].Weight, forward.Closeness.Value)

	// Two structural types between the same pair: the higher tier wins,
	// deterministically, not "whichever came first".
	spouseAndFriend := Compute(RawInputs{DirectRelationTypes: []string{"friend_of", "spouse_of"}}, cfg)
	assert.Equal(t, cfg.RelationCloseness["spouse_of"].Weight, spouseAndFriend.Closeness.Value)
	spouseAndFriendReversed := Compute(RawInputs{DirectRelationTypes: []string{"spouse_of", "friend_of"}}, cfg)
	assert.Equal(t, spouseAndFriend.Closeness.Value, spouseAndFriendReversed.Closeness.Value)
}

// TestCompute_ConflictsWithAloneDoesNotElevateCloseness is the named
// regression case ADR-0023/the plan calls for: an affinity-only edge
// (conflicts_with or gets_along_with) must score identically to "no edge at
// all", never like a structural connection.
func TestCompute_ConflictsWithAloneDoesNotElevateCloseness(t *testing.T) {
	cfg := defaultConfigForTest(t)

	noEdge := Compute(RawInputs{}, cfg)
	conflictsOnly := Compute(RawInputs{DirectRelationTypes: []string{"conflicts_with"}}, cfg)
	getsAlongOnly := Compute(RawInputs{DirectRelationTypes: []string{"gets_along_with"}}, cfg)

	assert.Equal(t, noEdge.Closeness.Value, conflictsOnly.Closeness.Value)
	assert.Equal(t, noEdge.Closeness.Value, getsAlongOnly.Closeness.Value)
	assert.Less(t, noEdge.Closeness.Value, cfg.RelationCloseness["friend_of"].Weight,
		"sanity: the neutral/unknown default must be lower than any real structural tier, or this test would pass vacuously")
}

func TestCompute_ClosenessHopDecay(t *testing.T) {
	cfg := defaultConfigForTest(t)

	hop := func(n int) float64 {
		h := n
		return Compute(RawInputs{ReachableHops: &h}, cfg).Closeness.Value
	}

	assert.InDelta(t, cfg.ClosenessHopBaseWeight, hop(1), 0.01)
	assert.InDelta(t, cfg.ClosenessHopBaseWeight-cfg.ClosenessHopDecayPerHop, hop(2), 0.01)
	assert.GreaterOrEqual(t, hop(50), cfg.ClosenessHopFloor, "hop decay must never go below the configured floor")
	assert.InDelta(t, cfg.ClosenessHopFloor, hop(50), 0.01)
}

func TestCompute_ClosenessUnknownDefault_NoSelfContactOrUnreachable(t *testing.T) {
	cfg := defaultConfigForTest(t)
	got := Compute(RawInputs{DirectRelationTypes: nil, ReachableHops: nil}, cfg)
	assert.Equal(t, cfg.UnknownClosenessWeight, got.Closeness.Value)
}

func TestCompute_ReachOut(t *testing.T) {
	cfg := defaultConfigForTest(t)

	pending := Compute(RawInputs{HasPendingReachOut: true}, cfg)
	assert.Equal(t, cfg.ReachOutPendingValue, pending.ReachOut.Value)

	none := Compute(RawInputs{HasPendingReachOut: false}, cfg)
	assert.Equal(t, 100.0, none.ReachOut.Value)
}

func TestCompute_LastUpdatedDecaysToFloorNeverBelow(t *testing.T) {
	cfg := defaultConfigForTest(t)

	assert.InDelta(t, 100, Compute(RawInputs{DaysSinceUpdated: 0}, cfg).LastUpdated.Value, 0.01)
	atWindow := Compute(RawInputs{DaysSinceUpdated: cfg.LastUpdatedWindowDays}, cfg)
	assert.InDelta(t, cfg.LastUpdatedFloor, atWindow.LastUpdated.Value, 0.01)
	beyondWindow := Compute(RawInputs{DaysSinceUpdated: cfg.LastUpdatedWindowDays * 10}, cfg)
	assert.InDelta(t, cfg.LastUpdatedFloor, beyondWindow.LastUpdated.Value, 0.01, "must clamp at the floor, never go below it")
}

// TestCompute_CadencePolicyIntervalOverridesClosenessDefault proves a real
// CadencePolicy's interval is used over the closeness-tier default, even
// when they differ, for both Recency and Frequency.
func TestCompute_CadencePolicyIntervalOverridesClosenessDefault(t *testing.T) {
	cfg := defaultConfigForTest(t)

	// coworker_of's tier default is 45 days; give it an explicit 10-day
	// policy instead and confirm the 10-day interval is what's used (a
	// contact 10 days out on a 10-day policy should score as "exactly due",
	// i.e. Recency == 50, not compute against the 45-day tier default).
	raw := RawInputs{
		HasCadencePolicy:         true,
		CadenceIntervalDays:      10,
		DirectRelationTypes:      []string{"coworker_of"},
		HasQualifyingInteraction: true,
		DaysSinceLastInteraction: 10,
	}
	got := Compute(raw, cfg)
	assert.InDelta(t, 50, got.Recency.Value, 0.01)
}

func TestCompute_DegenerateIntervalDoesNotProduceNaNOrInf(t *testing.T) {
	cfg := defaultConfigForTest(t)
	// A caller bug (interval of 0) with DaysSinceLastInteraction also 0 is
	// the actual 0/0 = NaN hazard in computeRecency's ratio — without
	// resolveClosenessAndInterval's <1 guard this produces NaN, which
	// encoding/json cannot marshal (a 500 on every read of the contact).
	raw := RawInputs{
		HasCadencePolicy:         true,
		CadenceIntervalDays:      0,
		HasQualifyingInteraction: true,
		DaysSinceLastInteraction: 0,
	}
	got := Compute(raw, cfg)
	assert.False(t, isNaNOrInf(got.Recency.Value))
	assert.False(t, isNaNOrInf(got.Frequency.Value))
	assert.GreaterOrEqual(t, got.Score, 0)
	assert.LessOrEqual(t, got.Score, 100)
}

func isNaNOrInf(v float64) bool {
	return v != v || v > 1e300 || v < -1e300
}

func TestCompute_EveryFacetIsClampedTo0To100(t *testing.T) {
	cfg := defaultConfigForTest(t)
	// Pathological inputs that would escape [0,100] without clamping:
	// negative days-since (shouldn't happen, but must not break), a
	// frequency count wildly over expected, and a huge hop count.
	hops := 1000
	got := Compute(RawInputs{
		HasCadencePolicy:               true,
		CadenceIntervalDays:            30,
		HasQualifyingInteraction:       true,
		DaysSinceLastInteraction:       -5,
		QualifyingInteractionsInWindow: 100000,
		ReachableHops:                  &hops,
		DaysSinceUpdated:               -100,
	}, cfg)

	for _, fr := range []FacetResult{got.Recency, got.Frequency, got.Closeness, got.ReachOut, got.LastUpdated} {
		assert.GreaterOrEqual(t, fr.Value, 0.0)
		assert.LessOrEqual(t, fr.Value, 100.0)
	}
	assert.GreaterOrEqual(t, got.Score, 0)
	assert.LessOrEqual(t, got.Score, 100)
}

func TestCompute_WeightsOnEveryFacetMatchConfig(t *testing.T) {
	cfg := defaultConfigForTest(t)
	got := Compute(RawInputs{}, cfg)
	assert.Equal(t, cfg.WeightRecency, got.Recency.Weight)
	assert.Equal(t, cfg.WeightFrequency, got.Frequency.Weight)
	assert.Equal(t, cfg.WeightCloseness, got.Closeness.Weight)
	assert.Equal(t, cfg.WeightReachOut, got.ReachOut.Weight)
	assert.Equal(t, cfg.WeightLastUpdated, got.LastUpdated.Weight)
}

// TestCompute_EveryFacetHasAReason proves the explainability requirement
// directly: no facet is ever a bare number with no explanation.
func TestCompute_EveryFacetHasAReason(t *testing.T) {
	cfg := defaultConfigForTest(t)
	got := Compute(RawInputs{}, cfg)
	for name, fr := range map[string]FacetResult{
		"recency": got.Recency, "frequency": got.Frequency, "closeness": got.Closeness,
		"reach_out": got.ReachOut, "last_updated": got.LastUpdated,
	} {
		assert.NotEmpty(t, fr.Reason, "%s facet must always carry an explanation", name)
	}
}
