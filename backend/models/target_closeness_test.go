package models

import (
	"testing"

	"mycorrhizal/contactmodel"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTargetClosenessTiers_Order(t *testing.T) {
	assert.Equal(t, []string{"immediate", "close", "casual", "distant"}, TargetClosenessTiers())
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
