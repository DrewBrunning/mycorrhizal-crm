package com.mycorrhizal.crm.feature.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.FieldDefinitionRepository
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/** Issue #830: Settings → Data → Custom fields, the list half. Mirrors `TagsViewModel`'s shape. */
data class FieldDefinitionsUiState(
    val definitions: List<FieldDefinition> = emptyList(),
    val isLoading: Boolean = false,
    val error: String? = null,
    val deletingId: String? = null,
)

@HiltViewModel
class FieldDefinitionsViewModel @Inject constructor(
    private val repository: FieldDefinitionRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(FieldDefinitionsUiState())
    val uiState: StateFlow<FieldDefinitionsUiState> = _uiState.asStateFlow()

    init {
        load()
    }

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
                },
            )
        }
    }

    fun delete(id: String) {
        if (_uiState.value.deletingId != null) return
        viewModelScope.launch {
            _uiState.update { it.copy(deletingId = id, error = null) }
            repository.delete(id).foldApiError(
                onSuccess = {
                    _uiState.update { state ->
                        state.copy(deletingId = null, definitions = state.definitions.filterNot { it.id == id })
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(deletingId = null, error = error.displayMessage) }
                },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}
