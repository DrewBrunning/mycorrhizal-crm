package com.mycorrhizal.crm.feature.settings

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.AutoAwesome
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedCard
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
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.domain.compat.ServerCapabilities
import com.mycorrhizal.crm.domain.compat.ServerFeature
import com.mycorrhizal.crm.model.network.ContactAddressSuggestion
import com.mycorrhizal.crm.model.network.formatSuggestionAddress
import com.mycorrhizal.crm.ui.LocalServerVersion
import com.mycorrhizal.crm.ui.R
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * The "propose data" screen (T104 + address suggestions): buttons that trigger
 * the two inference engines, the relationship-suggestion result banner, and
 * the address-suggestion review list with explicit Apply per row.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DataScreen(
    onBack: () -> Unit,
    onCustomExport: () -> Unit = {},
    onRestoreBundle: () -> Unit = {},
    viewModel: DataViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }
    val context = LocalContext.current
    val scope = rememberCoroutineScope()

    // Issue #1264: the account bundle is a user-controlled file outside app
    // storage, so it goes through the SAF create-document picker (not the
    // share sheet the lossy per-format exports use).
    val bundleDestination = rememberLauncherForActivityResult(
        ActivityResultContracts.CreateDocument("application/json"),
    ) { uri ->
        if (uri != null) {
            viewModel.exportAccountBundle { bytes ->
                withContext(Dispatchers.IO) { writeBundle(context, uri, bytes) }
            }
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.cd_back),
                        )
                    }
                },
                title = {
                    Text(stringResource(R.string.settings_data), style = MaterialTheme.typography.titleLarge)
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
                text = stringResource(R.string.data_export_section),
                style = MaterialTheme.typography.titleMedium,
            )
            Text(
                text = stringResource(R.string.data_export_description),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            // Issue #1264 / ADR 0028 Decision 3: the full-fidelity, re-importable
            // backup. First in the list — it is the export a Local profile's
            // only copy of the data depends on.
            ExportRow(
                labelRes = R.string.data_export_bundle,
                exporting = state.isExporting,
                onClick = { bundleDestination.launch(viewModel.accountBundleFileName()) },
                modifier = Modifier.testTag("export-account-bundle"),
            )
            if (state.canRestoreBundle) {
                ExportRow(
                    labelRes = R.string.data_restore_bundle,
                    exporting = state.isExporting,
                    onClick = onRestoreBundle,
                    modifier = Modifier.testTag("restore-account-bundle"),
                )
            }
            ExportRow(
                labelRes = R.string.data_export_csv,
                exporting = state.isExporting,
                onClick = { viewModel.export(DataExportKind.CSV) },
            )
            ExportRow(
                labelRes = R.string.data_export_vcard4,
                exporting = state.isExporting,
                onClick = { viewModel.export(DataExportKind.VCF4) },
            )
            ExportRow(
                labelRes = R.string.data_export_vcard3,
                exporting = state.isExporting,
                onClick = { viewModel.export(DataExportKind.VCF3) },
            )
            ExportRow(
                labelRes = R.string.data_export_jscontact,
                exporting = state.isExporting,
                onClick = { viewModel.export(DataExportKind.JSCONTACT) },
            )
            // Issue #692: the audit-log export endpoint shipped in v0.6.1, so on
            // a v0.6.0 server the row is hidden rather than 404ing.
            if (ServerCapabilities.isSupported(LocalServerVersion.current, ServerFeature.AUDIT_EXPORT)) {
                ExportRow(
                    labelRes = R.string.data_export_audit,
                    exporting = state.isExporting,
                    onClick = { viewModel.export(DataExportKind.AUDIT_CSV) },
                )
            }
            // Issue #835 (web parity, T9 follow-up): pick sections + opt in to
            // sensitive data for one export, instead of the fixed rows above.
            ExportRow(
                labelRes = R.string.data_export_custom,
                exporting = state.isExporting,
                onClick = onCustomExport,
            )

            HorizontalDivider(modifier = Modifier.padding(vertical = 4.dp))

            Text(
                text = stringResource(R.string.data_relationships_section),
                style = MaterialTheme.typography.titleMedium,
            )
            Text(
                text = stringResource(R.string.data_relationships_description),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            state.suggestedRelationshipCount?.let { count ->
                Text(
                    text = if (count > 0) {
                        stringResource(R.string.data_relationships_generated, count)
                    } else {
                        stringResource(R.string.data_relationships_none)
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            val suggestingLabel = stringResource(R.string.a11y_state_saving)
            Button(
                onClick = viewModel::suggestRelationships,
                enabled = !state.isSuggestingRelationships,
                modifier = Modifier
                    .fillMaxWidth()
                    .semantics { if (state.isSuggestingRelationships) stateDescription = suggestingLabel },
            ) {
                if (state.isSuggestingRelationships) {
                    CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp))
                }
                Text(stringResource(R.string.settings_suggest_relationships))
            }

            HorizontalDivider(modifier = Modifier.padding(vertical = 4.dp))

            Text(
                text = stringResource(R.string.data_address_section),
                style = MaterialTheme.typography.titleMedium,
            )
            Text(
                text = stringResource(R.string.data_address_description),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            val loadingLabel = stringResource(R.string.a11y_state_loading)
            // Filled Button, matching the sibling "Suggest relationships"
            // primary action above — the two scans are peer affordances.
            Button(
                onClick = viewModel::scanAddressSuggestions,
                enabled = !state.suggestionsLoading,
                modifier = Modifier
                    .fillMaxWidth()
                    .semantics { if (state.suggestionsLoading) stateDescription = loadingLabel },
            ) {
                if (state.suggestionsLoading) {
                    CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp))
                }
                Text(stringResource(R.string.data_suggest_addresses))
            }

            if (state.suggestionsLoaded) {
                if (state.addressSuggestions.isEmpty()) {
                    Text(
                        text = stringResource(R.string.data_address_suggestions_empty),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                } else {
                    state.addressSuggestions.forEach { suggestion ->
                        AddressSuggestionRow(
                            suggestion = suggestion,
                            pending = state.applyingKey == suggestionKey(suggestion),
                            onApply = { viewModel.applySuggestion(suggestion) },
                        )
                    }
                }
            }
        }
    }

    // A finished dataset export is written to the cache and handed to the
    // share sheet via FileProvider (the ContactDetail single-contact export
    // pattern). Consumed exactly once.
    state.exported?.let { export ->
        LaunchedEffect(export.kind) {
            shareExportFile(context, export)
            viewModel.onExportHandled()
        }
    }

    state.error?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
        }
    }

    state.infoRes?.let { res ->
        val message = state.infoCount?.let { stringResource(res, it) } ?: stringResource(res)
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onInfoShown()
        }
    }
}

@Composable
private fun AddressSuggestionRow(
    suggestion: ContactAddressSuggestion,
    pending: Boolean,
    onApply: () -> Unit,
) {
    OutlinedCard(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(12.dp)) {
            Text(
                text = suggestion.contactName,
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                text = formatSuggestionAddress(suggestion.address),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = reasonLabel(suggestion),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.padding(top = 8.dp),
            ) {
                TextButton(onClick = onApply, enabled = !pending) {
                    Icon(Icons.Outlined.AutoAwesome, contentDescription = null, modifier = Modifier.padding(end = 4.dp))
                    Text(stringResource(R.string.data_apply_address))
                }
            }
        }
    }
}

@Composable
private fun reasonLabel(suggestion: ContactAddressSuggestion): String = when (suggestion.sourceKind) {
    "household" -> stringResource(R.string.data_address_reason_household, suggestion.sourceName)
    else -> {
        val relation = relationTokenLabel(suggestion.relationType).ifEmpty { "related to" }
        stringResource(R.string.data_address_reason_relationship, suggestion.sourceName, relation)
    }
}

private fun suggestionKey(suggestion: ContactAddressSuggestion): String =
    "${suggestion.contactVCardUid}|${suggestion.addressKey}"

/**
 * One full-dataset export row. Filled-button peers to the two suggestion
 * scans above; every row is disabled while an export is in flight so two
 * formats can't race the single one-shot slot.
 */
@Composable
private fun ExportRow(
    labelRes: Int,
    exporting: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val loadingLabel = stringResource(R.string.a11y_state_loading)
    OutlinedButton(
        onClick = onClick,
        enabled = !exporting,
        modifier = modifier
            .fillMaxWidth()
            .semantics { if (exporting) stateDescription = loadingLabel },
    ) {
        if (exporting) {
            CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp))
        }
        Text(stringResource(labelRes))
    }
}


/** Writes [bytes] to the SAF [uri] the user picked. A provider that yields no stream is a failure. */
@Suppress("TooGenericExceptionCaught")
private fun writeBundle(context: android.content.Context, uri: android.net.Uri, bytes: ByteArray): Result<Unit> =
    try {
        val stream = context.contentResolver.openOutputStream(uri, "wt")
            ?: throw java.io.IOException("No output stream for $uri")
        stream.use { it.write(bytes) }
        Result.success(Unit)
    } catch (e: Exception) {
        Result.failure(e)
    }
