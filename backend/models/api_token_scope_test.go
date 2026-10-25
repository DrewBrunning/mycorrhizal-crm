package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScopesForLegacyScope(t *testing.T) {
	assert.Equal(t, []string{"carddav", "caldav", "api:read", "api:write", "mcp"}, ScopesForLegacyScope("full"))
	assert.Equal(t, []string{"carddav", "caldav"}, ScopesForLegacyScope("carddav"))
	assert.Empty(t, ScopesForLegacyScope("unknown"))
}

func TestApiTokenEffectiveScopes(t *testing.T) {
	explicit := ApiToken{Scope: "carddav", Scopes: []string{"api:read"}}
	assert.Equal(t, []string{"api:read"}, explicit.EffectiveScopes(), "an explicit scope set wins over the legacy column")
	assert.True(t, explicit.HasScope("api:read"))
	assert.False(t, explicit.HasScope("carddav"))

	legacy := ApiToken{Scope: "carddav"}
	assert.Equal(t, []string{"carddav", "caldav"}, legacy.EffectiveScopes(), "a pre-migration row falls back to its old scope")
	assert.True(t, legacy.HasScope("caldav"))
	assert.False(t, legacy.HasScope("api:read"))

	full := ApiToken{Scope: "full"}
	assert.True(t, full.HasAnyAPIScope())
}

func TestLegacyScopeForScopes(t *testing.T) {
	assert.Equal(t, "full", LegacyScopeForScopes(ScopesForLegacyScope("full")))
	assert.Equal(t, "carddav", LegacyScopeForScopes(ScopesForLegacyScope("carddav")))
	assert.Equal(t, "custom", LegacyScopeForScopes([]string{"api:read"}))
}

func TestNormalizeTokenScopes(t *testing.T) {
	assert.Equal(t, []string{"carddav", "caldav"}, NormalizeTokenScopes([]string{"caldav", "carddav", "caldav"}))
	assert.Nil(t, NormalizeTokenScopes(nil))
}

func TestIsValidTokenScope(t *testing.T) {
	for _, s := range []string{"carddav", "caldav", "api:read", "api:write", "mcp"} {
		assert.Truef(t, IsValidTokenScope(s), "%q should be valid", s)
	}
	for _, s := range []string{"", "full", "api", "carddav "} {
		assert.Falsef(t, IsValidTokenScope(s), "%q should be invalid", s)
	}
}
