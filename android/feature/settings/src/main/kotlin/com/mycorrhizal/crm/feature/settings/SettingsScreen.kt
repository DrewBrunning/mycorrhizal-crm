package com.mycorrhizal.crm.feature.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Menu
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.ui.theme.AppTypography
import com.mycorrhizal.crm.ui.R

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(
    onMenuClick: () -> Unit,
    onLoggedOut: () -> Unit,
    onCustomLinks: () -> Unit = {},
    viewModel: SettingsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val events by viewModel.events.collectAsStateWithLifecycle()

    LaunchedEffect(events) {
        if (events is SettingsEvent.LoggedOut) {
            viewModel.onLoggedOutShown()
            onLoggedOut()
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onMenuClick) {
                        Icon(Icons.Outlined.Menu, contentDescription = stringResource(R.string.cd_menu))
                    }
                },
                title = {
                    Text(stringResource(R.string.settings_title), style = AppTypography.appBarTitle)
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
    ) { padding ->
        SettingsContent(
            state = state,
            onCustomLinks = onCustomLinks,
            onCallTrackingChange = viewModel::setCallTrackingEnabled,
            onSmsTrackingChange = viewModel::setSmsTrackingEnabled,
            onNotificationsChange = viewModel::setNotificationsEnabled,
            onLogout = viewModel::logout,
            modifier = Modifier.padding(padding),
        )
    }
}

@Composable
fun SettingsContent(
    state: SettingsUiState,
    onCustomLinks: () -> Unit = {},
    onCallTrackingChange: (Boolean) -> Unit = {},
    onSmsTrackingChange: (Boolean) -> Unit = {},
    onNotificationsChange: (Boolean) -> Unit = {},
    onLogout: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var confirmLogout by remember { mutableStateOf(false) }

    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Text(stringResource(R.string.settings_session), style = MaterialTheme.typography.titleMedium)
        val dash = stringResource(R.string.settings_value_placeholder)
        InfoRow(stringResource(R.string.settings_server), state.session.serverUrl ?: dash)
        InfoRow(stringResource(R.string.settings_username), state.session.username ?: dash)
        InfoRow(stringResource(R.string.settings_language), state.session.language ?: dash)
        InfoRow(stringResource(R.string.settings_date_format), state.session.dateFormat ?: dash)
        InfoRow(
            stringResource(R.string.settings_admin),
            if (state.session.isAdmin) stringResource(R.string.settings_yes) else stringResource(R.string.settings_no),
        )

        Text(stringResource(R.string.settings_tracking), style = MaterialTheme.typography.titleMedium)
        ToggleRow(
            label = stringResource(R.string.settings_call_tracking),
            checked = state.callTrackingEnabled,
            onCheckedChange = onCallTrackingChange,
        )
        ToggleRow(
            label = stringResource(R.string.settings_sms_tracking),
            checked = state.smsTrackingEnabled,
            onCheckedChange = onSmsTrackingChange,
        )
        ToggleRow(
            label = stringResource(R.string.settings_notifications),
            checked = state.notificationsEnabled,
            onCheckedChange = onNotificationsChange,
        )
        Button(
            onClick = onCustomLinks,
            modifier = Modifier.fillMaxWidth(),
        ) {
            Text(stringResource(R.string.custom_links_title))
        }

        Button(
            onClick = { confirmLogout = true },
            enabled = !state.isLoggingOut,
            modifier = Modifier.fillMaxWidth(),
        ) {
            if (state.isLoggingOut) {
                CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp))
            }
            Text(stringResource(R.string.settings_log_out))
        }
    }

    if (confirmLogout) {
        androidx.compose.material3.AlertDialog(
            onDismissRequest = { confirmLogout = false },
            title = { Text(stringResource(R.string.settings_log_out_title)) },
            text = { Text(stringResource(R.string.settings_log_out_body)) },
            confirmButton = {
                Button(onClick = { confirmLogout = false; onLogout() }) {
                    Text(stringResource(R.string.settings_log_out))
                }
            },
            dismissButton = {
                androidx.compose.material3.TextButton(onClick = { confirmLogout = false }) {
                    Text(stringResource(R.string.settings_cancel))
                }
            },
        )
    }
}

@Composable
private fun InfoRow(label: String, value: String) {
    Column {
        Text(
            text = label,
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(text = value, style = MaterialTheme.typography.bodyLarge)
    }
}

@Composable
private fun ToggleRow(
    label: String,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 4.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = androidx.compose.ui.Alignment.CenterVertically,
    ) {
        Text(label, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.weight(1f))
        Switch(checked = checked, onCheckedChange = onCheckedChange)
    }
}
