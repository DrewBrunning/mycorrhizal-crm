package middleware

import (
	"testing"

	apperrors "mycorrhizal/errors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2105: every validator tag formatValidationError knows maps to one
// registered reason code, and the default branch maps to validation_invalid.
func TestValidationReasonCoversEveryTag(t *testing.T) {
	field := "Name"
	tests := []struct {
		tag    string
		param  string
		want   apperrors.ReasonCode
		params map[string]any
	}{
		{"required", "", apperrors.ReasonValidationRequired, map[string]any{"field": field}},
		{"email", "", apperrors.ReasonValidationEmail, map[string]any{"field": field}},
		{"min", "3", apperrors.ReasonValidationMin, map[string]any{"field": field, "min": "3"}},
		{"max", "5", apperrors.ReasonValidationMax, map[string]any{"field": field, "max": "5"}},
		{"phone", "", apperrors.ReasonValidationPhone, map[string]any{"field": field}},
		{"birthday", "", apperrors.ReasonValidationBirthday, map[string]any{"field": field}},
		{"strong_password", "", apperrors.ReasonValidationStrongPassword, map[string]any{"field": field}},
		{"unique_circles", "", apperrors.ReasonValidationUniqueCircles, map[string]any{"field": field}},
		{"no_at_sign", "", apperrors.ReasonValidationNoAtSign, map[string]any{"field": field}},
		{"url", "", apperrors.ReasonValidationURL, map[string]any{"field": field}},
		{"safeurl", "", apperrors.ReasonValidationSafeURL, map[string]any{"field": field}},
		{"httpurl", "", apperrors.ReasonValidationHTTPURL, map[string]any{"field": field}},
		{"relation_type", "", apperrors.ReasonValidationRelationType, map[string]any{"field": field}},
		{"geouri", "", apperrors.ReasonValidationGeoURI, map[string]any{"field": field}},
		{"oneof", "a b", apperrors.ReasonValidationInvalid, map[string]any{"field": field}},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			code, params := validationReason(tt.tag, tt.param, field)
			assert.Equal(t, tt.want, code)
			assert.Equal(t, tt.params, params)
		})
	}
}

func TestValidateStructAttachesReasonCodes(t *testing.T) {
	type fixture struct {
		Name  string `json:"name" validate:"required"`
		Email string `json:"email" validate:"email"`
		Code  string `json:"code" validate:"min=3"`
		Tag   string `json:"tag" validate:"max=2"`
	}

	verrs := ValidateStruct(&fixture{
		Name:  "",
		Email: "not-an-email",
		Code:  "ab",
		Tag:   "toolong",
	})
	require.Len(t, verrs, 4)

	assert.Equal(t, "Name is required", verrs[0].Message)
	assert.Equal(t, apperrors.ReasonValidationRequired, verrs[0].ReasonCode)
	assert.Equal(t, map[string]any{"field": "Name"}, verrs[0].ReasonParams)

	assert.Equal(t, "Email must be a valid email address", verrs[1].Message)
	assert.Equal(t, apperrors.ReasonValidationEmail, verrs[1].ReasonCode)

	assert.Equal(t, "Code must be at least 3 characters", verrs[2].Message)
	assert.Equal(t, apperrors.ReasonValidationMin, verrs[2].ReasonCode)
	assert.Equal(t, map[string]any{"field": "Code", "min": "3"}, verrs[2].ReasonParams)

	assert.Equal(t, "Tag must be at most 2 characters", verrs[3].Message)
	assert.Equal(t, apperrors.ReasonValidationMax, verrs[3].ReasonCode)
	assert.Equal(t, map[string]any{"field": "Tag", "max": "2"}, verrs[3].ReasonParams)
}
