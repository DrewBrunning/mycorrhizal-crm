package com.mycorrhizal.crm.feature.settings

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.ui.R

/**
 * Settings > Account (issue #1759): who you are signed in as, the server, and the security
 * entries that belong to the account. Changing the password is its own screen
 * ([ChangePasswordScreen]); the rest open the screens that already existed.
 */
@Composable
fun AccountSettingsScreen(
    onBack: () -> Unit,
    onServers: () -> Unit = {},
    onChangePassword: () -> Unit = {},
    onTwoFactor: () -> Unit = {},
    onPasskeys: () -> Unit = {},
    onApiTokens: () -> Unit = {},
    viewModel: SettingsViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    SettingsScaffold(
        title = stringResource(R.string.settings_group_account),
        onBack = onBack
    ) { padding ->
        AccountSettingsContent(
            state = state,
            onServers = onServers,
            onChangePassword = onChangePassword,
            onTwoFactor = onTwoFactor,
            onPasskeys = onPasskeys,
            onApiTokens = onApiTokens,
            modifier = padding
        )
    }
}

@Composable
fun AccountSettingsContent(
    state: SettingsUiState,
    modifier: Modifier = Modifier,
    onServers: () -> Unit = {},
    onChangePassword: () -> Unit = {},
    onTwoFactor: () -> Unit = {},
    onPasskeys: () -> Unit = {},
    onApiTokens: () -> Unit = {}
) {
    val gating = rememberSettingsGating()
    SettingsColumn(modifier = modifier) {
        // #208: section titles carry heading semantics so TalkBack's heading navigation finds them.
        SettingsSectionHeader(stringResource(R.string.settings_session))
        val dash = stringResource(R.string.settings_value_placeholder)
        SettingsInfoRow(stringResource(R.string.settings_server), state.session.serverUrl ?: dash)
        SettingsInfoRow(stringResource(R.string.settings_username), state.session.username ?: dash)
        SettingsInfoRow(
            stringResource(R.string.settings_admin),
            if (state.session.isAdmin) {
                stringResource(
                    R.string.settings_yes
                )
            } else {
                stringResource(R.string.settings_no)
            }
        )
        // ADR 0028 Decision 1: manage/toggle server profiles.
        SettingsNavigationRow(stringResource(R.string.settings_servers_title), onClick = onServers)

        SettingsSectionHeader(stringResource(R.string.settings_security))
        SettingsNavigationRow(
            stringResource(R.string.settings_password_title),
            onClick = onChangePassword
        )
        // N8 (issue #814): TOTP two-factor enrollment/management (web parity).
        // Issue #1263: absent on a deployment that does not offer 2FA.
        if (gating.twoFactorSupported) {
            SettingsNavigationRow(
                stringResource(R.string.settings_two_factor_title),
                onClick = onTwoFactor
            )
        }
        // Issue #1293 / ADR 0034: passkeys, offered only through an open gate (Remote profile +
        // server capability + Credential Manager on this device). An OIDC-provisioned account still
        // sees the entry and gets the server-worded refusal on the screen, exactly like the TOTP
        // screen.
        if (state.passkeysAvailable) {
            SettingsNavigationRow(
                stringResource(R.string.settings_passkeys_title),
                onClick = onPasskeys
            )
        }
        // Issue #413's Android follow-up (#573): API token management.
        if (gating.apiTokensSupported) {
            SettingsNavigationRow(
                stringResource(R.string.settings_api_tokens_title),
                onClick = onApiTokens
            )
        }
    }
}

/**
 * Settings > Account > Change password (issue #1759): the form that used to sit inline on the
 * Settings page. A successful change invalidates every JWT server-side, so the session is cleared
 * and [onLoggedOut] hands over to the login screen.
 */
@Composable
fun ChangePasswordScreen(
    onBack: () -> Unit,
    onLoggedOut: () -> Unit,
    viewModel: SettingsViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val events by viewModel.events.collectAsStateWithLifecycle()

    LaunchedEffect(events) {
        when (events) {
            // A password change invalidated every JWT; the session is cleared and the login
            // screen takes over (see onLoggedOut).
            SettingsEvent.PasswordChanged -> {
                viewModel.onEventShown()
                onLoggedOut()
            }

            else -> Unit
        }
    }

    SettingsScaffold(
        title = stringResource(R.string.settings_password_title),
        onBack = onBack
    ) { padding ->
        ChangePasswordContent(
            state = state,
            onChangePassword = viewModel::changePassword,
            modifier = padding
        )
    }
}

@Composable
fun ChangePasswordContent(
    state: SettingsUiState,
    modifier: Modifier = Modifier,
    onChangePassword: (String, String, String) -> Unit = { _, _, _ -> }
) {
    var currentPassword by remember { mutableStateOf("") }
    var newPassword by remember { mutableStateOf("") }
    var confirmPassword by remember { mutableStateOf("") }

    SettingsColumn(modifier = modifier) {
        state.passwordErrorRes?.let { res ->
            SettingsHelperText(
                text = stringResource(res),
                color = androidx.compose.material3.MaterialTheme.colorScheme.error,
                modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive }
            )
        }
        state.passwordError?.let { error ->
            SettingsHelperText(
                text = error,
                color = androidx.compose.material3.MaterialTheme.colorScheme.error,
                modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive }
            )
        }
        OutlinedTextField(
            value = currentPassword,
            onValueChange = { currentPassword = it },
            label = { Text(stringResource(R.string.settings_password_current)) },
            singleLine = true,
            visualTransformation = PasswordVisualTransformation(),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
            modifier = Modifier.fillMaxWidth()
        )
        OutlinedTextField(
            value = newPassword,
            onValueChange = { newPassword = it },
            label = { Text(stringResource(R.string.settings_password_new)) },
            singleLine = true,
            visualTransformation = PasswordVisualTransformation(),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
            modifier = Modifier.fillMaxWidth()
        )
        OutlinedTextField(
            value = confirmPassword,
            onValueChange = { confirmPassword = it },
            label = { Text(stringResource(R.string.settings_password_confirm)) },
            singleLine = true,
            visualTransformation = PasswordVisualTransformation(),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
            modifier = Modifier.fillMaxWidth()
        )
        val changingPasswordLabel = stringResource(R.string.a11y_state_saving)
        // #1760: a disabled button says what enables it, both as visible helper text and as the
        // button's TalkBack state description.
        val canChangePassword = newPassword.isNotBlank() && confirmPassword.isNotBlank()
        val passwordHint = stringResource(R.string.settings_password_change_hint)
        OutlinedButton(
            onClick = {
                onChangePassword(currentPassword, newPassword, confirmPassword)
                currentPassword = ""
                newPassword = ""
                confirmPassword = ""
            },
            enabled = !state.isChangingPassword && canChangePassword,
            modifier = Modifier
                .fillMaxWidth()
                .semantics {
                    if (state.isChangingPassword) {
                        stateDescription = changingPasswordLabel
                    } else if (!canChangePassword) {
                        stateDescription = passwordHint
                    }
                }
        ) {
            if (state.isChangingPassword) {
                CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp))
            }
            Text(stringResource(R.string.settings_password_change_button))
        }
        if (!canChangePassword && !state.isChangingPassword) {
            SettingsHelperText(passwordHint, modifier = Modifier.testTag("password-disabled-hint"))
        }
    }
}
