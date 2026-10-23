package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.GiftRepository
import com.mycorrhizal.crm.domain.repository.PreferenceRepository
import com.mycorrhizal.crm.model.network.ContactBriefing
import com.mycorrhizal.crm.model.network.Gift
import com.mycorrhizal.crm.model.network.GiftStatuses
import com.mycorrhizal.crm.model.network.Preference
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.network.toApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch
import javax.inject.Inject

data class PrepViewUiState(
    val briefing: ContactBriefing? = null,
    val isLoading: Boolean = false,
    val error: String? = null,
    /** The signed-in user's `date_format` preference; falls back to "eu" when absent. */
    val dateFormat: String? = null,
    /**
     * Issue #1774: the contact's preferences and still-unbought gift ideas —
     * the two "what do I know about them" blocks the prep view was missing.
     * Loaded from their own endpoints (they are not part of the briefing
     * contract), and kept empty on failure so the briefing itself still
     * renders.
     */
    val preferences: List<Preference> = emptyList(),
    val giftIdeas: List<Gift> = emptyList(),
)

/**
 * Loads the N2 prep-view briefing (GET /contacts/:id/briefing) for the screen
 * backing the `contacts/{contactId}/prep` route. The briefing is a pure
 * read-only composition — the backend assembles every block, so there is no
 * local cache to mirror and no write path (the same "read a composite via
 * [ApiClient] directly" precedent as [DashboardViewModel]).
 *
 * State machine: loading → success (briefing populated) or error (with a
 * retry that re-issues the call). The empty-contact case is NOT an error —
 * [ContactBriefing] normalizes absent/null collections to empty lists, so a
 * briefing with no history parses and renders its empty states (M11 test case 1).
 */
@HiltViewModel
class PrepViewModel @Inject constructor(
    private val apiClient: ApiClient,
    private val authRepository: AuthRepository,
    private val preferenceRepository: PreferenceRepository,
    private val giftRepository: GiftRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val contactId: Int = run {
        val raw: Any? = savedStateHandle["contactId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull() ?: 0
    }

    private val _uiState = MutableStateFlow(PrepViewUiState())
    val uiState: StateFlow<PrepViewUiState> = _uiState.asStateFlow()

    /** The in-flight load, so a retry tapped twice doesn't fire two overlapping fetches. */
    private var loadJob: Job? = null

    init {
        load()
        viewModelScope.launch {
            authRepository.observeSession().collect { session ->
                _uiState.update { it.copy(dateFormat = session.dateFormat) }
            }
        }
    }

    fun load() {
        if (contactId == 0) {
            _uiState.update {
                it.copy(isLoading = false, briefing = null, error = null)
            }
            return
        }
        if (loadJob?.isActive == true) return
        loadJob = viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            val result = apiClient.getBriefing(contactId)
            val briefing = result.getOrNull()
            if (briefing == null) {
                val error = result.exceptionOrNull()?.toApiError()
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        briefing = null,
                        preferences = emptyList(),
                        giftIdeas = emptyList(),
                        error = error?.displayMessage,
                    )
                }
                return@launch
            }
            // Issue #1774: preferences and gift ideas live on their own list
            // endpoints, not in the briefing. A failure of either is not a
            // prep-view failure — the briefing is still useful on its own, so
            // fold each to an empty list.
            val uid = briefing.uid
            val preferences = if (uid.isBlank()) {
                emptyList()
            } else {
                preferenceRepository.listForContact(uid).getOrDefault(emptyList())
            }
            val giftIdeas = if (uid.isBlank()) {
                emptyList()
            } else {
                giftRepository.listForContact(uid)
                    .getOrDefault(emptyList())
                    .filter { it.status == GiftStatuses.IDEA }
            }
            _uiState.update {
                it.copy(
                    isLoading = false,
                    briefing = briefing,
                    preferences = preferences,
                    giftIdeas = giftIdeas,
                )
            }
        }
    }
}
