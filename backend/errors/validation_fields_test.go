package errors

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2105: ErrValidationFields is additive — details stays a field ->
// English message map, and field_reasons carries the translatable reason.
func TestErrValidationFieldsKeepsDetailsAndAddsFieldReasons(t *testing.T) {
	appErr := ErrValidationFields([]ValidationError{
		{
			Field:        "Email",
			Message:      "Email must be a valid email address",
			ReasonCode:   ReasonValidationEmail,
			ReasonParams: map[string]any{"field": "Email"},
		},
		{
			Field:        "Legacy",
			Message:      "Legacy is invalid",
			ReasonCode:   ReasonValidationInvalid,
			ReasonParams: map[string]any{"field": "Legacy"},
		},
		{
			// A caller with no registered reason keeps the details entry but
			// is omitted from field_reasons.
			Field:   "NoReason",
			Message: "NoReason is invalid",
		},
	})

	assert.Equal(t, ErrCodeValidation, appErr.Code)
	assert.Equal(t, http.StatusBadRequest, appErr.HTTPStatus)
	assert.Equal(t, "Request validation failed", appErr.Message)
	assert.Equal(t, map[string]interface{}{
		"Email":    "Email must be a valid email address",
		"Legacy":   "Legacy is invalid",
		"NoReason": "NoReason is invalid",
	}, appErr.Details)

	require.Len(t, appErr.FieldReasons, 2)
	assert.Equal(t, FieldReason{
		ReasonCode:   ReasonValidationEmail,
		ReasonParams: map[string]any{"field": "Email"},
	}, appErr.FieldReasons["Email"])
	assert.Equal(t, ReasonValidationInvalid, appErr.FieldReasons["Legacy"].ReasonCode)
	assert.NotContains(t, appErr.FieldReasons, "NoReason")
}

func TestRespondWithError_FieldReasonsEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/x", nil)

	RespondWithError(c, ErrValidationFields([]ValidationError{
		{
			Field:        "Name",
			Message:      "Name is required",
			ReasonCode:   ReasonValidationRequired,
			ReasonParams: map[string]any{"field": "Name"},
		},
	}))

	detail := decodeErrorDetail(t, decodeErrorEnvelope(t, rec))
	assert.Equal(t, ErrCodeValidation, detail["code"])
	assert.Equal(t, map[string]interface{}{"Name": "Name is required"}, detail["details"])
	fieldReasons, ok := detail["field_reasons"].(map[string]interface{})
	require.True(t, ok, "field_reasons must be an object, got %T", detail["field_reasons"])
	name, ok := fieldReasons["Name"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "validation_required", name["reason_code"])
	assert.Equal(t, map[string]interface{}{"field": "Name"}, name["reason_params"])
}
