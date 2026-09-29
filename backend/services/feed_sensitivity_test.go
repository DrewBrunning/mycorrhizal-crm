package services

import (
	"reflect"
	"strings"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/require"
)

// timelineModels maps every models.TimelineTypes entry to the model struct
// behind it. A seventh timeline type added to models.TimelineTypes without an
// entry here fails TestFeedSensitivityCompleteness.
var timelineModels = map[string]any{
	models.TimelineTypeNote:             models.Note{},
	models.TimelineTypeActivity:         models.Activity{},
	models.TimelineTypeCompletion:       models.ReminderCompletion{},
	models.TimelineTypeLifeEvent:        models.LifeEvent{},
	models.TimelineTypeExternalActivity: models.ExternalActivity{},
	models.TimelineTypeGift:             models.Gift{},
}

// TestFeedSensitivityCompleteness is ADR 0030 decision 5's structural
// guarantee: if any timeline model gains a Sensitivity field, the feed
// composer must filter it (feedSensitivityFiltered), or this test fails. The
// fix is to add the filter, never to allowlist the finding.
func TestFeedSensitivityCompleteness(t *testing.T) {
	// The map must cover every timeline type, in both directions.
	for _, typ := range models.TimelineTypes {
		require.Contains(t, timelineModels, typ, "timeline type %q has no model mapping in timelineModels", typ)
	}
	require.Len(t, timelineModels, len(models.TimelineTypes),
		"timelineModels has entries that are not models.TimelineTypes values")

	for typ, model := range timelineModels {
		if !hasSensitivityField(reflect.TypeOf(model)) {
			continue
		}
		require.Truef(t, feedSensitivityFiltered[typ],
			"timeline model for %q has a Sensitivity field but %q is not in feedSensitivityFiltered; "+
				"add the sensitivity='normal' filter to the feed composer", typ, typ)
	}
}

// hasSensitivityField reports whether a struct type exposes a Sensitivity
// field at any depth of embedded structs.
func hasSensitivityField(t reflect.Type) bool {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if strings.EqualFold(f.Name, "Sensitivity") {
			return true
		}
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			if hasSensitivityField(f.Type) {
				return true
			}
		}
	}
	return false
}
