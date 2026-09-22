package com.mycorrhizal.crm.feature.settings

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.ShareFieldSections
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton

/**
 * Issue #835 (T9 selective-export Android parity, web's ExportFieldPickerDialog):
 * pick a format, pick which of the T9 field sections to include, optionally
 * reveal and include sensitivity-gated sections, optionally preview the
 * DATA-02 loss report, then export — reached from DataScreen's Export
 * section via "Custom export...". A form this size gets its own route,
 * matching ShareContactScreen/ContactFieldSettingsScreen rather than a
 * dialog.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CustomExportScreen(
    onBack: () -> Unit,
    viewModel: CustomExportViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }
    var showSensitiveConfirm by remember { mutableStateOf(false) }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    AccessibleIconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.cd_back),
                        )
                    }
                },
                title = {
                    Text(stringResource(R.string.data_custom_export_title), style = MaterialTheme.typography.titleLarge)
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text(
                text = stringResource(R.string.data_custom_export_description),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )

            Text(stringResource(R.string.data_custom_export_format), style = MaterialTheme.typography.titleMedium)
            Column(modifier = Modifier.selectableGroup()) {
                FormatOptionRow(
                    labelRes = R.string.data_custom_export_format_vcf4,
                    selected = state.format == ExportFormatChoice.VCF4,
                    onClick = { viewModel.setFormat(ExportFormatChoice.VCF4) },
                )
                FormatOptionRow(
                    labelRes = R.string.data_custom_export_format_vcf3,
                    selected = state.format == ExportFormatChoice.VCF3,
                    onClick = { viewModel.setFormat(ExportFormatChoice.VCF3) },
                )
                FormatOptionRow(
                    labelRes = R.string.data_custom_export_format_jscontact,
                    selected = state.format == ExportFormatChoice.JSCONTACT,
                    onClick = { viewModel.setFormat(ExportFormatChoice.JSCONTACT) },
                )
            }

            HorizontalDivider(modifier = Modifier.padding(vertical = 4.dp))

            Text(stringResource(R.string.shares_fields_label), style = MaterialTheme.typography.titleMedium)
            val lockedLabel = stringResource(R.string.a11y_share_field_locked)
            ShareFieldSections.ALL.forEach { section ->
                val checked = state.selected.contains(section.token)
                val locked = section.sensitive && !state.sensitiveRevealed
                // Same accessible pattern as ShareContactScreen (#199): the row's
                // Modifier.toggleable merges the label into the checkbox's
                // accessible name, and stateDescription names the sensitivity
                // lock rather than relying on the lock icon's null description.
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    modifier = Modifier
                        .fillMaxWidth()
                        .toggleable(
                            value = if (locked) false else checked,
                            onValueChange = { viewModel.toggleSection(section.token, it) },
                            enabled = !locked,
                            role = Role.Checkbox,
                        )
                        .semantics { if (locked) stateDescription = lockedLabel },
                ) {
                    Checkbox(
                        checked = if (locked) false else checked,
                        onCheckedChange = null,
                        enabled = !locked,
                    )
                    Text(
                        text = stringResource(customExportSectionLabelRes(section.token)),
                        style = MaterialTheme.typography.bodyMedium,
                        color = if (locked) {
                            MaterialTheme.colorScheme.onSurfaceVariant
                        } else {
                            MaterialTheme.colorScheme.onSurface
                        },
                    )
                    if (locked) {
                        Icon(
                            imageVector = Icons.Outlined.Lock,
                            contentDescription = null,
                            tint = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.padding(start = 4.dp),
                        )
                    }
                }
            }

            if (!state.sensitiveRevealed) {
                OutlinedButton(
                    onClick = { showSensitiveConfirm = true },
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Icon(imageVector = Icons.Outlined.Lock, contentDescription = null)
                    Text(stringResource(R.string.shares_reveal_button), modifier = Modifier.padding(start = 4.dp))
                }
            }

            HorizontalDivider(modifier = Modifier.padding(vertical = 4.dp))

            TextButton(
                onClick = viewModel::checkLossReport,
                enabled = state.selected.isNotEmpty() && !state.isCheckingLoss,
                modifier = Modifier.fillMaxWidth(),
            ) {
                if (state.isCheckingLoss) {
                    CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp))
                    Text(stringResource(R.string.data_custom_export_preview_checking))
                } else {
                    Text(stringResource(R.string.data_custom_export_preview_button))
                }
            }

            val exportingLabel = stringResource(R.string.data_custom_export_exporting)
            Button(
                onClick = viewModel::export,
                enabled = state.canExport,
                modifier = Modifier
                    .fillMaxWidth()
                    .semantics { if (state.isExporting) stateDescription = exportingLabel },
            ) {
                if (state.isExporting) {
                    CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp))
                    Text(stringResource(R.string.data_custom_export_exporting))
                } else {
                    Text(stringResource(R.string.data_custom_export_button))
                }
            }
        }
    }

    if (showSensitiveConfirm) {
        AlertDialog(
            onDismissRequest = { showSensitiveConfirm = false },
            title = { Text(stringResource(R.string.shares_reveal_title)) },
            text = { Text(stringResource(R.string.shares_reveal_confirm)) },
            confirmButton = {
                TextButton(
                    onClick = {
                        viewModel.revealSensitive()
                        showSensitiveConfirm = false
                    },
                ) {
                    Text(stringResource(R.string.shares_reveal_confirm_button))
                }
            },
            dismissButton = {
                TextButton(onClick = { showSensitiveConfirm = false }) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }

    state.lossReport?.let { report ->
        AlertDialog(
            onDismissRequest = viewModel::onLossReportShown,
            title = { Text(stringResource(R.string.data_custom_export_preview_title)) },
            text = {
                if (report.diagnostics.isEmpty()) {
                    Text(stringResource(R.string.data_custom_export_preview_empty))
                } else {
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        report.diagnostics.forEach { diagnostic ->
                            Text("${diagnostic.contactName}: ${diagnostic.message}", style = MaterialTheme.typography.bodySmall)
                        }
                    }
                }
            },
            confirmButton = {
                TextButton(onClick = viewModel::onLossReportShown) {
                    Text(stringResource(R.string.data_custom_export_preview_close))
                }
            },
        )
    }

    // A finished export is written to the cache and handed to the share
    // sheet via FileProvider — same one-shot pattern as DataScreen.
    state.exported?.let { export ->
        val context = LocalContext.current
        LaunchedEffect(export.kind) {
            shareExportFile(context, export)
            viewModel.onExportHandled()
            onBack()
        }
    }

    state.error?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
        }
    }
}

@Composable
private fun FormatOptionRow(
    @StringRes labelRes: Int,
    selected: Boolean,
    onClick: () -> Unit,
) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier.selectable(
            selected = selected,
            onClick = onClick,
            role = Role.RadioButton,
        ),
    ) {
        RadioButton(selected = selected, onClick = null)
        Text(stringResource(labelRes))
    }
}

/** @StringRes label for a section token — mirrors ShareContactScreen.sectionLabelRes. */
@StringRes
private fun customExportSectionLabelRes(token: String): Int = when (token) {
    "emails" -> R.string.shares_section_emails
    "phones" -> R.string.shares_section_phones
    "addresses" -> R.string.shares_section_addresses
    "organizations" -> R.string.shares_section_organizations
    "anniversaries" -> R.string.shares_section_anniversaries
    "media" -> R.string.shares_section_media
    "online_services" -> R.string.shares_section_online_services
    "links" -> R.string.shares_section_links
    "notes" -> R.string.shares_section_notes
    "keywords" -> R.string.shares_section_keywords
    "related_to" -> R.string.shares_section_related_to
    "personal_info" -> R.string.shares_section_personal_info
    "speak_to_as" -> R.string.shares_section_speak_to_as
    "members" -> R.string.shares_section_members
    "languages" -> R.string.shares_section_languages
    "custom_fields" -> R.string.shares_section_custom_fields
    else -> R.string.shares_section_emails
}
