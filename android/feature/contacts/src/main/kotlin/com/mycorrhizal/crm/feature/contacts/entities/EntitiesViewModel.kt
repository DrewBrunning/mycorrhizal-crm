package com.mycorrhizal.crm.feature.contacts.entities

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.EntityRepository
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityInput
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.EntitySuggestionPair
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
    val error: ApiError? = null,
    val isSaving: Boolean = false,
    val formError: ApiError? = null,
    /** #2499: null until "Find duplicates" is pressed; then the pairs of [kind] still shown. */
    val suggestions: List<EntitySuggestionPair>? = null,
    val suggestionsLoading: Boolean = false,
    val suggestionsError: ApiError? = null
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
    private var loadedOnce = false

    // Same guard for the duplicate scan: a scan of a tab the user has left is dropped.
    private var suggestionsGeneration = 0

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
                        loadedOnce = true
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

    /** Back on this screen (e.g. after renaming on the detail page): re-read the list. */
    fun onResumed() {
        if (loadedOnce && !_uiState.value.isLoading) refresh()
    }

    fun selectKind(kind: String) {
        if (kind == _uiState.value.kind) return
        searchJob?.cancel()
        suggestionsGeneration++
        _uiState.update {
            it.copy(
                kind = kind,
                query = "",
                entities = emptyList(),
                nextCursor = null,
                suggestions = null,
                suggestionsLoading = false,
                suggestionsError = null
            )
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

    /** Create an entity of the shown kind; [onCreated] runs on success so the form can close. */
    fun create(input: EntityInput, onCreated: (Entity) -> Unit) {
        if (_uiState.value.isSaving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, formError = null) }
            repository.create(input).foldApiError(
                onSuccess = { entity ->
                    _uiState.update {
                        it.copy(
                            isSaving = false,
                            entities = if (entity.kind == it.kind) {
                                (it.entities + entity).sortedBy { e -> e.name.lowercase() }
                            } else {
                                it.entities
                            }
                        )
                    }
                    onCreated(entity)
                },
                onError = { error ->
                    _uiState.update {
                        it.copy(
                            isSaving = false,
                            formError = error
                        )
                    }
                }
            )
        }
    }

    /** Read-only scan for duplicate pairs of the shown kind; nothing merges or hides by itself. */
    fun findDuplicates() {
        val state = _uiState.value
        if (state.suggestionsLoading) return
        val generationAtStart = ++suggestionsGeneration
        viewModelScope.launch {
            _uiState.update { it.copy(suggestionsLoading = true, suggestionsError = null) }
            repository.suggestions(state.kind).foldApiError(
                onSuccess = { response ->
                    if (generationAtStart != suggestionsGeneration) return@foldApiError
                    _uiState.update {
                        it.copy(suggestionsLoading = false, suggestions = response.pairsOrEmpty)
                    }
                },
                onError = { error ->
                    if (generationAtStart != suggestionsGeneration) return@foldApiError
                    _uiState.update {
                        it.copy(suggestionsLoading = false, suggestionsError = error)
                    }
                }
            )
        }
    }

    /** "Not a duplicate": hide the pair for good; it leaves the list only once the server agrees. */
    fun dismissSuggestion(pair: EntitySuggestionPair) {
        viewModelScope.launch {
            repository.dismissSuggestion(pair.a.id, pair.b.id).foldApiError(
                onSuccess = {
                    _uiState.update {
                        it.copy(
                            suggestions = it.suggestions?.filterNot { p -> p == pair },
                            suggestionsError = null
                        )
                    }
                },
                onError = { error -> _uiState.update { it.copy(suggestionsError = error) } }
            )
        }
    }

    fun onFormDismissed() {
        _uiState.update { it.copy(formError = null) }
    }
}
