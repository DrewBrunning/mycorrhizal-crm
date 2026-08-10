package com.mycorrhizal.crm.feature.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.CustomLinkAction
import com.mycorrhizal.crm.domain.repository.CustomLinkActionRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class CustomLinkActionsUiState(
    val actions: List<CustomLinkAction> = emptyList(),
    val isLoading: Boolean = false,
    val error: String? = null,
)

@HiltViewModel
class CustomLinkActionsViewModel @Inject constructor(
    private val repository: CustomLinkActionRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(CustomLinkActionsUiState())
    val uiState: StateFlow<CustomLinkActionsUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            _uiState.update { it.copy(isLoading = false, actions = repository.getAll()) }
        }
    }

    fun add(protocol: String, label: String, mimeType: String, template: String) {
        val p = protocol.trim()
        val l = label.trim()
        val m = mimeType.trim()
        val t = template.trim()
        if (p.isEmpty() || l.isEmpty() || m.isEmpty() || t.isEmpty()) return
        viewModelScope.launch {
            repository.upsert(
                CustomLinkAction(
                    protocol = p,
                    label = l,
                    kind = "APP_OPEN",
                    mimeType = m,
                    intentUriTemplate = t,
                ),
            )
            load()
        }
    }

    fun delete(action: CustomLinkAction) {
        viewModelScope.launch {
            repository.delete(action)
            load()
        }
    }
}
