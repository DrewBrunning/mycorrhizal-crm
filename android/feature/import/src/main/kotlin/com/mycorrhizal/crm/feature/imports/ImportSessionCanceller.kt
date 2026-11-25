package com.mycorrhizal.crm.feature.imports

import com.mycorrhizal.crm.network.ApiClient
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Issue #2664 / #2734: discards an unconfirmed CSV / VCF / records import session on the server
 * when the user leaves the flow (Cancel, Close, Back, a popped destination), so abandoned
 * sessions do not pile up toward the per-user cap.
 *
 * Fire-and-forget by design: it never blocks the UI and a failed call is swallowed (the server
 * also expires and evicts sessions). The caller passes the [CoroutineScope] to launch on, so a
 * test can supply its own; the call runs [NonCancellable] because a ViewModel's own scope is
 * already cancelled by the time `onCleared` runs.
 *
 * Once a confirm has started the session is no longer "unconfirmed" (the server may have run it),
 * so nothing is sent, unless the server definitively refused the confirm ([confirmRefused]).
 */
internal class ImportSessionCanceller(private val apiClient: ApiClient) {

    private var settled = false

    /** A confirm is in flight or has run: do not discard the session. */
    fun confirmStarted() {
        settled = true
    }

    /** The server answered the confirm with a 4xx: it did not run, so the session is open again. */
    fun confirmRefused() {
        settled = false
    }

    /** Sends the cancel at most once per settled session; no-op for a null or blank id. */
    fun cancel(scope: CoroutineScope, sessionId: String?) {
        if (settled || sessionId.isNullOrBlank()) return
        settled = true
        scope.launch(start = CoroutineStart.ATOMIC) {
            withContext(NonCancellable) {
                runCatching { apiClient.cancelImportSession(sessionId) }
            }
        }
    }
}
