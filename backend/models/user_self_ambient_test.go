package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// users.self_contact_ambient_set (issues #2193/#2192) is CRM-only provenance:
// it must not appear in the user's JSON, and it lives on User, not Contact, so
// no Card, vCard, JSContact or CardDAV projection can carry it.
func TestSelfContactAmbientSet_StaysOutOfWireShapes(t *testing.T) {
	raw, err := json.Marshal(User{SelfContactAmbientSet: true})
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "ambient_set")

	contactJSON, err := json.Marshal(RecordFromContact(&Contact{Firstname: "Me", Ambient: true}, ""))
	require.NoError(t, err)
	assert.NotContains(t, string(contactJSON), "ambient_set")
	assert.NotContains(t, string(contactJSON), "self_contact")
}
