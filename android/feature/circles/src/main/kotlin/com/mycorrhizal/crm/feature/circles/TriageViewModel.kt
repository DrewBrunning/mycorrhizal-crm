package com.mycorrhizal.crm.feature.circles

import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.network.toApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

enum class TriageClassification { CIRCLE, TAG, SKIP }

data class TriageItem(
    /** The untouched legacy string from the old flat `contacts.circles` column. */
    val original: String,
    /** The name that will be created (inline-renamed; defaults to [original]). */
    val name: String,
    val classification: TriageClassification,
    val contactCount: Int,
)

data class TriageUiState(
    val items: List<TriageItem> = emptyList(),
    val isLoading: Boolean = false,
    val applying: Boolean = false,
    val error: String? = null,
    @StringRes val errorRes: Int? = null,
    /** True once the apply pass completed. */
    val done: Boolean = false,
    val appliedCircles: Int = 0,
    val appliedTags: Int = 0,
    /** Items that could not be created (create failed and no existing entity found). */
    val failedItems: Int = 0,
    /** Member adds that failed (a contact could not be added to a created entity). */
    val memberAddFailures: Int = 0,
) {
    val circleItems: List<TriageItem> get() = items.filter { it.classification == TriageClassification.CIRCLE }
    val tagItems: List<TriageItem> get() = items.filter { it.classification == TriageClassification.TAG }
    val skippedCount: Int get() = items.count { it.classification == TriageClassification.SKIP }
    /** The Apply button is disabled when nothing is classified to create. */
    val hasWork: Boolean get() = circleItems.isNotEmpty() || tagItems.isNotEmpty()
}

/**
 * M26: the circle/tag-triage tool — one-time cleanup of the legacy free-text
 * circle strings inherited from the meerkat fork. Collects the distinct
 * strings, classifies each as circle/tag/skip with inline rename, then on
 * apply creates the entities and adds their members (mirroring web's
 * CircleTagTriagePage classify -> preview -> apply flow).
 */
@HiltViewModel
class TriageViewModel @Inject constructor(
    private val contactRepository: ContactRepository,
    private val circleRepository: CircleRepository,
    private val tagRepository: TagRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(TriageUiState())
    val uiState: StateFlow<TriageUiState> = _uiState.asStateFlow()

    init { load() }

    fun load() {
        if (_uiState.value.isLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
            val legacy = contactRepository.listLegacyCircles().getOrElse { error ->
                _uiState.update { it.copy(isLoading = false, error = error.toApiError().displayMessage) }
                return@launch
            }
            // Per-string contact counts (web does the same fan-out:
            // getContactsByLegacyCircle per string), sorted by count descending
            // so the biggest cleanups sit first. The for-loop (not a map) so
            // the suspend countFor is callable in the coroutine body.
            val items = mutableListOf<TriageItem>()
            for (name in legacy) {
                items += TriageItem(
                    original = name,
                    name = name,
                    classification = TriageClassification.CIRCLE,
                    contactCount = contactRepository.listContacts(circleLegacy = name, limit = 500)
                        .getOrNull()?.contacts?.size ?: 0,
                )
            }
            _uiState.update { it.copy(isLoading = false, items = items.sortedByDescending { it.contactCount }) }
        }
    }

    fun setClassification(index: Int, classification: TriageClassification) {
        _uiState.update { state ->
            state.copy(items = state.items.mapIndexed { i, item -> if (i == index) item.copy(classification = classification) else item })
        }
    }

    fun setName(index: Int, name: String) {
        _uiState.update { state ->
            state.copy(items = state.items.mapIndexed { i, item -> if (i == index) item.copy(name = name) else item })
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null, errorRes = null) }
    }

    /**
     * Apply: create the classified circles/tags (reusing an existing entity of
     * the same name so a re-run doesn't duplicate), then add every contact
     * that still carries the legacy string as a member. A create that fails
     * falls back to a fresh name lookup (matching web's treat-a-duplicate-
     * create-as-success) before the item is counted as failed; member-add
     * failures are counted, never fatal.
     */
    fun apply() {
        val state = _uiState.value
        if (state.applying || !state.hasWork) return
        viewModelScope.launch {
            _uiState.update { it.copy(applying = true, error = null) }
            val existingCircles = circleRepository.list(limit = 200).getOrNull().orEmpty().associate { it.name to it.id }
            val existingTags = tagRepository.list(limit = 200).getOrNull().orEmpty().associate { it.name to it.id }

            var appliedCircles = 0
            var appliedTags = 0
            var failedItems = 0
            var memberAddFailures = 0
            for (item in state.circleItems) {
                val name = item.name.trim()
                if (name.isBlank()) continue
                val circleId = existingCircles[name] ?: createCircleWithFallback(name, existingCircles)
                if (circleId != null) {
                    memberAddFailures += addMembers(circleId, item, isCircle = true)
                    appliedCircles++
                } else {
                    failedItems++
                }
            }
            for (item in state.tagItems) {
                val name = item.name.trim()
                if (name.isBlank()) continue
                val tagId = existingTags[name] ?: createTagWithFallback(name, existingTags)
                if (tagId != null) {
                    memberAddFailures += addMembers(tagId, item, isCircle = false)
                    appliedTags++
                } else {
                    failedItems++
                }
            }
            _uiState.update {
                it.copy(
                    applying = false,
                    done = true,
                    appliedCircles = appliedCircles,
                    appliedTags = appliedTags,
                    failedItems = failedItems,
                    memberAddFailures = memberAddFailures,
                )
            }
        }
    }

    /** Create, falling back to a fresh name lookup when the create fails (web's 409-as-success). */
    private suspend fun createCircleWithFallback(name: String, known: Map<String, String>): String? {
        val created = circleRepository.create(name).getOrNull()?.id
        if (created != null) return created
        return circleRepository.list(limit = 200).getOrNull().orEmpty()
            .firstOrNull { it.name == name }?.id ?: known[name]
    }

    private suspend fun createTagWithFallback(name: String, known: Map<String, String>): String? {
        val created = tagRepository.create(name).getOrNull()?.id
        if (created != null) return created
        return tagRepository.list(limit = 200).getOrNull().orEmpty()
            .firstOrNull { it.name == name }?.id ?: known[name]
    }

    /** Adds every contact carrying the legacy string; returns the number of failed member adds. */
    private suspend fun addMembers(entityId: String, item: TriageItem, isCircle: Boolean): Int {
        val contacts = contactRepository.listContacts(circleLegacy = item.original, limit = 500).getOrNull()?.contacts.orEmpty()
        var failures = 0
        for (contact in contacts) {
            val uid = contact.uid ?: continue
            val ok = if (isCircle) {
                circleRepository.addMember(entityId, uid).isSuccess
            } else {
                tagRepository.addContact(entityId, uid).isSuccess
            }
            if (!ok) failures++
        }
        return failures
    }
}
