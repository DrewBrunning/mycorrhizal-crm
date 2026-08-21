package com.mycorrhizal.crm.feature.circles

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.CircleMember
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class CircleDetailUiState(
    val circleId: String = "",
    val circle: Circle? = null,
    val members: List<CircleMember> = emptyList(),
    /** Member VCardUID -> resolved summary (issue #218: carries primaryPhone for group SMS). */
    val contactsByUid: Map<String, ContactSummary> = emptyMap(),
    val isLoading: Boolean = false,
    val removingUid: String? = null,
    val error: String? = null,
    @StringRes val errorRes: Int? = null,
)

@HiltViewModel
class CircleDetailViewModel @Inject constructor(
    private val circleRepository: CircleRepository,
    private val contactRepository: ContactRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val circleId: String = run {
        val raw: Any? = savedStateHandle["circleId"]
        (raw as? String) ?: raw?.toString().orEmpty()
    }

    private val _uiState = MutableStateFlow(CircleDetailUiState(circleId = circleId))
    val uiState: StateFlow<CircleDetailUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        if (circleId.isBlank()) {
            _uiState.update { it.copy(isLoading = false, errorRes = R.string.circles_error_missing_id, error = null) }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
            circleRepository.getWithMembers(circleId).foldApiError(
                onSuccess = { detail ->
                    _uiState.update {
                        it.copy(isLoading = false, circle = detail.circle, members = detail.members)
                    }
                    resolveMemberNames(detail.members)
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    /**
     * Best-effort name/phone resolution for member VCardUIDs (mirrors
     * HouseholdDetailViewModel.resolveMemberNames — issue #218 needs the
     * resolved ContactSummary.primaryPhone for the "text everyone" action). A
     * failure here is NOT surfaced as the screen's primary error.
     */
    private fun resolveMemberNames(members: List<CircleMember>) {
        val uids = members.map { it.memberVCardUid }.filter { it.isNotBlank() }.distinct()
        if (uids.isEmpty()) return
        viewModelScope.launch {
            contactRepository.resolveByUid(uids).onSuccess { resolved ->
                _uiState.update { it.copy(contactsByUid = it.contactsByUid + resolved) }
            }
        }
    }

    fun addMember(vcardUid: String) {
        val uid = vcardUid.trim()
        if (uid.isEmpty()) return
        viewModelScope.launch {
            circleRepository.addMember(circleId, uid).foldApiError(
                onSuccess = { member ->
                    _uiState.update { it.copy(members = it.members + member) }
                    resolveMemberNames(listOf(member))
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
            circleRepository.removeMember(circleId, vcardUid).foldApiError(
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
        _uiState.update { it.copy(error = null, errorRes = null) }
    }
}
