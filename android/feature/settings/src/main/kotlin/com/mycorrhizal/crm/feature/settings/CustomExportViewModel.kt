package com.mycorrhizal.crm.feature.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycorrhizal.crm.domain.repository.ExportRepository
import com.mycorrhizal.crm.model.network.ExportLossPreflightResponse
import com.mycorrhizal.crm.model.network.ShareFieldSections
import com.mycorrhizal.crm.network.foldApiError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/** The three formats web's ExportFieldPickerDialog offers. */
enum class ExportFormatChoice(val preflightToken: String) {
    VCF4("vcard4"),
    VCF3("vcard3"),
    JSCONTACT("jscontact"),
}

/**
 * Issue #835 (T9 selective-export Android parity): state for the "Custom
 * export" screen — pick a format, pick sections, optionally reveal and
 * include sensitivity-gated sections, optionally preview the DATA-02 loss
 * report, then export. Reuses [ShareFieldSections] (the same token/
 * sensitivity list T9's sharing picker already mirrors from the backend)
 * rather than a parallel copy.
 */
data class CustomExportUiState(
    val format: ExportFormatChoice = ExportFormatChoice.VCF4,
    val selected: Set<String> = ShareFieldSections.DEFAULT_SELECTED.toSet(),
    val sensitiveRevealed: Boolean = false,
    val isExporting: Boolean = false,
    val isCheckingLoss: Boolean = false,
    val lossReport: ExportLossPreflightResponse? = null,
    val exported: DataExport? = null,
    val error: String? = null,
) {
    val canExport: Boolean
        get() = selected.isNotEmpty() && !isExporting
}

@HiltViewModel
class CustomExportViewModel @Inject constructor(
    private val exportRepository: ExportRepository,
) : ViewModel() {

    private val _uiState = MutableStateFlow(CustomExportUiState())
    val uiState: StateFlow<CustomExportUiState> = _uiState.asStateFlow()

    fun setFormat(format: ExportFormatChoice) {
        _uiState.update { it.copy(format = format) }
    }

    fun toggleSection(token: String, checked: Boolean) {
        _uiState.update { state ->
            val next = state.selected.toMutableSet()
            if (checked) next.add(token) else next.remove(token)
            state.copy(selected = next)
        }
    }

    /** Deliberate opt-in: unlocks the sensitivity-gated sections (mirrors ShareContactViewModel). */
    fun revealSensitive() {
        _uiState.update { it.copy(sensitiveRevealed = true) }
    }

    /**
     * include_sensitive is only true when the user has deliberately revealed
     * AND selected a sensitivity-marked section — the same foot-gun guard as
     * web's ExportFieldPickerDialog.handleExport: an ordinary unchecked box
     * cannot imply it.
     */
    private fun includeSensitive(state: CustomExportUiState): Boolean =
        state.sensitiveRevealed && state.selected.any { token ->
            ShareFieldSections.ALL.any { it.token == token && it.sensitive }
        }

    /** DATA-02 (issue #442): preview what the current selection would lose, without exporting. */
    fun checkLossReport() {
        val state = _uiState.value
        if (state.isCheckingLoss || state.selected.isEmpty()) return
        _uiState.update { it.copy(isCheckingLoss = true, error = null) }
        viewModelScope.launch {
            exportRepository.preflight(
                state.format.preflightToken,
                state.selected.toList(),
                includeSensitive(state),
            ).foldApiError(
                onSuccess = { report ->
                    _uiState.update { it.copy(isCheckingLoss = false, lossReport = report) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isCheckingLoss = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun onLossReportShown() {
        _uiState.update { it.copy(lossReport = null) }
    }

    fun export() {
        val state = _uiState.value
        if (!state.canExport) return
        _uiState.update { it.copy(isExporting = true, error = null) }
        val sections = state.selected.toList()
        val includeSensitive = includeSensitive(state)
        val kind = when (state.format) {
            ExportFormatChoice.VCF4 -> DataExportKind.CUSTOM_VCF4
            ExportFormatChoice.VCF3 -> DataExportKind.CUSTOM_VCF3
            ExportFormatChoice.JSCONTACT -> DataExportKind.CUSTOM_JSCONTACT
        }
        viewModelScope.launch {
            val result = when (state.format) {
                ExportFormatChoice.VCF4 -> exportRepository.exportContactsVcf(null, sections, includeSensitive)
                ExportFormatChoice.VCF3 -> exportRepository.exportContactsVcf(3, sections, includeSensitive)
                ExportFormatChoice.JSCONTACT -> exportRepository.exportContactsJsContact(sections, includeSensitive)
            }
            result.foldApiError(
                onSuccess = { bytes ->
                    _uiState.update { it.copy(isExporting = false, exported = DataExport(kind, bytes)) }
                },
                onError = { error ->
                    _uiState.update { it.copy(isExporting = false, error = error.displayMessage) }
                },
            )
        }
    }

    fun onExportHandled() {
        _uiState.update { it.copy(exported = null) }
    }

    fun onErrorShown() {
        _uiState.update { it.copy(error = null) }
    }
}
