package com.mycorrhizal.crm.feature.imports

import android.content.ContentResolver
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.ImportConfirmRequest
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.RowImportAction
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import javax.inject.Inject

data class DeviceContactCandidate(
    val device: DeviceContact,
    /** A locally-cached contact this device contact duplicates (§7.4), if any. */
    val duplicateOf: ContactSummary? = null,
)

enum class ImportStep { LIST, REVIEW, RESULT }

data class ImportUiState(
    val contacts: List<DeviceContactCandidate> = emptyList(),
    val selected: Set<Long> = emptySet(),
    val isLoading: Boolean = false,
    val isImporting: Boolean = false,
    val step: ImportStep = ImportStep.LIST,
    val preview: ImportPreviewResponse? = null,
    /** row_index -> "skip" | "add" | "update", editable per row on the review
     *  step (T96); rows with validation errors are forced to "skip". */
    val rowActions: Map<Int, String> = emptyMap(),
    val importedCount: Int = 0,
    val error: String? = null,
)

/**
 * T96: the
 * device-contacts import no longer creates every selected contact
 * unconditionally. Selecting a set now submits them to
 * [ApiClient.uploadImportRecords] — the server's preview pipeline: validation,
 * duplicate detection against existing contacts with a per-row merge diff, and
 * within-batch detection — then the review step lets the user choose Merge /
 * Keep Both / Discard New per row before confirming through the shared VCF
 * confirm endpoint.
 *
 * [readDeviceContacts] and [ioDispatcher] are `internal var` test seams (the
 * Hilt constructor stays limited to injectable dependencies; Dagger cannot
 * provide function types or dispatchers).
 */
@HiltViewModel
class ImportContactsViewModel @Inject constructor(
    private val apiClient: ApiClient,
    private val contactRepository: ContactRepository,
    @ApplicationContext private val appContext: android.content.Context,
) : ViewModel() {

    internal var readDeviceContacts: (ContentResolver) -> List<DeviceContact> =
        { resolver -> DeviceContactsReader(resolver).readAll() }
    internal var ioDispatcher: CoroutineDispatcher = Dispatchers.IO

    private val _uiState = MutableStateFlow(ImportUiState())
    val uiState: StateFlow<ImportUiState> = _uiState.asStateFlow()

    fun load(contentResolver: ContentResolver = appContext.contentResolver) {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            val deviceContacts = withContext(ioDispatcher) {
                readDeviceContacts(contentResolver)
            }
            // Dedup against the local cache (§7.4): match by email then phone.
            // This flag only decorates the LIST step — the actual duplicate
            // decision happens server-side on the review step.
            val candidates = deviceContacts.map { device ->
                val duplicate = withContext(ioDispatcher) {
                    findDuplicate(device)
                }
                DeviceContactCandidate(device, duplicate)
            }
            _uiState.update {
                it.copy(isLoading = false, contacts = candidates, selected = emptySet())
            }
        }
    }

    private suspend fun findDuplicate(device: DeviceContact): ContactSummary? {
        for (email in device.emails) {
            contactRepository.findByEmail(email)?.let { return it }
        }
        for ((number, _) in device.phones) {
            contactRepository.findByPhone(number)?.let { return it }
        }
        return null
    }

    fun toggle(contactId: Long) {
        _uiState.update { state ->
            val next = state.selected.toMutableSet()
            if (!next.add(contactId)) next.remove(contactId)
            state.copy(selected = next)
        }
    }

    /** T96: submit the selected device contacts to the server's preview
     *  pipeline and move to the review step. The old flow created contacts
     *  here directly; that decision now happens per row on review. */
    fun submitSelected() {
        val toImport = _uiState.value.contacts.filter { it.device.contactId in _uiState.value.selected }
        if (toImport.isEmpty() || _uiState.value.isImporting) return
        viewModelScope.launch {
            _uiState.update { it.copy(isImporting = true, error = null) }
            val records = toImport.map { DeviceContactMapper.toInput(it.device) }
            apiClient.uploadImportRecords(records).foldApiError(
                onSuccess = { preview ->
                    val actions = preview.rows.associate { row ->
                        row.rowIndex to if (row.validationErrors.isNotEmpty()) "skip" else row.suggestedAction
                    }
                    _uiState.update {
                        it.copy(isImporting = false, step = ImportStep.REVIEW, preview = preview, rowActions = actions)
                    }
                },
                onError = { error -> _uiState.update { it.copy(isImporting = false, error = error.displayMessage) } },
            )
        }
    }

    /** No-op for a row with validation errors — it stays forced to "skip". */
    fun setRowAction(rowIndex: Int, action: String) {
        val row = _uiState.value.preview?.rows?.find { it.rowIndex == rowIndex } ?: return
        if (row.validationErrors.isNotEmpty()) return
        _uiState.update { it.copy(rowActions = it.rowActions + (rowIndex to action)) }
    }

    /** "Resolve all as merged": every valid row takes its suggested action. */
    fun resolveAll() {
        val preview = _uiState.value.preview ?: return
        _uiState.update { state ->
            val next = state.rowActions.toMutableMap()
            preview.rows.forEach { row ->
                if (row.validationErrors.isEmpty()) next[row.rowIndex] = row.suggestedAction
            }
            state.copy(rowActions = next)
        }
    }

    fun confirmImport() {
        val preview = _uiState.value.preview ?: return
        if (_uiState.value.isImporting) return
        viewModelScope.launch {
            _uiState.update { it.copy(isImporting = true, error = null) }
            val actions = _uiState.value.rowActions.map { (rowIndex, action) -> RowImportAction(rowIndex, action) }
            apiClient.confirmVcfImport(ImportConfirmRequest(sessionId = preview.sessionId, actions = actions)).foldApiError(
                onSuccess = { result ->
                    _uiState.update {
                        it.copy(isImporting = false, step = ImportStep.RESULT, importedCount = result.created + result.updated)
                    }
                },
                onError = { error -> _uiState.update { it.copy(isImporting = false, error = error.displayMessage) } },
            )
        }
    }

    /** Returns to the LIST step so the user can import again. Reloads the
     *  device contacts so the just-imported ones' duplicate flags reflect the
     *  fresh server state rather than the stale pre-import cache. */
    fun startOver() {
        _uiState.update {
            it.copy(step = ImportStep.LIST, preview = null, rowActions = emptyMap(), importedCount = 0, selected = emptySet())
        }
        load()
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}
