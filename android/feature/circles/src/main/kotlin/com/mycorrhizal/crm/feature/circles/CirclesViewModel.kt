package com.mycorrhizal.crm.feature.circles

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class CirclesUiState(
    val circles: List<Circle> = emptyList(),
    val isLoading: Boolean = false,
    val error: String? = null,
    val isSaving: Boolean = false,
    val deletingId: String? = null,
)

@HiltViewModel
class CirclesViewModel @Inject constructor(
    private val circleRepository: CircleRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(CirclesUiState())
    val uiState: StateFlow<CirclesUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        if (_uiState.value.isLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            circleRepository.list().foldApiError(
                onSuccess = { circles ->
                    _uiState.update { it.copy(isLoading = false, circles = circles) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun create(name: String, onDone: () -> Unit = {}) {
        val trimmed = name.trim()
        if (trimmed.isEmpty() || _uiState.value.isSaving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, error = null) }
            circleRepository.create(trimmed).foldApiError(
                onSuccess = { circle ->
                    _uiState.update {
                        it.copy(isSaving = false, circles = (it.circles + circle).sortedBy { c -> c.name.lowercase() })
                    }
                    onDone()
                },
                onError = { error ->
                    _uiState.update { it.copy(isSaving = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun rename(id: String, name: String) {
        val trimmed = name.trim()
        if (trimmed.isEmpty()) return
        viewModelScope.launch {
            _uiState.update { it.copy(error = null) }
            circleRepository.rename(id, trimmed).foldApiError(
                onSuccess = { circle ->
                    _uiState.update { state ->
                        state.copy(
                            circles = state.circles.map { if (it.id == id) circle else it }
                                .sortedBy { c -> c.name.lowercase() },
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(error = error.displayMessage) }
                },
            )
        }
    }

    fun delete(id: String) {
        if (_uiState.value.deletingId != null) return
        viewModelScope.launch {
            _uiState.update { it.copy(deletingId = id, error = null) }
            circleRepository.delete(id).foldApiError(
                onSuccess = {
                    _uiState.update { state ->
                        state.copy(deletingId = null, circles = state.circles.filterNot { it.id == id })
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
