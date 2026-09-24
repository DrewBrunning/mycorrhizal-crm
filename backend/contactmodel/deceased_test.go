package contactmodel

import "testing"

func TestCard_DeathAnniversary(t *testing.T) {
	t.Run("nil when no anniversaries are recorded", func(t *testing.T) {
		c := Card{}
		if got := c.DeathAnniversary(); got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("nil when only a birth anniversary is recorded", func(t *testing.T) {
		c := Card{Anniversaries: []Anniversary{
			{Kind: "birth", Date: AnniversaryDate{Partial: date(1990, 6, 15)}},
		}}
		if got := c.DeathAnniversary(); got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("returns the death anniversary among mixed kinds", func(t *testing.T) {
		death := Anniversary{Kind: "death", Date: AnniversaryDate{Partial: date(2020, 5, 1)}}
		c := Card{Anniversaries: []Anniversary{
			{Kind: "birth", Date: AnniversaryDate{Partial: date(1990, 6, 15)}},
			death,
			{Kind: "wedding", Date: AnniversaryDate{Partial: date(2015, 9, 1)}},
		}}
		got := c.DeathAnniversary()
		if got == nil {
			t.Fatal("expected a death anniversary, got nil")
		}
		if got.Date.Partial.Year == nil || *got.Date.Partial.Year != 2020 {
			t.Errorf("expected the death anniversary's year 2020, got %+v", got.Date.Partial)
		}
	})

	t.Run("first match wins when more than one death anniversary is present", func(t *testing.T) {
		c := Card{Anniversaries: []Anniversary{
			{Kind: "death", Date: AnniversaryDate{Partial: date(2020, 5, 1)}},
			{Kind: "death", Date: AnniversaryDate{Partial: date(2021, 1, 1)}},
		}}
		got := c.DeathAnniversary()
		if got == nil || got.Date.Partial.Year == nil || *got.Date.Partial.Year != 2020 {
			t.Errorf("expected the first death anniversary (2020), got %+v", got)
		}
	})
}

func TestCard_IsDeceased(t *testing.T) {
	t.Run("false with no anniversaries", func(t *testing.T) {
		if (Card{}).IsDeceased() {
			t.Error("expected false for a Card with no anniversaries")
		}
	})

	t.Run("false with only a birth anniversary", func(t *testing.T) {
		c := Card{Anniversaries: []Anniversary{
			{Kind: "birth", Date: AnniversaryDate{Partial: date(1990, 6, 15)}},
		}}
		if c.IsDeceased() {
			t.Error("expected false for a Card with only a birth anniversary")
		}
	})

	t.Run("true with a death anniversary", func(t *testing.T) {
		c := Card{Anniversaries: []Anniversary{
			{Kind: "death", Date: AnniversaryDate{Partial: date(2020, 5, 1)}},
		}}
		if !c.IsDeceased() {
			t.Error("expected true for a Card with a death anniversary")
		}
	})
}
