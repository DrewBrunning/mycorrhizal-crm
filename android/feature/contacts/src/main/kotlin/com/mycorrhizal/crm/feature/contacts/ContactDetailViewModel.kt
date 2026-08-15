package com.mycorrhizal.crm.feature.contacts

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ReminderRepository
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.ReminderCompletion
import com.mycorrhizal.crm.model.network.Tag
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class ContactDetailUiState(
    val contact: ContactRecordResponse? = null,
    val deviceLookupKey: String? = null,
    val isLoading: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
    /** The signed-in user's `date_format` preference (see `SessionState`); null until loaded. */
    val dateFormat: String? = null,
    /**
     * T84 (read-only slice): the user's custom field definitions and this contact's values for
     * them, keyed by `FieldDefinition.id`. Fetched separately from the contact and from each
     * other — a value's definition may no longer exist (deleted since the value was set); such
     * values are silently skipped at render time rather than crashing, since the two lists are
     * never fetched atomically. A fetch failure for either just leaves it empty; this is an
     * optional enhancement to the contact record, not core data, so it never blocks or errors
     * the screen.
     */
    val fieldDefinitions: List<FieldDefinition> = emptyList(),
    val fieldValuesByDefinitionId: Map<String, Any?> = emptyMap(),
    // M24: inline circle/tag editors. `contactCircles`/`contactTags` are derived from the
    // CircleMember/ContactTag join rows (the contact payload only carries the legacy flat
    // `crm.circles` names and no tags at all); `allCircles`/`allTags` back the add menus. A
    // fetch failure for either leaves the section's chips empty but never errors the screen.
    val allCircles: List<Circle> = emptyList(),
    val contactCircles: List<Circle> = emptyList(),
    val allTags: List<Tag> = emptyList(),
    val contactTags: List<Tag> = emptyList(),
    /** True while a mutating action (delete/archive/unarchive/export) is in flight. */
    val isMutating: Boolean = false,
    /**
     * M20: the contact's reminder completions (web's "Reminder completed"
     * timeline entries). Fetched independently from the contact record — the
     * record payload carries reminders, not completions. A fetch failure
     * silently leaves it empty; the timeline simply lacks completion rows.
     */
    val completions: List<ReminderCompletion> = emptyList(),
)

sealed interface ContactDetailEvent {
    /** A successful soft-delete — the screen should navigate back. */
    data object ContactDeleted : ContactDetailEvent

    /**
     * A single-contact vCard export is ready to hand off to the share sheet.
     * [version] is null (vCard 4.0) or 3 (vCard 3.0), for the filename.
     */
    data class ExportReady(val version: Int?, val bytes: ByteArray) : ContactDetailEvent
}

@HiltViewModel
class ContactDetailViewModel @Inject constructor(
    private val contactRepository: ContactRepository,
    private val reminderRepository: ReminderRepository,
    private val authRepository: AuthRepository,
    // T84: custom field definitions/values are cross-cutting (definitions are per-user, not
    // per-contact) and read-only here, so ApiClient is injected directly rather than adding a
    // repository for a slice with no write path yet — same precedent as DashboardViewModel and
    // T87's ContactListViewModel.
    private val apiClient: ApiClient,
    // M24: the inline circle/tag editors write through the repositories (so the Room mirrors
    // stay in sync) and read the join-row derivations they expose.
    private val circleRepository: CircleRepository,
    private val tagRepository: TagRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val contactId: Int = run {
        val raw: Any? = savedStateHandle["contactId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull() ?: 0
    }

    private val _uiState = MutableStateFlow(ContactDetailUiState())
    val uiState: StateFlow<ContactDetailUiState> = _uiState.asStateFlow()

    private val _events = MutableStateFlow<ContactDetailEvent?>(null)
    val events: StateFlow<ContactDetailEvent?> = _events

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
            _uiState.update { it.copy(isLoading = false, errorRes = R.string.contact_error_missing_id, error = null) }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            val lookupKey = contactRepository.getDeviceLookupKey(contactId)
            contactRepository.getContact(contactId).foldApiError(
                onSuccess = { contact ->
                    _uiState.update {
                        it.copy(isLoading = false, contact = contact, deviceLookupKey = lookupKey)
                    }
                    loadCirclesAndTags(contact)
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
        loadCustomFields()
        loadCompletions()
    }

    /**
     * M20: reminder completions are a separate endpoint from the contact
     * record, so the timeline can show (and undo) "Reminder completed" rows.
     * A failure silently leaves the list empty — never errors the screen.
     */
    private fun loadCompletions() {
        if (contactId == 0) return
        viewModelScope.launch {
            reminderRepository.listCompletions(contactId).foldApiError(
                onSuccess = { completions -> _uiState.update { it.copy(completions = completions) } },
                onError = {},
            )
        }
    }

    /**
     * M20: delete a reminder completion (web's undo of a "Reminder completed"
     * timeline entry), then reload so the row disappears.
     */
    fun undoCompletion(id: Int) {
        if (contactId == 0) return
        viewModelScope.launch {
            reminderRepository.deleteCompletion(id).foldApiError(
                onSuccess = { loadCompletions() },
                onError = { error ->
                    _uiState.update { it.copy(error = error.displayMessage) }
                },
            )
        }
    }

    /**
     * T84 (read-only slice): fetched independently of the contact record and of each other, and
     * never surfaces its own error — see [ContactDetailUiState.fieldDefinitions]' doc comment
     * for why a failure here silently leaves the section empty rather than erroring the screen.
     */
    private fun loadCustomFields() {
        viewModelScope.launch {
            apiClient.listFieldDefinitions().foldApiError(
                onSuccess = { response -> _uiState.update { it.copy(fieldDefinitions = response.definitions) } },
                onError = {},
            )
        }
        viewModelScope.launch {
            apiClient.listContactFieldValues(contactId).foldApiError(
                onSuccess = { response ->
                    val byDefinitionId = response.values.associate { it.fieldDefinitionId to it.value }
                    _uiState.update { it.copy(fieldValuesByDefinitionId = byDefinitionId) }
                },
                onError = {},
            )
        }
    }

    /**
     * M24: refresh the inline circle/tag editors. Needs the contact's VCard UID (the join-row
     * derivations are keyed on it), so it is only called once the contact has loaded. Failures
     * silently leave the sections empty — see the state fields' doc comment.
     */
    private fun loadCirclesAndTags(contact: ContactRecordResponse) {
        val uid = contact.uid
        if (uid.isNullOrBlank()) return
        viewModelScope.launch {
            circleRepository.circlesForContact(uid).foldApiError(
                onSuccess = { circles -> _uiState.update { it.copy(contactCircles = circles) } },
                onError = {},
            )
        }
        viewModelScope.launch {
            circleRepository.list().foldApiError(
                onSuccess = { circles -> _uiState.update { it.copy(allCircles = circles) } },
                onError = {},
            )
        }
        viewModelScope.launch {
            tagRepository.tagsForContact(uid).foldApiError(
                onSuccess = { tags -> _uiState.update { it.copy(contactTags = tags) } },
                onError = {},
            )
        }
        viewModelScope.launch {
            tagRepository.list().foldApiError(
                onSuccess = { tags -> _uiState.update { it.copy(allTags = tags) } },
                onError = {},
            )
        }
    }

    // --- M24: top-level actions (delete / archive / unarchive / export) ---

    /**
     * Soft-delete the contact. On success the screen navigates back (a screen bound to a
     * deleted contact is stale); the list screen refetches on resume and the row disappears.
     */
    fun deleteContact() {
        if (contactId == 0 || _uiState.value.isMutating) return
        viewModelScope.launch {
            _uiState.update { it.copy(isMutating = true, error = null) }
            contactRepository.deleteContact(contactId).foldApiError(
                onSuccess = {
                    _uiState.update { it.copy(isMutating = false) }
                    _events.value = ContactDetailEvent.ContactDeleted
                },
                onError = { error ->
                    _uiState.update { it.copy(isMutating = false, error = error.displayMessage) }
                },
            )
        }
    }

    /**
     * Archive (true) or unarchive (false) the contact, matching web's semantics — archive
     * retires the contact's reminders server-side; the contact stays visible on this screen
     * and the action menu flips. Reloads the contact afterwards so the archived flag (and
     * cache) reflect the change.
     */
    fun setArchived(archived: Boolean) {
        if (contactId == 0 || _uiState.value.isMutating) return
        viewModelScope.launch {
            _uiState.update { it.copy(isMutating = true, error = null) }
            val result = if (archived) {
                contactRepository.archiveContact(contactId)
            } else {
                contactRepository.unarchiveContact(contactId)
            }
            result.foldApiError(
                onSuccess = {
                    _uiState.update { it.copy(isMutating = false) }
                    load()
                },
                onError = { error ->
                    _uiState.update { it.copy(isMutating = false, error = error.displayMessage) }
                },
            )
        }
    }

    /**
     * Export this contact as vCard 4.0 (version == null) or 3.0 (version == 3), emitting
     * [ContactDetailEvent.ExportReady] with the raw file bytes for the share sheet.
     */
    fun exportVcf(version: Int? = null) {
        val uid = _uiState.value.contact?.uid
        if (uid.isNullOrBlank() || _uiState.value.isMutating) return
        viewModelScope.launch {
            _uiState.update { it.copy(isMutating = true, error = null) }
            contactRepository.exportContactVcf(uid, version).foldApiError(
                onSuccess = { bytes ->
                    _uiState.update { it.copy(isMutating = false) }
                    _events.value = ContactDetailEvent.ExportReady(version, bytes)
                },
                onError = { error ->
                    _uiState.update { it.copy(isMutating = false, error = error.displayMessage) }
                },
            )
        }
    }

    // --- M24: inline circle/tag editors ---

    /** Add [circle]'s membership for this contact (needs the VCard UID; no-op without one). */
    fun addCircle(circle: Circle) {
        val uid = _uiState.value.contact?.uid ?: return
        viewModelScope.launch {
            circleRepository.addMember(circle.id, uid).foldApiError(
                onSuccess = { reloadMemberships() },
                onError = { error -> _uiState.update { it.copy(error = error.displayMessage) } },
            )
        }
    }

    fun removeCircle(circle: Circle) {
        val uid = _uiState.value.contact?.uid ?: return
        viewModelScope.launch {
            circleRepository.removeMember(circle.id, uid).foldApiError(
                onSuccess = { reloadMemberships() },
                onError = { error -> _uiState.update { it.copy(error = error.displayMessage) } },
            )
        }
    }

    /** Tag this contact with [tag] (needs the VCard UID; no-op without one). */
    fun addTag(tag: Tag) {
        val uid = _uiState.value.contact?.uid ?: return
        viewModelScope.launch {
            tagRepository.addContact(tag.id, uid).foldApiError(
                onSuccess = { reloadMemberships() },
                onError = { error -> _uiState.update { it.copy(error = error.displayMessage) } },
            )
        }
    }

    fun removeTag(tag: Tag) {
        val uid = _uiState.value.contact?.uid ?: return
        viewModelScope.launch {
            tagRepository.removeContact(tag.id, uid).foldApiError(
                onSuccess = { reloadMemberships() },
                onError = { error -> _uiState.update { it.copy(error = error.displayMessage) } },
            )
        }
    }

    /** Refetch the join-row derivations after a membership/tagging write. */
    private fun reloadMemberships() {
        _uiState.value.contact?.let { loadCirclesAndTags(it) }
    }

    /** Complete a reminder from the timeline; reload the contact so the list refreshes. */
    fun completeReminder(id: Int) {
        viewModelScope.launch {
            reminderRepository.complete(id).foldApiError(
                onSuccess = { load() },
                onError = { error ->
                    _uiState.update { it.copy(error = error.displayMessage) }
                },
            )
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(errorRes = null, error = null) }
    }

    fun onEventShown() {
        _events.value = null
    }
}
