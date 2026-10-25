package com.mycorrhizal.crm.feature.settings

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.ui.R

/**
 * Settings > Contacts & fields (issue #1759): everything that shapes what a contact holds. Custom
 * link actions was a full-width button on the old page; it is a chevron row like its siblings.
 */
@Composable
fun ContactsFieldsSettingsScreen(
    onBack: () -> Unit,
    onCustomFields: () -> Unit = {},
    onContactFieldSettings: () -> Unit = {},
    onCustomLinks: () -> Unit = {},
    onCircleTagTriage: () -> Unit = {}
) {
    SettingsScaffold(
        title = stringResource(R.string.settings_group_contacts_fields),
        onBack = onBack
    ) { padding ->
        ContactsFieldsSettingsContent(
            onCustomFields = onCustomFields,
            onContactFieldSettings = onContactFieldSettings,
            onCustomLinks = onCustomLinks,
            onCircleTagTriage = onCircleTagTriage,
            modifier = padding
        )
    }
}

@Composable
fun ContactsFieldsSettingsContent(
    onCustomFields: () -> Unit = {},
    onContactFieldSettings: () -> Unit = {},
    onCustomLinks: () -> Unit = {},
    onCircleTagTriage: () -> Unit = {},
    modifier: Modifier = Modifier
) {
    SettingsColumn(modifier = modifier) {
        // Issue #830: create/edit/delete custom field definitions.
        SettingsNavigationRow(
            stringResource(R.string.settings_custom_fields_title),
            onClick = onCustomFields
        )
        // Issue #832: per-field show/edit toggles for the contact detail/form screens.
        SettingsNavigationRow(
            stringResource(R.string.settings_contact_fields_title),
            onClick = onContactFieldSettings
        )
        SettingsNavigationRow(stringResource(R.string.custom_links_title), onClick = onCustomLinks)
        // M26: one-time legacy circle/tag cleanup.
        SettingsNavigationRow(
            stringResource(R.string.settings_circle_tag_triage),
            onClick = onCircleTagTriage
        )
    }
}

/**
 * Settings > Integrations (issue #1759): calendar sync, webhooks and the outbound file/photo
 * integrations. A row is hidden when the deployment does not offer it.
 */
@Composable
fun IntegrationsSettingsScreen(
    onBack: () -> Unit,
    onCalendarSync: () -> Unit = {},
    onWebhooks: () -> Unit = {},
    onImmichSettings: () -> Unit = {},
    onPaperlessSettings: () -> Unit = {},
    onSeafileSettings: () -> Unit = {},
    onNextcloudSettings: () -> Unit = {},
    onGeoPulseSettings: () -> Unit = {}
) {
    SettingsScaffold(
        title = stringResource(R.string.settings_group_integrations),
        onBack = onBack
    ) { padding ->
        IntegrationsSettingsContent(
            onCalendarSync = onCalendarSync,
            onWebhooks = onWebhooks,
            onImmichSettings = onImmichSettings,
            onPaperlessSettings = onPaperlessSettings,
            onSeafileSettings = onSeafileSettings,
            onNextcloudSettings = onNextcloudSettings,
            onGeoPulseSettings = onGeoPulseSettings,
            modifier = padding
        )
    }
}

@Composable
fun IntegrationsSettingsContent(
    onCalendarSync: () -> Unit = {},
    onWebhooks: () -> Unit = {},
    onImmichSettings: () -> Unit = {},
    onPaperlessSettings: () -> Unit = {},
    onSeafileSettings: () -> Unit = {},
    onNextcloudSettings: () -> Unit = {},
    onGeoPulseSettings: () -> Unit = {},
    modifier: Modifier = Modifier
) {
    val gating = rememberSettingsGating()
    SettingsColumn(modifier = modifier) {
        // Issue #1263: each is hidden when the deployment does not declare its capability (an
        // embedded local server omits webhooks, DAV serving and push).
        // Issue #390's Android follow-up (#628): calendar/contact sync management.
        if (gating.calendarSyncSupported) {
            SettingsNavigationRow(
                stringResource(R.string.settings_calendar_sync_title),
                onClick = onCalendarSync
            )
        }
        if (gating.webhooksSupported) {
            SettingsNavigationRow(
                stringResource(R.string.settings_webhooks_title),
                onClick = onWebhooks
            )
        }
        if (gating.integrationsSupported) {
            // Issue #236: the Immich connection-config settings screen.
            SettingsNavigationRow(
                stringResource(R.string.settings_immich_title),
                onClick = onImmichSettings
            )
            // Issue #833: the Paperless/Seafile/Nextcloud connection-config settings screens.
            SettingsNavigationRow(
                stringResource(R.string.settings_paperless_title),
                onClick = onPaperlessSettings
            )
            SettingsNavigationRow(
                stringResource(R.string.settings_seafile_title),
                onClick = onSeafileSettings
            )
            SettingsNavigationRow(
                stringResource(R.string.settings_nextcloud_title),
                onClick = onNextcloudSettings
            )
            // Issue #160: GeoPulse location history (an outbound integration, so hidden on
            // embedded profiles).
            SettingsNavigationRow(
                stringResource(R.string.settings_geopulse_title),
                onClick = onGeoPulseSettings
            )
        }
    }
}

/**
 * Settings > Administration (issue #1759, admin accounts only): user management and the
 * operational-event timeline.
 */
@Composable
fun AdministrationSettingsScreen(
    onBack: () -> Unit,
    onManageUsers: () -> Unit = {},
    onSystemEvents: () -> Unit = {}
) {
    SettingsScaffold(
        title = stringResource(R.string.settings_group_administration),
        onBack = onBack
    ) { padding ->
        AdministrationSettingsContent(
            onManageUsers = onManageUsers,
            onSystemEvents = onSystemEvents,
            modifier = padding
        )
    }
}

@Composable
fun AdministrationSettingsContent(
    onManageUsers: () -> Unit = {},
    onSystemEvents: () -> Unit = {},
    modifier: Modifier = Modifier
) {
    val gating = rememberSettingsGating()
    SettingsColumn(modifier = modifier) {
        // Issue #348: the backend also 403s every admin route for non-admins, so the root's gate
        // is a navigation affordance, not a guard.
        if (gating.multiUserAdminSupported) {
            SettingsNavigationRow(stringResource(R.string.users_title), onClick = onManageUsers)
            // Issue #692: the system-events surface needs the v0.6.2 admin endpoints; an older
            // server has none of them.
            if (gating.systemEventsSupported) {
                SettingsNavigationRow(
                    stringResource(R.string.sysevents_title),
                    onClick = onSystemEvents
                )
            }
        }
    }
}

/** Settings > About (issue #1759). */
@Composable
fun AboutSettingsScreen(
    onBack: () -> Unit,
    onLicenses: () -> Unit = {},
    viewModel: SettingsViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    SettingsScaffold(title = stringResource(R.string.settings_about), onBack = onBack) { padding ->
        AboutSettingsContent(state = state, onLicenses = onLicenses, modifier = padding)
    }
}

/**
 * Issue #1420: which build is running — version name, version code (the only thing that tells
 * release candidates apart), build type/flavor (a debug or `.localtest` build is visibly not the
 * release), optional commit/build date, and the active server's version. The server lookup is
 * informational: a failure (or the wait) shows a dash and the rest of Settings is unaffected.
 */
@Composable
fun AboutSettingsContent(
    state: SettingsUiState,
    modifier: Modifier = Modifier,
    onLicenses: () -> Unit = {}
) {
    val context = LocalContext.current
    val dash = stringResource(R.string.settings_value_placeholder)
    SettingsColumn(modifier = modifier) {
        val info = state.buildInfo
        if (info != null) {
            SettingsInfoRow(stringResource(R.string.settings_about_version), info.versionName)
            SettingsInfoRow(
                stringResource(R.string.settings_about_version_code),
                info.versionCode.toString()
            )
            SettingsInfoRow(
                stringResource(R.string.settings_about_build_type),
                AboutDetails.buildLabel(info)
            )
            info.commit?.let { SettingsInfoRow(stringResource(R.string.settings_about_commit), it) }
            info.buildDate?.let {
                SettingsInfoRow(
                    stringResource(R.string.settings_about_built),
                    it
                )
            }
            val serverVersion = state.serverHealth?.version?.takeIf { it.isNotBlank() }
            SettingsInfoRow(
                stringResource(R.string.settings_about_server_version),
                serverVersion?.let {
                    AboutDetails.versionWithCommit(
                        it,
                        state.serverHealth.commit
                    )
                } ?: dash
            )
            state.serverHealth?.buildDate?.takeIf { it.isNotBlank() }?.let {
                SettingsInfoRow(stringResource(R.string.settings_about_server_built), it)
            }
            val label = stringResource(R.string.settings_about_copy)
            OutlinedButton(
                onClick = {
                    val clipboard = context.getSystemService(
                        android.content.Context.CLIPBOARD_SERVICE
                    )
                        as android.content.ClipboardManager
                    clipboard.setPrimaryClip(
                        android.content.ClipData.newPlainText(
                            label,
                            AboutDetails.format(info, state.serverHealth)
                        )
                    )
                },
                modifier = Modifier.fillMaxWidth()
            ) {
                Text(label)
            }
            // Issue #1880: attribution required by the dependencies the APK/AAB
            // distributes, generated at build time by AboutLibraries.
            SettingsNavigationRow(
                title = stringResource(R.string.settings_about_licenses),
                onClick = onLicenses
            )
        }
    }
}
