package com.mycorrhizal.crm.feature.circles

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import java.text.Collator
import java.util.Locale
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class CirclesUiState(
    val circles: List<Circle> = emptyList(),
    val isLoading: Boolean = false,
    val error: ApiError? = null,
    /** Create/rename hit `circle_already_exists`; shown inline in the name dialog, not as a snackbar. */
    val nameTaken: Boolean = false,
    val isSaving: Boolean = false,
    val deletingId: String? = null
)

/**
 * Circle name order, mirroring the web `compareCirclesByName`: accent- and case-insensitive
 * (collator PRIMARY strength), then id so the order is total.
 */
internal fun circleNameComparator(locale: Locale = Locale.getDefault()): Comparator<Circle> {
    val collator = Collator.getInstance(locale).apply { strength = Collator.PRIMARY }
    return Comparator<Circle> { a, b -> collator.compare(a.name, b.name) }.thenBy { it.id }
}

private fun ApiError.isCircleNameTaken(): Boolean =
    this is ApiError.Client && reason?.code == ErrorReasonCode.CIRCLE_ALREADY_EXISTS

@HiltViewModel
class CirclesViewModel @Inject constructor(private val circleRepository: CircleRepository) :
    ViewModel() {

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
                    val sorted = circles.sortedWith(circleNameComparator())
                    _uiState.update { it.copy(isLoading = false, circles = sorted) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error) }
                }
            )
        }
    }

    fun create(name: String, onDone: () -> Unit = {}) {
        val trimmed = name.trim()
        if (trimmed.isEmpty() || _uiState.value.isSaving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, error = null, nameTaken = false) }
            circleRepository.create(trimmed).foldApiError(
                onSuccess = { circle ->
                    _uiState.update {
                        it.copy(
                            isSaving = false,
                            circles = (it.circles + circle).sortedWith(circleNameComparator())
                        )
                    }
                    onDone()
                },
                onError = { error ->
                    _uiState.update {
                        if (error.isCircleNameTaken()) {
                            it.copy(isSaving = false, nameTaken = true)
                        } else {
                            it.copy(isSaving = false, error = error)
                        }
                    }
                }
            )
        }
    }

    fun rename(id: String, name: String, onDone: () -> Unit = {}) {
        val trimmed = name.trim()
        if (trimmed.isEmpty()) return
        viewModelScope.launch {
            _uiState.update { it.copy(error = null, nameTaken = false) }
            circleRepository.rename(id, trimmed).foldApiError(
                onSuccess = { circle ->
                    _uiState.update { state ->
                        state.copy(
                            circles = state.circles.map { if (it.id == id) circle else it }
                                .sortedWith(circleNameComparator())
                        )
                    }
                    onDone()
                },
                onError = { error ->
                    _uiState.update {
                        if (error.isCircleNameTaken()) {
                            it.copy(nameTaken = true)
                        } else {
                            it.copy(error = error)
                        }
                    }
                }
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
                        state.copy(
                            deletingId = null,
                            circles = state.circles.filterNot { it.id == id }
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(deletingId = null, error = error) }
                }
            )
        }
    }

    fun clearNameError() {
        _uiState.update { it.copy(nameTaken = false) }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}
