package errors

import "sync/atomic"

// ReasonObserver sees every reason code and params an error is given: the
// top-level reason (WithReason and constructor defaults) and each per-field
// reason of a validation error.
type ReasonObserver func(code ReasonCode, params map[string]any)

var reasonObserver atomic.Pointer[ReasonObserver]

// ObserveReasons installs fn for every reason attached from now on and returns
// a func that restores the previous observer. It exists for whole-package test
// guards (issue #2920: the reason-param prose check runs from TestMain so every
// failure path a suite exercises is checked, not only the ones a test author
// remembered to assert). Production code never installs one.
func ObserveReasons(fn ReasonObserver) (restore func()) {
	prev := reasonObserver.Swap(&fn)
	return func() { reasonObserver.Store(prev) }
}

func observeReason(code ReasonCode, params map[string]any) {
	if fn := reasonObserver.Load(); fn != nil && *fn != nil {
		(*fn)(code, params)
	}
}
