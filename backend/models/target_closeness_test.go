package models

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTargetClosenessTiers_MirrorsMatch pins every hand-written copy of the
// tier vocabulary (frontend trap #4) to TargetClosenessTiers(), in order: the
// migration's CHECK, the API's enum, and both clients' constants. A constant
// compared with itself proves nothing; drift between the copies is the bug.
func TestTargetClosenessTiers_MirrorsMatch(t *testing.T) {
	want := TargetClosenessTiers()
	quoted := func(src string) []string {
		var out []string
		for _, m := range regexp.MustCompile(`['"]([a-z_]+)['"]`).FindAllStringSubmatch(src, -1) {
			out = append(out, m[1])
		}
		return out
	}
	read := func(path string) string {
		b, err := os.ReadFile(path)
		require.NoError(t, err)
		return string(b)
	}

	migration := read("../database/migrations/000076_target_closeness.up.sql")
	check := regexp.MustCompile(`target_closeness IN \(([^)]*)\)`).FindStringSubmatch(migration)
	require.NotNil(t, check, "000076 no longer carries the target_closeness CHECK")
	assert.Equal(t, want, quoted(check[1]), "migration 000076 CHECK")

	spec := read("../openapi.yaml")
	input := regexp.MustCompile(`(?s)ContactTargetClosenessInput:.*?enum: \[([^\]]*)\]`).FindStringSubmatch(spec)
	require.NotNil(t, input)
	assert.Equal(t, want, strings.Split(strings.ReplaceAll(input[1], " ", ""), ","), "openapi ContactTargetClosenessInput.tier")

	web := regexp.MustCompile(`TARGET_CLOSENESS_TIERS = \[([^\]]*)\]`).FindStringSubmatch(read("../../frontend/src/api/targetCloseness.ts"))
	require.NotNil(t, web)
	assert.Equal(t, want, quoted(web[1]), "frontend TARGET_CLOSENESS_TIERS")

	android := read("../../android/core/model/src/main/kotlin/com/mycorrhizal/crm/model/network/TargetCloseness.kt")
	consts := regexp.MustCompile(`(?s)object TargetClosenessTier \{(.*?)val ALL`).FindStringSubmatch(android)
	require.NotNil(t, consts)
	assert.Equal(t, want, quoted(consts[1]), "Android TargetClosenessTier constants")
}

func TestContactTargetClosenessInput_Validation(t *testing.T) {
	v := validator.New()
	for _, tier := range TargetClosenessTiers() {
		assert.NoError(t, v.Struct(ContactTargetClosenessInput{Tier: tier}), tier)
	}
	assert.Error(t, v.Struct(ContactTargetClosenessInput{Tier: "bogus"}))
	assert.Error(t, v.Struct(ContactTargetClosenessInput{}))
}

// RecordFromContact / RecordForContact echo the flat column into the envelope;
// ApplyRecordToContact never writes it back (ADR 0023 amendment).
func TestTargetCloseness_FlatColumnIsAuthoritative(t *testing.T) {
	tier := "close"
	c := &Contact{Firstname: "Ada", TargetCloseness: &tier}

	rec := RecordFromContact(c, "")
	require.NotNil(t, rec.Envelope.TargetCloseness)
	assert.Equal(t, "close", *rec.Envelope.TargetCloseness)

	// Even if a stale crm JSON column disagrees, the read overlays the flat value.
	stale := "distant"
	c.CRM = contactmodel.CRMEnvelope{TargetCloseness: &stale}
	rec = RecordForContact(c, "", nil)
	require.NotNil(t, rec.Envelope.TargetCloseness)
	assert.Equal(t, "close", *rec.Envelope.TargetCloseness)

	none := &Contact{Firstname: "Bob"}
	assert.Nil(t, RecordFromContact(none, "").Envelope.TargetCloseness)

	forged := "immediate"
	target := &Contact{TargetCloseness: &tier}
	ApplyRecordToContact(target, &contactmodel.Record{Envelope: contactmodel.CRMEnvelope{TargetCloseness: &forged}}, "")
	require.NotNil(t, target.TargetCloseness)
	assert.Equal(t, "close", *target.TargetCloseness, "ApplyRecordToContact must not write target_closeness")

	empty := &Contact{}
	ApplyRecordToContact(empty, &contactmodel.Record{Envelope: contactmodel.CRMEnvelope{TargetCloseness: &forged}}, "")
	assert.Nil(t, empty.TargetCloseness)
}

func TestMergeRecordFromFlat_TakesTargetClosenessFromFlat(t *testing.T) {
	stale := "distant"
	loaded := contactmodel.Record{Envelope: contactmodel.CRMEnvelope{TargetCloseness: &stale}}
	fresh := contactmodel.Record{}
	assert.Nil(t, mergeRecordFromFlat(loaded, fresh).Envelope.TargetCloseness)

	tier := "casual"
	fresh.Envelope.TargetCloseness = &tier
	got := mergeRecordFromFlat(loaded, fresh).Envelope.TargetCloseness
	require.NotNil(t, got)
	assert.Equal(t, "casual", *got)
}

func TestEnvelopeExportLossDiagnostics_NamesTargetClosenessOnlyWhenSet(t *testing.T) {
	has := func(rec *contactmodel.Record) bool {
		for _, d := range EnvelopeExportLossDiagnostics(rec) {
			if d.Concept == "crm.target_closeness" {
				return true
			}
		}
		return false
	}
	tier := "close"
	empty := ""
	assert.True(t, has(&contactmodel.Record{Envelope: contactmodel.CRMEnvelope{TargetCloseness: &tier}}))
	assert.False(t, has(&contactmodel.Record{}))
	assert.False(t, has(&contactmodel.Record{Envelope: contactmodel.CRMEnvelope{TargetCloseness: &empty}}))
}

// A plain Save of a loaded contact (photo upload, import merge, audit undo)
// keeps the target in both the flat column and the persisted envelope.
func TestContactBeforeSave_MirrorsTargetClosenessIntoEnvelope(t *testing.T) {
	tier := "close"
	c := &Contact{Firstname: "Ada"}
	forged := "distant"
	c.CRM = contactmodel.CRMEnvelope{TargetCloseness: &forged}
	c.TargetCloseness = &tier
	require.NoError(t, c.BeforeSave(nil))
	require.NotNil(t, c.CRM.TargetCloseness)
	assert.Equal(t, "close", *c.CRM.TargetCloseness)
}
