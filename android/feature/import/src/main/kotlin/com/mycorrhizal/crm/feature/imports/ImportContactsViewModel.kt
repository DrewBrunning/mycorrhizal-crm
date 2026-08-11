package com.mycorrhizal.crm.feature.imports

import android.content.ContentResolver
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import dagger.hilt.android.qualifiers.ApplicationContext
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

data class ImportUiState(
    val contacts: List<DeviceContactCandidate> = emptyList(),
    val selected: Set<Long> = emptySet(),
    val isLoading: Boolean = false,
    val isImporting: Boolean = false,
    val importedCount: Int = 0,
    val error: String? = null,
)

@HiltViewModel
class ImportContactsViewModel @Inject constructor(
    private val contactRepository: ContactRepository,
    @ApplicationContext private val appContext: android.content.Context,
) : ViewModel() {

    private val _uiState = MutableStateFlow(ImportUiState())
    val uiState: StateFlow<ImportUiState> = _uiState.asStateFlow()

    fun load(contentResolver: ContentResolver = appContext.contentResolver) {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            val deviceContacts = withContext(Dispatchers.IO) {
                DeviceContactsReader(contentResolver).readAll()
            }
            // Dedup against the local cache (§7.4): match by email then phone.
            val candidates = deviceContacts.map { device ->
                val duplicate = withContext(Dispatchers.IO) {
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

    fun importSelected() {
        val toImport = _uiState.value.contacts.filter { it.device.contactId in _uiState.value.selected }
        if (toImport.isEmpty() || _uiState.value.isImporting) return
        viewModelScope.launch {
            _uiState.update { it.copy(isImporting = true, error = null) }
            var imported = 0
            for (candidate in toImport) {
                val input = DeviceContactMapper.toInput(candidate.device)
                val result = contactRepository.createContact(input)
                if (result.isSuccess) {
                    imported++
                    // Store the device LOOKUP_KEY so QuickContact + future
                    // re-import dedup can link back (§7.5.4).
                    val newId = result.getOrNull()?.id
                    if (newId != null && newId != 0) {
                        contactRepository.setDeviceLookupKey(newId, candidate.device.lookupKey)
                    }
                }
            }
            _uiState.update {
                it.copy(isImporting = false, importedCount = imported, selected = emptySet())
            }
        }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}
