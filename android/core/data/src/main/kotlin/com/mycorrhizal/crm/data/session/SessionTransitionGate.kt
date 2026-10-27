package com.mycorrhizal.crm.data.session

import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.sync.Mutex

/**
 * Issue #2065: serializes the pending_interactions outbox flush against session
 * identity transitions (switch, logout, profile removal, Local activation).
 *
 * [DefaultSessionManager] holds [mutex] for the whole drain → wipe → swap
 * sequence; the periodic worker's flush takes it for the whole batch. A flush
 * can therefore never run between a switch's drain and its wipe and send an
 * old profile's rows through the new profile's API. The mutex is not reentrant:
 * the drain the manager triggers calls the flusher's lock-free path.
 */
@Singleton
class SessionTransitionGate @Inject constructor() {
    val mutex = Mutex()
}
