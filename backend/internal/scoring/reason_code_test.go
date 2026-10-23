package scoring

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The reason codes are a hand-mirrored vocabulary (frontend trap #4): the API
// enum must list exactly ReasonCodes().
func TestReasonCodes_MatchOpenAPIEnum(t *testing.T) {
	spec, err := os.ReadFile("../../openapi.yaml")
	require.NoError(t, err)
	m := regexp.MustCompile(`(?s)ContactScoreFacet:.*?reason_code:.*?enum: \[([^\]]*)\]`).FindSubmatch(spec)
	require.NotNil(t, m, "ContactScoreFacet.reason_code enum not found in openapi.yaml")
	assert.Equal(t, ReasonCodes(), strings.Split(strings.ReplaceAll(string(m[1]), " ", ""), ","))
}

// Every facet and target result carries a known code, and the params the code
// interpolates are present: a client rendering the localized sentence must
// never find a hole.
func TestCompute_EveryReasonHasCodeAndParams(t *testing.T) {
	cfg, err := LoadConfig()
	require.NoError(t, err)
	known := map[string]bool{}
	for _, c := range ReasonCodes() {
		known[c] = true
	}
	hops := 2
	inputs := []RawInputs{
		{},
		{HasQualifyingInteraction: true, DaysSinceLastInteraction: 0, QualifyingInteractionsInWindow: 0, DaysSinceUpdated: 0},
		{HasQualifyingInteraction: true, DaysSinceLastInteraction: 40, QualifyingInteractionsInWindow: 3, DaysSinceUpdated: 12, DirectRelationTypes: []string{"spouse_of"}, HasPendingReachOut: true, TargetTier: "close"},
		{ReachableHops: &hops, TargetTier: "casual", HasCadencePolicy: true, CadenceIntervalDays: 21},
		{Ambient: true, AmbientReason: AmbientReasonHousehold, TargetTier: "distant"},
		{Ambient: true, AmbientReason: AmbientReasonFlag},
	}
	seen := map[string]bool{}
	for _, raw := range inputs {
		res := Compute(raw, cfg)
		for name, f := range map[string]FacetResult{
			"recency": res.Recency, "frequency": res.Frequency, "closeness": res.Closeness,
			"reach_out": res.ReachOut, "last_updated": res.LastUpdated,
		} {
			assert.True(t, known[f.ReasonCode], "%s: unknown reason code %q", name, f.ReasonCode)
			seen[f.ReasonCode] = true
			assertParams(t, f.ReasonCode, f.ReasonParams)
		}
		if res.Target != nil {
			assert.True(t, known[res.Target.ReasonCode], "target: unknown reason code %q", res.Target.ReasonCode)
			seen[res.Target.ReasonCode] = true
			assertParams(t, res.Target.ReasonCode, res.Target.ReasonParams)
		}
	}
	for _, c := range ReasonCodes() {
		assert.True(t, seen[c], "no input above produces %q; extend the inputs", c)
	}
}

func assertParams(t *testing.T, code string, p ReasonParams) {
	t.Helper()
	switch code {
	case ReasonClosenessDirect:
		assert.NotEmpty(t, p.Relation, code)
	case ReasonClosenessHops:
		assert.NotNil(t, p.Hops, code)
	case ReasonRecencyDays:
		assert.NotNil(t, p.Days, code)
		assert.NotNil(t, p.IntervalDays, code)
	case ReasonFrequencyCount:
		assert.NotNil(t, p.Count, code)
		assert.NotNil(t, p.WindowDays, code)
		assert.NotNil(t, p.Expected, code)
	case ReasonLastUpdatedDays:
		assert.NotNil(t, p.Days, code)
	case ReasonTargetPolicy:
		assert.NotNil(t, p.Score, code)
		assert.NotNil(t, p.IntervalDays, code)
	case ReasonTargetTier:
		assert.NotNil(t, p.Score, code)
		assert.NotNil(t, p.IntervalDays, code)
		assert.NotEmpty(t, p.Tier, code)
	default:
		assert.Equal(t, ReasonParams{}, p, "%s takes no params", code)
	}
}
