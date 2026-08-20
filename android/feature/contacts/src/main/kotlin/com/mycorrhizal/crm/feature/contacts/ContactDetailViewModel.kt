package com.mycorrhizal.crm.feature.contacts

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ExternalIdentityRepository
import com.mycorrhizal.crm.domain.repository.ImmichRepository
import com.mycorrhizal.crm.domain.repository.NextcloudRepository
import com.mycorrhizal.crm.domain.repository.PaperlessRepository
import com.mycorrhizal.crm.domain.repository.ReminderRepository
import com.mycorrhizal.crm.domain.repository.SeafileRepository
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ExternalIdentity
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.ImmichAssetSummary
import com.mycorrhizal.crm.model.network.ImmichPerson
import com.mycorrhizal.crm.model.network.ImmichPersonSummary
import com.mycorrhizal.crm.model.network.PaperlessDocument
import com.mycorrhizal.crm.model.network.ReminderCompletion
import com.mycorrhizal.crm.model.network.SeafileItem
import com.mycorrhizal.crm.model.network.SeafileLibrary
import com.mycorrhizal.crm.model.network.SeafileLinkRequest
import com.mycorrhizal.crm.model.network.Tag
import com.mycorrhizal.crm.model.network.WebDAVItem
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
    // Issue #220: the External Links panel (ExternalIdentity substrate). Loaded
    // once the contact's VCardUID is known; failures silently leave the section
    // empty (like the circle/tag editors) — never errors the screen. The Immich
    // link renders as a rich row (thumbnail, person name, photo count) via
    // [immichSummary]; other systems render generically.
    val externalIdentities: List<ExternalIdentity> = emptyList(),
    val immichSummary: ImmichPersonSummary? = null,
    /**
     * Whether the user's Immich connection is configured (has an API key).
     * Gates the "Choose from Immich" photo entry point, mirroring web (the
     * button only appears when Immich is configured). A fetch failure hides
     * the entry point — a safe default.
     */
    val immichConfigured: Boolean = false,
    /** True while an external-link mutation (delete/unlink) is in flight. */
    val isExternalLinkMutating: Boolean = false,
    // The Immich "choose from Immich" photo flow: people for the link step,
    // recent assets for the browse step.
    val immichPeople: List<ImmichPerson> = emptyList(),
    val immichPeopleLoading: Boolean = false,
    val immichAssets: List<ImmichAssetSummary> = emptyList(),
    val immichAssetsLoading: Boolean = false,
    // Issue #236: whether each file/document integration is configured (has a
    // stored token) — gates the "Add link" entry points in the External Links
    // panel, same safe-default-on-failure treatment as [immichConfigured].
    val paperlessConfigured: Boolean = false,
    val seafileConfigured: Boolean = false,
    val nextcloudConfigured: Boolean = false,
    // Paperless document picker: a client-side-filtered full list, like web's.
    val paperlessDocuments: List<PaperlessDocument> = emptyList(),
    val paperlessDocumentsLoading: Boolean = false,
    // Seafile picker: two-level nav (library list, then a directory browse
    // within one). `seafileBrowseRepoId == null` means the library list.
    val seafileLibraries: List<SeafileLibrary> = emptyList(),
    val seafileBrowseRepoId: String? = null,
    val seafileBrowsePath: String = "/",
    val seafileBrowseItems: List<SeafileItem> = emptyList(),
    val seafileBrowseLoading: Boolean = false,
    // Nextcloud picker: single-level directory browse.
    val nextcloudBrowsePath: String = "/",
    val nextcloudBrowseItems: List<WebDAVItem> = emptyList(),
    val nextcloudBrowseLoading: Boolean = false,
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
    // Issue #220: the External Links panel (ExternalIdentity substrate) and the
    // Immich "choose from Immich" profile-photo flow. Both are online-only.
    private val externalIdentityRepository: ExternalIdentityRepository,
    private val immichRepository: ImmichRepository,
    // Issue #236: the Paperless/Seafile/Nextcloud create/link pickers.
    private val paperlessRepository: PaperlessRepository,
    private val seafileRepository: SeafileRepository,
    private val nextcloudRepository: NextcloudRepository,
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
                    loadExternalLinks(contact)
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

    // --- Issue #220: External Links panel (ExternalIdentity) + Immich ---

    /**
     * Loads the contact's ExternalIdentity links, the Immich connection gate,
     * and the Immich link summary. Needs the VCardUID; a missing one is a
     * no-op. Failures silently leave the sections empty — the panel is
     * display-only and must never fail the screen.
     */
    private fun loadExternalLinks(contact: ContactRecordResponse) {
        val uid = contact.uid
        if (uid.isNullOrBlank()) return
        viewModelScope.launch {
            externalIdentityRepository.listForContact(uid).foldApiError(
                onSuccess = { identities ->
                    _uiState.update { it.copy(externalIdentities = identities) }
                },
                onError = {},
            )
        }
        viewModelScope.launch {
            immichRepository.isConfigured().foldApiError(
                onSuccess = { configured -> _uiState.update { it.copy(immichConfigured = configured) } },
                onError = {},
            )
        }
        viewModelScope.launch {
            paperlessRepository.isConfigured().foldApiError(
                onSuccess = { configured -> _uiState.update { it.copy(paperlessConfigured = configured) } },
                onError = {},
            )
        }
        viewModelScope.launch {
            seafileRepository.isConfigured().foldApiError(
                onSuccess = { configured -> _uiState.update { it.copy(seafileConfigured = configured) } },
                onError = {},
            )
        }
        viewModelScope.launch {
            nextcloudRepository.isConfigured().foldApiError(
                onSuccess = { configured -> _uiState.update { it.copy(nextcloudConfigured = configured) } },
                onError = {},
            )
        }
        loadImmichSummary(uid)
    }

    private fun loadImmichSummary(uid: String) {
        viewModelScope.launch {
            immichRepository.getContactSummary(uid).foldApiError(
                onSuccess = { summary -> _uiState.update { it.copy(immichSummary = summary) } },
                onError = {},
            )
        }
    }

    /** Delete an ExternalIdentity link (or unlink Immich) — confirms first in the UI. */
    fun deleteExternalIdentity(id: String) {
        if (_uiState.value.isExternalLinkMutating) return
        viewModelScope.launch {
            _uiState.update { it.copy(isExternalLinkMutating = true, error = null) }
            externalIdentityRepository.delete(id).foldApiError(
                onSuccess = {
                    _uiState.update { it.copy(isExternalLinkMutating = false) }
                    _uiState.value.contact?.let { loadExternalLinks(it) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isExternalLinkMutating = false, error = error.displayMessage) }
                },
            )
        }
    }

    /**
     * Unlink a contact's Immich person via the dedicated endpoint (keeps its
     * ExternalActivity history), then refresh the panel.
     */
    fun unlinkImmichPerson() {
        val uid = _uiState.value.contact?.uid ?: return
        if (_uiState.value.isExternalLinkMutating) return
        viewModelScope.launch {
            _uiState.update { it.copy(isExternalLinkMutating = true, error = null) }
            immichRepository.unlinkPerson(uid).foldApiError(
                onSuccess = {
                    _uiState.update { it.copy(isExternalLinkMutating = false, immichSummary = null) }
                    _uiState.value.contact?.let { loadExternalLinks(it) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isExternalLinkMutating = false, error = error.displayMessage) }
                },
            )
        }
    }

    // --- Issue #219: the Immich "choose from Immich" profile-photo flow ---

    /** Fetch the user's Immich people for the link step (called once when the picker opens). */
    fun loadImmichPeople() {
        if (_uiState.value.immichPeopleLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(immichPeopleLoading = true, error = null) }
            immichRepository.listPeople().foldApiError(
                onSuccess = { people ->
                    _uiState.update { it.copy(immichPeople = people, immichPeopleLoading = false) }
                },
                onError = { error ->
                    _uiState.update { it.copy(immichPeopleLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    /**
     * Link an Immich person to this contact (writes the ExternalIdentity),
     * then refresh the panel and load the person's recent photos for browsing.
     */
    fun linkImmichPerson(person: ImmichPerson) {
        val contact = _uiState.value.contact
        val uid = contact?.uid ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(error = null) }
            immichRepository.linkPerson(uid, person).foldApiError(
                onSuccess = {
                    loadExternalLinks(contact)
                    loadImmichAssets()
                },
                onError = { error -> _uiState.update { it.copy(error = error.displayMessage) } },
            )
        }
    }

    /** Fetch the linked person's recent photos for the browse step. */
    fun loadImmichAssets() {
        val uid = _uiState.value.contact?.uid ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(immichAssetsLoading = true, error = null) }
            immichRepository.listContactAssets(uid).foldApiError(
                onSuccess = { assets ->
                    _uiState.update { it.copy(immichAssets = assets, immichAssetsLoading = false) }
                },
                onError = { error ->
                    _uiState.update { it.copy(immichAssetsLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    /**
     * Fetch a chosen photo's bytes for cropping+upload. [assetId] null means
     * the linked person's primary thumbnail; otherwise one of the assets from
     * [loadImmichAssets]. Bytes are handed to [onBytes] (called on the main
     * thread); failures surface via the screen's snackbar.
     */
    fun pickImmichAsset(assetId: String?, onBytes: (ByteArray) -> Unit) {
        val uid = _uiState.value.contact?.uid ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(error = null) }
            val result = if (assetId == null) {
                immichRepository.getThumbnailBytes(uid)
            } else {
                immichRepository.getAssetImageBytes(uid, assetId)
            }
            result.foldApiError(
                onSuccess = { bytes -> onBytes(bytes) },
                onError = { error -> _uiState.update { it.copy(error = error.displayMessage) } },
            )
        }
    }

    // --- Issue #236: Paperless/Seafile/Nextcloud create/link pickers ---
    //
    // Each dialog's open/close visibility is local Compose state in
    // ContactDetailScreen (mirroring immichPickerOpen); this ViewModel only
    // owns the data (lists, loading flags, browse position). Tapping an item
    // fires the link call and the screen closes its dialog immediately —
    // success refreshes the panel, failure surfaces via `state.error`, the
    // same fire-and-refresh shape as `linkImmichPerson`/`deleteExternalIdentity`.

    /** Fetch the full Paperless document list once (client-side filter happens in the dialog). */
    fun loadPaperlessDocuments() {
        if (_uiState.value.paperlessDocumentsLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(paperlessDocumentsLoading = true, error = null) }
            paperlessRepository.searchDocuments().foldApiError(
                onSuccess = { documents ->
                    _uiState.update { it.copy(paperlessDocuments = documents, paperlessDocumentsLoading = false) }
                },
                onError = { error ->
                    _uiState.update { it.copy(paperlessDocumentsLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    /** Link a Paperless document to this contact, then refresh the External Links panel. */
    fun linkPaperlessDocument(document: PaperlessDocument) {
        val contact = _uiState.value.contact ?: return
        val uid = contact.uid ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(error = null) }
            paperlessRepository.linkDocument(uid, document.id.toString()).foldApiError(
                onSuccess = { loadExternalLinks(contact) },
                onError = { error -> _uiState.update { it.copy(error = error.displayMessage) } },
            )
        }
    }

    /** Fetch the user's Seafile libraries for the picker's top level. */
    fun loadSeafileLibraries() {
        if (_uiState.value.seafileBrowseLoading) return
        viewModelScope.launch {
            _uiState.update {
                it.copy(seafileBrowseLoading = true, error = null, seafileBrowseRepoId = null, seafileBrowsePath = "/")
            }
            seafileRepository.listLibraries().foldApiError(
                onSuccess = { libraries ->
                    _uiState.update { it.copy(seafileLibraries = libraries, seafileBrowseLoading = false) }
                },
                onError = { error ->
                    _uiState.update { it.copy(seafileBrowseLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    /** Enter a Seafile library or subdirectory (path defaults to the library root). */
    fun enterSeafileDir(repoId: String, path: String = "/") {
        viewModelScope.launch {
            _uiState.update {
                it.copy(seafileBrowseLoading = true, error = null, seafileBrowseRepoId = repoId, seafileBrowsePath = path)
            }
            seafileRepository.listDir(repoId, path).foldApiError(
                onSuccess = { items ->
                    _uiState.update { it.copy(seafileBrowseItems = items, seafileBrowseLoading = false) }
                },
                onError = { error ->
                    _uiState.update { it.copy(seafileBrowseLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    /** Step up one Seafile directory level, or back out to the library list from a root. */
    fun backSeafileDir() {
        val repoId = _uiState.value.seafileBrowseRepoId ?: return
        val parent = parentDirPath(_uiState.value.seafileBrowsePath)
        if (parent == null) {
            loadSeafileLibraries()
        } else {
            enterSeafileDir(repoId, parent)
        }
    }

    /** Link a Seafile file to this contact, then refresh the External Links panel. */
    fun linkSeafileItem(item: SeafileItem) {
        val contact = _uiState.value.contact ?: return
        val uid = contact.uid ?: return
        val repoId = _uiState.value.seafileBrowseRepoId ?: return
        val path = joinDirPath(_uiState.value.seafileBrowsePath, item.name)
        viewModelScope.launch {
            _uiState.update { it.copy(error = null) }
            seafileRepository.linkItem(
                uid,
                SeafileLinkRequest(
                    repoId = repoId,
                    path = path,
                    name = item.name,
                    type = item.type,
                    size = item.size.takeIf { it > 0 },
                    mtime = item.mtime.takeIf { it > 0 },
                ),
            ).foldApiError(
                onSuccess = { loadExternalLinks(contact) },
                onError = { error -> _uiState.update { it.copy(error = error.displayMessage) } },
            )
        }
    }

    /** Browse a Nextcloud directory (path defaults to the dav root). */
    fun loadNextcloudDir(path: String = "/") {
        viewModelScope.launch {
            _uiState.update { it.copy(nextcloudBrowseLoading = true, error = null, nextcloudBrowsePath = path) }
            nextcloudRepository.listDir(path).foldApiError(
                onSuccess = { items ->
                    _uiState.update { it.copy(nextcloudBrowseItems = items, nextcloudBrowseLoading = false) }
                },
                onError = { error ->
                    _uiState.update { it.copy(nextcloudBrowseLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    /** Link a Nextcloud file to this contact, then refresh the External Links panel. */
    fun linkNextcloudItem(item: WebDAVItem) {
        val contact = _uiState.value.contact ?: return
        val uid = contact.uid ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(error = null) }
            nextcloudRepository.linkItem(uid, item).foldApiError(
                onSuccess = { loadExternalLinks(contact) },
                onError = { error -> _uiState.update { it.copy(error = error.displayMessage) } },
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

    /**
     * Upload a new profile photo (bytes read from the gallery picker, ≤
     * [MAX_PHOTO_SIZE_BYTES] — the screen guards the size before reading, the
     * server re-enforces it). On success the contact is reloaded so the new
     * `photoThumbnail`/`card.photoUri` renders immediately (mirroring web's
     * post-upload refresh); failures (size, demo-mode 403, network) surface
     * through the existing snackbar path.
     */
    fun uploadPhoto(bytes: ByteArray, mimeType: String) {
        if (contactId == 0 || _uiState.value.isMutating) return
        viewModelScope.launch {
            _uiState.update { it.copy(isMutating = true, error = null) }
            contactRepository.uploadPhoto(contactId, bytes, mimeType).foldApiError(
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

    // --- Issue #212: favorite toggle (web #173) ---

    /**
     * Toggles the contact's favorite flag in the detail header, mirroring web
     * `ContactDetailPage.handleToggleFavorite`: optimistic into state, with a
     * rollback on failure so the star can never silently disagree with the
     * database. The repository updates the Room mirror on success so the
     * flag survives offline; the next full load reconciles from the server.
     */
    fun toggleFavorite() {
        val contact = _uiState.value.contact ?: return
        val wasFavorite = contact.isFavorite
        viewModelScope.launch {
            _uiState.update { it.copy(contact = contact.copy(isFavorite = !wasFavorite)) }
            val result = if (wasFavorite) {
                contactRepository.unfavoriteContact(contactId)
            } else {
                contactRepository.favoriteContact(contactId)
            }
            result.foldApiError(
                onSuccess = {},
                onError = { error ->
                    _uiState.update {
                        it.copy(contact = it.contact?.copy(isFavorite = wasFavorite), error = error.displayMessage)
                    }
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

    companion object {
        /**
         * The 10MB cap the backend's `AddPhotoToContact` enforces
         * (`backend/controllers/photo_controller.go`). The screen probes the
         * picked file's declared size against this BEFORE reading it into
         * memory (reading a multi-GB video would OOM the app), and keeps the
         * same bound as a post-read backstop for providers that report no size.
         */
        const val MAX_PHOTO_SIZE_BYTES = 10 * 1024 * 1024
    }
}
