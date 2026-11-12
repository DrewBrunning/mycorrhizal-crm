package com.mycorrhizal.crm.feature.trash

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.EntityRepository
import com.mycorrhizal.crm.domain.repository.TrashRepository
import com.mycorrhizal.crm.model.network.TrashedContact
import com.mycorrhizal.crm.model.network.TrashedEntity
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
    /** Deleted organisations, places and pets (issue #2498); restore-only. */
    val entities: List<TrashedEntity> = emptyList(),
    /** DELETED_RETENTION_DAYS; 0 = trashed contacts are kept until the trash is emptied. */
    val retentionDays: Int = 0,
    val isLoading: Boolean = false,
    /** List-load failure; cleared at the start of every load. */
    val error: ApiError? = null,
    /** A restore / purge / empty is in flight; further actions are ignored. */
    val isWorking: Boolean = false
) {
    /** Nothing at all is in the trash: no contact and no entity. */
    val isEmpty: Boolean get() = items.isEmpty() && entities.isEmpty()
}

sealed interface TrashUiEvent {
    /** [lostRows] is how many join rows the server could not bring back (0 = a full restore). */
    data class Restored(val name: String, val lostRows: Int) : TrashUiEvent

    /** 409: a live contact already uses this contact's UID; nothing changed. */
    data class RestoreConflict(val name: String) : TrashUiEvent

    /** An entity came back; [sameNameLive] warns that a live entity already has its name. */
    data class EntityRestored(val name: String, val sameNameLive: Boolean) : TrashUiEvent

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
    private val contactRepository: ContactRepository,
    private val entityRepository: EntityRepository
) : ViewModel() {

    private val _uiState = MutableStateFlow(TrashUiState())
    val uiState: StateFlow<TrashUiState> = _uiState.asStateFlow()

    private val _events = Channel<TrashUiEvent>(Channel.BUFFERED)
    val events = _events.receiveAsFlow()

    private var loadJob: Job? = null

    /**
     * Whether the connected server serves the entity trash (`GET /trash/entities`, ServerFeature
     * ENTITIES, v1.7.0); null until the screen has told us. A v1.6.0 server answers 404 for that
     * route, which would fail the whole screen including the contact trash it does have, so the
     * entity listing is skipped there (issue #2527).
     */
    private var entitiesSupported: Boolean? = null

    /**
     * Starts the first load once the screen knows the server's version. A change of the answer
     * (the version resolves after the screen opened) reloads.
     */
    fun start(entitiesSupported: Boolean) {
        if (this.entitiesSupported == entitiesSupported) return
        this.entitiesSupported = entitiesSupported
        loadJob?.cancel()
        load()
    }

    /**
     * Refetch on screen resume. The drawer restores the saved back-stack entry, so this view
     * model outlives leaving the screen and its list goes stale; the first resume lands while
     * the init load is still running and is skipped.
     */
    fun reload() {
        if (entitiesSupported != null && loadJob?.isActive != true) load()
    }

    fun load() {
        loadJob = viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            val contacts = mutableListOf<TrashedContact>()
            val entities = mutableListOf<TrashedEntity>()
            var retention = _uiState.value.retentionDays
            val failure = walkPages(
                fetch = { page -> trashRepository.list(page = page, limit = PAGE_SIZE) },
                rows = { it.contacts },
                total = { it.total }
            ) { response ->
                contacts += response.contacts
                retention = response.retentionDays
            } ?: if (entitiesSupported == true) {
                walkPages(
                    fetch = { page ->
                        trashRepository.listEntities(
                        page = page,
                        limit = PAGE_SIZE
                    )
                    },
                    rows = { it.entitiesOrEmpty },
                    total = { it.total }
                ) { response -> entities += response.entitiesOrEmpty }
            } else {
                null
            }
            if (failure != null) {
                _uiState.update { it.copy(isLoading = false, error = failure) }
            } else {
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        items = contacts,
                        entities = entities,
                        retentionDays = retention
                    )
                }
            }
        }
    }

    /** Walks a paged list until it is exhausted; returns the first failure, or null. */
    private suspend fun <R> walkPages(
        fetch: suspend (page: Int) -> Result<R>,
        rows: (R) -> List<Any>,
        total: (R) -> Int,
        onPage: (R) -> Unit
    ): ApiError? {
        var seen = 0
        for (page in 1..MAX_PAGES) {
            val result = fetch(page)
            val response = result.getOrNull() ?: return result.apiError()
            onPage(response)
            val count = rows(response).size
            seen += count
            if (seen >= total(response) || count == 0) break
        }
        return null
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

    /** Restores an entity through the entity restore route (links revive with it). */
    fun restoreEntity(item: TrashedEntity) = runAction {
        val result = entityRepository.restore(item.entity.id)
        val response = result.getOrNull()
        if (response != null) {
            _uiState.update { state ->
                state.copy(entities = state.entities.filterNot { it.entity.id == item.entity.id })
            }
            _events.send(TrashUiEvent.EntityRestored(item.entity.name, response.sameNameLive))
        } else {
            _events.send(TrashUiEvent.Failed(result.apiError()))
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
