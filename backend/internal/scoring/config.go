package scoring

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"mycorrhizal/models"
)

// ConfigFile is the committed tunables file's path, relative to this
// package — mirrors internal/perfbench/budgets.go's BudgetsFile convention.
const ConfigFile = "testdata/weights.json"

//go:embed testdata/weights.json
var embeddedConfig []byte

// affinityRelationTypes are compatibility/sentiment edges layered on top of
// a structural bond, not a structural bond themselves (see the "Affinity
// edges" section of models/relationship_type_registry.go). Deliberately
// excluded from closeness-tier assignment — see ADR-0023 — so e.g. a
// conflicts_with-only edge does not read as "closer" than a stranger.
var affinityRelationTypes = map[string]bool{
	"gets_along_with": true,
	"conflicts_with":  true,
}

// RelationCloseness is one relation type's resolved closeness weight and its
// fallback interval (used when the contact has no real CadencePolicy).
type RelationCloseness struct {
	Weight              float64
	DefaultIntervalDays int
}

// Config is the fully resolved, validated set of scoring tunables — the
// flattened runtime form of the hand-authored testdata/weights.json (which
// groups relation types into named tiers for authorability; Config itself
// just wants a flat per-type lookup).
type Config struct {
	WeightRecency     float64
	WeightFrequency   float64
	WeightCloseness   float64
	WeightReachOut    float64
	WeightLastUpdated float64

	MossMin        float64
	ChanterelleMin float64

	// RelationCloseness holds only structural relation types (never
	// affinityRelationTypes) — see computeCloseness's lookup.
	RelationCloseness map[string]RelationCloseness

	UnknownClosenessWeight       float64
	UnknownClosenessIntervalDays int

	RecencyNoInteractionValue float64
	RecencyOverdueRatioCap    float64

	FrequencyWindowDays int

	ClosenessHopBaseWeight  float64
	ClosenessHopDecayPerHop float64
	ClosenessHopFloor       float64

	ReachOutPendingValue float64

	LastUpdatedWindowDays int
	LastUpdatedFloor      float64
}

// --- On-disk JSON shape (testdata/weights.json) ---

type weightEntry struct {
	Facet  string  `json:"facet"`
	Weight float64 `json:"weight"`
	Reason string  `json:"reason"`
}

type thresholdsDoc struct {
	MossMin        float64 `json:"moss_min"`
	ChanterelleMin float64 `json:"chanterelle_min"`
	Reason         string  `json:"reason"`
}

type tierAssignmentDoc struct {
	Tier   string `json:"tier"`
	Reason string `json:"reason"`
}

type tierDefinitionDoc struct {
	Weight              float64 `json:"weight"`
	DefaultIntervalDays int     `json:"default_interval_days"`
	Reason              string  `json:"reason"`
}

type unknownClosenessDoc struct {
	Weight              float64 `json:"weight"`
	DefaultIntervalDays int     `json:"default_interval_days"`
	Reason              string  `json:"reason"`
}

type recencyDoc struct {
	NoInteractionValue float64 `json:"no_interaction_value"`
	OverdueRatioCap    float64 `json:"overdue_ratio_cap"`
	Reason             string  `json:"reason"`
}

type frequencyDoc struct {
	WindowDays int    `json:"window_days"`
	Reason     string `json:"reason"`
}

type closenessDoc struct {
	HopBaseWeight  float64 `json:"hop_base_weight"`
	HopDecayPerHop float64 `json:"hop_decay_per_hop"`
	HopFloor       float64 `json:"hop_floor"`
	Reason         string  `json:"reason"`
}

type reachOutDoc struct {
	PendingValue float64 `json:"pending_value"`
	Reason       string  `json:"reason"`
}

type lastUpdatedDoc struct {
	WindowDays int     `json:"window_days"`
	Floor      float64 `json:"floor"`
	Reason     string  `json:"reason"`
}

type configDoc struct {
	Note             string                       `json:"note"`
	Weights          []weightEntry                `json:"weights"`
	Thresholds       thresholdsDoc                `json:"thresholds"`
	ClosenessTiers   map[string]tierAssignmentDoc `json:"closeness_tiers"`
	TierDefinitions  map[string]tierDefinitionDoc `json:"tier_definitions"`
	UnknownCloseness unknownClosenessDoc          `json:"unknown_closeness"`
	Recency          recencyDoc                   `json:"recency"`
	Frequency        frequencyDoc                 `json:"frequency"`
	Closeness        closenessDoc                 `json:"closeness"`
	ReachOut         reachOutDoc                  `json:"reach_out"`
	LastUpdated      lastUpdatedDoc               `json:"last_updated"`
}

// LoadConfig parses and validates the embedded testdata/weights.json.
// Production code and tests both call this rather than embedding a second
// copy — there is exactly one tunables file.
func LoadConfig() (Config, error) {
	return parseConfig(embeddedConfig)
}

// parseConfig is LoadConfig's testable core: exported so scoring_test.go can
// feed a synthetic broken doc to prove Validate rejects it, mirroring
// perfbench's TestEmbeddedBudgetsValid pattern (the embedded file must be
// valid AND Validate must be capable of catching a bad one).
func parseConfig(raw []byte) (Config, error) {
	var doc configDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Config{}, fmt.Errorf("scoring: parsing config: %w", err)
	}
	cfg, err := flatten(doc)
	if err != nil {
		return Config{}, err
	}
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// flatten converts the tier-grouped on-disk doc into Config's flat
// per-relation-type lookup.
func flatten(doc configDoc) (Config, error) {
	cfg := Config{
		MossMin:                      doc.Thresholds.MossMin,
		ChanterelleMin:               doc.Thresholds.ChanterelleMin,
		UnknownClosenessWeight:       doc.UnknownCloseness.Weight,
		UnknownClosenessIntervalDays: doc.UnknownCloseness.DefaultIntervalDays,
		RecencyNoInteractionValue:    doc.Recency.NoInteractionValue,
		RecencyOverdueRatioCap:       doc.Recency.OverdueRatioCap,
		FrequencyWindowDays:          doc.Frequency.WindowDays,
		ClosenessHopBaseWeight:       doc.Closeness.HopBaseWeight,
		ClosenessHopDecayPerHop:      doc.Closeness.HopDecayPerHop,
		ClosenessHopFloor:            doc.Closeness.HopFloor,
		ReachOutPendingValue:         doc.ReachOut.PendingValue,
		LastUpdatedWindowDays:        doc.LastUpdated.WindowDays,
		LastUpdatedFloor:             doc.LastUpdated.Floor,
	}

	for _, w := range doc.Weights {
		switch w.Facet {
		case "recency":
			cfg.WeightRecency = w.Weight
		case "frequency":
			cfg.WeightFrequency = w.Weight
		case "closeness":
			cfg.WeightCloseness = w.Weight
		case "reach_out":
			cfg.WeightReachOut = w.Weight
		case "last_updated":
			cfg.WeightLastUpdated = w.Weight
		default:
			return Config{}, fmt.Errorf("scoring: unknown weight facet %q", w.Facet)
		}
	}

	cfg.RelationCloseness = make(map[string]RelationCloseness, len(doc.ClosenessTiers))
	for relationType, assignment := range doc.ClosenessTiers {
		def, ok := doc.TierDefinitions[assignment.Tier]
		if !ok {
			return Config{}, fmt.Errorf("scoring: relation type %q assigned to undefined tier %q", relationType, assignment.Tier)
		}
		cfg.RelationCloseness[relationType] = RelationCloseness{
			Weight:              def.Weight,
			DefaultIntervalDays: def.DefaultIntervalDays,
		}
	}

	return cfg, nil
}

// validate rejects a config that would produce a silently-wrong score:
// weights that don't sum to 100 (so the weighted sum wouldn't land in
// [0,100]), inverted/out-of-range thresholds, non-positive intervals (which
// facets.go's own <1 guard would otherwise mask instead of surfacing at load
// time), and — the drift check — any structural relation type from the live
// registry (models/relationship_type_registry.go) missing a tier assignment,
// or an affinity type wrongly given one.
func validate(cfg Config) error {
	sum := cfg.WeightRecency + cfg.WeightFrequency + cfg.WeightCloseness + cfg.WeightReachOut + cfg.WeightLastUpdated
	if math.Abs(sum-100) > 0.001 {
		return fmt.Errorf("scoring: weights must sum to 100, got %.4f", sum)
	}
	for name, w := range map[string]float64{
		"recency": cfg.WeightRecency, "frequency": cfg.WeightFrequency,
		"closeness": cfg.WeightCloseness, "reach_out": cfg.WeightReachOut,
		"last_updated": cfg.WeightLastUpdated,
	} {
		if w < 0 || w > 100 {
			return fmt.Errorf("scoring: weight %q out of [0,100]: %.4f", name, w)
		}
	}

	if cfg.ChanterelleMin < 0 || cfg.MossMin > 100 || cfg.ChanterelleMin >= cfg.MossMin {
		return fmt.Errorf("scoring: thresholds must satisfy 0 <= chanterelle_min < moss_min <= 100, got chanterelle_min=%.2f moss_min=%.2f", cfg.ChanterelleMin, cfg.MossMin)
	}

	if cfg.UnknownClosenessWeight < 0 || cfg.UnknownClosenessWeight > 100 {
		return fmt.Errorf("scoring: unknown_closeness.weight out of [0,100]: %.2f", cfg.UnknownClosenessWeight)
	}
	if cfg.UnknownClosenessIntervalDays < 1 {
		return fmt.Errorf("scoring: unknown_closeness.default_interval_days must be >= 1, got %d", cfg.UnknownClosenessIntervalDays)
	}

	if cfg.RecencyNoInteractionValue < 0 || cfg.RecencyNoInteractionValue > 100 {
		return fmt.Errorf("scoring: recency.no_interaction_value out of [0,100]: %.2f", cfg.RecencyNoInteractionValue)
	}
	if cfg.RecencyOverdueRatioCap <= 0 {
		return fmt.Errorf("scoring: recency.overdue_ratio_cap must be > 0, got %.4f", cfg.RecencyOverdueRatioCap)
	}

	if cfg.FrequencyWindowDays < 1 {
		return fmt.Errorf("scoring: frequency.window_days must be >= 1, got %d", cfg.FrequencyWindowDays)
	}

	if cfg.ClosenessHopBaseWeight < 0 || cfg.ClosenessHopBaseWeight > 100 {
		return fmt.Errorf("scoring: closeness.hop_base_weight out of [0,100]: %.2f", cfg.ClosenessHopBaseWeight)
	}
	if cfg.ClosenessHopDecayPerHop < 0 {
		return fmt.Errorf("scoring: closeness.hop_decay_per_hop must be >= 0, got %.2f", cfg.ClosenessHopDecayPerHop)
	}
	if cfg.ClosenessHopFloor < 0 || cfg.ClosenessHopFloor > cfg.ClosenessHopBaseWeight {
		return fmt.Errorf("scoring: closeness.hop_floor must be in [0, hop_base_weight], got %.2f", cfg.ClosenessHopFloor)
	}

	if cfg.ReachOutPendingValue < 0 || cfg.ReachOutPendingValue > 100 {
		return fmt.Errorf("scoring: reach_out.pending_value out of [0,100]: %.2f", cfg.ReachOutPendingValue)
	}

	if cfg.LastUpdatedWindowDays < 1 {
		return fmt.Errorf("scoring: last_updated.window_days must be >= 1, got %d", cfg.LastUpdatedWindowDays)
	}
	if cfg.LastUpdatedFloor < 0 || cfg.LastUpdatedFloor > 100 {
		return fmt.Errorf("scoring: last_updated.floor out of [0,100]: %.2f", cfg.LastUpdatedFloor)
	}

	for _, rc := range cfg.RelationCloseness {
		if rc.Weight < 0 || rc.Weight > 100 {
			return fmt.Errorf("scoring: closeness tier weight out of [0,100]: %.2f", rc.Weight)
		}
		if rc.DefaultIntervalDays < 1 {
			return fmt.Errorf("scoring: closeness tier default_interval_days must be >= 1, got %d", rc.DefaultIntervalDays)
		}
	}

	for _, token := range models.KnownRelationTypes() {
		_, tiered := cfg.RelationCloseness[token]
		switch {
		case affinityRelationTypes[token] && tiered:
			return fmt.Errorf("scoring: affinity relation type %q must not have a closeness tier", token)
		case !affinityRelationTypes[token] && !tiered:
			return fmt.Errorf("scoring: structural relation type %q has no closeness tier assignment — add one to %s", token, ConfigFile)
		}
	}

	return nil
}
