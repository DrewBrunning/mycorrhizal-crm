package models

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Resolution order (issue #1997): per-contact override > account default
// (enabled_contact_fields) > built-in default.
func TestResolveFieldVisibility_Order(t *testing.T) {
	t.Run("built-in default when the account never configured fields", func(t *testing.T) {
		defaults, resolved := ResolveFieldVisibility(nil, false, nil)
		assert.True(t, resolved.Fields["emails"], "emails is in the built-in default")
		assert.False(t, resolved.Fields["suffix"], "suffix is not in the built-in default")
		assert.Equal(t, defaults, resolved, "no overrides: resolved == the account-level defaults")
		assert.Len(t, resolved.Fields, len(ContactFieldKeys), "every known key is decided")
	})

	t.Run("account default beats the built-in default", func(t *testing.T) {
		_, resolved := ResolveFieldVisibility([]string{"suffix"}, false, nil)
		assert.True(t, resolved.Fields["suffix"])
		assert.False(t, resolved.Fields["emails"], "an explicit account list REPLACES the built-in set")
	})

	t.Run("an empty account list hides everything", func(t *testing.T) {
		_, resolved := ResolveFieldVisibility([]string{}, false, nil)
		for k, v := range resolved.Fields {
			assert.False(t, v, k)
		}
	})

	t.Run("per-contact override beats the account default in both directions", func(t *testing.T) {
		defaults, resolved := ResolveFieldVisibility([]string{"emails", "suffix"}, false, map[string]string{
			"emails": FieldStateHide, // default on -> hidden
			"phones": FieldStateShow, // default off -> shown
		})
		assert.False(t, resolved.Fields["emails"])
		assert.True(t, resolved.Fields["phones"])
		assert.True(t, resolved.Fields["suffix"], "an untouched field inherits the account default")
		assert.True(t, defaults.Fields["emails"], "defaults never reflect overrides")
		assert.False(t, defaults.Fields["phones"])
	})

	t.Run("hide-empty: per-contact beats account in both directions", func(t *testing.T) {
		_, on := ResolveFieldVisibility(nil, true, nil)
		assert.True(t, on.HideEmpty, "account default on")
		_, off := ResolveFieldVisibility(nil, true, map[string]string{HideEmptyOverrideKey: FieldStateShow})
		assert.False(t, off.HideEmpty, "override show beats account hide-empty")
		_, forced := ResolveFieldVisibility(nil, false, map[string]string{HideEmptyOverrideKey: FieldStateHide})
		assert.True(t, forced.HideEmpty, "override hide beats account show-all")
		d, _ := ResolveFieldVisibility(nil, true, map[string]string{HideEmptyOverrideKey: FieldStateShow})
		assert.True(t, d.HideEmpty, "defaults carry the account value, not the override")
	})

	t.Run("unknown and inherit values never widen visibility", func(t *testing.T) {
		_, resolved := ResolveFieldVisibility(nil, false, map[string]string{
			"suffix":      "bogus",
			"not_a_field": FieldStateShow,
			"emails":      FieldStateInherit,
		})
		assert.False(t, resolved.Fields["suffix"], "an unrecognised state falls through to the default")
		assert.NotContains(t, resolved.Fields, "not_a_field", "only allowlisted keys are ever decided")
		assert.True(t, resolved.Fields["emails"])
	})
}

func TestIsContactFieldKey(t *testing.T) {
	assert.True(t, IsContactFieldKey("emails"))
	assert.False(t, IsContactFieldKey(HideEmptyOverrideKey), "the reserved key is not a contact field")
	assert.False(t, IsContactFieldKey(""))
	assert.False(t, IsContactFieldKey("firstname"), "firstname/lastname are always shown, not toggleable")
}

func TestDefaultEnabledContactFieldsAreKnownKeys(t *testing.T) {
	for _, k := range DefaultEnabledContactFields {
		assert.True(t, IsContactFieldKey(k), k)
	}
}

// The Go allowlist and defaults mirror frontend/src/contactFields.ts; this
// pins them so a field added on one side cannot silently be unoverridable (or
// accepted-but-unrendered) on the other.
func TestContactFieldKeysMatchFrontend(t *testing.T) {
	src, err := os.ReadFile("../../frontend/src/contactFields.ts")
	require.NoError(t, err)
	text := string(src)

	union := regexp.MustCompile(`(?s)export type ContactFieldKey =(.*?);`).FindStringSubmatch(text)
	require.NotNil(t, union, "ContactFieldKey union not found")
	var fe []string
	for _, m := range regexp.MustCompile(`'([A-Za-z_]+)'`).FindAllStringSubmatch(union[1], -1) {
		fe = append(fe, m[1])
	}
	assert.Equal(t, sorted(fe), sorted(ContactFieldKeys), "ContactFieldKeys differs from the frontend ContactFieldKey union")

	def := regexp.MustCompile(`(?s)DEFAULT_ENABLED_CONTACT_FIELDS: ContactFieldKey\[\] = \[(.*?)\];`).FindStringSubmatch(text)
	require.NotNil(t, def, "DEFAULT_ENABLED_CONTACT_FIELDS not found")
	var feDefault []string
	for _, m := range regexp.MustCompile(`'([A-Za-z_]+)'`).FindAllStringSubmatch(def[1], -1) {
		feDefault = append(feDefault, m[1])
	}
	assert.Equal(t, sorted(feDefault), sorted(DefaultEnabledContactFields), "DefaultEnabledContactFields differs from the frontend default set")
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func TestContactFieldKeysHaveNoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range ContactFieldKeys {
		assert.False(t, seen[k], "duplicate key %s", k)
		assert.False(t, strings.HasPrefix(k, "__"), "%s collides with the reserved-key namespace", k)
		seen[k] = true
	}
}
