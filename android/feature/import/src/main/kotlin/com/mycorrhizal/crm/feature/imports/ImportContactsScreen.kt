package com.mycorrhizal.crm.feature.imports

import android.Manifest
import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.provider.Settings
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.selection.toggleable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.core.content.ContextCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.FileUpload
import androidx.compose.material.icons.outlined.History
import androidx.compose.material.icons.outlined.Menu
import androidx.compose.material.icons.outlined.Warning
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import androidx.compose.material3.MaterialTheme
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
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.R

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ImportContactsScreen(
    // Issue #150: null hides the hamburger — there is no drawer at Expanded.
    onMenuClick: (() -> Unit)? = {},
    onImported: () -> Unit = {},
    onImportVcf: () -> Unit = {},
    // Issue #834: CSV-file import + import history, sibling entry points to onImportVcf.
    onImportCsv: () -> Unit = {},
    onImportHistory: () -> Unit = {},
    viewModel: ImportContactsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    val context = LocalContext.current

    fun hasContactsPermission() =
        ContextCompat.checkSelfPermission(context, Manifest.permission.READ_CONTACTS) ==
            PackageManager.PERMISSION_GRANTED

    // Issue #1750: nothing reads device contacts until READ_CONTACTS is held.
    var granted by remember { mutableStateOf(hasContactsPermission()) }
    // Set once the user has refused the system dialog and it will not show again.
    var permanentlyDenied by remember { mutableStateOf(false) }

    val permissionLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestPermission(),
    ) { isGranted ->
        granted = isGranted
        if (isGranted) {
            permanentlyDenied = false
            viewModel.load()
        } else {
            permanentlyDenied = isPermanentlyDenied(
                shouldShowRationale = (context as? Activity)
                    ?.shouldShowRequestPermissionRationale(Manifest.permission.READ_CONTACTS) == true,
            )
        }
    }

    // Re-check on every resume: a grant/revoke in system settings (the Open
    // settings route) is reflected on return, and the device list refreshes
    // instead of staying stale until an app restart.
    val lifecycleOwner = LocalLifecycleOwner.current
    DisposableEffect(lifecycleOwner) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) {
                granted = hasContactsPermission()
                if (granted) {
                    permanentlyDenied = false
                    if (viewModel.uiState.value.let { it.step == ImportStep.LIST && !it.isLoading }) viewModel.load()
                }
            }
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
    }
    val needsPermission = !granted || state.permissionRequired

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    onMenuClick?.let { onMenu ->
                        AccessibleIconButton(onClick = onMenu) {
                            Icon(Icons.Outlined.Menu, contentDescription = stringResource(R.string.cd_menu))
                        }
                    }
                },
                title = {
                    Text(stringResource(R.string.import_title), style = MaterialTheme.typography.titleLarge)
                },
                actions = {
                    // Issue #834: web parity for the persisted import-run history (issue #651).
                    AccessibleIconButton(onClick = onImportHistory) {
                        Icon(Icons.Outlined.History, contentDescription = stringResource(R.string.import_history_entry))
                    }
                },
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
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            val preview = state.preview
            val showFileEntries = state.step == ImportStep.LIST && preview == null
            if (showFileEntries) {
                // M9 item 4 / issue #834: file-based imports. Issue #1750: always
                // reachable, independent of the contacts permission or whether the
                // device has any contacts to list.
                TextButton(
                    onClick = onImportVcf,
                    modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
                ) {
                    Icon(Icons.Outlined.FileUpload, contentDescription = null, modifier = Modifier.padding(end = 8.dp))
                    Text(stringResource(R.string.import_vcf_entry))
                }
                TextButton(
                    onClick = onImportCsv,
                    modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
                ) {
                    Icon(Icons.Outlined.FileUpload, contentDescription = null, modifier = Modifier.padding(end = 8.dp))
                    Text(stringResource(R.string.import_csv_entry))
                }
            }
            when {
                state.step == ImportStep.LIST && needsPermission ->
                    ContactsPermissionRationale(
                        permanentlyDenied = permanentlyDenied,
                        onGrant = { permissionLauncher.launch(Manifest.permission.READ_CONTACTS) },
                        onOpenSettings = {
                            context.startActivity(
                                Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.fromParts("package", context.packageName, null))
                                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
                            )
                        },
                    )
                state.isLoading -> LoadingSkeleton()
                state.step == ImportStep.REVIEW && preview != null ->
                    ImportReviewStep(
                        rows = preview.rows,
                        rowActions = state.rowActions,
                        onRowActionChange = viewModel::setRowAction,
                        onResolveAll = viewModel::resolveAll,
                        onConfirm = viewModel::confirmImport,
                    )
                state.step == ImportStep.RESULT -> {
                    Column(
                        horizontalAlignment = Alignment.CenterHorizontally,
                        modifier = Modifier.fillMaxSize().padding(24.dp),
                    ) {
                        Text(
                            text = stringResource(R.string.import_done, state.importedCount),
                            style = MaterialTheme.typography.bodyLarge,
                            modifier = Modifier.padding(top = 48.dp, bottom = 16.dp),
                        )
                        Button(
                            onClick = {
                                viewModel.startOver()
                                onImported()
                            },
                        ) {
                            Text(stringResource(R.string.action_confirm))
                        }
                    }
                }
                state.contacts.isEmpty() && state.error == null ->
                    EmptyState(message = stringResource(R.string.import_empty))
                state.contacts.isEmpty() && state.error != null -> {
                    Text(
                        text = state.error.orEmpty(),
                        color = MaterialTheme.colorScheme.error,
                        modifier = Modifier
                            .padding(24.dp)
                            .semantics { liveRegion = LiveRegionMode.Assertive },
                    )
                }
                else -> {
                    LazyColumn(modifier = Modifier.weight(1f)) {
                        items(state.contacts, key = { it.device.contactId }) { candidate ->
                            DeviceContactRow(
                                candidate = candidate,
                                checked = candidate.device.contactId in state.selected,
                                onToggle = { viewModel.toggle(candidate.device.contactId) },
                            )
                        }
                    }
                    Button(
                        onClick = { viewModel.submitSelected() },
                        enabled = state.selected.isNotEmpty() && !state.isImporting,
                        modifier = Modifier.fillMaxWidth().padding(16.dp),
                    ) {
                        Text(
                            if (state.selected.isEmpty()) {
                                stringResource(R.string.import_button_none)
                            } else {
                                stringResource(
                                    R.string.import_button,
                                    state.selected.size,
                                    state.contacts.count { it.duplicateOf != null },
                                )
                            },
                        )
                    }
                }
            }
        }
    }

    state.error?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
        }
    }
}

/** After a refused request, the OS will not show the dialog again exactly when
 *  it also says no rationale should be shown (the user ticked "don't ask again"). */
internal fun isPermanentlyDenied(shouldShowRationale: Boolean): Boolean = !shouldShowRationale

@Composable
private fun ContactsPermissionRationale(
    permanentlyDenied: Boolean,
    onGrant: () -> Unit,
    onOpenSettings: () -> Unit,
) {
    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(16.dp),
        modifier = Modifier.fillMaxWidth().padding(24.dp),
    ) {
        Text(
            text = stringResource(R.string.import_permission_rationale),
            style = MaterialTheme.typography.bodyLarge,
        )
        if (permanentlyDenied) {
            Button(onClick = onOpenSettings) { Text(stringResource(R.string.import_permission_open_settings)) }
        } else {
            Button(onClick = onGrant) { Text(stringResource(R.string.import_permission_grant)) }
        }
    }
}

@Composable
private fun DeviceContactRow(
    candidate: DeviceContactCandidate,
    checked: Boolean,
    onToggle: () -> Unit,
) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp),
        // #214: a bare Checkbox has no text/contentDescription of its own —
        // it's independently screen-reader-focusable (nested clickable), so
        // it never picks up the row's Text on merge, and announced with no
        // name. Modifier.toggleable on the row merges the display name into
        // the checkbox's accessible name (the standard Material3
        // labeled-checkbox pattern); the inner Checkbox becomes decorative.
        modifier = Modifier
            .fillMaxWidth()
            .toggleable(value = checked, onValueChange = { onToggle() }, role = Role.Checkbox)
            .padding(horizontal = 16.dp, vertical = 8.dp),
    ) {
        Checkbox(checked = checked, onCheckedChange = null)
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = candidate.device.displayName ?: stringResource(R.string.import_unnamed),
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            if (candidate.duplicateOf != null) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(
                        Icons.Outlined.Warning,
                        contentDescription = null,
                        tint = MaterialTheme.colorScheme.error,
                        modifier = Modifier.padding(end = 4.dp),
                    )
                    Text(
                        text = stringResource(R.string.import_duplicate, candidate.duplicateOf.displayName),
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }
        }
    }
}
