package com.mycorrhizal.crm.feature.occasions

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.OccasionObligationRepository
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.model.network.OccasionObligation
import com.mycorrhizal.crm.model.network.OccasionObligationInput
import com.mycorrhizal.crm.model.network.OccasionObligationKinds
import com.mycorrhizal.crm.model.registry.OccasionSensitivity
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

private const val MAX_LEAD_TIME_DAYS = 365

/** What the obligation form edits. [leadTimeText] is the raw field text, validated on save. */
data class OccasionObligationFormData(
    val kind: String = OccasionObligationKinds.CARD,
    val label: String = "",
    val anchorMonth: Int? = null,
    val anchorDay: Int? = null,
    val leadTimeText: String = "0",
    val active: Boolean = true,
    val sensitivity: String = OccasionSensitivity.NORMAL,
    val notes: String = ""
)

data class ContactOccasionsUiState(
    /** The contact's `VCardUID`, the `entity_id` every obligation is keyed by. */
    val entityId: String = "",
    val items: List<OccasionObligation> = emptyList(),
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val deletingId: String? = null,
    /** Set after a successful create/update so the screen can close the form; see [ContactOccasionsViewModel.onSavedHandled]. */
    val saved: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: ApiError? = null
)

/**
 * The per-contact standing-obligation list (docs/adrs/0024-occasions.md, issue #2521): the
 * Android counterpart of web's contact-page `OccasionObligationList`. Obligations are managed per
 * contact (not from the Occasions page), keyed by the contact's `VCardUID`.
 */
@HiltViewModel
class ContactOccasionsViewModel @Inject constructor(
    private val repository: OccasionObligationRepository,
    private val contactRepository: ContactRepository,
    savedStateHandle: SavedStateHandle
) : ViewModel() {

    private val contactId: Int = (savedStateHandle["contactId"] as? Int) ?: 0
    private val _uiState = MutableStateFlow(ContactOccasionsUiState())
    val uiState: StateFlow<ContactOccasionsUiState> = _uiState.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null, errorRes = null) }
            val uid = contactRepository.getContact(contactId).getOrNull()?.vcardUid
            if (uid.isNullOrBlank()) {
                _uiState.update {
                    it.copy(isLoading = false, errorRes = R.string.entities_error_no_vcard_uid)
                }
                return@launch
            }
            _uiState.update { it.copy(entityId = uid) }
            reload(uid)
        }
    }

    private suspend fun reload(uid: String) {
        repository.listForContact(uid).foldApiError(
            onSuccess = { items ->
                _uiState.update { it.copy(isLoading = false, items = items) }
            },
            onError = { error -> _uiState.update { it.copy(isLoading = false, error = error) } }
        )
    }

    /** Create ([original] null) or fully replace ([original] set) one obligation. */
    fun save(original: OccasionObligation?, form: OccasionObligationFormData) {
        val state = _uiState.value
        if (state.isSaving || state.entityId.isBlank()) return
        val leadTime = validate(original, form) ?: return
        val input = OccasionObligationInput(
            entityId = state.entityId,
            kind = form.kind.trim().ifEmpty { OccasionObligationKinds.CARD },
            label = form.label.trim(),
            anchorMonth = form.anchorMonth,
            anchorDay = form.anchorDay,
            // The form never shows the link, but PUT is a full replace: carry it forward.
            linkedLifeEventId = original?.linkedLifeEventId,
            leadTimeDays = leadTime,
            active = form.active,
            sensitivity = form.sensitivity,
            notes = form.notes.trim().ifEmpty { null }
        )
        // Flip the guard synchronously: a second tap before the coroutine starts must not re-send.
        _uiState.update { it.copy(isSaving = true, error = null, errorRes = null) }
        viewModelScope.launch {
            val result = if (original == null) {
                repository.create(input)
            } else {
                repository.update(original.id, input)
            }
            result.foldApiError(
                onSuccess = {
                    _uiState.update { it.copy(isSaving = false, saved = true) }
                    viewModelScope.launch { reload(state.entityId) }
                },
                onError = { error -> _uiState.update { it.copy(isSaving = false, error = error) } }
            )
        }
    }

    /** Returns the parsed lead time, or null after posting the first validation error. */
    private fun validate(original: OccasionObligation?, form: OccasionObligationFormData): Int? {
        val errors = validateOccasionForm(form, original)
        if (errors.any) {
            _uiState.update { it.copy(errorRes = errors.firstMessage, error = null) }
            return null
        }
        return parseLeadTime(form.leadTimeText)
    }

    fun delete(id: String) {
        if (_uiState.value.deletingId != null) return
        _uiState.update { it.copy(deletingId = id, error = null, errorRes = null) }
        viewModelScope.launch {
            repository.delete(id).foldApiError(
                onSuccess = {
                    _uiState.update { it.copy(deletingId = null) }
                    viewModelScope.launch { reload(_uiState.value.entityId) }
                },
                onError = { error ->
                    _uiState.update { it.copy(deletingId = null, error = error) }
                }
            )
        }
    }

    fun onSavedHandled() {
        _uiState.update { it.copy(saved = false) }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null, errorRes = null) }
    }
}

/**
 * The longest [month] gets in a yearly recurrence; February is 29 so a leap-day anchor stays
 * expressible, and no month yet (null) allows 31. Mirrors the backend's `occasionMaxDay` table
 * (controllers/occasion_obligation_controller.go) -- keep the two in sync.
 */
internal fun daysInMonth(month: Int?): Int = when (month) {
    2 -> 29
    4, 6, 9, 11 -> 30
    else -> 31
}

/** Per-field validation results for the obligation form; a null field is valid. */
internal data class OccasionFormErrors(
    @StringRes val label: Int? = null,
    @StringRes val month: Int? = null,
    @StringRes val day: Int? = null,
    /** Marks the Day field invalid without text of its own (the Month field carries it). */
    val dayFlagged: Boolean = false,
    @StringRes val leadTime: Int? = null
) {
    val any: Boolean get() = firstMessage != null

    @get:StringRes
    val firstMessage: Int? get() = label ?: month ?: day ?: leadTime
}

/** The anchor-date rule shared with the server (ADR 0024; issues #2648 #2670 #2671). */
internal fun validateOccasionForm(
    form: OccasionObligationFormData,
    original: OccasionObligation?
): OccasionFormErrors {
    val month = form.anchorMonth
    val day = form.anchorDay
    // A row saved before dates were required may still be retired (saved inactive, still
    // dateless); anything else needs a real month and day, as the server decides.
    val legacyRetire = original != null && original.anchorMonth == null &&
        original.anchorDay == null && !form.active
    var errors = OccasionFormErrors(
        label = R.string.occasion_ob_validation_label_required.takeIf { form.label.isBlank() },
        leadTime = R.string.occasion_ob_validation_lead_time
            .takeIf { parseLeadTime(form.leadTimeText) == null }
    )
    errors = when {
        month == null && day == null ->
            if (legacyRetire) {
                errors
            } else {
                errors.copy(
                    month = R.string.error_reason_occasion_anchor_date_required,
                    dayFlagged = true
                )
            }

        month == null ->
            errors.copy(month = R.string.error_reason_occasion_anchor_date_incomplete)

        day == null ->
            errors.copy(day = R.string.error_reason_occasion_anchor_date_incomplete)

        day > daysInMonth(month) ->
            errors.copy(day = R.string.error_reason_occasion_anchor_date_invalid)

        else -> errors
    }
    return errors
}

/** The lead time in days (blank means 0), or null when it is not a whole number from 0 to 365. */
internal fun parseLeadTime(text: String): Int? =
    text.trim().ifEmpty { "0" }.toIntOrNull()?.takeIf { it in 0..MAX_LEAD_TIME_DAYS }

/** Which form field a server anchor-date rejection belongs on (mirrors the web dialog). */
internal enum class AnchorField { MONTH, DAY }

internal fun ApiError.anchorDateField(): AnchorField? {
    val code = when (this) {
        is ApiError.Client -> reason?.code
        is ApiError.Server -> reason?.code
        else -> null
    }
    return when (code) {
        ErrorReasonCode.OCCASION_ANCHOR_DATE_REQUIRED -> AnchorField.MONTH

        ErrorReasonCode.OCCASION_ANCHOR_DATE_INCOMPLETE,
        ErrorReasonCode.OCCASION_ANCHOR_DATE_INVALID -> AnchorField.DAY

        else -> null
    }
}
