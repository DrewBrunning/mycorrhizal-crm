package a

import apperrors "mycorrhizal/errors"

// Test files build fixtures, not production responses: never reported.
func testFixture() {
	apperrors.ErrInvalidInput("f", "r").WithReason(apperrors.ReasonX, map[string]any{"reason": "a long fixture sentence here"})
}
