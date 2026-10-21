package com.mycorrhizal.crm.feature.settings

import android.app.Activity
import android.content.Intent
import android.net.Uri
import android.provider.Settings
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.res.stringResource
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.feature.tracking.TrackingPermissions
import com.mycorrhizal.crm.ui.R

/** The OS permissions a [TrackingPermissionRequest] needs (issue #721). */
private fun permissionsFor(request: TrackingPermissionRequest): Array<String> = when (request) {
    TrackingPermissionRequest.CALL_TRACKING -> TrackingPermissions.CALL_TRACKING
    TrackingPermissionRequest.SMS_TRACKING -> TrackingPermissions.SMS_TRACKING
}

/** Deep link to this app's page in the system settings (permanent-denial path). */
private fun openAppSettings(context: android.content.Context) {
    val activity = context as? Activity ?: return
    activity.startActivity(
        Intent(
            Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
            Uri.fromParts("package", activity.packageName, null),
        ),
    )
}

/**
 * Settings > Tracking & notifications (issue #1759): the call/SMS capture toggles, the
 * notification toggle and the notification-channels entry.
 */
@Composable
fun TrackingSettingsScreen(
    onBack: () -> Unit,
    onNotificationChannels: () -> Unit = {},
    viewModel: SettingsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()

    // --- Issue #721: runtime-permission flow for the tracking toggles --------
    // The ViewModel decides *when* a permission is needed (state.pendingPermissionRequest);
    // this screen is the thin adapter that turns that into a system dialog and
    // reports the outcome (plus whether a denial is permanent — only an Activity
    // can answer shouldShowRequestPermissionRationale) back to the ViewModel.
    val context = LocalContext.current

    // Which request the in-flight system dialog is answering, so the callback
    // knows what to report. remember (not a VM field): it is UI-only state that
    // exists to bridge the launcher callback to the request that spawned it.
    var launchedRequest by remember { mutableStateOf<TrackingPermissionRequest?>(null) }

    val permissionLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestMultiplePermissions(),
    ) { grantResults ->
        val request = launchedRequest
        launchedRequest = null
        if (request == null) return@rememberLauncherForActivityResult
        // "Permanently denied" = denied and the OS will not show the dialog
        // again (the user checked "don't ask again"); a first denial still
        // allows a rationale + retry.
        val activity = context as? Activity
        val permanentlyDenied = permissionsFor(request).any { permission ->
            grantResults[permission] != true &&
                (activity?.shouldShowRequestPermissionRationale(permission) != true)
        }
        viewModel.onPermissionRequestResult(request, permanentlyDenied)
    }

    LaunchedEffect(state.pendingPermissionRequest) {
        val request = state.pendingPermissionRequest ?: return@LaunchedEffect
        if (launchedRequest == request) return@LaunchedEffect
        launchedRequest = request
        permissionLauncher.launch(permissionsFor(request))
    }

    // Reconcile the toggles against the real OS grant state every time the
    // screen resumes: a permission revoked (or granted) in system settings
    // must be reflected the moment the user returns.
    val lifecycleOwner = LocalLifecycleOwner.current
    DisposableEffect(lifecycleOwner) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) viewModel.refreshPermissionState()
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
    }

    SettingsScaffold(title = stringResource(R.string.settings_tracking), onBack = onBack) { padding ->
        TrackingSettingsContent(
            state = state,
            onCallTrackingChange = viewModel::setCallTrackingEnabled,
            onSmsTrackingChange = viewModel::setSmsTrackingEnabled,
            onNotificationsChange = viewModel::setNotificationsEnabled,
            onIncludeUnknownChange = viewModel::setIncludeUnknownNumbers,
            onNotificationChannels = onNotificationChannels,
            onPermissionDialogDismiss = viewModel::onPermissionDialogDismiss,
            onPermissionDialogRetry = viewModel::onPermissionDialogRetry,
            onPermissionDialogOpenSettings = {
                viewModel.onPermissionDialogDismiss()
                openAppSettings(context)
            },
            modifier = padding,
        )
    }
}

@Composable
fun TrackingSettingsContent(
    state: SettingsUiState,
    onCallTrackingChange: (Boolean) -> Unit = {},
    onSmsTrackingChange: (Boolean) -> Unit = {},
    onNotificationsChange: (Boolean) -> Unit = {},
    // Issue #1029: the unknown-number capture escape hatch.
    onIncludeUnknownChange: (Boolean) -> Unit = {},
    onNotificationChannels: () -> Unit = {},
    // Issue #721: tracking-permission denial dialogs (rationale / open settings).
    onPermissionDialogDismiss: () -> Unit = {},
    onPermissionDialogRetry: () -> Unit = {},
    onPermissionDialogOpenSettings: () -> Unit = {},
    modifier: Modifier = Modifier,
) {
    val gating = rememberSettingsGating()
    SettingsColumn(modifier = modifier) {
        // Issue #1200: the call/SMS capture toggles (and the unknown-numbers option, which only
        // makes sense with capture on) exist only in builds that declare and hold the capture
        // permissions. The play flavor omits them — showing a switch that can never work would be
        // worse than explaining the absence.
        if (state.callSmsTrackingAvailable) {
            SettingsToggleRow(
                label = stringResource(R.string.settings_call_tracking),
                checked = state.callTrackingEnabled,
                onCheckedChange = onCallTrackingChange,
            )
            SettingsToggleRow(
                label = stringResource(R.string.settings_sms_tracking),
                checked = state.smsTrackingEnabled,
                onCheckedChange = onSmsTrackingChange,
            )
        } else {
            SettingsHelperText(stringResource(R.string.settings_call_sms_unavailable))
        }
        SettingsToggleRow(
            label = stringResource(R.string.settings_notifications),
            checked = state.notificationsEnabled,
            onCheckedChange = onNotificationsChange,
        )
        if (state.callSmsTrackingAvailable) {
            // Issue #1029: by default only interactions whose number maps to a contact are logged;
            // this opts back into logging unknown numbers too.
            SettingsToggleRow(
                label = stringResource(R.string.settings_include_unknown_numbers),
                checked = state.includeUnknownNumbers,
                onCheckedChange = onIncludeUnknownChange,
            )
            if (state.filteredUnknownCount > 0) {
                SettingsHelperText(stringResource(R.string.settings_filtered_unknown_count, state.filteredUnknownCount))
            }
        }
        // Issue #1263: hidden when the deployment does not declare push.
        if (gating.notificationChannelsSupported) {
            SettingsNavigationRow(stringResource(R.string.settings_notifications_title), onClick = onNotificationChannels)
        }
    }

    // Issue #721: a tracking toggle was switched on but its permissions were denied. A Rationale
    // dialog explains why and offers a retry; an AppSettings dialog (permanent denial) offers the
    // system settings deep link. The toggle stays off either way until the permissions are
    // actually granted.
    state.permissionDialog?.let { dialog ->
        val request = when (dialog) {
            is TrackingPermissionDialog.Rationale -> dialog.request
            is TrackingPermissionDialog.AppSettings -> dialog.request
        }
        val title = when (dialog) {
            is TrackingPermissionDialog.Rationale -> stringResource(R.string.settings_permission_rationale_title)
            is TrackingPermissionDialog.AppSettings -> stringResource(R.string.settings_permission_settings_title)
        }
        val body = when (dialog) {
            is TrackingPermissionDialog.Rationale -> when (request) {
                TrackingPermissionRequest.CALL_TRACKING ->
                    stringResource(R.string.settings_call_permission_rationale)
                TrackingPermissionRequest.SMS_TRACKING ->
                    stringResource(R.string.settings_sms_permission_rationale)
            }
            is TrackingPermissionDialog.AppSettings -> when (request) {
                TrackingPermissionRequest.CALL_TRACKING ->
                    stringResource(R.string.settings_call_permission_settings)
                TrackingPermissionRequest.SMS_TRACKING ->
                    stringResource(R.string.settings_sms_permission_settings)
            }
        }
        val confirmLabel = when (dialog) {
            is TrackingPermissionDialog.Rationale -> stringResource(R.string.settings_permission_try_again)
            is TrackingPermissionDialog.AppSettings -> stringResource(R.string.settings_permission_open_settings)
        }
        val confirmAction = when (dialog) {
            is TrackingPermissionDialog.Rationale -> onPermissionDialogRetry
            is TrackingPermissionDialog.AppSettings -> onPermissionDialogOpenSettings
        }
        AlertDialog(
            onDismissRequest = onPermissionDialogDismiss,
            title = { Text(title) },
            text = { Text(body) },
            confirmButton = {
                TextButton(onClick = confirmAction) { Text(confirmLabel) }
            },
            dismissButton = {
                TextButton(onClick = onPermissionDialogDismiss) {
                    Text(stringResource(R.string.settings_permission_not_now))
                }
            },
        )
    }
}
