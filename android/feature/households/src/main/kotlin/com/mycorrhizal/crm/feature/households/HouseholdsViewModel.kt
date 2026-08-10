package com.mycorrhizal.crm.feature.households

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.HouseholdRepository
import com.mycorrhizal.crm.model.network.Household
import com.mycorrhizal.crm.model.network.HouseholdMember
import com.mycorrhizal.crm.model.network.HouseholdTypes
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class HouseholdsUiState(
    val households: List<Household> = emptyList(),
    val isLoading: Boolean = false,
    val error: String? = null,
    val isSaving: Boolean = false,
    val deletingId: String? = null,
)

@HiltViewModel
class HouseholdsViewModel @Inject constructor(
    private val householdRepository: HouseholdRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(HouseholdsUiState())
    val uiState: StateFlow<HouseholdsUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        if (_uiState.value.isLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            householdRepository.list().foldApiError(
                onSuccess = { households ->
                    _uiState.update { it.copy(isLoading = false, households = households) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun create(name: String, type: String, onDone: () -> Unit = {}) {
        val trimmed = name.trim()
        if (trimmed.isEmpty() || _uiState.value.isSaving) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, error = null) }
            householdRepository.create(trimmed, type).foldApiError(
                onSuccess = { household ->
                    _uiState.update {
                        it.copy(isSaving = false, households = (it.households + household)
                            .sortedBy { h -> h.name.lowercase() })
                    }
                    onDone()
                },
                onError = { error ->
                    _uiState.update { it.copy(isSaving = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun rename(id: String, name: String, type: String) {
        val trimmed = name.trim()
        if (trimmed.isEmpty()) return
        viewModelScope.launch {
            _uiState.update { it.copy(error = null) }
            householdRepository.update(id, trimmed, type).foldApiError(
                onSuccess = { household ->
                    _uiState.update { state ->
                        state.copy(
                            households = state.households.map { if (it.id == id) household else it }
                                .sortedBy { h -> h.name.lowercase() },
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
            householdRepository.delete(id).foldApiError(
                onSuccess = {
                    _uiState.update { state ->
                        state.copy(deletingId = null, households = state.households.filterNot { it.id == id })
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

data class HouseholdDetailUiState(
    val householdId: String = "",
    val household: Household? = null,
    val members: List<HouseholdMember> = emptyList(),
    val isLoading: Boolean = false,
    val removingUid: String? = null,
    val error: String? = null,
)

@HiltViewModel
class HouseholdDetailViewModel @Inject constructor(
    private val householdRepository: HouseholdRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val householdId: String = run {
        val raw: Any? = savedStateHandle["householdId"]
        (raw as? String) ?: raw?.toString().orEmpty()
    }

    private val _uiState = MutableStateFlow(HouseholdDetailUiState(householdId = householdId))
    val uiState: StateFlow<HouseholdDetailUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        if (householdId.isBlank()) {
            _uiState.update { it.copy(isLoading = false, error = "Missing household id") }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            householdRepository.getWithMembers(householdId).foldApiError(
                onSuccess = { detail ->
                    _uiState.update {
                        it.copy(isLoading = false, household = detail.household, members = detail.members)
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun addMember(vcardUid: String, role: String? = null) {
        val uid = vcardUid.trim()
        if (uid.isEmpty()) return
        viewModelScope.launch {
            householdRepository.addMember(householdId, uid, role).foldApiError(
                onSuccess = { member ->
                    _uiState.update { it.copy(members = it.members + member) }
                },
                onError = { error ->
                    _uiState.update { it.copy(error = error.displayMessage) }
                },
            )
        }
    }

    fun removeMember(vcardUid: String) {
        if (_uiState.value.removingUid != null) return
        viewModelScope.launch {
            _uiState.update { it.copy(removingUid = vcardUid) }
            householdRepository.removeMember(householdId, vcardUid).foldApiError(
                onSuccess = {
                    _uiState.update { state ->
                        state.copy(
                            removingUid = null,
                            members = state.members.filterNot { it.memberVCardUid == vcardUid },
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(removingUid = null, error = error.displayMessage) }
                },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}

object HouseholdTypeLabels {
    fun label(type: String): String = when (type) {
        HouseholdTypes.FAMILY_UNIT -> "Family unit"
        HouseholdTypes.ROOMMATES -> "Roommates"
        else -> "Other"
    }
}
