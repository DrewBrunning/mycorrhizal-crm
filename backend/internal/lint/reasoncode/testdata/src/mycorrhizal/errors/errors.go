// Package errors is a stub of mycorrhizal/errors for the analyzer tests.
package errors

type ReasonCode string

const ReasonX ReasonCode = "common_x"

type AppError struct{}

func (e *AppError) WithDetails(k string, v any) *AppError               { return e }
func (e *AppError) WithReason(c ReasonCode, p map[string]any) *AppError { return e }

func ErrInvalidInput(field, reason string) *AppError { return &AppError{} }
func ErrConflict(message string) *AppError           { return &AppError{} }
func ErrDatabase(op string) *AppError                { return &AppError{} }

// Inside the errors package nothing is reported.
func inside() *AppError { return ErrConflict("x") }
