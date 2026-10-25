package com.mycorrhizal.crm.feature.settings

import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.Logout
import androidx.compose.material.icons.outlined.AdminPanelSettings
import androidx.compose.material.icons.outlined.Contacts
import androidx.compose.material.icons.outlined.Extension
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Notifications
import androidx.compose.material.icons.outlined.Palette
import androidx.compose.material.icons.outlined.Person
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.domain.compat.ServerCapabilities
import com.mycorrhizal.crm.domain.compat.ServerCapability
import com.mycorrhizal.crm.domain.compat.ServerFeature
import com.mycorrhizal.crm.domain.repository.AutoLockDelay
import com.mycorrhizal.crm.model.util.DateFormat
import com.mycorrhizal.crm.ui.LocalServerCapabilities
import com.mycorrhizal.crm.ui.LocalServerVersion
import com.mycorrhizal.crm.ui.R

/**
 * Issue #1759: the routes of the Settings root and its grouped sub-screens. The app's NavHost
 * registers each of them; the root opens them through its `onNavigate` callback, so the root and
 * its tests never name a screen class. Screens that already existed (`webhooks`, `two-factor`, ...)
 * keep their own routes and are opened from inside the group that owns them.
 */
object SettingsRoutes {
    const val ROOT = "settings"
    const val ACCOUNT = "settings/account"
    const val CHANGE_PASSWORD = "settings/account/password"
    const val APPEARANCE = "settings/appearance"
    const val PRIVACY = "settings/privacy"
    const val CONTACTS_FIELDS = "settings/contacts-fields"
    const val TRACKING = "settings/tracking"
    const val INTEGRATIONS = "settings/integrations"
    const val ADMINISTRATION = "settings/administration"
    const val ABOUT = "settings/about"
    const val ABOUT_LICENSES = "settings/about/licenses"
}

/**
 * What the connected deployment offers, resolved once per screen from the two composition locals.
 * Every Settings screen gates its rows with the same rules, so the rules live here.
 */
internal data class SettingsGating(
    val multiUserAdminSupported: Boolean,
    val systemEventsSupported: Boolean,
    val deviceGrantSignInSupported: Boolean,
    val twoFactorSupported: Boolean,
    val webhooksSupported: Boolean,
    val apiTokensSupported: Boolean,
    val calendarSyncSupported: Boolean,
    val notificationChannelsSupported: Boolean,
    val integrationsSupported: Boolean
) {
    /** The Integrations group has at least one row on this deployment. */
    val anyIntegration: Boolean get() = webhooksSupported || calendarSyncSupported ||
        integrationsSupported
}

@Composable
internal fun rememberSettingsGating(): SettingsGating {
    // Issue #692: capabilities the connected server is too old to provide are hidden. Null server
    // version (unknown / fail open) leaves everything visible.
    val serverVersion = LocalServerVersion.current
    // Issue #1263 / ADR 0028 Decision 2: entries whose capability the active deployment does not
    // declare are hidden. Unknown (fail open) shows everything, so an unreachable /health never
    // strips the UI.
    val capabilities = LocalServerCapabilities.current
    // Admin user management and operator system events are surfaces of a multi-user server
    // deployment; a deployment that cannot register users (the embedded single-user store) omits
    // `registration`, so both are hidden there. There is no dedicated admin token in the health
    // contract.
    val multiUserAdminSupported = capabilities.supports(ServerCapability.REGISTRATION)
    return SettingsGating(
        multiUserAdminSupported = multiUserAdminSupported,
        systemEventsSupported =
            multiUserAdminSupported && ServerCapabilities.isSupported(
                serverVersion,
                ServerFeature.SYSTEM_EVENTS
            ),
        deviceGrantSignInSupported =
            capabilities.supports(ServerCapability.DEVICE_GRANTS) &&
                ServerCapabilities.isSupported(serverVersion, ServerFeature.DEVICE_GRANT_SIGNIN),
        twoFactorSupported = capabilities.supports(ServerCapability.TWO_FACTOR),
        webhooksSupported = capabilities.supports(ServerCapability.WEBHOOKS),
        apiTokensSupported = capabilities.supports(ServerCapability.API_TOKENS),
        calendarSyncSupported = capabilities.supports(ServerCapability.CALENDAR),
        notificationChannelsSupported = capabilities.supports(ServerCapability.PUSH),
        // Issue #1367 / ADR 0028 Decision 2 amendment: the embedded (on-device) server is storage
        // only, so the outbound file/photo integrations are absent there. They have no capability
        // token of their own (a token would make an older remote server that predates it look like
        // it lacked the feature), so the deployment shape is the gate. Backend counterpart:
        // routes.go's `!cfg.IsEmbedded()` guards.
        integrationsSupported = !capabilities.isEmbedded
    )
}

/**
 * The Settings root (issue #1759): a short index of grouped chevron rows, each opening a
 * sub-screen. It carries no form fields; everything editable lives one level down.
 */
@Composable
fun SettingsScreen(
    // Issue #150: null hides the hamburger — there is no drawer at Expanded.
    onMenuClick: (() -> Unit)? = {},
    onLoggedOut: () -> Unit,
    onNavigate: (String) -> Unit = {},
    viewModel: SettingsViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val events by viewModel.events.collectAsStateWithLifecycle()

    LaunchedEffect(events) {
        when (events) {
            SettingsEvent.LoggedOut -> {
                viewModel.onEventShown()
                onLoggedOut()
            }

            else -> Unit
        }
    }

    SettingsScaffold(
        title = stringResource(R.string.settings_title),
        onMenuClick = onMenuClick
    ) { padding ->
        SettingsRootContent(
            state = state,
            onNavigate = onNavigate,
            onLogout = viewModel::logout,
            modifier = padding
        )
    }
}

@Composable
fun SettingsRootContent(
    state: SettingsUiState,
    onNavigate: (String) -> Unit = {},
    onLogout: () -> Unit,
    modifier: Modifier = Modifier
) {
    var confirmLogout by remember { mutableStateOf(false) }
    val gating = rememberSettingsGating()

    SettingsColumn(modifier = modifier) {
        SettingsNavigationRow(
            title = stringResource(R.string.settings_group_account),
            supporting = stringResource(R.string.settings_group_account_summary),
            icon = Icons.Outlined.Person,
            onClick = { onNavigate(SettingsRoutes.ACCOUNT) },
            modifier = Modifier.testTag("settings-group-account")
        )
        SettingsNavigationRow(
            title = stringResource(R.string.settings_appearance),
            supporting = stringResource(R.string.settings_group_appearance_summary),
            icon = Icons.Outlined.Palette,
            onClick = { onNavigate(SettingsRoutes.APPEARANCE) },
            modifier = Modifier.testTag("settings-group-appearance")
        )
        SettingsNavigationRow(
            title = stringResource(R.string.settings_group_privacy),
            supporting = stringResource(R.string.settings_group_privacy_summary),
            icon = Icons.Outlined.Lock,
            onClick = { onNavigate(SettingsRoutes.PRIVACY) },
            modifier = Modifier.testTag("settings-group-privacy")
        )
        SettingsNavigationRow(
            title = stringResource(R.string.settings_group_contacts_fields),
            supporting = stringResource(R.string.settings_group_contacts_fields_summary),
            icon = Icons.Outlined.Contacts,
            onClick = { onNavigate(SettingsRoutes.CONTACTS_FIELDS) },
            modifier = Modifier.testTag("settings-group-contacts-fields")
        )
        SettingsNavigationRow(
            title = stringResource(R.string.settings_tracking),
            supporting = stringResource(R.string.settings_group_tracking_summary),
            icon = Icons.Outlined.Notifications,
            onClick = { onNavigate(SettingsRoutes.TRACKING) },
            modifier = Modifier.testTag("settings-group-tracking")
        )
        if (gating.anyIntegration) {
            SettingsNavigationRow(
                title = stringResource(R.string.settings_group_integrations),
                supporting = stringResource(R.string.settings_group_integrations_summary),
                icon = Icons.Outlined.Extension,
                onClick = { onNavigate(SettingsRoutes.INTEGRATIONS) },
                modifier = Modifier.testTag("settings-group-integrations")
            )
        }
        // Issue #348: admin-only, reachable only when the session is an admin (the backend also
        // 403s every admin route for non-admins, so this gate is a navigation affordance, not a
        // guard).
        if (state.session.isAdmin && gating.multiUserAdminSupported) {
            SettingsNavigationRow(
                title = stringResource(R.string.settings_group_administration),
                supporting = stringResource(R.string.settings_group_administration_summary),
                icon = Icons.Outlined.AdminPanelSettings,
                onClick = { onNavigate(SettingsRoutes.ADMINISTRATION) },
                modifier = Modifier.testTag("settings-group-administration")
            )
        }
        if (state.buildInfo != null) {
            SettingsNavigationRow(
                title = stringResource(R.string.settings_about),
                supporting = stringResource(R.string.settings_group_about_summary),
                icon = Icons.Outlined.Info,
                onClick = { onNavigate(SettingsRoutes.ABOUT) },
                modifier = Modifier.testTag("settings-group-about")
            )
        }

        HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp))

        val loggingOutLabel = stringResource(R.string.a11y_state_saving)
        SettingsNavigationRow(
            title = stringResource(R.string.settings_log_out),
            icon = Icons.AutoMirrored.Outlined.Logout,
            destructive = true,
            showChevron = false,
            onClick = { if (!state.isLoggingOut) confirmLogout = true },
            modifier = Modifier
                .testTag("settings-log-out")
                .semantics { if (state.isLoggingOut) stateDescription = loggingOutLabel }
        )
        if (state.isLoggingOut) {
            CircularProgressIndicator(modifier = Modifier.padding(start = 40.dp))
        }
    }

    if (confirmLogout) {
        AlertDialog(
            onDismissRequest = { confirmLogout = false },
            title = { Text(stringResource(R.string.settings_log_out_title)) },
            text = { Text(stringResource(R.string.settings_log_out_body)) },
            confirmButton = {
                Button(onClick = {
                    confirmLogout = false
                    onLogout()
                }) {
                    Text(stringResource(R.string.settings_log_out))
                }
            },
            dismissButton = {
                TextButton(onClick = { confirmLogout = false }) {
                    Text(stringResource(R.string.settings_cancel))
                }
            }
        )
    }
}

@Composable
internal fun languageName(tag: String): String = when (tag) {
    "de" -> "Deutsch"
    "it" -> "Italiano"
    "es" -> "Español"
    "fr" -> "Français"
    else -> "English"
}

@Composable
internal fun dateFormatName(format: String): String = when (format) {
    "us" -> stringResource(R.string.settings_date_format_us)
    "iso" -> stringResource(R.string.settings_date_format_iso)
    "ca" -> stringResource(R.string.settings_date_format_ca)
    "eu-hyphen" -> stringResource(R.string.settings_date_format_eu_hyphen)
    "us-mmm" -> stringResource(R.string.settings_date_format_us_mmm)
    "us-mmmm" -> stringResource(R.string.settings_date_format_us_mmmm)
    "eu-mmm" -> stringResource(R.string.settings_date_format_eu_mmm)
    "eu-mmmm" -> stringResource(R.string.settings_date_format_eu_mmmm)
    else -> stringResource(R.string.settings_date_format_eu)
}

/** Issue #722: display label for an [AutoLockDelay] name (never a key string leaks to the UI). */
@Composable
internal fun lockDelayOptionLabel(delayName: String): String = when (
    AutoLockDelay.valueOf(
        delayName
    )
) {
    AutoLockDelay.IMMEDIATELY -> stringResource(R.string.settings_local_auth_delay_immediately)
    AutoLockDelay.ONE_MINUTE -> stringResource(R.string.settings_local_auth_delay_1m)
    AutoLockDelay.FIVE_MINUTES -> stringResource(R.string.settings_local_auth_delay_5m)
    AutoLockDelay.FIFTEEN_MINUTES -> stringResource(R.string.settings_local_auth_delay_15m)
    AutoLockDelay.ONE_HOUR -> stringResource(R.string.settings_local_auth_delay_1h)
}

// The single token list, shared with the renderer so the picker cannot offer a
// format DateFormat does not implement (issue #1752).
internal val DATE_FORMAT_OPTIONS = DateFormat.ALL_FORMATS
