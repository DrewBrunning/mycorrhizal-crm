package contactmodel

import "testing"

func TestJCardPropRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullJCardProp())
}

func TestPassthroughRoundTrip(t *testing.T) {
	t.Parallel()
	assertRoundTrip(t, fullPassthrough())
}
