package com.mycorrhizal.crm.feature.imports

import android.content.ContentResolver
import android.net.Uri
import android.provider.OpenableColumns
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.CloudUpload
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * M9 item 4: VCF-file import — pick a `.vcf` file, review the preview rows (each editable
 * skip/add/update, forced to skip when it has validation errors), confirm. Mirrors the VCF branch
 * of web's `ImportContactsDialog.tsx`, condensed (no column-mapping step — VCF skips that there
 * too). Overrides the M8 sign-off's CSV/VCF-file-import exclusion for the VCF-only path, per an
 * explicit decision on this ticket (CSV file import remains excluded).
 */
@Composable
fun VcfImportScreen(
    onBack: () -> Unit,
    onDone: () -> Unit,
    viewModel: VcfImportViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    VcfImportScreenContent(
        uiState = state,
        onBack = onBack,
        onDone = onDone,
        onFilePicked = viewModel::onFilePicked,
        onFileTooLarge = viewModel::onFileTooLarge,
        onRowActionChange = viewModel::setRowAction,
        onConfirm = viewModel::confirm,
        onErrorShown = viewModel::onErrorShown,
    )
}

/**
 * Stateless screen content, split out from [VcfImportScreen] so it's directly testable without a
 * Hilt-backed ViewModel (mirrors `ContactListScreenContent`'s split in `ContactListScreen.kt`).
 * File picking itself ([onFilePicked] takes the already-read bytes) stays in [VcfImportScreen]
 * since it needs a real `ContentResolver`.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun VcfImportScreenContent(
    uiState: VcfImportUiState,
    onBack: () -> Unit = {},
    onDone: () -> Unit = {},
    onFilePicked: (String, ByteArray) -> Unit = { _, _ -> },
    onFileTooLarge: () -> Unit = {},
    onRowActionChange: (Int, String) -> Unit = { _, _ -> },
    onConfirm: () -> Unit = {},
    onErrorShown: () -> Unit = {},
) {
    val snackbarHostState = remember { SnackbarHostState() }
    val context = LocalContext.current
    val scope = rememberCoroutineScope()

    val filePicker = rememberLauncherForActivityResult(ActivityResultContracts.GetContent()) { uri: Uri? ->
        if (uri == null) return@rememberLauncherForActivityResult
        scope.launch {
            val resolver = context.contentResolver
            // Probe the provider's declared size BEFORE reading anything. The picker is
            // launched with "*/*" (vCard MIME types are unreliable across providers), so the
            // user can pick a multi-gigabyte video — reading that into a ByteArray to then
            // reject it on size would OOM the app before the guard ever ran. Providers may
            // report no size at all, so VcfImportViewModel keeps its own post-read check as
            // the backstop for that case.
            val meta = withContext(Dispatchers.IO) { queryFileMeta(resolver, uri) }
            if (meta.size != null && meta.size > VcfImportViewModel.MAX_VCF_SIZE_BYTES) {
                onFileTooLarge()
                return@launch
            }
            val bytes = withContext(Dispatchers.IO) { readAllBytes(resolver, uri) }
            onFilePicked(meta.name, bytes)
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = { Text(stringResource(R.string.import_vcf_title), style = MaterialTheme.typography.titleLarge) },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                uiState.isLoading -> LoadingSkeleton(modifier = Modifier.testTag("vcf-import-loading"))
                uiState.step == VcfImportStep.PICK -> PickStep(onPick = { filePicker.launch("*/*") })
                uiState.step == VcfImportStep.PREVIEW && uiState.preview != null ->
                    PreviewStep(rows = uiState.preview.rows, rowActions = uiState.rowActions, onRowActionChange = onRowActionChange, onConfirm = onConfirm)
                uiState.step == VcfImportStep.RESULT && uiState.result != null ->
                    ResultStep(created = uiState.result.created, updated = uiState.result.updated, skipped = uiState.result.skipped, onDone = onDone)
                else -> EmptyState(message = stringResource(R.string.import_vcf_error_invalid_file))
            }
        }
    }

    val errorMessage = uiState.errorRes?.let { stringResource(it) } ?: uiState.error
    if (errorMessage != null) {
        LaunchedEffect(errorMessage) {
            snackbarHostState.showSnackbar(errorMessage)
            onErrorShown()
        }
    }
}

@Composable
private fun PickStep(onPick: () -> Unit) {
    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        modifier = Modifier.fillMaxSize().padding(24.dp),
    ) {
        Icon(
            Icons.Outlined.CloudUpload,
            contentDescription = null,
            modifier = Modifier.padding(top = 48.dp),
        )
        Text(
            text = stringResource(R.string.import_vcf_pick_hint),
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.padding(vertical = 16.dp),
        )
        Button(onClick = onPick) {
            Text(stringResource(R.string.import_vcf_pick))
        }
    }
}

@Composable
private fun PreviewStep(
    rows: List<ImportRowPreview>,
    rowActions: Map<Int, String>,
    onRowActionChange: (Int, String) -> Unit,
    onConfirm: () -> Unit,
) {
    Column(modifier = Modifier.fillMaxSize()) {
        LazyColumn(modifier = Modifier.weight(1f).testTag("vcf-import-preview-list")) {
            items(rows, key = { it.rowIndex }) { row ->
                PreviewRow(
                    row = row,
                    action = rowActions[row.rowIndex] ?: row.suggestedAction,
                    onActionChange = { action -> onRowActionChange(row.rowIndex, action) },
                )
            }
        }
        Button(
            onClick = onConfirm,
            modifier = Modifier.fillMaxWidth().padding(16.dp),
        ) {
            Text(stringResource(R.string.import_vcf_confirm))
        }
    }
}

@Composable
private fun PreviewRow(
    row: ImportRowPreview,
    action: String,
    onActionChange: (String) -> Unit,
) {
    val hasErrors = row.validationErrors.isNotEmpty()
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)) {
        Text(row.displayName(), style = MaterialTheme.typography.bodyLarge)
        row.duplicateMatch?.let { duplicate ->
            val label = listOfNotNull(duplicate.existingFirstname, duplicate.existingLastname).joinToString(" ")
            Text(
                text = stringResource(R.string.import_duplicate, label.ifBlank { "#${duplicate.existingContactId}" }),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.error,
            )
        }
        if (hasErrors) {
            row.validationErrors.forEach { error ->
                Text(error, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.error)
            }
        }
        Row(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            modifier = Modifier.padding(top = 4.dp),
        ) {
            listOf(
                "skip" to stringResource(R.string.import_vcf_row_skip),
                "add" to stringResource(R.string.import_vcf_row_add),
                "update" to stringResource(R.string.import_vcf_row_update),
            ).forEach { (value, label) ->
                FilterChip(
                    selected = action == value,
                    onClick = { if (!hasErrors) onActionChange(value) },
                    enabled = !hasErrors,
                    label = { Text(label) },
                )
            }
        }
    }
}

@Composable
private fun ResultStep(created: Int, updated: Int, skipped: Int, onDone: () -> Unit) {
    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        modifier = Modifier.fillMaxSize().padding(24.dp),
    ) {
        Text(
            text = stringResource(R.string.import_vcf_result, created, updated, skipped),
            style = MaterialTheme.typography.bodyLarge,
            modifier = Modifier.padding(top = 48.dp, bottom = 16.dp),
        )
        Button(onClick = onDone) {
            Text(stringResource(R.string.action_confirm))
        }
    }
}

private fun ImportRowPreview.displayName(): String {
    val first = parsedContact["firstname"] as? String
    val last = parsedContact["lastname"] as? String
    return listOfNotNull(first, last).joinToString(" ").ifBlank { "#${rowIndex + 1}" }
}

/** A picked file's display name and the size its provider declares (null when it declares none). */
private data class PickedFileMeta(val name: String, val size: Long?)

/**
 * Cheap metadata probe — reads only the provider's cursor, never the file contents, so an
 * oversized pick can be rejected without allocating it. `size` is null when the provider omits
 * the column or reports it as null, which some do.
 */
private fun queryFileMeta(resolver: ContentResolver, uri: Uri): PickedFileMeta {
    var name = "import.vcf"
    var size: Long? = null
    resolver.query(uri, null, null, null, null)?.use { cursor ->
        if (cursor.moveToFirst()) {
            val nameIndex = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
            if (nameIndex >= 0) cursor.getString(nameIndex)?.let { name = it }
            val sizeIndex = cursor.getColumnIndex(OpenableColumns.SIZE)
            if (sizeIndex >= 0 && !cursor.isNull(sizeIndex)) size = cursor.getLong(sizeIndex)
        }
    }
    return PickedFileMeta(name, size)
}

/** Reads the picked file's bytes off the main thread. Only called once the size probe passes. */
private fun readAllBytes(resolver: ContentResolver, uri: Uri): ByteArray =
    resolver.openInputStream(uri)?.use { it.readBytes() } ?: ByteArray(0)
