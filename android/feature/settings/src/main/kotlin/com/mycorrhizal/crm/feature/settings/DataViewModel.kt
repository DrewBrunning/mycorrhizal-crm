package com.mycorrhizal.crm.feature.settings

import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.RelationshipEdgeRepository
import com.mycorrhizal.crm.model.network.ApplyContactAddressSuggestionInput
import com.mycorrhizal.crm.model.network.ContactAddressSuggestion
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/**
 * T104 + address suggestions: the "propose data" screen. Two opt-in engines —
 * graph-inferred relationship suggestions and relationship/household-derived
 * address suggestions — plus the address-suggestion review list with explicit
 * Apply. Nothing is applied automatically; the user confirms each suggestion.
 */
data class DataUiState(
    val addressSuggestions: List<ContactAddressSuggestion> = emptyList(),
    val suggestionsLoaded: Boolean = false,
    val suggestionsLoading: Boolean = false,
    val isSuggestingRelationships: Boolean = false,
    /** The in-flight apply key: `contact_uid|address_key`. */
    val applyingKey: String? = null,
    /** Number of relationship edges the last suggest run newly created (null = not yet run). */
    val suggestedRelationshipCount: Int? = null,
    @StringRes val infoRes: Int? = null,
    val infoCount: Int? = null,
    val error: String? = null,
)

@HiltViewModel
class DataViewModel @Inject constructor(
    private val contactRepository: ContactRepository,
    private val relationshipEdgeRepository: RelationshipEdgeRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(DataUiState())
    val uiState: StateFlow<DataUiState> = _uiState.asStateFlow()

    /** T104: run one round of graph inference over confirmed edges. */
    fun suggestRelationships() {
        if (_uiState.value.isSuggestingRelationships) return
        _uiState.update { it.copy(isSuggestingRelationships = true, error = null) }
        viewModelScope.launch {
            relationshipEdgeRepository.suggest().foldApiError(
                onSuccess = { edges ->
                    _uiState.update {
                        it.copy(
                            isSuggestingRelationships = false,
                            suggestedRelationshipCount = edges.size,
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isSuggestingRelationships = false, error = error.displayMessage) }
                },
            )
        }
    }

    /**
     * 167: run the read-only address-suggestion scan. Nothing is persisted
     * until the user applies a specific suggestion.
     */
    fun scanAddressSuggestions() {
        if (_uiState.value.suggestionsLoading) return
        _uiState.update { it.copy(suggestionsLoading = true, error = null) }
        viewModelScope.launch {
            contactRepository.suggestContactAddresses().foldApiError(
                onSuccess = { suggestions ->
                    _uiState.update {
                        it.copy(
                            suggestionsLoading = false,
                            suggestionsLoaded = true,
                            addressSuggestions = suggestions,
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(suggestionsLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    /** Apply one address suggestion and remove it from the list. */
    fun applySuggestion(suggestion: ContactAddressSuggestion) {
        val key = suggestionKey(suggestion)
        if (_uiState.value.applyingKey != null) return
        _uiState.update { it.copy(applyingKey = key, error = null) }
        viewModelScope.launch {
            contactRepository.applyContactAddressSuggestion(
                ApplyContactAddressSuggestionInput(
                    contactVCardUid = suggestion.contactVCardUid,
                    sourceKind = suggestion.sourceKind,
                    sourceId = suggestion.sourceId,
                    addressKey = suggestion.addressKey,
                ),
            ).foldApiError(
                onSuccess = {
                    _uiState.update { state ->
                        state.copy(
                            applyingKey = null,
                            addressSuggestions = state.addressSuggestions.filterNot { suggestionKey(it) == key },
                            infoRes = R.string.data_address_applied,
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(applyingKey = null, error = error.displayMessage) }
                },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }

    fun onInfoShown() {
        _uiState.update { it.copy(infoRes = null, infoCount = null) }
    }

    private fun suggestionKey(suggestion: ContactAddressSuggestion): String =
        "${suggestion.contactVCardUid}|${suggestion.addressKey}"
}

/** Human label for a relation token in the address-reason line (e.g. "parent_of" -> "parent of"). */
fun relationTokenLabel(token: String?): String = token?.replace('_', ' ')?.trim().orEmpty()
