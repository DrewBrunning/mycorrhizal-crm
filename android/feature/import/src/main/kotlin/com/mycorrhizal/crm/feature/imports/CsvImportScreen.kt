package com.mycorrhizal.crm.feature.imports

import android.content.ContentResolver
import android.net.Uri
import android.provider.OpenableColumns
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.CloudUpload
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.ImportUploadResponse
import com.mycorrhizal.crm.model.registry.ImportableContactFields
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Issue #834: CSV-file import — pick a `.csv` file, map its columns to
 * contact fields, review the preview rows, confirm. A sibling path to
 * [VcfImportScreen], reusing the same [ImportReviewStep] for the
 * review/confirm halves; the mapping step in between has no VCF analog
 * (VCF skips column mapping entirely, same as web).
 */
@Composable
fun CsvImportScreen(
    onBack: () -> Unit,
    onDone: () -> Unit,
    viewModel: CsvImportViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    CsvImportScreenContent(
        uiState = state,
        onBack = onBack,
        onDone = onDone,
        onFilePicked = viewModel::onFilePicked,
        onFileTooLarge = viewModel::onFileTooLarge,
        onColumnFieldChange = viewModel::setColumnField,
        onSubmitMapping = viewModel::submitMapping,
        onRowActionChange = viewModel::setRowAction,
        onResolveAll = viewModel::resolveAll,
        onConfirm = viewModel::confirm,
        onErrorShown = viewModel::onErrorShown,
    )
}

/** Stateless split, directly testable without a Hilt-backed ViewModel (mirrors [VcfImportScreenContent]). */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CsvImportScreenContent(
    uiState: CsvImportUiState,
    onBack: () -> Unit = {},
    onDone: () -> Unit = {},
    onFilePicked: (String, ByteArray) -> Unit = { _, _ -> },
    onFileTooLarge: () -> Unit = {},
    onColumnFieldChange: (String, String) -> Unit = { _, _ -> },
    onSubmitMapping: () -> Unit = {},
    onRowActionChange: (Int, String) -> Unit = { _, _ -> },
    onResolveAll: () -> Unit = {},
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
            // Same declared-size-before-read probe as VcfImportScreen (the picker is launched
            // with "*/*" since CSV MIME types are unreliable across providers too).
            val meta = withContext(Dispatchers.IO) { queryFileMeta(resolver, uri) }
            if (meta.size != null && meta.size > CsvImportViewModel.MAX_CSV_SIZE_BYTES) {
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
                title = { Text(stringResource(R.string.import_csv_title), style = MaterialTheme.typography.titleLarge) },
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
                uiState.isLoading -> LoadingSkeleton(modifier = Modifier.testTag("csv-import-loading"))
                uiState.step == CsvImportStep.PICK -> CsvPickStep(onPick = { filePicker.launch("*/*") })
                uiState.step == CsvImportStep.MAPPING && uiState.upload != null ->
                    MappingStep(
                        upload = uiState.upload,
                        columnFields = uiState.columnFields,
                        onColumnFieldChange = onColumnFieldChange,
                        onContinue = onSubmitMapping,
                    )
                uiState.step == CsvImportStep.PREVIEW && uiState.preview != null ->
                    ImportReviewStep(
                        rows = uiState.preview.rows,
                        rowActions = uiState.rowActions,
                        onRowActionChange = onRowActionChange,
                        onResolveAll = onResolveAll,
                        onConfirm = onConfirm,
                    )
                uiState.step == CsvImportStep.RESULT && uiState.result != null ->
                    CsvResultStep(
                        created = uiState.result.created,
                        updated = uiState.result.updated,
                        skipped = uiState.result.skipped,
                        onDone = onDone,
                    )
                else -> CsvPickStep(onPick = { filePicker.launch("*/*") })
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
private fun CsvPickStep(onPick: () -> Unit) {
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
            text = stringResource(R.string.import_csv_pick_hint),
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.padding(vertical = 16.dp),
        )
        Button(onClick = onPick) {
            Text(stringResource(R.string.import_csv_pick))
        }
    }
}

@Composable
private fun MappingStep(
    upload: ImportUploadResponse,
    columnFields: Map<String, String>,
    onColumnFieldChange: (String, String) -> Unit,
    onContinue: () -> Unit,
) {
    Column(modifier = Modifier.fillMaxSize()) {
        Text(
            text = stringResource(R.string.import_csv_mapping_hint, upload.rowCount),
            style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        )
        LazyColumn(modifier = Modifier.weight(1f).testTag("csv-mapping-list")) {
            items(upload.headers, key = { it }) { header ->
                val sample = upload.sampleData.firstOrNull()?.getOrNull(upload.headers.indexOf(header)).orEmpty()
                ColumnMappingRow(
                    header = header,
                    sample = sample,
                    selectedField = columnFields[header].orEmpty(),
                    onFieldChange = { field -> onColumnFieldChange(header, field) },
                )
            }
        }
        Button(
            onClick = onContinue,
            modifier = Modifier.fillMaxWidth().padding(16.dp).testTag("csv-mapping-continue"),
        ) {
            Text(stringResource(R.string.import_csv_mapping_continue))
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ColumnMappingRow(
    header: String,
    sample: String,
    selectedField: String,
    onFieldChange: (String) -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)) {
        Text(text = header, style = MaterialTheme.typography.bodyLarge)
        if (sample.isNotBlank()) {
            Text(
                text = sample,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        ExposedDropdownMenuBox(expanded = expanded, onExpandedChange = { expanded = it }) {
            OutlinedTextField(
                value = fieldLabel(selectedField),
                onValueChange = {},
                readOnly = true,
                singleLine = true,
                trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = expanded) },
                modifier = Modifier.menuAnchor().fillMaxWidth().testTag("csv-mapping-field-$header"),
            )
            ExposedDropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.import_csv_mapping_ignore)) },
                    onClick = { onFieldChange(""); expanded = false },
                )
                ImportableContactFields.ALL.forEach { field ->
                    DropdownMenuItem(
                        text = { Text(fieldLabel(field)) },
                        onClick = { onFieldChange(field); expanded = false },
                    )
                }
            }
        }
    }
}

@Composable
private fun CsvResultStep(created: Int, updated: Int, skipped: Int, onDone: () -> Unit) {
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

@Composable
private fun fieldLabel(field: String): String = if (field.isBlank()) {
    stringResource(R.string.import_csv_mapping_ignore)
} else {
    stringResource(fieldLabelRes(field))
}

private fun fieldLabelRes(field: String): Int = when (field) {
    "firstname" -> R.string.import_field_firstname
    "lastname" -> R.string.import_field_lastname
    "middle_name" -> R.string.import_field_middle_name
    "prefix" -> R.string.import_field_prefix
    "suffix" -> R.string.import_field_suffix
    "nickname" -> R.string.import_field_nickname
    "gender" -> R.string.import_field_gender
    "birthday" -> R.string.import_field_birthday
    "anniversary" -> R.string.import_field_anniversary
    "email" -> R.string.import_field_email
    "email_label" -> R.string.import_field_email_label
    "phone" -> R.string.import_field_phone
    "phone_label" -> R.string.import_field_phone_label
    "address_street" -> R.string.import_field_address_street
    "address_city" -> R.string.import_field_address_city
    "address_region" -> R.string.import_field_address_region
    "address_postal" -> R.string.import_field_address_postal
    "address_country" -> R.string.import_field_address_country
    "address_label" -> R.string.import_field_address_label
    "url" -> R.string.import_field_url
    "url_label" -> R.string.import_field_url_label
    "impp" -> R.string.import_field_impp
    "impp_label" -> R.string.import_field_impp_label
    "organization" -> R.string.import_field_organization
    "department" -> R.string.import_field_department
    "job_title" -> R.string.import_field_job_title
    "role" -> R.string.import_field_role
    "how_we_met" -> R.string.import_field_how_we_met
    "work_information" -> R.string.import_field_work_information
    "contact_information" -> R.string.import_field_contact_information
    "circles" -> R.string.import_field_circles
    "tags" -> R.string.import_field_tags
    else -> R.string.import_csv_mapping_ignore
}

// Named distinctly from VcfImportScreen's own PickedFileMeta — both are top-level classes, so
// (unlike top-level functions, which are scoped per file's synthetic facade class) a same-named
// private top-level class in this package would be a genuine JVM class redeclaration.
private data class CsvPickedFileMeta(val name: String, val size: Long?)

private fun queryFileMeta(resolver: ContentResolver, uri: Uri): CsvPickedFileMeta {
    var name = "import.csv"
    var size: Long? = null
    resolver.query(uri, null, null, null, null)?.use { cursor ->
        if (cursor.moveToFirst()) {
            val nameIndex = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
            if (nameIndex >= 0) cursor.getString(nameIndex)?.let { name = it }
            val sizeIndex = cursor.getColumnIndex(OpenableColumns.SIZE)
            if (sizeIndex >= 0 && !cursor.isNull(sizeIndex)) size = cursor.getLong(sizeIndex)
        }
    }
    return CsvPickedFileMeta(name, size)
}

private fun readAllBytes(resolver: ContentResolver, uri: Uri): ByteArray =
    resolver.openInputStream(uri)?.use { it.readBytes() } ?: ByteArray(0)
