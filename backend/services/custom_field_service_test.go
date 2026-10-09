package services

import (
	"encoding/json"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func raw(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func TestValidateFieldValue_String(t *testing.T) {
	maxLen := 5
	def := models.FieldDefinition{Key: "nickname", Type: models.FieldTypeString, Constraints: models.FieldConstraints{MaxLength: &maxLen}}

	assert.NoError(t, ValidateFieldValue(def, raw(t, "short")))
	assert.ErrorContains(t, ValidateFieldValue(def, raw(t, "way too long")), "value exceeds maximum length of 5", "exceeds MaxLength")
	assert.ErrorContains(t, ValidateFieldValue(def, raw(t, 42)), "expected a string value", "wrong JSON type")
}

func TestValidateFieldValue_StringPattern(t *testing.T) {
	def := models.FieldDefinition{Key: "code", Type: models.FieldTypeString, Constraints: models.FieldConstraints{Pattern: `^[A-Z]{3}$`}}

	assert.NoError(t, ValidateFieldValue(def, raw(t, "ABC")))
	assert.ErrorContains(t, ValidateFieldValue(def, raw(t, "abc")), "value does not match the required pattern", "lowercase does not match pattern")
}

func TestValidateFieldValue_Number(t *testing.T) {
	min, max := 0.0, 10.0
	def := models.FieldDefinition{Key: "rating", Type: models.FieldTypeNumber, Constraints: models.FieldConstraints{Min: &min, Max: &max}}

	assert.NoError(t, ValidateFieldValue(def, raw(t, 7)))
	assert.ErrorContains(t, ValidateFieldValue(def, raw(t, 11)), "value 11 is out of the allowed range", "above max")
	assert.ErrorContains(t, ValidateFieldValue(def, raw(t, -1)), "value -1 is out of the allowed range", "below min")
	assert.ErrorContains(t, ValidateFieldValue(def, raw(t, "not a number")), "expected a number value")
	assert.ErrorContains(t, ValidateFieldValue(def, raw(t, nil)), "expected a number value", "JSON null is explicitly rejected, not silently cast to 0")
}

func TestValidateFieldValue_Boolean(t *testing.T) {
	def := models.FieldDefinition{Key: "opted_in", Type: models.FieldTypeBoolean}

	assert.NoError(t, ValidateFieldValue(def, raw(t, true)))
	assert.ErrorContains(t, ValidateFieldValue(def, raw(t, "true")), "expected a boolean value", "string is not a JSON boolean")
}

func TestValidateFieldValue_Date(t *testing.T) {
	def := models.FieldDefinition{Key: "anniversary", Type: models.FieldTypeDate}

	assert.NoError(t, ValidateFieldValue(def, raw(t, "2024-06-01")))
	assert.NoError(t, ValidateFieldValue(def, raw(t, "--06-01")), "year-optional partial date")
	assert.ErrorContains(t, ValidateFieldValue(def, raw(t, "June 1st")), `invalid date "June 1st"`, "not ISO 8601")
}

func TestValidateFieldValue_DateTime(t *testing.T) {
	def := models.FieldDefinition{Key: "last_contacted", Type: models.FieldTypeDateTime}

	assert.NoError(t, ValidateFieldValue(def, raw(t, "2024-06-01T12:00:00Z")))
	assert.ErrorContains(t, ValidateFieldValue(def, raw(t, "2024-06-01")), `invalid datetime "2024-06-01"`, "not RFC3339")
}

func TestValidateFieldValue_URI(t *testing.T) {
	def := models.FieldDefinition{Key: "profile_url", Type: models.FieldTypeURI}

	assert.NoError(t, ValidateFieldValue(def, raw(t, "https://example.com")))
	assert.ErrorContains(t, ValidateFieldValue(def, raw(t, "javascript:alert(1)")), "invalid uri", "unsafe scheme rejected")
}

func TestValidateFieldValue_Email(t *testing.T) {
	def := models.FieldDefinition{Key: "alt_email", Type: models.FieldTypeEmail}

	assert.NoError(t, ValidateFieldValue(def, raw(t, "a@example.com")))
	assert.ErrorContains(t, ValidateFieldValue(def, raw(t, "not-an-email")), `invalid email "not-an-email"`)
}

func TestValidateFieldValue_Phone(t *testing.T) {
	def := models.FieldDefinition{Key: "fax", Type: models.FieldTypePhone}

	assert.NoError(t, ValidateFieldValue(def, raw(t, "+12025551234")))
	assert.ErrorContains(t, ValidateFieldValue(def, raw(t, "x")), `invalid phone "x"`, "too short to be a phone number")
}

func TestValidateFieldValue_Enum(t *testing.T) {
	def := models.FieldDefinition{Key: "size", Type: models.FieldTypeEnum, Constraints: models.FieldConstraints{Values: []string{"S", "M", "L"}}}

	assert.NoError(t, ValidateFieldValue(def, raw(t, "M")))
	assert.ErrorContains(t, ValidateFieldValue(def, raw(t, "XL")), `"XL" is not one of the allowed values`, "not in the allowed values")
}

func TestValidateFieldValue_EnumWithNoAllowedValuesConfigured(t *testing.T) {
	def := models.FieldDefinition{Key: "size", Type: models.FieldTypeEnum}
	assert.ErrorContains(t, ValidateFieldValue(def, raw(t, "M")), "enum field has no allowed values configured", "misconfigured definition (no Values) must not silently pass")
}

// list<T>: Constraints.Multi turns any scalar type into a
// JSON-array-of-that-type, each element validated independently.
func TestValidateFieldValue_MultiValuedEnum(t *testing.T) {
	def := models.FieldDefinition{
		Key: "pronouns", Type: models.FieldTypeEnum,
		Constraints: models.FieldConstraints{Values: []string{"she/her", "he/him", "they/them"}, Multi: true},
	}

	assert.NoError(t, ValidateFieldValue(def, raw(t, []string{"she/her", "they/them"})))
	assert.ErrorContains(t, ValidateFieldValue(def, raw(t, []string{"she/her", "xyz"})), `item 1: "xyz" is not one of the allowed values`, "one invalid element fails the whole list")
	assert.ErrorContains(t, ValidateFieldValue(def, raw(t, "she/her")), "expected a list value", "a scalar is not a valid list value")
}

func TestValidateFieldValue_UnknownType(t *testing.T) {
	def := models.FieldDefinition{Key: "mystery", Type: "not-a-real-type"}
	assert.ErrorContains(t, ValidateFieldValue(def, raw(t, "anything")), `unknown field type "not-a-real-type"`)
}
