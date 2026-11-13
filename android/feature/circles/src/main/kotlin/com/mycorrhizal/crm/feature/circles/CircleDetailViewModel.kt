package com.mycorrhizal.crm.feature.circles

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.CircleColors
import com.mycorrhizal.crm.model.network.CircleMember
import com.mycorrhizal.crm.model.network.CircleTimelineItem
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class CircleDetailUiState(
    val circleId: String = "",
    val circle: Circle? = null,
    val members: List<CircleMember> = emptyList(),
    /** Member VCardUID -> resolved summary (issue #218: carries primaryPhone for group SMS). */
    val contactsByUid: Map<String, ContactSummary> = emptyMap(),
    val isLoading: Boolean = false,
    val removingUid: String? = null,
    val error: ApiError? = null,
    @StringRes val errorRes: Int? = null,
    /** Edit dialog: the save is in flight. */
    val isSaving: Boolean = false,
    /** Edit dialog: `circle_already_exists` is shown inline under the name field. */
    val nameTaken: Boolean = false,
    /** Aggregate timeline of the circle's live members (issue #1995/#2361). */
    val timeline: List<CircleTimelineItem> = emptyList(),
    /** Resume token for the next timeline page; null on the last page. */
    val timelineCursor: String? = null,
    /** Server-reported member count (archived members count but add no events); null until loaded. */
    val timelineMemberCount: Int? = null,
    val isTimelineLoading: Boolean = false,
    /** True once a first page has loaded, so "empty" is distinguishable from "not loaded yet". */
    val timelineLoaded: Boolean = false,
    /** Scoped to the timeline section; never replaces the member list or the snackbar. */
    val timelineError: ApiError? = null
)

@HiltViewModel
class CircleDetailViewModel @Inject constructor(
    private val circleRepository: CircleRepository,
    private val contactRepository: ContactRepository,
    savedStateHandle: SavedStateHandle
) : ViewModel() {

    private val circleId: String = run {
        val raw: Any? = savedStateHandle["circleId"]
        (raw as? String) ?: raw?.toString().orEmpty()
    }

    private val _uiState = MutableStateFlow(CircleDetailUiState(circleId = circleId))
    val uiState: StateFlow<CircleDetailUiState> = _uiState.asStateFlow()

    /** Bumped on every first-page load so a slow, superseded response cannot overwrite a newer one. */
    private var timelineGeneration = 0

    init {
        load()
    }

    fun load() {
        if (circleId.isBlank()) {
            _uiState.update {
                it.copy(
                    isLoading = false,
                    errorRes = R.string.circles_error_missing_id,
                    error = null
                )
            }
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
                    _uiState.update { it.copy(isLoading = false, error = error) }
                }
            )
        }
        loadTimeline(reset = true)
    }

    /** Loads the first page ([reset]) or the next page of the member timeline. */
    private fun loadTimeline(reset: Boolean) {
        val cursor = if (reset) null else _uiState.value.timelineCursor
        if (!reset && (cursor == null || _uiState.value.isTimelineLoading)) return
        val generation = if (reset) ++timelineGeneration else timelineGeneration
        // Set before launching so a second call in the same frame sees the load in flight.
        _uiState.update { it.copy(isTimelineLoading = true, timelineError = null) }
        viewModelScope.launch {
            circleRepository.getTimeline(circleId, cursor).foldApiError(
                onSuccess = { page ->
                    if (generation != timelineGeneration) return@foldApiError
                    _uiState.update {
                        it.copy(
                            isTimelineLoading = false,
                            timelineLoaded = true,
                            timeline = if (reset) page.items else it.timeline + page.items,
                            timelineCursor = page.nextCursor,
                            timelineMemberCount = page.memberCount
                        )
                    }
                },
                onError = { error ->
                    if (generation != timelineGeneration) return@foldApiError
                    _uiState.update { it.copy(isTimelineLoading = false, timelineError = error) }
                }
            )
        }
    }

    fun loadMoreTimeline() = loadTimeline(reset = false)

    /** Retry after a failed timeline load: the next page if one is pending, else the first. */
    fun retryTimeline() = loadTimeline(reset = !_uiState.value.timelineLoaded)

    /**
     * Saves name, description and colour. Blank names and colours outside the palette are
     * rejected client-side (the form never offers them); [onDone] runs on success only.
     */
    fun update(name: String, description: String, color: String, onDone: () -> Unit = {}) {
        val trimmedName = name.trim()
        if (trimmedName.isEmpty() || _uiState.value.isSaving) return
        if (color.isNotEmpty() && color !in CircleColors.ALL) return
        viewModelScope.launch {
            _uiState.update { it.copy(isSaving = true, error = null, nameTaken = false) }
            circleRepository.update(circleId, trimmedName, description.trim(), color).foldApiError(
                onSuccess = { circle ->
                    _uiState.update { it.copy(isSaving = false, circle = circle) }
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

    fun clearNameError() {
        _uiState.update { it.copy(nameTaken = false) }
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
                    _uiState.update { it.copy(error = error) }
                }
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
                            members = state.members.filterNot { it.memberVCardUid == vcardUid }
                        )
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(removingUid = null, error = error) }
                }
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null, errorRes = null) }
    }
}
