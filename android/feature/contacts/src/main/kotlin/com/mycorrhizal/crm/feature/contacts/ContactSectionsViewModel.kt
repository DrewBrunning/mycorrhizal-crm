package com.mycorrhizal.crm.feature.contacts

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.ContactFieldKey
import com.mycorrhizal.crm.model.network.ContactRecordInput
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.DEFAULT_ENABLED_CONTACT_FIELDS
import com.mycorrhizal.crm.model.network.resolveEnabledFields
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/**
 * Issue #1728: shared state for the three contact-page drill-downs that moved
 * out of the contact edit form — skills & interests (`Card.personalInfo`),
 * work (`Card.organizations`/`Card.titles`/`crm.work_information`) and
 * anniversaries (non-birth `Card.anniversaries`).
 *
 * Each drill-down loads the full contact record and saves through the existing
 * full-record PUT with `If-Match` (the same path the edit form uses), so there
 * is no backend change. A 412 sets [ContactSectionsUiState.conflict], which the
 * screens render with the shared [com.mycorrhizal.crm.ui.components.WriteConflictDialog].
 */
data class ContactSectionsUiState(
    val record: ContactRecordResponse? = null,
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    /** ADR 0008: the record changed elsewhere since it loaded (PUT rejected with 412). */
    val conflict: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
    /** The signed-in user's date-format preference for the anniversary list. */
    val dateFormat: String? = null,
    /** Issue #832: which extended fields the settings screen has enabled. */
    val enabledFields: Set<ContactFieldKey> = DEFAULT_ENABLED_CONTACT_FIELDS,
)

@HiltViewModel
class ContactSectionsViewModel @Inject constructor(
    private val contactRepository: ContactRepository,
    authRepository: AuthRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val contactId: Int = run {
        val raw: Any? = savedStateHandle["contactId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull() ?: 0
    }

    private val _uiState = MutableStateFlow(ContactSectionsUiState())
    val uiState: StateFlow<ContactSectionsUiState> = _uiState.asStateFlow()

    init {
        load()
        viewModelScope.launch {
            authRepository.observeSession().collect { session ->
                _uiState.update {
                    it.copy(
                        dateFormat = session.dateFormat,
                        enabledFields = resolveEnabledFields(session.enabledContactFields),
                    )
                }
            }
        }
    }

    fun load() {
        if (contactId == 0) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
            contactRepository.getContact(contactId).foldApiError(
                onSuccess = { record ->
                    _uiState.update { it.copy(record = record, isLoading = false) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun savePersonalInfo(editingId: String?, form: PersonalInfoFormData) {
        if (form.value.isBlank()) {
            _uiState.update { it.copy(errorRes = com.mycorrhizal.crm.ui.R.string.contact_section_value_required) }
            return
        }
        mutate { it.withPersonalInfo(editingId, form) }
    }

    fun deletePersonalInfo(id: String) = mutate { it.withoutPersonalInfo(id) }

    fun saveOrganization(editingId: String?, form: OrganizationFormData) {
        if (form.name.isBlank()) {
            _uiState.update { it.copy(errorRes = com.mycorrhizal.crm.ui.R.string.contact_section_value_required) }
            return
        }
        mutate { it.withOrganization(editingId, form) }
    }

    fun deleteOrganization(id: String) = mutate { it.withoutOrganization(id) }

    fun saveTitle(editingId: String?, form: TitleFormData) {
        if (form.name.isBlank()) {
            _uiState.update { it.copy(errorRes = com.mycorrhizal.crm.ui.R.string.contact_section_value_required) }
            return
        }
        mutate { it.withTitle(editingId, form) }
    }

    fun deleteTitle(id: String) = mutate { it.withoutTitle(id) }

    fun saveWorkInformation(text: String) = mutate { it.withWorkInformation(text) }

    fun saveAnniversary(editingId: String?, form: AnniversaryFormData) {
        if (form.date.isBlank()) {
            _uiState.update { it.copy(errorRes = com.mycorrhizal.crm.ui.R.string.contact_section_date_required) }
            return
        }
        mutate { it.withAnniversary(editingId, form) }
    }

    fun deleteAnniversary(id: String) = mutate { it.withoutAnniversary(id) }

    /**
     * Apply one pure record mapping (see ContactSectionMapping.kt) and PUT it.
     * [transform] receives the latest loaded record, so a save always builds on
     * exactly what the server last returned rather than on stale UI state.
     */
    private fun mutate(transform: (ContactRecordResponse) -> ContactRecordInput) {
        val record = _uiState.value.record ?: return
        if (_uiState.value.isSaving) return
        val input = transform(record)
        _uiState.update { it.copy(isSaving = true, error = null, errorRes = null) }
        viewModelScope.launch {
            contactRepository.updateContact(record.id, input, record.revision).foldApiError(
                onSuccess = { updated ->
                    _uiState.update { it.copy(record = updated, isSaving = false) }
                },
                onError = { error ->
                    _uiState.update {
                        if (error is ApiError.PreconditionFailed) {
                            it.copy(isSaving = false, conflict = true)
                        } else {
                            it.copy(isSaving = false, error = error.displayMessage)
                        }
                    }
                },
            )
        }
    }

    fun onConflictDismissed() = _uiState.update { it.copy(conflict = false) }

    fun onConflictLoadLatest() {
        _uiState.update { it.copy(conflict = false) }
        load()
    }

    fun onErrorShown() = _uiState.update { it.copy(error = null, errorRes = null) }
}
