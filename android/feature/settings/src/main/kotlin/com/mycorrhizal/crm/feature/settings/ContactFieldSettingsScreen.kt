package com.mycorrhizal.crm.feature.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
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
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.CONTACT_FIELD_GROUP
import com.mycorrhizal.crm.model.network.CONTACT_FIELD_GROUP_ORDER
import com.mycorrhizal.crm.model.network.ContactFieldKey
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.contactFieldGroupLabel
import com.mycorrhizal.crm.ui.components.contactFieldLabel

/**
 * Issue #832 (web parity): per-field show/edit toggles for the contact
 * detail/form screens, grouped exactly like web's `ContactFieldSettings.tsx`.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContactFieldSettingsScreen(
    onBack: () -> Unit,
    viewModel: ContactFieldSettingsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(stringResource(R.string.settings_contact_fields_title), style = MaterialTheme.typography.titleLarge)
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
        ContactFieldSettingsContent(
            state = state,
            onToggle = viewModel::onToggle,
            modifier = Modifier.padding(padding),
        )
    }
}

/** Stateless body for [ContactFieldSettingsScreen] — testable directly. */
@Composable
fun ContactFieldSettingsContent(
    state: ContactFieldSettingsUiState,
    onToggle: (ContactFieldKey) -> Unit = {},
    modifier: Modifier = Modifier,
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        if (state.isLoading) {
            Row(horizontalArrangement = Arrangement.Center, modifier = Modifier.fillMaxWidth()) {
                CircularProgressIndicator()
            }
            return@Column
        }

        Text(
            text = stringResource(R.string.settings_contact_fields_description),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        state.loadError?.let {
            Text(
                it,
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive },
            )
        }
        state.saveError?.let {
            Text(
                it,
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive },
            )
        }

        CONTACT_FIELD_GROUP_ORDER.forEach { group ->
            val keysInGroup = CONTACT_FIELD_GROUP.entries.filter { it.value == group }.map { it.key }
            if (keysInGroup.isEmpty()) return@forEach
            Text(
                text = contactFieldGroupLabel(group),
                style = MaterialTheme.typography.labelLarge,
                color = MaterialTheme.colorScheme.primary,
                modifier = Modifier.padding(top = 8.dp),
            )
            keysInGroup.forEach { key ->
                FieldToggleRow(
                    label = contactFieldLabel(key),
                    checked = key in state.enabled,
                    enabled = state.savingKey == null,
                    onCheckedChange = { onToggle(key) },
                )
            }
        }
    }
}

/**
 * Same labeled-switch a11y pattern as [SettingsScreen]'s private `ToggleRow`
 * (`Modifier.toggleable` merges the label into the switch's accessible name)
 * — kept local rather than exported from there to avoid widening that
 * screen's public surface for a single reuse.
 */
@Composable
private fun FieldToggleRow(
    label: String,
    checked: Boolean,
    enabled: Boolean,
    onCheckedChange: (Boolean) -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .toggleable(value = checked, enabled = enabled, onValueChange = onCheckedChange, role = Role.Switch)
            .padding(vertical = 4.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(label, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.weight(1f))
        Switch(checked = checked, onCheckedChange = null, enabled = enabled)
    }
}
