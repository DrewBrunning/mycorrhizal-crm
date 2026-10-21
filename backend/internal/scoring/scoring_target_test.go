package scoring

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func targetRaw(tier string, days, count int) RawInputs {
	return RawInputs{
		TargetTier:                     tier,
		HasQualifyingInteraction:       true,
		DaysSinceLastInteraction:       days,
		QualifyingInteractionsInWindow: count,
	}
}

func TestCompute_TargetNilWithoutTargetOrWithUnknownTier(t *testing.T) {
	cfg := defaultConfigForTest(t)

	assert.Nil(t, Compute(targetRaw("", 3, 6), cfg).Target)
	assert.Nil(t, Compute(targetRaw("bogus", 3, 6), cfg).Target)

	// An unrecognized tier must not change the interval either: it scores
	// byte-identically to no target at all.
	assert.Equal(t, Compute(targetRaw("", 3, 6), cfg), Compute(targetRaw("bogus", 3, 6), cfg))
}

func TestCompute_TargetIntervalPrecedence(t *testing.T) {
	cfg := defaultConfigForTest(t)

	// No policy, no edge: the closeness default is 90 days; the close tier
	// overrides it with 30.
	noTarget := Compute(targetRaw("", 10, 1), cfg)
	assert.Contains(t, noTarget.Recency.Reason, "target every 90 days")

	got := Compute(targetRaw("close", 10, 1), cfg)
	require.NotNil(t, got.Target)
	assert.Equal(t, 30, got.Target.IntervalDays)
	assert.Equal(t, IntervalSourceTargetTier, got.Target.IntervalSource)
	assert.Contains(t, got.Recency.Reason, "target every 30 days")
	assert.Contains(t, got.Target.Reason, "against a 30-day interval from the close target")

	// A direct edge's own closeness interval (spouse_of = 14) loses to the tier.
	edge := targetRaw("casual", 10, 1)
	edge.DirectRelationTypes = []string{"spouse_of"}
	assert.Equal(t, 45, Compute(edge, cfg).Target.IntervalDays)

	// A real cadence policy beats the tier.
	policy := targetRaw("close", 10, 1)
	policy.HasCadencePolicy = true
	policy.CadenceIntervalDays = 10
	got = Compute(policy, cfg)
	require.NotNil(t, got.Target)
	assert.Equal(t, 10, got.Target.IntervalDays)
	assert.Equal(t, IntervalSourceCadencePolicy, got.Target.IntervalSource)
	assert.Contains(t, got.Target.Reason, "against a 10-day interval from your cadence policy")
}

func TestCompute_TargetStatus(t *testing.T) {
	cfg := defaultConfigForTest(t)

	cases := []struct {
		name       string
		days, n    int
		wantStatus string
		wantValue  float64
	}{
		{"on track", 3, 6, TargetStatusOnTrack, 96.8},
		{"drifting", 15, 3, TargetStatusDrifting, 65.9},
		{"behind", 30, 1, TargetStatusBehind, 37.9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Compute(targetRaw("close", tc.days, tc.n), cfg)
			require.NotNil(t, got.Target)
			assert.Equal(t, tc.wantStatus, got.Target.Status)
			assert.InDelta(t, tc.wantValue, got.Target.Value, 0.2)
			// The value is exactly the weighted blend of the two facets.
			want := (got.Recency.Value*cfg.WeightRecency + got.Frequency.Value*cfg.WeightFrequency) / (cfg.WeightRecency + cfg.WeightFrequency)
			assert.InDelta(t, want, got.Target.Value, 1e-9)
			assert.Equal(t, "close", got.Target.Tier)
		})
	}
}

// The status thresholds apply to the ROUNDED value, like band() on the score.
func TestTargetStatus_Boundaries(t *testing.T) {
	cfg := defaultConfigForTest(t)
	assert.Equal(t, TargetStatusDrifting, targetStatus(69.4, cfg))
	assert.Equal(t, TargetStatusOnTrack, targetStatus(69.5, cfg))
	assert.Equal(t, TargetStatusBehind, targetStatus(39.4, cfg))
	assert.Equal(t, TargetStatusDrifting, targetStatus(39.5, cfg))
}

func TestCompute_TargetDoesNotChangeScoreForSameInterval(t *testing.T) {
	cfg := defaultConfigForTest(t)

	// close = 30 days. An explicit 30-day policy produces the same score and
	// band as the tier did, and Target itself never feeds the total.
	tier := Compute(targetRaw("close", 20, 2), cfg)
	policy := targetRaw("", 20, 2)
	policy.HasCadencePolicy = true
	policy.CadenceIntervalDays = 30
	p := Compute(policy, cfg)
	assert.Equal(t, p.Score, tier.Score)
	assert.Equal(t, p.Band, tier.Band)
}

func TestCompute_AmbientTargetIsAlwaysOnTrack(t *testing.T) {
	cfg := defaultConfigForTest(t)

	raw := targetRaw("immediate", 400, 0)
	raw.Ambient = true
	raw.AmbientReason = AmbientReasonFlag
	got := Compute(raw, cfg)
	require.NotNil(t, got.Target)
	assert.Equal(t, TargetStatusOnTrack, got.Target.Status)
	assert.Equal(t, "Ambient contact — always on track", got.Target.Reason)

	// Independent of the facet override: force the blended value to zero and the
	// status is still on_track.
	zero := cfg
	zero.WeightRecency, zero.WeightFrequency = 1, 1
	res := computeTarget(raw, zero, 14, FacetResult{Value: 0}, FacetResult{Value: 0})
	assert.Equal(t, TargetStatusOnTrack, res.Status)
	assert.Equal(t, 0.0, res.Value)
}

func TestValidate_TargetTierVocabularyMustMatch(t *testing.T) {
	good := defaultConfigForTest(t)

	extra := cloneConfig(good)
	extra.TargetTierIntervalDays = map[string]int{}
	for k, v := range good.TargetTierIntervalDays {
		extra.TargetTierIntervalDays[k] = v
	}
	extra.TargetTierIntervalDays["bestie"] = 7
	err := validate(extra)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tier_definitions keys must be exactly")

	renamed := cloneConfig(good)
	renamed.TargetTierIntervalDays = map[string]int{}
	for k, v := range good.TargetTierIntervalDays {
		if k == "distant" {
			k = "bestie"
		}
		renamed.TargetTierIntervalDays[k] = v
	}
	err = validate(renamed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `missing target closeness tier "distant"`)
}

// parseConfig must reject a committed-file shape whose tier_definitions drift
// from the token list, end to end (not only via validate()).
func TestParseConfig_RejectsTierDefinitionDrift(t *testing.T) {
	// An extra definition no relation uses.
	extra := strings.Replace(string(embeddedConfig), `"tier_definitions": {`,
		`"tier_definitions": { "bestie": { "weight": 100, "default_interval_days": 7, "reason": "x" },`, 1)
	_, err := parseConfig([]byte(extra))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tier_definitions keys must be exactly")
}
