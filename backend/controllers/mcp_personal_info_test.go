package controllers

// Issue #2053: MCP get_contact builds the owner-facing detail record, which
// deliberately keeps sensitive postal addresses and personalInfo so a
// full-overwrite REST edit cannot drop them. MCP is a copy out to the
// caller's model, so include_sensitive=false must apply the personalInfo half
// of the outward default-deny too — entries *and* their CRM metadata.

import (
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCP_GetContact_SensitivePersonalInfoProjection(t *testing.T) {
	f := newMCPFixture(t)
	subject := seedMCPContacts(t, f.db, f.alice.ID, 1, "Subject")[0]

	record := &contactmodel.Record{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Subject"}}},
			PersonalInfo: []contactmodel.PersonalInfo{
				{ID: "pi-normal", Kind: "hobby", Value: "Normal Chess"},
				{ID: "pi-private", Kind: "hobby", Value: "Private Birdwatching"},
				{ID: "pi-secret", Kind: "hobby", Value: "Secret Lockpicking"},
			},
		},
		Envelope: contactmodel.CRMEnvelope{PersonalInfoMeta: map[string]contactmodel.PersonalInfoMeta{
			"pi-normal":  {Favorite: true, Notes: "normal hobby notes"},
			"pi-private": {Sensitivity: models.RelationshipSensitivityPrivate, Notes: "private hobby notes"},
			"pi-secret":  {Sensitivity: models.RelationshipSensitivitySecret, Notes: "secret hobby notes"},
		}},
	}
	models.ApplyRecordToContact(&subject, record, "")
	require.NoError(t, f.db.Save(&subject).Error)

	get := func(include bool) ([]string, map[string]any) {
		t.Helper()
		r := f.call(t, f.alice.ID, "get_contact", map[string]any{"id": subject.ID, "include_sensitive": include})
		require.False(t, r.isError, r.text)
		contact := r.structured["contact"].(map[string]any)
		card := contact["card"].(map[string]any)
		rawEntries, _ := card["personalInfo"].([]any)
		values := make([]string, 0, len(rawEntries))
		for _, raw := range rawEntries {
			entry := raw.(map[string]any)
			if value, ok := entry["value"].(string); ok {
				values = append(values, value)
			}
		}
		crm := contact["crm"].(map[string]any)
		meta, _ := crm["personalInfoMeta"].(map[string]any)
		return values, meta
	}

	defaultValues, defaultMeta := get(false)
	assert.Equal(t, []string{"Normal Chess"}, defaultValues,
		"private/secret personalInfo must not project into MCP get_contact by default")
	assert.Contains(t, defaultMeta, "pi-normal")
	assert.NotContains(t, defaultMeta, "pi-private",
		"private personalInfo metadata must be pruned with its entry")
	assert.NotContains(t, defaultMeta, "pi-secret")

	optedInValues, optedInMeta := get(true)
	assert.ElementsMatch(t, []string{"Normal Chess", "Private Birdwatching", "Secret Lockpicking"}, optedInValues,
		"include_sensitive=true must include every personalInfo tier")
	assert.Contains(t, optedInMeta, "pi-private")
	assert.Contains(t, optedInMeta, "pi-secret")
}
