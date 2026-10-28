package models

import (
	"testing"
	"time"

	"mycorrhizal/contactmodel"

	"github.com/stretchr/testify/assert"
)

func ip(v int) *int { return &v }

// Issue #2001: an edge has ended once its end date (at its own precision) has
// passed; an absent, future, in-progress or year-less end date is still active.
func TestRelationshipEdgeEnded(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		end  *contactmodel.PartialDate
		want bool
	}{
		{"no end date", nil, false},
		{"past year", &contactmodel.PartialDate{Year: ip(2018)}, true},
		{"this year (not over)", &contactmodel.PartialDate{Year: ip(2026)}, false},
		{"future year", &contactmodel.PartialDate{Year: ip(2030)}, false},
		{"past month", &contactmodel.PartialDate{Year: ip(2026), Month: ip(9)}, true},
		{"this month (not over)", &contactmodel.PartialDate{Year: ip(2026), Month: ip(10)}, false},
		{"yesterday", &contactmodel.PartialDate{Year: ip(2026), Month: ip(10), Day: ip(26)}, true},
		{"today", &contactmodel.PartialDate{Year: ip(2026), Month: ip(10), Day: ip(27)}, true},
		{"tomorrow", &contactmodel.PartialDate{Year: ip(2026), Month: ip(10), Day: ip(28)}, false},
		{"year-less", &contactmodel.PartialDate{Month: ip(1), Day: ip(1)}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := RelationshipEdge{EndDate: c.end}
			assert.Equal(t, c.want, e.Ended(now))
		})
	}
}
