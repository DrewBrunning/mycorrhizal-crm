package com.mycorrhizal.crm.feature.tracking

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.network.ApiError
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch

/**
 * M5 §6.5 (issue #151): the in-overlay quick-capture form's editable state,
 * kept OUTSIDE the composable so the save path is unit-testable without
 * Compose (the overlay window has no ViewModelStoreOwner, so a ViewModel is
 * unavailable). The sheet binds its fields to this; the Save button calls
 * [save], which writes the interaction straight to the server so it can be
 * logged without leaving the call screen.
 *
 * [onFirstInteraction] is #201 (WCAG 2.2.1 Timing Adjustable): the overlay
 * auto-dismisses after [QuickCaptureAutoDismiss]'s window so an ignored sheet
 * does not sit on screen forever, but once the user has engaged the form the
 * limit must not keep ticking. The callback is invoked exactly once, on the
 * first field edit, and lets the owner cancel that timer. The sheet
 * additionally fires it on the first focus event, so a TalkBack or
 * switch-access user who navigates to a field without typing also engages.
 */
class QuickCaptureFormState(
    private val activityRepository: ActivityRepository,
    private val scope: CoroutineScope,
    val prefill: QuickCapturePrefill,
    private val blankTitleMessage: String = "Title is required",
    private val onFirstInteraction: () -> Unit = {},
) {
    var title by mutableStateOf(prefill.title)
        private set
    var type by mutableStateOf(prefill.type)
        private set
    var description by mutableStateOf("")
        private set
    var isSaving by mutableStateOf(false)
        private set
    var saved by mutableStateOf(false)
        private set
    var error by mutableStateOf<String?>(null)
        private set
    private var interactionNotified = false

    fun onTitleChange(value: String) {
        title = value
        error = null
        notifyFirstInteraction()
    }

    fun onTypeChange(value: String) {
        type = value
        error = null
        notifyFirstInteraction()
    }

    fun onDescriptionChange(value: String) {
        description = value
        error = null
        notifyFirstInteraction()
    }

    private fun notifyFirstInteraction() {
        if (interactionNotified) return
        interactionNotified = true
        onFirstInteraction()
    }

    fun save() {
        if (isSaving || saved) return
        val trimmedTitle = title.trim()
        if (trimmedTitle.isBlank()) {
            error = blankTitleMessage
            return
        }
        isSaving = true
        error = null
        scope.launch {
            activityRepository.create(
                ActivityInput(
                    title = trimmedTitle,
                    type = type.trim().ifBlank { prefill.type },
                    date = prefill.date,
                    description = description.trim().ifBlank { null },
                    contactIds = prefill.participants.map { it.id }.ifEmpty { null },
                ),
            ).fold(
                onSuccess = { saved = true },
                onFailure = { e ->
                    error = (e as? ApiError)?.displayMessage ?: e.message ?: "error"
                    isSaving = false
                },
            )
        }
    }
}
