package a

import apperrors "mycorrhizal/errors"

func positives() {
	_ = apperrors.ErrInvalidInput("f", "r")            // want `ErrInvalidInput without \.WithReason`
	_ = apperrors.ErrConflict("x").WithDetails("a", 1) // want `ErrConflict without \.WithReason`
	_ = apperrors.ReasonCode("free")                   // want `ReasonCode\("\.\.\."\) conversion`
}

func negatives() {
	_ = apperrors.ErrInvalidInput("f", "r").WithReason(apperrors.ReasonX, nil)
	_ = apperrors.ErrConflict("x").WithDetails("a", 1).WithReason(apperrors.ReasonX, nil)
	_ = apperrors.ErrDatabase("x")
	_ = apperrors.ReasonX
}
