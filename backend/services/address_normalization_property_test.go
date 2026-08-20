package services

import (
	"fmt"
	"math/rand"
	"testing"
	"unicode"

	"mycorrhizal/models"
)

// address_normalization_property_test.go: property-based (randomized,
// seeded) tests for AddressNormalizedKey (household_service.go) and the
// diffAddressChange guard (reach_out_trigger_service.go) — issue #255. The
// guard exists because PR #249 found that a distinct normalized-key pair can
// still render identically via models.FormatAddress, which would otherwise
// surface a meaningless "X -> X" reach-out suggestion; these tests generate
// many random address pairs per run to check that invariant broadly rather
// than against one hand-picked example.

const addressPropertyIterations = 200

// randFullAddress builds a random ContactAddress with every component
// (including the sub-street/Type fields AddressNormalizedKey deliberately
// excludes) non-empty, so mutating exactly one field never shifts which
// position lands in the "|"-joined key -- see the "postal-change
// sensitivity" property below for why that matters.
func randFullAddress(r *rand.Rand, tag string) models.ContactAddress {
	return models.ContactAddress{
		Type:      randAddrLabel(r),
		Street:    fmt.Sprintf("%s Street %d", tag, r.Intn(1000)),
		POBox:     fmt.Sprintf("PO Box %d", r.Intn(1000)),
		Apartment: fmt.Sprintf("Apt %d", r.Intn(1000)),
		Floor:     fmt.Sprintf("Floor %d", r.Intn(50)),
		City:      fmt.Sprintf("%s City %d", tag, r.Intn(1000)),
		Region:    fmt.Sprintf("%s Region %d", tag, r.Intn(1000)),
		Postal:    fmt.Sprintf("%05d", r.Intn(100_000)),
		Country:   fmt.Sprintf("%s Country %d", tag, r.Intn(1000)),
	}
}

func randAddrLabel(r *rand.Rand) string {
	labels := []string{"home", "work", "other"}
	return labels[r.Intn(len(labels))]
}

// noisyVariant returns a string that normalizeAddressPart must reduce to the
// exact same value as s: per-rune case flips (case-folded away), punctuation
// inserted at random positions (stripped entirely, regardless of position,
// by the isLetter/isDigit/isSpace filter), and extra whitespace only at
// existing word boundaries or at the very start/end (collapsed by
// strings.Fields). Never splits a word by inserting whitespace mid-token,
// which would change the normalized value rather than merely dress it up.
func noisyVariant(r *rand.Rand, s string) string {
	const punct = ".,-'\"!"
	out := make([]rune, 0, len(s)*2)
	for _, ru := range s {
		c := ru
		if unicode.IsLetter(c) && r.Intn(2) == 0 {
			if unicode.IsUpper(c) {
				c = unicode.ToLower(c)
			} else {
				c = unicode.ToUpper(c)
			}
		}
		out = append(out, c)
		if r.Intn(4) == 0 {
			out = append(out, rune(punct[r.Intn(len(punct))]))
		}
		if unicode.IsSpace(ru) && r.Intn(2) == 0 {
			out = append(out, ' ', ' ') // pad an existing word boundary, never split a word
		}
	}
	// Leading/trailing noise: whitespace is trimmed and punctuation is
	// filtered by normalizeAddressPart regardless of position.
	prefix := ""
	if r.Intn(2) == 0 {
		prefix = "  " + string(punct[r.Intn(len(punct))])
	}
	suffix := ""
	if r.Intn(2) == 0 {
		suffix = string(punct[r.Intn(len(punct))]) + "  "
	}
	return prefix + string(out) + suffix
}

// TestAddressNormalizedKey_NormalizationInsensitive is the property
// normalizeAddressPart's own doc comment claims: "Two addresses differing
// only in casing, trailing punctuation, or spacing ... normalize to the same
// part." Fuzzes every key-relevant component (Street/City/Region/Postal/
// Country) with a case/whitespace/punctuation-noisy variant and asserts the
// key is unaffected.
func TestAddressNormalizedKey_NormalizationInsensitive(t *testing.T) {
	for i := 0; i < addressPropertyIterations; i++ {
		r := rand.New(rand.NewSource(int64(4000 + i)))
		tag := fmt.Sprintf("norm-%d", i)

		base := randFullAddress(r, tag)
		noisy := base
		noisy.Street = noisyVariant(r, base.Street)
		noisy.City = noisyVariant(r, base.City)
		noisy.Region = noisyVariant(r, base.Region)
		noisy.Postal = noisyVariant(r, base.Postal)
		noisy.Country = noisyVariant(r, base.Country)

		baseKey := AddressNormalizedKey(base)
		noisyKey := AddressNormalizedKey(noisy)
		if baseKey != noisyKey {
			t.Fatalf("iteration %d (seed %d): AddressNormalizedKey(%+v) = %q, AddressNormalizedKey(%+v) = %q, want equal",
				i, 4000+i, base, baseKey, noisy, noisyKey)
		}
	}
}

// TestAddressNormalizedKey_SubStreetAndTypeIndependent pins
// AddressNormalizedKey's documented exclusion: POBox/Apartment/Floor/Type
// are deliberately not part of the key, so two addresses sharing a street
// but differing in unit are still "the same building".
func TestAddressNormalizedKey_SubStreetAndTypeIndependent(t *testing.T) {
	for i := 0; i < addressPropertyIterations; i++ {
		r := rand.New(rand.NewSource(int64(5000 + i)))
		tag := fmt.Sprintf("substreet-%d", i)

		base := randFullAddress(r, tag)
		other := base
		other.POBox = randPropertyString(r, tag+"-different-pobox")
		other.Apartment = randPropertyString(r, tag+"-different-apt")
		other.Floor = randPropertyString(r, tag+"-different-floor")
		other.Type = randAddrLabel(r)

		if got, want := AddressNormalizedKey(other), AddressNormalizedKey(base); got != want {
			t.Fatalf("iteration %d (seed %d): changing only POBox/Apartment/Floor/Type changed the key: got %q, want %q (base=%+v, other=%+v)",
				i, 5000+i, got, want, base, other)
		}
	}
}

// TestAddressNormalizedKey_PostalChangeIsDetected is the converse of the
// insensitivity property: a genuine change to a key-relevant component must
// change the key. Both addresses hold all five key-relevant fields non-empty
// so the "|"-joined position of Postal never shifts (see randFullAddress),
// which is what makes "the key changed" a safe, unconditional assertion here
// -- unlike the general case, where AddressNormalizedKey can collapse two
// materially different addresses that leave different components empty.
func TestAddressNormalizedKey_PostalChangeIsDetected(t *testing.T) {
	for i := 0; i < addressPropertyIterations; i++ {
		r := rand.New(rand.NewSource(int64(6000 + i)))
		tag := fmt.Sprintf("postal-%d", i)

		base := randFullAddress(r, tag)
		changed := base
		changed.Postal = base.Postal + "x" // guaranteed to normalize differently: strictly extends the last token

		if got, want := AddressNormalizedKey(changed), AddressNormalizedKey(base); got == want {
			t.Fatalf("iteration %d (seed %d): postal-only change (%q -> %q) did not change the key (%q); a real move must be detectable",
				i, 6000+i, base.Postal, changed.Postal, got)
		}
	}
}

// TestDiffAddressChange_NeverFiresANoOpLookingChange is the actual PR #249
// regression, generalized: whenever diffAddressChange reports a change, the
// displayed NewValue must be non-empty and different from OldValue --
// otherwise the reach-out suggestion reads as a meaningless "X -> X" (or
// blank), which is exactly what the guard in diffAddressChange
// (models.FormatAddress and AddressNormalizedKey don't cover identical
// component sets") exists to prevent.
func TestDiffAddressChange_NeverFiresANoOpLookingChange(t *testing.T) {
	for i := 0; i < addressPropertyIterations; i++ {
		r := rand.New(rand.NewSource(int64(7000 + i)))
		tag := fmt.Sprintf("diff-%d", i)

		var before, after []models.ContactAddress
		if i%3 == 0 {
			// Purely random full/sparse addresses essentially never collide
			// by chance -- the guard exists for a specific structural
			// collision (a value shifted from a key-field position into a
			// render-only field), so seed a third of iterations with that
			// shape directly rather than relying on luck to find it.
			before, after = randKeyRenderCollisionPair(r, tag)
		} else {
			before = randAddressSlice(r, tag+"-before")
			after = randAddressSlice(r, tag+"-after")
		}

		change, ok := diffAddressChange(before, after)
		if !ok {
			continue
		}
		if change.NewValue == "" {
			t.Fatalf("iteration %d (seed %d): diffAddressChange fired with a blank NewValue (before=%+v, after=%+v)",
				i, 7000+i, before, after)
		}
		if change.NewValue == change.OldValue {
			t.Fatalf("iteration %d (seed %d): diffAddressChange fired an \"X -> X\" change: %q -> %q (before=%+v, after=%+v)",
				i, 7000+i, change.OldValue, change.NewValue, before, after)
		}
	}
}

// TestDiffAddressChange_NoOpSaveNeverFires: re-saving the exact same address
// set (even reordered) must never surface a change.
func TestDiffAddressChange_NoOpSaveNeverFires(t *testing.T) {
	for i := 0; i < addressPropertyIterations; i++ {
		r := rand.New(rand.NewSource(int64(8000 + i)))
		tag := fmt.Sprintf("noop-diff-%d", i)

		before := randAddressSlice(r, tag)
		after := make([]models.ContactAddress, len(before))
		copy(after, before)
		if len(after) > 1 && r.Intn(2) == 0 {
			after[0], after[len(after)-1] = after[len(after)-1], after[0] // reorder only
		}

		if _, ok := diffAddressChange(before, after); ok {
			t.Fatalf("iteration %d (seed %d): diffAddressChange fired on an unchanged (possibly reordered) address set: %+v",
				i, 8000+i, before)
		}
	}
}

// randKeyRenderCollisionPair builds a before/after pair engineered to
// collide exactly the way AddressNormalizedKey and models.FormatAddress can
// disagree: AddressNormalizedKey never looks at POBox, but FormatAddress
// renders it right after Street -- so a value that moves from City into
// POBox renders identically ("street, value") while the key drops the value
// entirely (empty City) and so changes. This is the concrete shape of the
// "distinct key pair can still render the same" gap the diffAddressChange
// guard's own comment describes; without the guard, this pair would surface
// a meaningless "X -> X" reach-out suggestion.
func randKeyRenderCollisionPair(r *rand.Rand, tag string) (before, after []models.ContactAddress) {
	street := fmt.Sprintf("%s Street %d", tag, r.Intn(1000))
	shifted := fmt.Sprintf("%s Shifted %d", tag, r.Intn(1000))
	before = []models.ContactAddress{{Street: street, City: shifted}}
	after = []models.ContactAddress{{Street: street, POBox: shifted}}
	return before, after
}

func randAddressSlice(r *rand.Rand, tag string) []models.ContactAddress {
	n := r.Intn(3) // 0..2 addresses
	out := make([]models.ContactAddress, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, randFullAddress(r, fmt.Sprintf("%s-%d", tag, i)))
	}
	return out
}

func randPropertyString(r *rand.Rand, tag string) string {
	return fmt.Sprintf("%s-%d", tag, r.Intn(1_000_000))
}
