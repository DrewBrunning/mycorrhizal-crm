package errors

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2099: the generic constructors carry default reason codes.
func TestConstructorDefaultReasons(t *testing.T) {
	cases := []struct {
		name    string
		err     *AppError
		code    ReasonCode
		params  map[string]any
		message string
	}{
		{"database", ErrDatabase("save"), ReasonCommonDatabaseError, nil, "Something went wrong while saving or loading your data"},
		{"database empty op", ErrDatabase(""), ReasonCommonDatabaseError, nil, "Something went wrong while saving or loading your data"},
		{"database busy", ErrDatabaseBusy(), ReasonCommonDatabaseBusy, nil, "The server is busy. Try again in a moment"},
		{"invalid credentials", ErrInvalidCredentials(), ReasonAuthInvalidCredentials, nil, "Invalid email or password"},
		{"token expired", ErrTokenExpired(), ReasonAuthTokenExpired, nil, "Your session has expired. Sign in again"},
		{"token invalid", ErrTokenInvalid(), ReasonAuthTokenInvalid, nil, "Your session is no longer valid. Sign in again"},
		{"client not supported", ErrClientNotSupported("1.2.3"), ReasonCommonClientNotSupported, map[string]any{"required_version": "1.2.3"}, "This app version is no longer supported. Update to 1.2.3 or later"},
		{"rate limited", ErrRateLimitExceeded(), ReasonCommonRateLimited, nil, "Too many requests. Try again later"},
		{"payload too large default", ErrPayloadTooLarge(""), ReasonCommonPayloadTooLarge, nil, "The upload is too large"},
		{"idempotency reused", ErrIdempotencyKeyReused(), ReasonCommonIdempotencyKeyReused, nil, "Idempotency-Key was already used for a different request; use a fresh key"},
		{"idempotency in progress", ErrIdempotencyInProgress(), ReasonCommonRequestInProgress, nil, "a request with this Idempotency-Key is still being processed; retry shortly"},
		{"idempotency unavailable", ErrIdempotencyResultUnavailable(), ReasonCommonRequestResultUnavailable, nil, "a previous request with this Idempotency-Key completed but its response is not available for replay; do not resubmit it with this key"},
		{"missing field", ErrMissingField("email"), ReasonValidationMissingField, map[string]any{"field": "email"}, "Required field 'email' is missing"},
		{"not found contact", ErrNotFound("Contact"), "contact_not_found", nil, "Contact not found"},
		{"not found several", ErrNotFound("One or more contacts"), "contact_some_not_found", nil, "One or more contacts not found"},
		{"already exists user", ErrAlreadyExists("User"), "account_user_already_exists", nil, "User already exists"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, string(tc.code), tc.err.ReasonCode)
			assert.Equal(t, tc.message, tc.err.Message)
			if tc.params == nil {
				assert.Empty(t, tc.err.ReasonParams)
			} else {
				assert.Equal(t, tc.params, tc.err.ReasonParams)
			}
		})
	}
}

func TestDatabaseKeepsOperationInDetails(t *testing.T) {
	err := ErrDatabase("save")
	assert.Equal(t, "save", err.Details["operation"])
}

func TestConstructorsWithoutDefaultReasons(t *testing.T) {
	notMapped := "Some unmapped thing"
	for name, err := range map[string]*AppError{
		"not found variable":     ErrNotFound(notMapped),
		"not found empty":        ErrNotFound(""),
		"already exists unknown": ErrAlreadyExists(notMapped),
		"internal ordinary":      ErrInternal("x"),
		"payload custom":         ErrPayloadTooLarge("request body too large"),
		"missing field empty":    ErrMissingField(""),
		"client unsupported any": ErrClientNotSupported(""),
	} {
		t.Run(name, func(t *testing.T) {
			assert.Empty(t, err.ReasonCode)
			assert.Empty(t, err.ReasonParams)
		})
	}
}

func TestGetAppErrorFallbackHasReason(t *testing.T) {
	err := GetAppError(assert.AnError)
	assert.Equal(t, "common_unexpected_error", err.ReasonCode)
	assert.Equal(t, "An unexpected error occurred", err.Message)
	assert.Equal(t, http.StatusInternalServerError, err.HTTPStatus)
}

func TestNotFoundAndAlreadyExistsMapsAreRegistered(t *testing.T) {
	require.NotEmpty(t, notFoundReasons)
	require.NotEmpty(t, alreadyExistsReasons)
	for res, code := range notFoundReasons {
		tpl, params, ok := ReasonSpecFor(code)
		require.True(t, ok, "%s: %s not registered", res, code)
		assert.Empty(t, params)
		assert.Equal(t, res+" not found", tpl)
	}
	for res, code := range alreadyExistsReasons {
		tpl, params, ok := ReasonSpecFor(code)
		require.True(t, ok, "%s: %s not registered", res, code)
		assert.Empty(t, params)
		assert.Equal(t, res+" already exists", tpl)
	}
}
