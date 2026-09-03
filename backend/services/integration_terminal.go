package services

import (
	"errors"

	"mycorrhizal/integrations"
)

// classifySyncFailure decides whether a finished sync run's error is
// permanent-until-human (INT-04, issue #467) and, if so, returns the
// integrations.FailureMode slug behind it — the value stored in
// SyncHealthFields.TerminalReason and mapped by the frontend to an actionable
// message. "" means the failure is transient (or err is nil): the sync stays
// due and the next run retries.
//
// The judgment is not forked from integrations.Dispositions(): the two slugs
// returned here are exactly the two Persistence == PermanentUntilHuman modes a
// sync can actually hit (auth revoked, remote collection deleted), and
// TestSyncTerminalReasonsAreDispositionPermanent pins that.
func classifySyncFailure(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrCalendarUnauthorized), errors.Is(err, ErrContactSyncUnauthorized):
		return string(integrations.FailureAuthExpiry)
	case errors.Is(err, ErrCalendarNotFound), errors.Is(err, ErrContactSyncNotFound):
		return string(integrations.FailureRemoteResourceDeleted)
	default:
		// Unreachable host, timeout, malformed response, too-large, invalid
		// URL, private-address block — all transient or caller-fixable without
		// a "sync has stopped" banner. Retrying on the next run is correct.
		return ""
	}
}
