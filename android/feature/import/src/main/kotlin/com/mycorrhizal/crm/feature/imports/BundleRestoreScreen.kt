package com.mycorrhizal.crm.feature.imports

import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.CloudUpload
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
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
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Issue #1264: "Restore from bundle" — pick an account-bundle file (SAF
 * `ACTION_OPEN_DOCUMENT`), review it through the shared import review step,
 * confirm. See [BundleRestoreViewModel] for the server round-trip.
 */
@Composable
fun BundleRestoreScreen(
    onBack: () -> Unit,
    onDone: () -> Unit,
    viewModel: BundleRestoreViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    BundleRestoreScreenContent(
        uiState = state,
        onBack = {
            viewModel.cancel()
            onBack()
        },
        onDone = onDone,
        onFilePicked = viewModel::onFilePicked,
        onFileTooLarge = viewModel::onFileTooLarge,
        onRowActionChange = viewModel::setRowAction,
        onResolveAll = viewModel::resolveAll,
        onConfirm = viewModel::confirm,
        onErrorShown = viewModel::onErrorShown,
    )
}

/** Stateless content, split out so it is testable without a Hilt-backed ViewModel. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BundleRestoreScreenContent(
    uiState: BundleRestoreUiState,
    onBack: () -> Unit = {},
    onDone: () -> Unit = {},
    onFilePicked: (String, ByteArray) -> Unit = { _, _ -> },
    onFileTooLarge: () -> Unit = {},
    onRowActionChange: (Int, String) -> Unit = { _, _ -> },
    onResolveAll: () -> Unit = {},
    onConfirm: () -> Unit = {},
    onErrorShown: () -> Unit = {},
) {
    val snackbarHostState = remember { SnackbarHostState() }
    val context = LocalContext.current
    val scope = rememberCoroutineScope()

    val filePicker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri: Uri? ->
        if (uri == null) return@rememberLauncherForActivityResult
        scope.launch {
            val resolver = context.contentResolver
            // Probe the declared size before reading, so an oversized pick is
            // rejected without allocating it (same guard as the VCF import).
            val meta = withContext(Dispatchers.IO) { queryFileMeta(resolver, uri, "account-bundle.json") }
            if (meta.size != null && meta.size > BundleRestoreViewModel.MAX_BUNDLE_SIZE_BYTES) {
                onFileTooLarge()
                return@launch
            }
            val bytes = withContext(Dispatchers.IO) { readPickedBytes(resolver, uri) }
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
                title = {
                    Text(stringResource(R.string.bundle_restore_title), style = MaterialTheme.typography.titleLarge)
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
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when (uiState.step) {
                BundleRestoreStep.WORKING -> Column(modifier = Modifier.fillMaxSize()) {
                    Text(
                        text = stringResource(R.string.bundle_restore_working),
                        style = MaterialTheme.typography.bodyMedium,
                        modifier = Modifier.padding(16.dp).testTag("bundle-restore-working"),
                    )
                    LoadingSkeleton()
                }
                BundleRestoreStep.PICK -> PickStep(onPick = { filePicker.launch(arrayOf("application/json", "*/*")) })
                BundleRestoreStep.REVIEW -> uiState.preview?.let { preview ->
                    Column(modifier = Modifier.fillMaxSize()) {
                        if (preview.lossReport.isNotEmpty()) {
                            Text(
                                text = stringResource(R.string.bundle_restore_loss, preview.lossReport.size),
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.error,
                                modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp)
                                    .testTag("bundle-restore-loss"),
                            )
                        }
                        Box(modifier = Modifier.weight(1f)) {
                            ImportReviewStep(
                                rows = preview.rows,
                                rowActions = uiState.rowActions,
                                onRowActionChange = onRowActionChange,
                                onResolveAll = onResolveAll,
                                onConfirm = onConfirm,
                            )
                        }
                    }
                }
                BundleRestoreStep.RESULT -> uiState.result?.let { result ->
                    Column(
                        horizontalAlignment = Alignment.CenterHorizontally,
                        verticalArrangement = Arrangement.spacedBy(16.dp),
                        modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(24.dp),
                    ) {
                        Text(
                            text = stringResource(
                                R.string.bundle_restore_result,
                                result.created,
                                result.updated,
                                result.skipped,
                            ),
                            style = MaterialTheme.typography.bodyLarge,
                            modifier = Modifier.padding(top = 48.dp).testTag("bundle-restore-result"),
                        )
                        Button(onClick = onDone) { Text(stringResource(R.string.action_confirm)) }
                    }
                }
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
        Icon(Icons.Outlined.CloudUpload, contentDescription = null, modifier = Modifier.padding(top = 48.dp))
        Text(
            text = stringResource(R.string.bundle_restore_pick_hint),
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.padding(vertical = 16.dp),
        )
        Button(onClick = onPick, modifier = Modifier.testTag("bundle-restore-pick")) {
            Text(stringResource(R.string.bundle_restore_pick))
        }
    }
}
