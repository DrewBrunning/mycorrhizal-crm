package com.mycorrhizal.crm.feature.settings

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.FieldDefinitionRepository
import com.mycorrhizal.crm.model.network.FIELD_GROUP_MAX_LENGTH
import com.mycorrhizal.crm.model.network.FieldConstraints
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.FieldDefinitionInput
import com.mycorrhizal.crm.model.network.fieldGroupSuggestions
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

private const val PROJECTION_INTERNAL = "internal"
private const val PROJECTION_VCARD = "vcard"
private const val PROJECTION_VCARD_PREFIX = "vcard:X-"
const val FIELD_SENSITIVITY_NORMAL = "normal"

/**
 * Issue #830: create/edit form state, direct port of web's `FieldDefinitionDialog`'s `FormState`
 * (`frontend/src/components/FieldDefinitionDialog.tsx:36-64`). [key] is disabled in the UI once
 * [isEdit] — the backend ignores a changed key on update anyway (see `FieldDefinitionInput`'s doc
 * comment), so the form never even offers to change it.
 */
data class FieldDefinitionFormState(
    val fieldDefinitionId: String? = null,
    val label: String = "",
    val key: String = "",
    val type: String = "string",
    val multi: Boolean = false,
    val min: String = "",
    val max: String = "",
    val maxLength: String = "",
    val pattern: String = "",
    val enumValues: List<String> = emptyList(),
    val projectionMode: String = PROJECTION_INTERNAL,
    val vcardName: String = "",
    val sensitivity: String = FIELD_SENSITIVITY_NORMAL,
    /** Optional display-only group label (issue #1731); blank = ungrouped. */
    val group: String = "",
    /** The user's existing group labels, offered as suggestions (free entry still allowed). */
    val groupSuggestions: List<String> = emptyList(),
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val isDeleting: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
    /** Issue #1751: the editable state the form opened with, for [isDirty]. */
    val baseline: FieldDefinitionFormState? = null,
) {
    val isEdit: Boolean get() = fieldDefinitionId != null

    val isDirty: Boolean get() = baseline != null && editableSnapshot() != baseline

    private fun editableSnapshot(): FieldDefinitionFormState = copy(
        groupSuggestions = emptyList(),
        isLoading = false,
        isSaving = false,
        isDeleting = false,
        errorRes = null,
        error = null,
        baseline = null,
    )

    fun snapshotForBaseline(): FieldDefinitionFormState = editableSnapshot()

    /** Validation order/messages mirror `FieldDefinitionDialog.tsx:122-138` exactly. */
    fun validate(): Int? = when {
        label.isBlank() -> R.string.settings_custom_fields_error_label_required
        group.trim().length > FIELD_GROUP_MAX_LENGTH -> R.string.settings_custom_fields_error_group_too_long
        !isEdit && key.isBlank() -> R.string.settings_custom_fields_error_key_required
        type == "enum" && enumValues.none { it.isNotBlank() } -> R.string.settings_custom_fields_error_enum_required
        projectionMode == PROJECTION_VCARD && vcardName.isBlank() ->
            R.string.settings_custom_fields_error_vcard_name_required
        else -> null
    }

    /** Builds the request body — only the constraint fields relevant to [type] are populated,
     *  mirroring `FieldDefinitionDialog.tsx:140-153`'s conditional constraint-building exactly. */
    fun toInput(): FieldDefinitionInput {
        val constraints = FieldConstraints(
            min = if (type == "number") min.toDoubleOrNull() else null,
            max = if (type == "number") max.toDoubleOrNull() else null,
            maxLength = if (type == "string" || type == "text") maxLength.toIntOrNull() else null,
            pattern = if (type == "string" || type == "text") pattern.trim().ifBlank { null } else null,
            values = if (type == "enum") {
                enumValues.map { it.trim() }.filter { it.isNotBlank() }.ifEmpty { null }
            } else {
                null
            },
            multi = multi,
        )
        val projection = if (projectionMode == PROJECTION_VCARD) {
            "$PROJECTION_VCARD_PREFIX${vcardName.trim()}"
        } else {
            "internal-only"
        }
        return FieldDefinitionInput(
            label = label.trim(),
            key = key.trim(),
            type = type,
            constraints = constraints,
            projection = projection,
            sensitivity = sensitivity,
            // Always sent (never null) so an edit can't silently keep a group the user cleared:
            // the server treats an omitted key as "keep", "" as "clear".
            group = group.trim(),
        )
    }

    /** Prefills the form from a loaded [definition] — the inverse of [toInput], mirroring
     *  `FieldDefinitionDialog.tsx:82-106`'s edit-mode `useEffect`. */
    fun fromDefinition(definition: FieldDefinition): FieldDefinitionFormState {
        val constraints = definition.constraints ?: FieldConstraints()
        val projection = definition.projection ?: "internal-only"
        return copy(
            fieldDefinitionId = definition.id,
            label = definition.label.orEmpty(),
            key = definition.key.orEmpty(),
            type = definition.type ?: "string",
            multi = constraints.multi,
            min = constraints.min?.let { formatConstraintNumber(it) } ?: "",
            max = constraints.max?.let { formatConstraintNumber(it) } ?: "",
            maxLength = constraints.maxLength?.toString() ?: "",
            pattern = constraints.pattern.orEmpty(),
            enumValues = constraints.values.orEmpty(),
            projectionMode = if (projection.startsWith("vcard:")) PROJECTION_VCARD else PROJECTION_INTERNAL,
            vcardName = projection.removePrefix(PROJECTION_VCARD_PREFIX),
            sensitivity = definition.sensitivity ?: FIELD_SENSITIVITY_NORMAL,
            group = definition.group,
        )
    }
}

/** Renders a constraint `Double` without a trailing ".0" for a whole number, matching how the
 *  user is expected to have typed it (mirrors `FieldDefinition.kt`'s `scalarValueDisplay`). */
private fun formatConstraintNumber(value: Double): String =
    if (value == Math.floor(value) && !value.isInfinite()) value.toLong().toString() else value.toString()

sealed interface FieldDefinitionFormEvent {
    data object Saved : FieldDefinitionFormEvent
    data object Deleted : FieldDefinitionFormEvent
}

@HiltViewModel
class FieldDefinitionFormViewModel @Inject constructor(
    private val repository: FieldDefinitionRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val fieldDefinitionId: String? = run {
        val raw: Any? = savedStateHandle["fieldDefinitionId"]
        (raw as? String)?.takeIf { it.isNotBlank() }
    }

    private val _uiState = MutableStateFlow(
        FieldDefinitionFormState(fieldDefinitionId = fieldDefinitionId)
            .let { it.copy(baseline = it.snapshotForBaseline()) },
    )
    val uiState: StateFlow<FieldDefinitionFormState> = _uiState.asStateFlow()

    private val _events = MutableStateFlow<FieldDefinitionFormEvent?>(null)
    val events: StateFlow<FieldDefinitionFormEvent?> = _events

    init {
        if (fieldDefinitionId != null) loadExisting(fieldDefinitionId)
        loadGroupSuggestions()
    }

    /** Best-effort: a failure just means no suggestions, free entry still works. */
    private fun loadGroupSuggestions() {
        viewModelScope.launch {
            repository.list().foldApiError(
                onSuccess = { definitions ->
                    _uiState.update { it.copy(groupSuggestions = fieldGroupSuggestions(definitions)) }
                },
                onError = { },
            )
        }
    }

    private fun loadExisting(id: String) {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, errorRes = null, error = null) }
            repository.get(id).foldApiError(
                onSuccess = { definition ->
                    _uiState.update { state ->
                        val loaded = state.fromDefinition(definition).copy(isLoading = false)
                        loaded.copy(baseline = loaded.snapshotForBaseline())
                    }
                },
                onError = { error ->
                    _uiState.update { it.copy(isLoading = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun onLabelChange(value: String) = _uiState.update { it.copy(label = value) }
    fun onKeyChange(value: String) = _uiState.update { if (it.isEdit) it else it.copy(key = value) }
    fun onTypeChange(value: String) = _uiState.update { it.copy(type = value) }
    fun onMultiChange(value: Boolean) = _uiState.update { it.copy(multi = value) }
    fun onMinChange(value: String) = _uiState.update { it.copy(min = value) }
    fun onMaxChange(value: String) = _uiState.update { it.copy(max = value) }
    fun onMaxLengthChange(value: String) = _uiState.update { it.copy(maxLength = value) }
    fun onPatternChange(value: String) = _uiState.update { it.copy(pattern = value) }
    fun onProjectionModeChange(value: String) = _uiState.update { it.copy(projectionMode = value) }
    fun onVcardNameChange(value: String) = _uiState.update { it.copy(vcardName = value) }
    fun onSensitivityChange(value: String) = _uiState.update { it.copy(sensitivity = value) }
    fun onGroupChange(value: String) = _uiState.update { it.copy(group = value) }

    fun addEnumValue() = _uiState.update { it.copy(enumValues = it.enumValues + "") }
    fun updateEnumValue(index: Int, value: String) = _uiState.update {
        it.copy(enumValues = it.enumValues.mapIndexed { i, v -> if (i == index) value else v })
    }
    fun removeEnumValue(index: Int) = _uiState.update {
        it.copy(enumValues = it.enumValues.filterIndexed { i, _ -> i != index })
    }

    fun onErrorShown() = _uiState.update { it.copy(errorRes = null, error = null) }

    fun save() {
        val state = _uiState.value
        if (state.isSaving) return

        val problem = state.validate()
        if (problem != null) {
            _uiState.update { it.copy(errorRes = problem, error = null) }
            return
        }

        val input = state.toInput()
        _uiState.update { it.copy(isSaving = true, errorRes = null, error = null) }
        viewModelScope.launch {
            val result = if (state.fieldDefinitionId != null) {
                repository.update(state.fieldDefinitionId, input)
            } else {
                repository.create(input)
            }
            result.foldApiError(
                onSuccess = {
                    _uiState.update { it.copy(isSaving = false) }
                    _events.value = FieldDefinitionFormEvent.Saved
                },
                onError = { error ->
                    _uiState.update { it.copy(isSaving = false, error = error.displayMessage) }
                },
            )
        }
    }

    /**
     * Issue #1775: delete lives on the edit screen now, not as a per-row list action.
     * Emits [FieldDefinitionFormEvent.Deleted] so the caller can pop back; a failure keeps
     * the form open and surfaces the error like a save failure.
     */
    fun delete() {
        val id = _uiState.value.fieldDefinitionId ?: return
        if (_uiState.value.isDeleting) return
        viewModelScope.launch {
            _uiState.update { it.copy(isDeleting = true, errorRes = null, error = null) }
            repository.delete(id).foldApiError(
                onSuccess = {
                    _uiState.update { it.copy(isDeleting = false) }
                    _events.value = FieldDefinitionFormEvent.Deleted
                },
                onError = { error ->
                    _uiState.update { it.copy(isDeleting = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun onEventShown() {
        _events.value = null
    }
}
