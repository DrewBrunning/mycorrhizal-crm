package com.mycorrhizal.crm.feature.shares

import com.mycorrhizal.crm.domain.repository.ContactShareRepository
import com.mycorrhizal.crm.model.network.UserDirectoryEntry
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/** SHARE_DIRECTORY_MODE=search needs at least this many characters of the username. */
internal const val SHARE_SEARCH_MIN_CHARS = 3

private const val SEARCH_DEBOUNCE_MS = 250L

/** What the recipient picker shows; mirrored into a view model's UI state. */
internal data class RecipientSearchState(
    /** The raw text in the field (search mode only). */
    val query: String = "",
    val recipients: List<UserDirectoryEntry> = emptyList(),
    /** Inferred, never reported by the server; see [RecipientDirectorySearch]. */
    val searchMode: Boolean = false,
    val isLoading: Boolean = false,
    /** True once the newest request for the current query has answered (hint vs "no match"). */
    val settled: Boolean = false
)

/**
 * Drives the share recipient pickers against `GET /users/directory?q=` (issues #2787 / #2658),
 * shared by the one-time/Keep-sharing screen and the standing-share screen.
 *
 * The response does not say which SHARE_DIRECTORY_MODE the operator chose, so it is inferred: a
 * query shorter than [SHARE_SEARCH_MIN_CHARS] (the initial, empty one included) that returns no
 * users means `search` mode. In `all` mode (the default) the server ignores `q` and lists
 * everyone, so the picker behaves exactly as before and typing never sends a request. Once search
 * mode is known a query that is too short is never sent. Only the newest request is applied.
 */
internal class RecipientDirectorySearch(
    private val scope: CoroutineScope,
    private val repository: ContactShareRepository,
    private val onState: (RecipientSearchState) -> Unit,
    private val onError: (ApiError?) -> Unit
) {
    private var state = RecipientSearchState()
    private var job: Job? = null

    /** Bumped for every request; an answer for an older number is dropped. */
    private var generation = 0

    /** First load and Retry. In search mode it re-asks only for a query long enough to send. */
    fun load() {
        val q = state.query.trim()
        if (state.searchMode && q.length < SHARE_SEARCH_MIN_CHARS) {
            job?.cancel()
            generation++
            publish(state.copy(recipients = emptyList(), isLoading = false, settled = true))
            return
        }
        request(if (state.searchMode) q else "", debounce = false)
    }

    /** The text typed into the field. Only meaningful once search mode is known. */
    fun setQuery(raw: String) {
        if (!state.searchMode) {
            // `all` mode lists everyone and ignores q; the field is not editable there.
            return
        }
        val q = raw.trim()
        job?.cancel()
        generation++
        if (q.length < SHARE_SEARCH_MIN_CHARS) {
            publish(
                state.copy(query = raw, recipients = emptyList(), isLoading = false, settled = true)
            )
            return
        }
        publish(state.copy(query = raw, isLoading = false, settled = false))
        request(q, debounce = true)
    }

    /** Displays [text] in the field without searching (a recipient was just picked). */
    fun showQuery(text: String) {
        publish(state.copy(query = text))
    }

    private fun request(q: String, debounce: Boolean) {
        job?.cancel()
        val mine = ++generation
        job = scope.launch {
            if (debounce) delay(SEARCH_DEBOUNCE_MS)
            if (mine != generation) return@launch
            onError(null)
            repository.userDirectory(q).foldApiError(
                onSuccess = { users ->
                    if (mine != generation) return@foldApiError
                    val inferred = q.length < SHARE_SEARCH_MIN_CHARS && users.isEmpty()
                    publish(
                        state.copy(
                            recipients = users,
                            searchMode = state.searchMode || inferred,
                            isLoading = false,
                            settled = true
                        )
                    )
                },
                onError = { error ->
                    if (mine != generation) return@foldApiError
                    publish(state.copy(recipients = emptyList(), isLoading = false, settled = true))
                    onError(error)
                }
            )
        }
    }

    private fun publish(next: RecipientSearchState) {
        state = next
        onState(next)
    }
}
