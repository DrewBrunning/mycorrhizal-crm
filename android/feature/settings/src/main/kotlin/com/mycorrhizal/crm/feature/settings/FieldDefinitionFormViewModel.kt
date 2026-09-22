package com.mycorrhizal.crm.feature.settings

import androidx.annotation.StringRes
import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.FieldDefinitionRepository
import com.mycorrhizal.crm.model.network.FieldConstraints
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.FieldDefinitionInput
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
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: String? = null,
) {
    val isEdit: Boolean get() = fieldDefinitionId != null

    /** Validation order/messages mirror `FieldDefinitionDialog.tsx:122-138` exactly. */
    fun validate(): Int? = when {
        label.isBlank() -> R.string.settings_custom_fields_error_label_required
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
        )
    }
}

/** Renders a constraint `Double` without a trailing ".0" for a whole number, matching how the
 *  user is expected to have typed it (mirrors `FieldDefinition.kt`'s `scalarValueDisplay`). */
private fun formatConstraintNumber(value: Double): String =
    if (value == Math.floor(value) && !value.isInfinite()) value.toLong().toString() else value.toString()

sealed interface FieldDefinitionFormEvent {
    data object Saved : FieldDefinitionFormEvent
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

    private val _uiState = MutableStateFlow(FieldDefinitionFormState(fieldDefinitionId = fieldDefinitionId))
    val uiState: StateFlow<FieldDefinitionFormState> = _uiState.asStateFlow()

    private val _events = MutableStateFlow<FieldDefinitionFormEvent?>(null)
    val events: StateFlow<FieldDefinitionFormEvent?> = _events

    init {
        if (fieldDefinitionId != null) loadExisting(fieldDefinitionId)
    }

    private fun loadExisting(id: String) {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, errorRes = null, error = null) }
            repository.get(id).foldApiError(
                onSuccess = { definition ->
                    _uiState.update { it.fromDefinition(definition).copy(isLoading = false) }
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

    fun onEventShown() {
        _events.value = null
    }
}
