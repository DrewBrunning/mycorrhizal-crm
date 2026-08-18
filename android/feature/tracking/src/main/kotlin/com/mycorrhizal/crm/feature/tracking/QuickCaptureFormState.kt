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
 */
class QuickCaptureFormState(
    private val activityRepository: ActivityRepository,
    private val scope: CoroutineScope,
    val prefill: QuickCapturePrefill,
    private val blankTitleMessage: String = "Title is required",
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

    fun onTitleChange(value: String) {
        title = value
        error = null
    }

    fun onTypeChange(value: String) {
        type = value
        error = null
    }

    fun onDescriptionChange(value: String) {
        description = value
        error = null
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
