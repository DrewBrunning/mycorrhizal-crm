package com.mycorrhizal.crm.feature.imports

import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.model.network.DiscoveredCustomProperty
import com.mycorrhizal.crm.model.network.FIELD_TYPES
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.IMPORT_FIELD_ACTION_CREATE
import com.mycorrhizal.crm.model.network.IMPORT_FIELD_ACTION_IGNORE
import com.mycorrhizal.crm.model.network.IMPORT_FIELD_ACTION_MAP
import com.mycorrhizal.crm.model.network.ImportConfirmRequest
import com.mycorrhizal.crm.model.network.ImportFieldMapping
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.ImportRowEdit
import com.mycorrhizal.crm.model.network.RowImportAction
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.foldApiError
import com.mycorrhizal.crm.network.toApiError
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** What the custom fields step can ask of its owner; [VcfImportViewModel] implements it. */
interface CustomFieldActions {
    fun setFieldMappingAction(propertyName: String, action: String)

    fun setFieldMappingDefinition(propertyName: String, definitionId: String)

    fun setFieldMappingLabel(propertyName: String, label: String)

    fun setFieldMappingType(propertyName: String, type: String)

    fun continueFromCustomFields()
}

enum class VcfImportStep { PICK, CUSTOM_FIELDS, PREVIEW, RESULT }

data class VcfImportUiState(
    val step: VcfImportStep = VcfImportStep.PICK,
    val fileName: String? = null,
    val preview: ImportPreviewResponse? = null,
    /** row_index -> "skip" | "add" | "update", editable per row; rows with validation errors are
     *  forced to "skip" at load time (mirrors web's "Errors are disabled in the UI and always
     *  stay skip" — `ImportContactsDialog.tsx`'s `handleAcceptAll`/`handleSkipAll`). */
    val rowActions: Map<Int, String> = emptyMap(),
    /** Issue #2186: row_index -> the user's fix for a row that failed validation. */
    val rowEdits: Map<Int, ImportRowEdit> = emptyMap(),
    /** Issue #514: the X-* properties discovered in the file (union over rows, by name). */
    val customFieldCandidates: List<DiscoveredCustomProperty> = emptyList(),
    /** The custom-field definitions a property can map to (fetched, plus any server-matched one). */
    val fieldDefinitions: List<FieldDefinition> = emptyList(),
    /** property name -> the user's decision; every candidate always has an entry. */
    val fieldMappings: Map<String, ImportFieldMapping> = emptyMap(),
    val result: ImportResult? = null,
    val isLoading: Boolean = false,
    @StringRes val errorRes: Int? = null,
    val error: ApiError? = null
) {
    /** A "map" decision needs a chosen definition before the user can move on to review. */
    val canContinueFromCustomFields: Boolean
        get() = fieldMappings.values.none {
            it.action == IMPORT_FIELD_ACTION_MAP && it.fieldDefinitionId.isNullOrEmpty()
        }
}

/**
 * M9 item 4: VCF-file import. `uploadVcfImport`/`ImportPreviewResponse`/`ImportConfirmRequest`/
 * `ImportResult`/`RowImportAction` already existed with zero UI callers; this ViewModel (and the
 * new `confirmVcfImport` client method) is their first real caller. No repository layer exists
 * for import (same precedent as `ContactListViewModel` injecting `ApiClient` directly for
 * `/search` — a composite flow with no single-entity home).
 */
@HiltViewModel
class VcfImportViewModel @Inject constructor(private val apiClient: ApiClient) :
    ViewModel(),
    CustomFieldActions {

    private val _uiState = MutableStateFlow(VcfImportUiState())
    val uiState: StateFlow<VcfImportUiState> = _uiState.asStateFlow()

    /**
     * The picked file's provider-declared size already exceeded [MAX_VCF_SIZE_BYTES], so its
     * bytes were never read. Separate from [onFilePicked]'s own size check, which is the
     * backstop for providers that declare no size at all.
     */
    fun onFileTooLarge() {
        _uiState.update { it.copy(errorRes = R.string.import_vcf_error_too_large, error = null) }
    }

    /** Client-side gate matching `backend/services/import_service.go`'s `MaxVCFSize` (VCF files
     *  can embed photos, hence the higher limit than CSV) — avoids an upload doomed to fail. */
    fun onFilePicked(fileName: String, bytes: ByteArray) {
        if (_uiState.value.isLoading) return
        if (bytes.isEmpty()) {
            _uiState.update {
                it.copy(
                    errorRes = R.string.import_vcf_error_invalid_file,
                    error = null
                )
            }
            return
        }
        if (bytes.size > MAX_VCF_SIZE_BYTES) {
            _uiState.update {
                it.copy(
                    errorRes = R.string.import_vcf_error_too_large,
                    error = null
                )
            }
            return
        }
        viewModelScope.launch {
            _uiState.update {
                it.copy(
                    isLoading = true,
                    errorRes = null,
                    error = null,
                    fileName = fileName
                )
            }
            apiClient.uploadVcfImport(bytes, fileName).foldApiError(
                onSuccess = { preview ->
                    val actions = preview.rows.associate { row ->
                        row.rowIndex to
                            if (row.validationErrors.isNotEmpty()) "skip" else row.suggestedAction
                    }
                    val candidates = collectCandidates(preview)
                    if (candidates.isEmpty()) {
                        _uiState.update {
                            it.copy(
                                isLoading = false,
                                step = VcfImportStep.PREVIEW,
                                preview = preview,
                                rowActions = actions
                            )
                        }
                    } else {
                        // Stay in the loading state until the definitions arrive, so the step
                        // never renders "map is unavailable" for a list that is merely in flight.
                        viewModelScope.launch {
                            enterCustomFieldsStep(
                                preview,
                                actions,
                                candidates
                            )
                        }
                    }
                },
                onError = { error ->
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            error = error
                        )
                    }
                }
            )
        }
    }

    private suspend fun enterCustomFieldsStep(
        preview: ImportPreviewResponse,
        actions: Map<Int, String>,
        candidates: List<DiscoveredCustomProperty>
    ) {
        val fetched = apiClient.listFieldDefinitions()
        // A failed fetch is surfaced, not read as "you have no custom fields"; a definition the
        // server already matched stays usable so the export round-trip is not lost with it.
        val fetchError = fetched.exceptionOrNull()?.toApiError()
        // Only contact definitions can receive an X- property (the web importer and the contact
        // page filter the same way); a missing target is a legacy contact definition (#2554).
        val known = fetched.getOrNull()?.definitions.orEmpty()
            .filter { it.target.isNullOrBlank() || it.target == "contact" }
        val matched = candidates.mapNotNull { c ->
            c.matchedDefinitionId?.takeIf { it.isNotEmpty() }?.let { id ->
                FieldDefinition(id = id, label = c.matchedDefinitionLabel)
            }
        }
        val definitions = known + matched.filter { m -> known.none { it.id == m.id } }
            .distinctBy { it.id }
        _uiState.update {
            it.copy(
                isLoading = false,
                step = VcfImportStep.CUSTOM_FIELDS,
                preview = preview,
                rowActions = actions,
                customFieldCandidates = candidates,
                fieldDefinitions = definitions,
                fieldMappings = candidates.associate { c ->
                    c.name to initialMapping(
                        c,
                        definitions
                    )
                },
                error = fetchError
            )
        }
    }

    /** One entry per property name (first value seen wins), in first-seen order. */
    private fun collectCandidates(preview: ImportPreviewResponse): List<DiscoveredCustomProperty> =
        preview.rows.flatMap { it.customFieldCandidates.orEmpty() }.distinctBy { it.name }

    /** A property whose projection already matches a definition defaults to "map" (the export
     *  round-trip); everything else defaults to keeping it in passthrough. */
    private fun initialMapping(
        candidate: DiscoveredCustomProperty,
        definitions: List<FieldDefinition>
    ): ImportFieldMapping {
        val matchedId = candidate.matchedDefinitionId?.takeIf { id -> definitions.any { it.id == id } }
        return ImportFieldMapping(
            propertyName = candidate.name,
            action = if (matchedId != null) IMPORT_FIELD_ACTION_MAP else IMPORT_FIELD_ACTION_IGNORE,
            fieldDefinitionId = matchedId,
            label = titleCaseFromProperty(candidate.name),
            type = DEFAULT_FIELD_TYPE
        )
    }

    private fun updateMapping(
        propertyName: String,
        change: (ImportFieldMapping) -> ImportFieldMapping
    ) {
        _uiState.update { state ->
            val current = state.fieldMappings[propertyName] ?: return@update state
            state.copy(fieldMappings = state.fieldMappings + (propertyName to change(current)))
        }
    }

    /** No-op for a row with validation errors until it has a saved fix — then it decides like any
     *  other row (issue #2186). */
    fun setRowAction(rowIndex: Int, action: String) {
        val row = _uiState.value.preview?.rows?.find { it.rowIndex == rowIndex } ?: return
        if (row.validationErrors.isNotEmpty() && rowIndex !in _uiState.value.rowEdits) return
        _uiState.update { it.copy(rowActions = it.rowActions + (rowIndex to action)) }
    }

    /**
     * Issue #2186: keep an errored row with the user's fix. The server re-validates at confirm
     * and reports a row that is still invalid in the result, so saving only selects "add".
     * Ignored for a row that has no validation errors.
     */
    fun saveRowEdit(edit: ImportRowEdit) {
        val row = _uiState.value.preview?.rows?.find { it.rowIndex == edit.rowIndex } ?: return
        if (row.validationErrors.isEmpty()) return
        _uiState.update {
            it.copy(
                rowEdits = it.rowEdits + (edit.rowIndex to edit),
                rowActions = it.rowActions + (edit.rowIndex to "add")
            )
        }
    }

    /** T96: "Resolve all as merged" — every valid row takes its suggested action
     *  (merge for duplicates, keep both for new, discard for within-batch
     *  duplicates); errored rows stay forced to skip. */
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

    /** Unknown actions are ignored; "map" is refused while the user has no definitions to map to. */
    override fun setFieldMappingAction(propertyName: String, action: String) {
        if (action !in FIELD_MAPPING_ACTIONS) return
        if (action == IMPORT_FIELD_ACTION_MAP && _uiState.value.fieldDefinitions.isEmpty()) return
        updateMapping(propertyName) { it.copy(action = action) }
    }

    /** Only one of the user's own definitions can be chosen. */
    override fun setFieldMappingDefinition(propertyName: String, definitionId: String) {
        if (_uiState.value.fieldDefinitions.none { it.id == definitionId }) return
        updateMapping(propertyName) { it.copy(fieldDefinitionId = definitionId) }
    }

    override fun setFieldMappingLabel(propertyName: String, label: String) {
        updateMapping(propertyName) { it.copy(label = label) }
    }

    override fun setFieldMappingType(propertyName: String, type: String) {
        if (type !in FIELD_TYPES) return
        updateMapping(propertyName) { it.copy(type = type) }
    }

    /** Moves from the custom-fields step to review once every "map" decision has a field. */
    override fun continueFromCustomFields() {
        val state = _uiState.value
        if (state.step != VcfImportStep.CUSTOM_FIELDS || !state.canContinueFromCustomFields) return
        _uiState.update { it.copy(step = VcfImportStep.PREVIEW) }
    }

    fun confirm() {
        val preview = _uiState.value.preview ?: return
        if (_uiState.value.isLoading) return
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, error = null) }
            val actions = _uiState.value.rowActions.map { (rowIndex, action) ->
                RowImportAction(
                    rowIndex,
                    action
                )
            }
            apiClient.confirmVcfImport(
                ImportConfirmRequest(
                    sessionId = preview.sessionId,
                    actions = actions,
                    rowEdits = activeRowEdits(),
                    fieldMappings = confirmFieldMappings()
                )
            ).foldApiError(
                onSuccess = { result ->
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            step = VcfImportStep.RESULT,
                            result = result
                        )
                    }
                },
                onError = { error ->
                    _uiState.update {
                        it.copy(
                            isLoading = false,
                            error = error
                        )
                    }
                }
            )
        }
    }

    /** Only the fixes for rows still being imported; null (omitted) when there are none. */
    private fun activeRowEdits(): List<ImportRowEdit>? = _uiState.value.let { state ->
        state.rowEdits.values.filter { state.rowActions[it.rowIndex] != "skip" }.ifEmpty { null }
    }

    /** One decision per discovered property, carrying only what its action uses; null (key
     *  omitted) when the file had no X- properties. */
    private fun confirmFieldMappings(): List<ImportFieldMapping>? = _uiState.value.let { state ->
        state.customFieldCandidates.mapNotNull { state.fieldMappings[it.name] }.map { m ->
            when (m.action) {
                IMPORT_FIELD_ACTION_MAP -> ImportFieldMapping(
                    propertyName = m.propertyName,
                    action = m.action,
                    fieldDefinitionId = m.fieldDefinitionId
                )

                IMPORT_FIELD_ACTION_CREATE -> ImportFieldMapping(
                    propertyName = m.propertyName,
                    action = m.action,
                    label = m.label?.trim()?.takeIf { it.isNotEmpty() },
                    type = m.type
                )

                else -> ImportFieldMapping(m.propertyName, IMPORT_FIELD_ACTION_IGNORE)
            }
        }.ifEmpty { null }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(errorRes = null, error = null) }
    }

    companion object {
        const val MAX_VCF_SIZE_BYTES = 50 * 1024 * 1024
        private const val DEFAULT_FIELD_TYPE = "text"
        private val FIELD_MAPPING_ACTIONS = setOf(
            IMPORT_FIELD_ACTION_IGNORE,
            IMPORT_FIELD_ACTION_MAP,
            IMPORT_FIELD_ACTION_CREATE
        )

        /** `x-favorite-color` -> "Favorite Color"; mirrors web's `titleCaseFromProperty` and the
         *  server's default derivation. */
        internal fun titleCaseFromProperty(name: String): String =
            name.replace(Regex("^x-", RegexOption.IGNORE_CASE), "")
                .split(Regex("[^a-zA-Z0-9]+"))
                .filter { it.isNotEmpty() }
                .joinToString(" ") { it.replaceFirstChar(Char::uppercaseChar) }
    }
}
