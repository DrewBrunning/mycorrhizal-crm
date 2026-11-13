package com.mycorrhizal.crm.feature.shares

import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow

/**
 * Process-wide "a share grant changed" signal (issue #2557). The recipient screen emits it after
 * every accept, decline, block, leave or unblock; the drawer's pending-share badge listens and
 * re-reads its count, so the badge does not wait for the drawer to reopen or the route to change.
 * Carries no payload: the listener re-reads the server's count.
 */
@Singleton
class ShareGrantChanges @Inject constructor() {
    private val _changes = MutableSharedFlow<Unit>(
        extraBufferCapacity = 1,
        onBufferOverflow = BufferOverflow.DROP_OLDEST
    )
    val changes: SharedFlow<Unit> = _changes.asSharedFlow()

    fun notifyChanged() {
        _changes.tryEmit(Unit)
    }
}
