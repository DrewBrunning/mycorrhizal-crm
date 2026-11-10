package com.mycorrhizal.crm.feature.contacts.entities

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.EntityRepository
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

private const val PAGE_SIZE = 50
private const val SEARCH_DEBOUNCE_MS = 300L

/** Issue #2302: the entity list - one kind tab at a time, name search, cursor paging. */
data class EntitiesUiState(
    val kind: String = EntityKind.ORGANISATION,
    val query: String = "",
    val entities: List<Entity> = emptyList(),
    val nextCursor: String? = null,
    val isLoading: Boolean = false,
    val isLoadingMore: Boolean = false,
    val error: ApiError? = null
)

@HiltViewModel
class EntitiesViewModel @Inject constructor(private val repository: EntityRepository) :
    ViewModel() {

    private val _uiState = MutableStateFlow(EntitiesUiState())
    val uiState: StateFlow<EntitiesUiState> = _uiState.asStateFlow()

    private var searchJob: Job? = null

    // Bumped on every (kind, query) change so a slow response for a previous tab or search can
    // never overwrite the current one (the same guard the web list uses).
    private var generation = 0

    init {
        refresh()
    }

    fun refresh() {
        val generationAtStart = ++generation
        val state = _uiState.value
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, isLoadingMore = false, error = null) }
            repository.list(state.kind, state.query.trim().ifEmpty { null }, null, PAGE_SIZE)
                .foldApiError(
                    onSuccess = { page ->
                        if (generationAtStart != generation) return@foldApiError
                        _uiState.update {
                            it.copy(
                                isLoading = false,
                                entities = page.entitiesOrEmpty,
                                nextCursor = page.nextCursor?.takeIf { c -> c.isNotEmpty() }
                            )
                        }
                    },
                    onError = { error ->
                        if (generationAtStart != generation) return@foldApiError
                        _uiState.update { it.copy(isLoading = false, error = error) }
                    }
                )
        }
    }

    fun selectKind(kind: String) {
        if (kind == _uiState.value.kind) return
        searchJob?.cancel()
        _uiState.update {
            it.copy(kind = kind, query = "", entities = emptyList(), nextCursor = null)
        }
        refresh()
    }

    fun onQueryChange(query: String) {
        _uiState.update { it.copy(query = query) }
        searchJob?.cancel()
        searchJob = viewModelScope.launch {
            delay(SEARCH_DEBOUNCE_MS)
            refresh()
        }
    }

    fun loadMore() {
        val state = _uiState.value
        val cursor = state.nextCursor ?: return
        if (state.isLoading || state.isLoadingMore) return
        val generationAtStart = generation
        viewModelScope.launch {
            _uiState.update { it.copy(isLoadingMore = true) }
            repository.list(state.kind, state.query.trim().ifEmpty { null }, cursor, PAGE_SIZE)
                .foldApiError(
                    onSuccess = { page ->
                        if (generationAtStart != generation) return@foldApiError
                        _uiState.update {
                            it.copy(
                                isLoadingMore = false,
                                entities = it.entities + page.entitiesOrEmpty,
                                nextCursor = page.nextCursor?.takeIf { c -> c.isNotEmpty() }
                            )
                        }
                    },
                    onError = { error ->
                        if (generationAtStart != generation) return@foldApiError
                        _uiState.update { it.copy(isLoadingMore = false, error = error) }
                    }
                )
        }
    }
}
