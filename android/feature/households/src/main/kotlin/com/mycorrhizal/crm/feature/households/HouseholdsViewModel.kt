package com.mycorrhizal.crm.feature.households

import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.HouseholdRepository
import com.mycorrhizal.crm.model.network.AcceptHouseholdSuggestionInput
import com.mycorrhizal.crm.model.network.AddressHouseholdSuggestion
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.DismissHouseholdSuggestionInput
import com.mycorrhizal.crm.model.network.Household
import com.mycorrhizal.crm.model.network.HouseholdMember
import com.mycorrhizal.crm.model.network.HouseholdTypes
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class HouseholdsUiState(
    val households: List<Household> = emptyList(),
    /** T40: shared-address household suggestions, loaded on demand. */
    val addressSuggestions: List<AddressHouseholdSuggestion> = emptyList(),
    val suggestionsLoaded: Boolean = false,
    val suggestionsLoading: Boolean = false,
    /** The in-flight accept/dismiss key: `accept:<hash>:<hash>` or `dismiss:<hash>:<hash>`. */
    val pendingSuggestionKey: String? = null,
    /** Member VCardUID -> resolved summary, for suggestion member names. */
    val contactsByUid: Map<String, ContactSummary> = emptyMap(),
    val isLoading: Boolean = false,
    val error: String? = null,
    val isSaving: Boolean = false,
    val deletingId: String? = null,
    @StringRes val infoRes: Int? = null,
    val infoCount: Int? = null,
)

@HiltViewModel
class HouseholdsViewModel @Inject constructor(
    private val householdRepository: HouseholdRepository,
    private val contactRepository: ContactRepository,
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

    /**
     * T40: run the shared-address scan and load its suggestions. Read-only
     * server-side; nothing is persisted until the user accepts or dismisses.
     * The empty case is guarded from the start (T64): the backend can omit the
     * collection key entirely, and the model's default `emptyList()` means a
     * scan with nothing to offer simply renders an empty state.
     */
    fun scanAddressSuggestions() {
        if (_uiState.value.suggestionsLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(suggestionsLoading = true, error = null) }
            householdRepository.suggestAddressHouseholds().foldApiError(
                onSuccess = { suggestions ->
                    _uiState.update {
                        it.copy(
                            suggestionsLoading = false,
                            suggestionsLoaded = true,
                            addressSuggestions = suggestions,
                        )
                    }
                    resolveSuggestionMemberNames(suggestions)
                },
                onError = { error ->
                    _uiState.update { it.copy(suggestionsLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun acceptSuggestion(suggestion: AddressHouseholdSuggestion) {
        val key = suggestionKey(suggestion)
        if (_uiState.value.pendingSuggestionKey != null) return
        viewModelScope.launch {
            _uiState.update { it.copy(pendingSuggestionKey = "accept:$key", error = null) }
            householdRepository.acceptAddressSuggestion(
                AcceptHouseholdSuggestionInput(memberVCardUids = suggestion.memberVCardUids),
            ).foldApiError(
                onSuccess = { _ ->
                    _uiState.update { state ->
                        state.copy(
                            pendingSuggestionKey = null,
                            addressSuggestions = state.addressSuggestions.filterNot { suggestionKey(it) == key },
                            infoRes = R.string.households_suggestion_accepted,
                        )
                    }
                    // Refresh the list so the materialized household appears.
                    load()
                },
                onError = { error ->
                    _uiState.update { it.copy(pendingSuggestionKey = null, error = error.displayMessage) }
                },
            )
        }
    }

    fun dismissSuggestion(suggestion: AddressHouseholdSuggestion) {
        val key = suggestionKey(suggestion)
        if (_uiState.value.pendingSuggestionKey != null) return
        viewModelScope.launch {
            _uiState.update { it.copy(pendingSuggestionKey = "dismiss:$key", error = null) }
            householdRepository.dismissAddressSuggestion(
                DismissHouseholdSuggestionInput(memberVCardUids = suggestion.memberVCardUids),
            ).foldApiError(
                onSuccess = {
                    _uiState.update { state ->
                        state.copy(
                            pendingSuggestionKey = null,
                            addressSuggestions = state.addressSuggestions.filterNot { suggestionKey(it) == key },
                            infoRes = R.string.households_suggestion_dismissed,
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(pendingSuggestionKey = null, error = error.displayMessage) }
                },
            )
        }
    }

    /**
     * Best-effort name resolution for suggestion members. A failure here is
     * NOT surfaced as the screen's primary error -- rows fall back to the raw
     * UID, degrading gracefully (same policy as M21's relationships list).
     */
    private fun resolveSuggestionMemberNames(suggestions: List<AddressHouseholdSuggestion>) {
        val uids = suggestions.flatMap { it.memberVCardUids }
            .filter { it.isNotBlank() }
            .distinct()
        if (uids.isEmpty()) return
        viewModelScope.launch {
            contactRepository.resolveByUid(uids).onSuccess { resolved ->
                _uiState.update { it.copy(contactsByUid = it.contactsByUid + resolved) }
            }
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }

    fun onInfoShown() {
        _uiState.update { it.copy(infoRes = null, infoCount = null) }
    }

    private fun suggestionKey(suggestion: AddressHouseholdSuggestion): String =
        "${suggestion.addressHash}:${suggestion.memberHash}"
}

data class HouseholdDetailUiState(
    val householdId: String = "",
    val household: Household? = null,
    val members: List<HouseholdMember> = emptyList(),
    /** Member VCardUID -> resolved summary, for name display/navigation. */
    val contactsByUid: Map<String, ContactSummary> = emptyMap(),
    val isLoading: Boolean = false,
    val removingUid: String? = null,
    val updatingRoleUid: String? = null,
    val isSuggestingRelationships: Boolean = false,
    val memberSearchQuery: String = "",
    val memberSearchResults: List<ContactSummary> = emptyList(),
    val memberSearchLoading: Boolean = false,
    val error: String? = null,
    @StringRes val errorRes: Int? = null,
    @StringRes val infoRes: Int? = null,
    val infoCount: Int? = null,
)

@HiltViewModel
class HouseholdDetailViewModel @Inject constructor(
    private val householdRepository: HouseholdRepository,
    private val contactRepository: ContactRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val householdId: String = run {
        val raw: Any? = savedStateHandle["householdId"]
        (raw as? String) ?: raw?.toString().orEmpty()
    }

    private val _uiState = MutableStateFlow(HouseholdDetailUiState(householdId = householdId))
    val uiState: StateFlow<HouseholdDetailUiState> = _uiState.asStateFlow()

    private var searchJob: Job? = null

    init {
        load()
    }

    fun load() {
        if (householdId.isBlank()) {
            _uiState.update {
                it.copy(isLoading = false, errorRes = R.string.households_error_missing_id, error = null)
            }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
            householdRepository.getWithMembers(householdId).foldApiError(
                onSuccess = { detail ->
                    _uiState.update {
                        it.copy(isLoading = false, household = detail.household, members = detail.members)
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
     * Best-effort name resolution for member VCardUIDs. A failure here is NOT
     * surfaced as the screen's primary error -- rows fall back to the
     * "unknown contact" label (same policy as M21's relationships list).
     */
    private fun resolveMemberNames(members: List<HouseholdMember>) {
        val uids = members.map { it.memberVCardUid }.filter { it.isNotBlank() }.distinct()
        if (uids.isEmpty()) return
        viewModelScope.launch {
            contactRepository.resolveByUid(uids).onSuccess { resolved ->
                _uiState.update { it.copy(contactsByUid = it.contactsByUid + resolved) }
            }
        }
    }

    fun addMember(vcardUid: String, role: String? = null) {
        val uid = vcardUid.trim()
        if (uid.isEmpty()) return
        viewModelScope.launch {
            householdRepository.addMember(householdId, uid, role).foldApiError(
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

    /** Change a member's role in place (PATCH) — no remove-then-re-add. */
    fun updateMemberRole(vcardUid: String, role: String?) {
        if (_uiState.value.updatingRoleUid != null) return
        // "" represents "no role" on the wire (a JSON null 400s server-side);
        // the role picker's "No role" option maps onto it.
        val normalizedRole = role.orEmpty()
        viewModelScope.launch {
            _uiState.update { it.copy(updatingRoleUid = vcardUid, error = null) }
            householdRepository.updateMember(householdId, vcardUid, normalizedRole).foldApiError(
                onSuccess = {
                    _uiState.update { state ->
                        state.copy(
                            updatingRoleUid = null,
                            members = state.members.map { member ->
                                if (member.memberVCardUid == vcardUid) member.copy(role = normalizedRole) else member
                            },
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(updatingRoleUid = null, error = error.displayMessage) }
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

    /** Trigger the relationship-suggestion engine for this household. */
    fun suggestRelationships() {
        if (_uiState.value.isSuggestingRelationships) return
        if (_uiState.value.members.size < 2) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSuggestingRelationships = true, error = null) }
            householdRepository.suggestRelationships(householdId).foldApiError(
                onSuccess = { edges ->
                    val count = edges.size
                    _uiState.update {
                        it.copy(
                            isSuggestingRelationships = false,
                            infoRes = if (count > 0) {
                                R.string.households_suggestions_generated
                            } else {
                                R.string.households_no_new_suggestions
                            },
                            infoCount = if (count > 0) count else null,
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isSuggestingRelationships = false, error = error.displayMessage) }
                },
            )
        }
    }

    /** Debounced contact search for the add-member dialog. Cancel-and-relaunch. */
    fun searchContacts(query: String) {
        _uiState.update { it.copy(memberSearchQuery = query) }
        searchJob?.cancel()
        if (query.isBlank()) {
            _uiState.update { it.copy(memberSearchResults = emptyList(), memberSearchLoading = false) }
            return
        }
        searchJob = viewModelScope.launch {
            delay(SEARCH_DEBOUNCE_MS)
            _uiState.update { it.copy(memberSearchLoading = true) }
            val memberUids = _uiState.value.members.map { m -> m.memberVCardUid }.toSet()
            contactRepository.listContacts(search = query, limit = 25).fold(
                onSuccess = { page ->
                    _uiState.update {
                        it.copy(
                            memberSearchResults = page.contacts.filter { c -> c.uid != null && c.uid !in memberUids },
                            memberSearchLoading = false,
                        )
                    }
                },
                onFailure = {
                    _uiState.update { it.copy(memberSearchLoading = false) }
                },
            )
        }
    }

    fun clearContactSearch() {
        searchJob?.cancel()
        _uiState.update {
            it.copy(memberSearchQuery = "", memberSearchResults = emptyList(), memberSearchLoading = false)
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null, errorRes = null) }
    }

    fun onInfoShown() {
        _uiState.update { it.copy(infoRes = null, infoCount = null) }
    }

    private companion object {
        const val SEARCH_DEBOUNCE_MS = 300L
    }
}

object HouseholdTypeLabels {
    @Composable
    fun label(type: String): String = when (type) {
        HouseholdTypes.FAMILY_UNIT -> stringResource(R.string.household_type_family_unit)
        HouseholdTypes.ROOMMATES -> stringResource(R.string.household_type_roommates)
        else -> stringResource(R.string.household_type_other)
    }
}
