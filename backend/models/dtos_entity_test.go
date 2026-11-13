package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NewEntityResponse always emits a JSON object for core: an empty stored core
// becomes "{}", never null or an empty (invalid) RawMessage (issue #2569).
func TestNewEntityResponse_EmptyCoreBecomesEmptyObject(t *testing.T) {
	resp := NewEntityResponse(&Entity{ID: "e1", Kind: EntityKindOrganisation, Name: "Acme", Core: ""})
	assert.JSONEq(t, "{}", string(resp.Core))
	out, err := json.Marshal(resp)
	require.NoError(t, err, "an empty core must still marshal")
	assert.Contains(t, string(out), `"core":{}`)
}

func TestNewEntityResponse_KeepsStoredCore(t *testing.T) {
	resp := NewEntityResponse(&Entity{ID: "e2", Kind: EntityKindOrganisation, Name: "Beta", Core: `{"website":"https://x.test"}`})
	assert.JSONEq(t, `{"website":"https://x.test"}`, string(resp.Core))
}
