package com.mycorrhizal.crm.feature.settings

import android.os.Build
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.domain.repository.AppSettingsRepository
import com.mycorrhizal.crm.domain.repository.AutoLockDelay
import com.mycorrhizal.crm.domain.repository.BiometricEnrollmentStatus
import com.mycorrhizal.crm.ui.R

/**
 * Settings > Appearance (issue #1759): language, date format and theme. The date format lives
 * here, beside the other display preferences, so a time format picker (issue #1830) sits naturally
 * next to it.
 */
@Composable
fun AppearanceSettingsScreen(
    onBack: () -> Unit,
    onLocaleChanged: () -> Unit = {},
    viewModel: SettingsViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val events by viewModel.events.collectAsStateWithLifecycle()

    LaunchedEffect(events) {
        when (events) {
            // The activity recreates to re-resolve values-XX resources.
            SettingsEvent.LocaleChanged -> {
                viewModel.onEventShown()
                onLocaleChanged()
            }

            else -> Unit
        }
    }

    SettingsScaffold(
        title = stringResource(R.string.settings_appearance),
        onBack = onBack
    ) { padding ->
        AppearanceSettingsContent(
            state = state,
            onLanguageChange = viewModel::updateLanguage,
            onDateFormatChange = viewModel::updateDateFormat,
            onThemeChange = viewModel::setThemePreference,
            modifier = padding
        )
    }
}

@Composable
fun AppearanceSettingsContent(
    state: SettingsUiState,
    modifier: Modifier = Modifier,
    onLanguageChange: (String) -> Unit = {},
    onDateFormatChange: (String) -> Unit = {},
    onThemeChange: (String) -> Unit = {}
) {
    SettingsColumn(modifier = modifier) {
        SettingsDropdown(
            label = stringResource(R.string.settings_language),
            value = state.session.language.orEmpty(),
            options = listOf("en", "de", "it", "es", "fr"),
            optionLabel = { lang -> languageName(lang) },
            onSelect = onLanguageChange
        )
        SettingsDropdown(
            label = stringResource(R.string.settings_date_format),
            value = state.session.dateFormat.orEmpty(),
            options = DATE_FORMAT_OPTIONS,
            optionLabel = { fmt -> dateFormatName(fmt) },
            onSelect = onDateFormatChange
        )
        SettingsDropdown(
            label = stringResource(R.string.settings_theme),
            value = state.themePreference,
            options = listOf(
                AppSettingsRepository.THEME_SYSTEM,
                AppSettingsRepository.THEME_LIGHT,
                AppSettingsRepository.THEME_DARK
            ),
            optionLabel = { pref ->
                when (pref) {
                    AppSettingsRepository.THEME_LIGHT -> stringResource(
                        R.string.settings_theme_light
                    )

                    AppSettingsRepository.THEME_DARK -> stringResource(R.string.settings_theme_dark)

                    else -> stringResource(R.string.settings_theme_system)
                }
            },
            onSelect = onThemeChange
        )
    }
}

/**
 * Settings > Privacy & security (issue #1759): the app lock (one control, biometric sign-in is
 * enrolled inline when it is switched on) and the screenshot block.
 */
@Composable
fun PrivacySettingsScreen(onBack: () -> Unit, viewModel: SettingsViewModel = hiltViewModel()) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val gating = rememberSettingsGating()
    SettingsScaffold(
        title = stringResource(R.string.settings_group_privacy),
        onBack = onBack
    ) { padding ->
        PrivacySettingsContent(
            state = state,
            onAppLockChange = { enabled ->
                viewModel.setAppLock(
                    enabled,
                    gating.deviceGrantSignInSupported
                )
            },
            onAutoLockDelayChange = viewModel::setAutoLockDelay,
            onBlockScreenshotsChange = viewModel::setBlockScreenshots,
            onRemoveBiometricSignIn = viewModel::removeBiometricSignIn,
            modifier = padding
        )
    }
}

@Composable
fun PrivacySettingsContent(
    state: SettingsUiState,
    modifier: Modifier = Modifier,
    onAppLockChange: (Boolean) -> Unit = {},
    onAutoLockDelayChange: (AutoLockDelay) -> Unit = {},
    onBlockScreenshotsChange: (Boolean) -> Unit = {},
    onRemoveBiometricSignIn: () -> Unit = {}
) {
    SettingsColumn(modifier = modifier) {
        // Issue #722: the opt-in local app lock — a biometric / device-PIN check before a persisted
        // session resumes. Default off: existing installs behave exactly as before until the user
        // opts in. Devices without a Class 3 biometric or a secure lock screen get the disabled
        // toggle plus the reason, never a toggle that cannot be satisfied.
        SettingsSectionHeader(stringResource(R.string.settings_local_auth))
        if (!state.localAuthSupported) {
            SettingsHelperText(stringResource(R.string.settings_local_auth_unavailable))
        }
        SettingsToggleRow(
            label = stringResource(R.string.settings_local_auth_enable),
            checked = state.requireLocalAuth,
            enabled = state.localAuthSupported,
            onCheckedChange = onAppLockChange
        )
        if (state.requireLocalAuth) {
            SettingsHelperText(stringResource(R.string.settings_local_auth_enable_hint))
            SettingsDropdown(
                label = stringResource(R.string.settings_local_auth_delay),
                value = state.autoLockDelay.name,
                options = AutoLockDelay.entries.map { it.name },
                optionLabel = { lockDelayOptionLabel(it) },
                onSelect = { name -> onAutoLockDelayChange(AutoLockDelay.valueOf(name)) }
            )
        }
        // Issue #722: fully biometric login. Enrollment happens inline when the lock is switched
        // on (SettingsViewModel.setAppLock); here is only its status.
        if (state.isBiometricBusy) {
            CircularProgressIndicator(modifier = Modifier.padding(vertical = 8.dp))
        }
        if (state.biometricEnrollmentStatus == BiometricEnrollmentStatus.ENROLLED) {
            SettingsHelperText(stringResource(R.string.settings_biometric_signin_enabled))
            // The post-login "Sign in with biometrics?" prompt enrolls without switching the lock on
            // (and so did the old Settings button), so an install can hold a grant with the lock off.
            // With the lock on, switching it off revokes the grant; with it off there is no switch to
            // do that, so the revoke stays reachable here.
            if (!state.requireLocalAuth) {
                OutlinedButton(
                    onClick = onRemoveBiometricSignIn,
                    enabled = !state.isBiometricBusy,
                    modifier = Modifier.fillMaxWidth().testTag("biometric-signin-remove")
                ) {
                    Text(stringResource(R.string.settings_biometric_signin_remove))
                }
            }
        }
        state.biometricErrorRes?.let { res ->
            SettingsHelperText(
                text = stringResource(res),
                color = androidx.compose.material3.MaterialTheme.colorScheme.error,
                modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive }
            )
        }

        // Issue #1741: opt-in screenshot / screen-recording block (FLAG_SECURE). Default off. The
        // recent-apps preview is blanked on API 33+ regardless; below that this switch is the only
        // thing hiding it.
        SettingsSectionHeader(stringResource(R.string.settings_screen_privacy))
        SettingsToggleRow(
            label = stringResource(R.string.settings_block_screenshots),
            checked = state.blockScreenshots,
            onCheckedChange = onBlockScreenshotsChange
        )
        SettingsHelperText(
            stringResource(R.string.settings_block_screenshots_hint) +
                if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) {
                    " " + stringResource(R.string.settings_block_screenshots_hint_legacy)
                } else {
                    ""
                }
        )
    }
}
