// Package runfixture is a compiling fixture for the Run-level allowlist tests
// (issue #2098). It lives under testdata so the real ./... pass never sees it.
package runfixture

import apperrors "mycorrhizal/errors"

func Unmigrated() *apperrors.AppError {
	return apperrors.ErrConflict("x")
}

func Migrated() *apperrors.AppError {
	return apperrors.ErrConflict("x").WithReason(apperrors.ReasonCommonTestOnly, map[string]any{"n": 1})
}

func AllConstructors() {
	_ = apperrors.ErrInvalidInput("f", "r")
	_ = apperrors.ErrConflict("x").WithDetails("a", 1).WithReason(apperrors.ReasonCommonTestOnly, map[string]any{"n": 1})
	_ = apperrors.ReasonCode("free")
}
