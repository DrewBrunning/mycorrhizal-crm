package a

import (
	"errors"
	"fmt"

	apperrors "mycorrhizal/errors"
)

type other struct{}

// WithReason on a type outside mycorrhizal/errors is not the reason API.
func (other) WithReason(c string, p map[string]any) {}

func cases(err error, name string, n int) {
	e := apperrors.ErrInvalidInput("f", "r")

	// Reported: the three prose sources.
	e.WithReason(apperrors.ReasonX, map[string]any{"reason": err.Error()})                                    // want `common_x/reason: err.Error\(\) passes the error chain's English text through`
	e.WithReason(apperrors.ReasonX, map[string]any{"reason": "The server could not be reached"})              // want `common_x/reason: constant "The server could not be reached" is prose`
	e.WithReason(apperrors.ReasonY, map[string]any{"detail": fmt.Sprintf("could not reach %s at all", name)}) // want `common_y/detail: fmt.Sprintf\("could not reach %s at all", ...\) builds prose`
	e.WithReason(apperrors.ReasonY, map[string]any{"detail": errors.New("wrapped failure").Error()})          // want `common_y/detail: err.Error\(\)`
	e.WithReason(apperrors.ReasonY, map[string]any{"detail": name + " is over the account limit"})            // want `common_y/detail: concatenation with " is over the account limit" builds prose`
	e.WithReason(apperrors.ReasonY, map[string]any{"detail": "prefix: " + err.Error()})                       // want `common_y/detail: err.Error\(\)`
	const long = "this constant reads as a sentence"
	e.WithReason(apperrors.ReasonX, map[string]any{"reason": long}) // want `common_x/reason: constant`

	// Not reported: data the template interpolates.
	e.WithReason(apperrors.ReasonX, map[string]any{"reason": name})
	e.WithReason(apperrors.ReasonX, map[string]any{"reason": name + ", " + name})
	e.WithReason(apperrors.ReasonX, map[string]any{"reason": "Work email"})
	e.WithReason(apperrors.ReasonX, map[string]any{"count": n})
	e.WithReason(apperrors.ReasonX, map[string]any{"reason": fmt.Sprintf("%d items", n)})
	e.WithReason(apperrors.ReasonX, nil)
	other{}.WithReason("common_x", map[string]any{"reason": err.Error()})
	// A key that is not a constant cannot be matched to an allowlist entry or
	// a template slot; it is left to the runtime guard.
	key := "reason"
	e.WithReason(apperrors.ReasonX, map[string]any{key: "The server could not be reached"})
}
