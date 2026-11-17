package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Issue #2631 (dedupe half): the organisation comparison key must treat
// spelling variants of one name as equal, and must not equate different names.
func TestOrganisationComparisonKey_VariantsMatch(t *testing.T) {
	groups := [][]string{
		// hyphen / spacing / case inside a word
		{"Coop", "Co-op", "co op", "CO-OP", "Co.op"},
		{"Coca-Cola", "Coca Cola", "Cocacola"},
		{"Ace Hardware", "AceHardware", "ace  hardware"},
		// legal-suffix synonyms, with and without punctuation
		{"Acme Ltd", "Acme Limited", "Acme, Ltd.", "ACME LIMITED", "Acme"},
		{"Acme Inc", "Acme Incorporated", "Acme, Inc.", "Acme Inc"},
		{"Acme Co", "Acme Company", "Acme Co.", "Acme"},
		{"Acme Corp", "Acme Corporation", "Acme Corp."},
		{"Acme LLC", "Acme L.L.C.", "Acme, LLC"},
		{"Acme GmbH", "Acme G.m.b.H.", "Acme gmbh"},
		// a suffix on a hyphenated name
		{"Co-op Ltd", "Coop Limited", "Coop"},
	}
	for _, g := range groups {
		want := OrganisationComparisonKey(g[0])
		assert.NotEmpty(t, want, g[0])
		for _, name := range g[1:] {
			assert.Equal(t, want, OrganisationComparisonKey(name), "%q should match %q", name, g[0])
		}
	}
}

func TestOrganisationComparisonKey_DifferentNamesStayApart(t *testing.T) {
	pairs := [][2]string{
		// a longer word that merely starts with another
		{"Coop", "Cooper"},
		{"Co-op", "Cooper"},
		// different names sharing the same legal suffix
		{"Acme Ltd", "Globex Ltd"},
		{"Acme Limited", "Globex Limited"},
		{"Acme Inc", "Globex Incorporated"},
		// a suffix word that is not trailing is part of the name
		{"Limited Brands", "Brands"},
		{"Company Store", "Store"},
		{"Acme Incorporated Holdings", "Acme"},
		{"Corporation Street Cafe", "Street Cafe"},
		// different suffix-like words are not synonyms of each other
		{"Acme Ltd", "Acme Inc"}, // still equal after dropping: see below
	}
	for _, p := range pairs[:len(pairs)-1] {
		assert.NotEqual(t, OrganisationComparisonKey(p[0]), OrganisationComparisonKey(p[1]), "%q must not match %q", p[0], p[1])
	}
	// Documented behaviour (ADR 0043 decision 8): trailing legal suffixes are
	// dropped whatever they are, so "Acme Ltd" and "Acme Inc" do pair.
	last := pairs[len(pairs)-1]
	assert.Equal(t, OrganisationComparisonKey(last[0]), OrganisationComparisonKey(last[1]))
}

func TestOrganisationComparisonKey_Edges(t *testing.T) {
	assert.Equal(t, "", OrganisationComparisonKey("???"), "punctuation only has no key")
	assert.Equal(t, "ltd", OrganisationComparisonKey("Ltd"), "a name that is only a suffix keeps it")
	assert.Equal(t, "limited", OrganisationComparisonKey("Limited"))
	assert.Equal(t, "café", OrganisationComparisonKey("Café"), "no diacritic folding (ADR 0016)")
	assert.NotEqual(t, OrganisationComparisonKey("Café Noir"), OrganisationComparisonKey("Cafe Noir"))
}
