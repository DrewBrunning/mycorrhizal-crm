package controllers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// The three password handlers defend against a missing or mistyped validated
// body even though the validation middleware normally prevents it (issue
// #2105). Call them directly so both guard branches are exercised.
func TestPasswordHandlersGuardValidatedInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name    string
		handler func(*gin.Context, *config.Config)
	}{
		{"request", RequestPasswordReset},
		{"confirm", ConfirmPasswordReset},
		{"change", ChangePassword},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, step := range []struct {
				name      string
				validated any
				set       bool
				reason    string
			}{
				{"missing", nil, false, "validation_data_not_found"},
				{"wrong type", "not-a-pointer", true, "validation_data_type_invalid"},
			} {
				t.Run(step.name, func(t *testing.T) {
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
					if step.set {
						c.Set("validated", step.validated)
					}
					tc.handler(c, &config.Config{})
					assert.Equal(t, http.StatusBadRequest, rec.Code)
					code, _ := wireReason(t, rec)
					assert.Equal(t, step.reason, code)
				})
			}
		})
	}
}
