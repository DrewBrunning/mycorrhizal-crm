package com.mycorrhizal.crm.feature.timeline

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ReminderRepository
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.network.ReminderRecurrence
import com.mycorrhizal.crm.model.util.DateFieldValue
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

/**
 * Editable form state for a reminder. `message`, `remind_at`, `recurrence`
 * are required by the backend. `reoccur_from_completion` mirrors web's
 * ReminderDialog default (true); it only applies to recurring reminders.
 */
data class ReminderFormState(
    val contactId: Int = 0,
    val reminderId: Int? = null,
    val message: String = "",
    val remindAt: String = "",
    val recurrence: String = ReminderRecurrence.ONCE,
    val byMail: Boolean = false,
    val reoccurFromCompletion: Boolean = true,
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
    /** ADR 0008: the `revision` the record had when this edit began; sent as `If-Match` on save. */
    val revision: Int? = null,
    /** ADR 0008: the save was rejected with 412 — the record changed elsewhere since it loaded. */
    val conflict: Boolean = false,
    /** Issue #1751: the editable state the form opened with, for [isDirty]. */
    val baseline: ReminderFormState? = null
) {
    val isEdit: Boolean get() = reminderId != null
    val hasMessage: Boolean get() = message.isNotBlank()

    val isDirty: Boolean get() = baseline != null && editableSnapshot() != baseline

    private fun editableSnapshot(): ReminderFormState = copy(
        isLoading = false,
        isSaving = false,
        errorRes = null,
        error = null,
        conflict = false,
        baseline = null
    )

    fun snapshotForBaseline(): ReminderFormState = editableSnapshot()

    fun toReminder(): Reminder = Reminder(
        id = reminderId ?: 0,
        message = message.trim(),
        remindAt = DateFieldValue.normalizeCalendarDayInstant(remindAt),
        recurrence = recurrence,
        byMail = byMail,
        reoccurFromCompletion = reoccurFromCompletion,
        contactId = contactId.takeIf { it != 0 }
    )

    fun validate(): Int? = when {
        !hasMessage -> R.string.reminder_error_message
        remindAt.isBlank() -> R.string.reminder_error_remind_at_required
        recurrence !in ReminderRecurrence.ALL -> R.string.reminder_error_recurrence
        else -> null
    }

    companion object {
        /**
         * Web's `getDateForRecurrence` (ReminderDialog.tsx): the default due
         * date on create for a chosen recurrence, computed from today. Returns
         * `YYYY-MM-DD`; the form stores full ISO-8601, so callers append the
         * time part.
         */
        fun dateForRecurrence(recurrence: String): String {
            val today = java.time.LocalDate.now()
            return when (recurrence) {
                ReminderRecurrence.WEEKLY -> today.plusWeeks(1)
                ReminderRecurrence.MONTHLY -> today.plusMonths(1)
                ReminderRecurrence.QUARTERLY -> today.plusMonths(3)
                ReminderRecurrence.SIX_MONTHS -> today.plusMonths(6)
                ReminderRecurrence.YEARLY -> today.plusYears(1)
                else -> today
            }.toString()
        }
    }
}

sealed interface ReminderFormEvent {
    data object Saved : ReminderFormEvent
}

@HiltViewModel
class ReminderFormViewModel @Inject constructor(
    private val reminderRepository: ReminderRepository,
    savedStateHandle: SavedStateHandle
) : ViewModel() {

    private val contactId: Int = run {
        val raw: Any? = savedStateHandle["contactId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull() ?: 0
    }

    private val reminderId: Int? = run {
        val raw: Any? = savedStateHandle["reminderId"]
        (raw as? Int) ?: (raw as? String)?.toIntOrNull()
    }

    // M24 stay-in-touch: the detail screen navigates here with a prefilled message +
    // recurrence ("Catch up with <name>", quarterly) so the one-tap action only needs a date.
    private val prefillMessage: String? = run {
        val raw: Any? = savedStateHandle["message"]
        (raw as? String)?.takeIf { it.isNotBlank() }
    }

    private val prefillRecurrence: String? = run {
        val raw: Any? = savedStateHandle["recurrence"]
        (raw as? String)?.takeIf { it in ReminderRecurrence.ALL }
    }

    private val _uiState = MutableStateFlow(
        ReminderFormState(
            contactId = contactId,
            reminderId = reminderId,
            message = prefillMessage.orEmpty(),
            recurrence = prefillRecurrence ?: ReminderRecurrence.ONCE,
            // Web's ReminderDialog prefills the due date on create (getDateForRecurrence
            // of the initial recurrence — today for `once`). M20 mirrors that; the
            // create form never opens with an empty date.
            remindAt = if (reminderId == null) {
                "${ReminderFormState.dateForRecurrence(
                    prefillRecurrence ?: ReminderRecurrence.ONCE
                )}T00:00:00Z"
            } else {
                ""
            }
        ).let { it.copy(baseline = it.snapshotForBaseline()) }
    )
    val uiState: StateFlow<ReminderFormState> = _uiState.asStateFlow()

    private val _events = MutableStateFlow<ReminderFormEvent?>(null)
    val events: StateFlow<ReminderFormEvent?> = _events

    init {
        if (reminderId != null) loadExisting()
    }

    fun loadExisting() {
        val id = reminderId ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, errorRes = null, error = null) }
            reminderRepository.get(id).foldApiError(
                onSuccess = { reminder ->
                    _uiState.update { state ->
                        val loaded = state.toFormState(reminder).copy(isLoading = false)
                        loaded.copy(baseline = loaded.snapshotForBaseline())
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                }
            )
        }
    }

    fun onMessageChange(value: String) = _uiState.update { it.copy(message = value) }
    fun onRemindAtChange(value: String) = _uiState.update { it.copy(remindAt = value) }

    /**
     * Mirrors web's `handleRecurrenceChange` (ReminderDialog.tsx:81-86): in
     * create mode, choosing a recurrence auto-fills the due date with the
     * recurrence's default offset (weekly → +1 week, ...). Web does this for
     * **every** recurrence — including switching back to `once`, which resets
     * to today — so the Android mirror does too. Edit mode never overwrites
     * the existing date.
     */
    fun onRecurrenceChange(value: String) = _uiState.update { state ->
        if (state.reminderId == null) {
            state.copy(
                recurrence = value,
                remindAt = "${ReminderFormState.dateForRecurrence(value)}T00:00:00Z"
            )
        } else {
            state.copy(recurrence = value)
        }
    }

    fun onByMailChange(value: Boolean) = _uiState.update { it.copy(byMail = value) }
    fun onReoccurFromCompletionChange(value: Boolean) = _uiState.update {
        it.copy(
            reoccurFromCompletion = value
        )
    }
    fun onErrorShown() = _uiState.update { it.copy(errorRes = null, error = null) }

    /** Conflict dialog "Keep editing": leave the form as typed (nothing was saved). */
    fun onConflictKeepEditing() = _uiState.update { it.copy(conflict = false) }

    /** Conflict dialog "Load latest version": discard the edit and refetch the server's copy. */
    fun onConflictLoadLatest() {
        _uiState.update { it.copy(conflict = false) }
        loadExisting()
    }

    fun save() {
        val state = _uiState.value
        if (state.isSaving) return

        val problem = state.validate()
        if (problem != null) {
            _uiState.update { it.copy(errorRes = problem, error = null) }
            return
        }

        val reminder = state.toReminder()
        _uiState.update { it.copy(isSaving = true, errorRes = null, error = null) }
        viewModelScope.launch {
            val result = if (state.reminderId != null) {
                reminderRepository.update(state.reminderId, reminder, state.revision)
            } else {
                reminderRepository.create(state.contactId, reminder)
            }
            result.foldApiError(
                onSuccess = {
                    _uiState.update {
                        it.copy(
                            isSaving = false
                        )
                    }
                    _events.value = ReminderFormEvent.Saved
                },
                onError = { error ->
                    _uiState.update {
                        if (error is ApiError.PreconditionFailed) {
                            it.copy(isSaving = false, conflict = true)
                        } else {
                            it.copy(isSaving = false, error = error.displayMessage)
                        }
                    }
                }
            )
        }
    }

    fun onSaveShown() {
        _events.value = null
    }

    private fun ReminderFormState.toFormState(reminder: Reminder): ReminderFormState = copy(
        message = reminder.message.orEmpty(),
        remindAt = reminder.remindAt.orEmpty(),
        recurrence = reminder.recurrence ?: ReminderRecurrence.ONCE,
        byMail = reminder.byMail ?: false,
        reoccurFromCompletion = reminder.reoccurFromCompletion ?: true,
        revision = reminder.revision
    )
}
