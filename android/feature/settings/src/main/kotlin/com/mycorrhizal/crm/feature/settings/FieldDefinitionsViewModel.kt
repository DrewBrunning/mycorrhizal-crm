package com.mycorrhizal.crm.feature.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.FieldDefinitionRepository
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** Issue #830: Settings → Data → Custom fields, the list half. Mirrors `TagsViewModel`'s shape. */
data class FieldDefinitionsUiState(
    val definitions: List<FieldDefinition> = emptyList(),
    val isLoading: Boolean = false,
    val error: String? = null,
    val isReordering: Boolean = false
)

@HiltViewModel
class FieldDefinitionsViewModel @Inject constructor(
    private val repository: FieldDefinitionRepository
) : ViewModel() {

    private val _uiState = MutableStateFlow(FieldDefinitionsUiState())
    val uiState: StateFlow<FieldDefinitionsUiState> = _uiState.asStateFlow()

    fun load() {
        if (_uiState.value.isLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            repository.list().foldApiError(
                onSuccess = { definitions ->
                    _uiState.update { it.copy(isLoading = false, definitions = definitions) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                }
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }

    /** Move one step (-1 = up, +1 = down) — the TalkBack custom-action path. */
    fun move(id: String, direction: Int) = moveBy(id, direction)

    /**
     * Move [id] by [delta] positions and persist the full resulting order in one call
     * (issue #1210). This is what a completed drag calls with the number of rows crossed;
     * [move] is the one-step custom-action form. A clamped (boundary) move is a no-op, and a
     * reorder already in flight blocks another so two overlapping full-set calls can't race.
     */
    fun moveBy(id: String, delta: Int) {
        if (delta == 0 || _uiState.value.isReordering) return
        val ids = _uiState.value.definitions.map { it.id }.toMutableList()
        val index = ids.indexOf(id)
        if (index == -1) return
        val target = (index + delta).coerceIn(0, ids.lastIndex)
        if (target == index) return
        ids.removeAt(index)
        ids.add(target, id)

        viewModelScope.launch {
            _uiState.update { it.copy(isReordering = true, error = null) }
            repository.reorder(ids).foldApiError(
                onSuccess = { definitions ->
                    _uiState.update { it.copy(isReordering = false, definitions = definitions) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isReordering = false, error = error.displayMessage) }
                }
            )
        }
    }
}
