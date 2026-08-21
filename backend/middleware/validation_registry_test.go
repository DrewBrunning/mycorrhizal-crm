package middleware

import (
	"testing"

	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
)

// TestValidateRelationType exercises the relation_type custom validator tag
// (registered in init()) through ValidateVar, the same primitive the
// definition-driven FieldValue validation uses. The valid list is derived from
// the registry in models/relationship_type_registry.go (the single source of
// truth) so a newly added token is covered automatically rather than drifting.
func TestValidateRelationType(t *testing.T) {
	for _, token := range models.KnownRelationTypes() {
		if !ValidateVar(token, "relation_type") {
			t.Errorf("ValidateVar(%q, relation_type) = false, want true", token)
		}
	}

	invalid := []string{"", "boss_of", "friend", "FRIEND_OF", " spouse_of", "parent_of ", "unknown"}
	for _, token := range invalid {
		if ValidateVar(token, "relation_type") {
			t.Errorf("ValidateVar(%q, relation_type) = true, want false", token)
		}
	}
}

// TestValidateLifeEventCategory exercises the life_event_category custom
// validator tag against the registry in models/life_event_type_registry.go.
func TestValidateLifeEventCategory(t *testing.T) {
	for _, token := range models.LifeEventCategories() {
		if !ValidateVar(token, "life_event_category") {
			t.Errorf("ValidateVar(%q, life_event_category) = false, want true", token)
		}
	}

	invalid := []string{"", "unknown", "home", "HOME_LIVING", "health"}
	for _, token := range invalid {
		if ValidateVar(token, "life_event_category") {
			t.Errorf("ValidateVar(%q, life_event_category) = true, want false", token)
		}
	}
}

// TestValidateFieldDefinitionProjection exercises the fielddefprojection
// custom validator tag. The accepted grammar is "internal-only" (the default,
// never exported) or "vcard:X-<NAME>" per the ADR; the doc explicitly rejects
// a raw "jscontact:<pointer>" form.
func TestValidateFieldDefinitionProjection(t *testing.T) {
	valid := []string{"internal-only", "vcard:X-SOCIALPROFILE", "vcard:X-ABLabel", "vcard:X-CUSTOM", "vcard:X-ABC-123"}
	for _, v := range valid {
		if !ValidateVar(v, "fielddefprojection") {
			t.Errorf("ValidateVar(%q, fielddefprojection) = false, want true", v)
		}
	}

	invalid := []string{"", "jscontact:/x", "vcard:NO_HYPHEN", "vcard:", "internal", "vcard:X", "internal-only ", "X-ABLabel"}
	for _, v := range invalid {
		if ValidateVar(v, "fielddefprojection") {
			t.Errorf("ValidateVar(%q, fielddefprojection) = true, want false", v)
		}
	}
}

// TestGetValidated pins the type-safe accessor's behavior: a missing value in
// context and a mismatched type both yield a typed INVALID_INPUT error.
func TestGetValidated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("missing value", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		got, err := GetValidated[map[string]string](c)
		if got != nil {
			t.Errorf("expected nil result, got %+v", got)
		}
		if err == nil {
			t.Fatal("expected an error for a missing validated value")
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		c.Set("validated", "not-a-struct")
		got, err := GetValidated[map[string]string](c)
		if got != nil {
			t.Errorf("expected nil result, got %+v", got)
		}
		if err == nil {
			t.Fatal("expected an error for a type mismatch")
		}
	})

	t.Run("correct type", func(t *testing.T) {
		c, _ := gin.CreateTestContext(nil)
		want := map[string]string{"email": "a@b.c"}
		c.Set("validated", &want)
		got, err := GetValidated[map[string]string](c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || (*got)["email"] != "a@b.c" {
			t.Errorf("got %+v, want map with email=a@b.c", got)
		}
	})
}
