// Package errors is a stub of mycorrhizal/errors for the reasonprose analyzer tests.
package errors

type ReasonCode string

const (
	ReasonX ReasonCode = "common_x"
	ReasonY ReasonCode = "common_y"
)

type AppError struct{}

func (e *AppError) WithReason(c ReasonCode, p map[string]any) *AppError { return e }

func ErrInvalidInput(field, reason string) *AppError { return &AppError{} }
