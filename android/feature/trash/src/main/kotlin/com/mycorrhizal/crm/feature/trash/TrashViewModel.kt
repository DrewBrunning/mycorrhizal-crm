package com.mycorrhizal.crm.feature.trash

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.TrashRepository
import com.mycorrhizal.crm.model.network.TrashedContact
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.toApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.Job
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** The trash is small and bounded by the retention window; walk pages of this size. */
private const val PAGE_SIZE = 100
private const val MAX_PAGES = 50
private const val HTTP_CONFLICT = 409

data class TrashUiState(
    val items: List<TrashedContact> = emptyList(),
    /** DELETED_RETENTION_DAYS; 0 = trashed contacts are kept until the trash is emptied. */
    val retentionDays: Int = 0,
    val isLoading: Boolean = false,
    /** List-load failure; cleared at the start of every load. */
    val error: ApiError? = null,
    /** A restore / purge / empty is in flight; further actions are ignored. */
    val isWorking: Boolean = false
)

sealed interface TrashUiEvent {
    /** [lostRows] is how many join rows the server could not bring back (0 = a full restore). */
    data class Restored(val name: String, val lostRows: Int) : TrashUiEvent

    /** 409: a live contact already uses this contact's UID; nothing changed. */
    data class RestoreConflict(val name: String) : TrashUiEvent

    data class Purged(val name: String) : TrashUiEvent

    data class Emptied(val count: Int) : TrashUiEvent

    data class Failed(val error: ApiError) : TrashUiEvent
}

/** The failure of a [Result] that is known to have failed, as an [ApiError]. */
private fun Result<*>.apiError(): ApiError =
    (exceptionOrNull() ?: IllegalStateException("no failure recorded")).toApiError()

/** Drives the contact Trash screen (issue #2002, mirroring web's useTrash / TrashPage). */
@HiltViewModel
class TrashViewModel @Inject constructor(
    private val trashRepository: TrashRepository,
    private val contactRepository: ContactRepository
) : ViewModel() {

    private val _uiState = MutableStateFlow(TrashUiState())
    val uiState: StateFlow<TrashUiState> = _uiState.asStateFlow()

    private val _events = Channel<TrashUiEvent>(Channel.BUFFERED)
    val events = _events.receiveAsFlow()

    init {
        load()
    }

    private var loadJob: Job? = null

    /**
     * Refetch on screen resume. The drawer restores the saved back-stack entry, so this view
     * model outlives leaving the screen and its list goes stale; the first resume lands while
     * the init load is still running and is skipped.
     */
    fun reload() {
        if (loadJob?.isActive != true) load()
    }

    fun load() {
        loadJob = viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            val all = mutableListOf<TrashedContact>()
            var retention = _uiState.value.retentionDays
            var page = 1
            var failure: ApiError? = null
            while (page <= MAX_PAGES) {
                val result = trashRepository.list(page = page, limit = PAGE_SIZE)
                val response = result.getOrNull()
                if (response == null) {
                    failure = result.apiError()
                    break
                }
                all += response.contacts
                retention = response.retentionDays
                if (all.size >= response.total || response.contacts.isEmpty()) break
                page += 1
            }
            if (failure != null) {
                _uiState.update { it.copy(isLoading = false, error = failure) }
            } else {
                _uiState.update {
                    it.copy(isLoading = false, items = all, retentionDays = retention)
                }
            }
        }
    }

    fun restore(item: TrashedContact) = runAction {
        val name = item.contact.displayName
        val result = trashRepository.restore(item.contact.id)
        val response = result.getOrNull()
        if (response != null) {
            removeItem(item)
            // The restore bumped the contact's revision, so the change feed
            // returns it; pull it into the local cache now so it shows up in
            // the list without waiting for the next background sync.
            contactRepository.syncContacts()
            _events.send(TrashUiEvent.Restored(name, response.skipped.sumOf { it.count }))
        } else {
            val error = result.apiError()
            val conflict = error is ApiError.Client && error.code == HTTP_CONFLICT
            _events.send(
                if (conflict) {
                    TrashUiEvent.RestoreConflict(name)
                } else {
                    TrashUiEvent.Failed(error)
                }
            )
        }
    }

    fun purge(item: TrashedContact) = runAction {
        val result = trashRepository.purge(item.contact.id)
        if (result.isSuccess) {
            removeItem(item)
            _events.send(TrashUiEvent.Purged(item.contact.displayName))
        } else {
            _events.send(TrashUiEvent.Failed(result.apiError()))
        }
    }

    fun emptyTrash() = runAction {
        val result = trashRepository.empty()
        val count = result.getOrNull()
        if (count != null) {
            _uiState.update { it.copy(items = emptyList()) }
            _events.send(TrashUiEvent.Emptied(count))
        } else {
            _events.send(TrashUiEvent.Failed(result.apiError()))
        }
    }

    private fun removeItem(item: TrashedContact) {
        _uiState.update { state ->
            state.copy(items = state.items.filterNot { it.contact.id == item.contact.id })
        }
    }

    /**
     * Runs one mutation at a time. The flag is set synchronously (not inside the
     * coroutine) so a double tap can never launch a second request.
     */
    private fun runAction(block: suspend () -> Unit) {
        if (_uiState.value.isWorking) return
        _uiState.update { it.copy(isWorking = true) }
        viewModelScope.launch {
            try {
                block()
            } finally {
                _uiState.update { it.copy(isWorking = false) }
            }
        }
    }
}
