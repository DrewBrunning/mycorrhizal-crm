package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.mycorrhizal.crm.model.network.CONTACT_FIELD_GROUP
import com.mycorrhizal.crm.model.network.CONTACT_FIELD_GROUP_ORDER
import com.mycorrhizal.crm.model.network.ContactFieldGroup
import com.mycorrhizal.crm.model.network.ContactFieldKey
import com.mycorrhizal.crm.model.network.ContactFieldVisibility
import com.mycorrhizal.crm.model.network.ContactFieldVisibilityInput
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.contactFieldGroupLabel
import com.mycorrhizal.crm.ui.components.contactFieldLabel

/**
 * Issue #1997: per-contact "Customize fields" dialog. Each field is a
 * tri-state (inherit / show / hide) and the hide-empty mode has the same
 * tri-state. The PUT is a full replace; the server stores only explicit
 * show/hide rows. Cosmetic only — it never touches the contact's data.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun ContactFieldVisibilityDialog(
    visibility: ContactFieldVisibility?,
    saving: Boolean,
    error: String?,
    onDismiss: () -> Unit,
    onSave: (ContactFieldVisibilityInput) -> Unit,
    onReset: () -> Unit
) {
    val initialDraft = remember(visibility) {
        visibility?.overrides.orEmpty().mapNotNull { (wireKey, state) ->
            ContactFieldKey.fromWireKey(wireKey)?.let { it to state }
        }.toMap()
    }
    var draft by remember(visibility) { mutableStateOf(initialDraft) }
    var hideEmpty by remember(visibility) {
        mutableStateOf(visibility?.hideEmpty ?: "inherit")
    }
    val accountHideEmpty = visibility?.defaults?.hideEmpty ?: false

    Dialog(
        onDismissRequest = { if (!saving) onDismiss() },
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        Surface(
            shape = RoundedCornerShape(16.dp),
            modifier = Modifier.fillMaxWidth(0.95f).fillMaxHeight(0.9f)
        ) {
            Column(modifier = Modifier.fillMaxSize().padding(16.dp)) {
                Text(
                    stringResource(R.string.contact_field_visibility_title),
                    style = MaterialTheme.typography.headlineSmall
                )
                Text(
                    stringResource(R.string.contact_field_visibility_description),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(top = 4.dp)
                )
                if (error != null) {
                    Text(
                        error,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                        modifier = Modifier.padding(top = 8.dp)
                    )
                }
                HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp))

                LazyColumn(modifier = Modifier.weight(1f)) {
                    item {
                        Text(
                            stringResource(R.string.contact_field_visibility_empty_fields),
                            style = MaterialTheme.typography.labelLarge,
                            color = MaterialTheme.colorScheme.primary
                        )
                        StateChips(
                            value = hideEmpty,
                            inheritLabel = stringResource(
                                R.string.contact_field_visibility_inherit,
                                stringResource(
                                    if (accountHideEmpty) {
                                        R.string.contact_field_visibility_default_on
                                    } else {
                                        R.string.contact_field_visibility_default_off
                                    }
                                )
                            ),
                            enabled = !saving,
                            onSelect = { hideEmpty = it }
                        )
                        Text(
                            stringResource(
                                if (accountHideEmpty) {
                                    R.string.contact_field_visibility_hide_empty_on
                                } else {
                                    R.string.contact_field_visibility_hide_empty_off
                                }
                            ),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                    CONTACT_FIELD_GROUP_ORDER.forEach { group ->
                        val keys = CONTACT_FIELD_GROUP.entries
                            .filter { it.value == group }
                            .map { it.key }
                        if (keys.isNotEmpty()) {
                            item(key = group.name) {
                                FieldGroup(
                                    group = group,
                                    keys = keys,
                                    draft = draft,
                                    defaults = visibility?.defaults?.fields.orEmpty(),
                                    enabled = !saving,
                                    onSelect = { key, state ->
                                        draft = draft + (key to state)
                                    }
                                )
                            }
                        }
                    }
                }

                HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp))
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    TextButton(onClick = onReset, enabled = !saving) {
                        Text(stringResource(R.string.contact_field_visibility_reset))
                    }
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        TextButton(onClick = onDismiss, enabled = !saving) {
                            Text(stringResource(R.string.action_cancel))
                        }
                        Button(
                            onClick = {
                                onSave(
                                    ContactFieldVisibilityInput(
                                        overrides = draft.mapKeys { it.key.wireKey },
                                        hideEmpty = hideEmpty
                                    )
                                )
                            },
                            enabled = !saving
                        ) {
                            if (saving) {
                                CircularProgressIndicator(
                                    modifier = Modifier.padding(end = 8.dp),
                                    strokeWidth = 2.dp
                                )
                            }
                            Text(stringResource(R.string.action_save))
                        }
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun FieldGroup(
    group: ContactFieldGroup,
    keys: List<ContactFieldKey>,
    draft: Map<ContactFieldKey, String>,
    defaults: Map<String, Boolean>,
    enabled: Boolean,
    onSelect: (ContactFieldKey, String) -> Unit
) {
    Column(modifier = Modifier.padding(top = 12.dp)) {
        Text(
            contactFieldGroupLabel(group),
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.primary
        )
        keys.forEach { key ->
            Column(modifier = Modifier.padding(top = 6.dp)) {
                Text(contactFieldLabel(key), style = MaterialTheme.typography.bodyMedium)
                val shown = defaults[key.wireKey] == true
                StateChips(
                    value = draft[key] ?: "inherit",
                    inheritLabel = stringResource(
                        R.string.contact_field_visibility_inherit,
                        stringResource(
                            if (shown) {
                                R.string.contact_field_visibility_default_shown
                            } else {
                                R.string.contact_field_visibility_default_hidden
                            }
                        )
                    ),
                    enabled = enabled,
                    onSelect = { onSelect(key, it) }
                )
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun StateChips(
    value: String,
    inheritLabel: String,
    enabled: Boolean,
    onSelect: (String) -> Unit
) {
    FlowRow(
        horizontalArrangement = Arrangement.spacedBy(6.dp),
        modifier = Modifier.fillMaxWidth()
    ) {
        listOf(
            "inherit" to inheritLabel,
            "show" to stringResource(R.string.contact_field_visibility_show),
            "hide" to stringResource(R.string.contact_field_visibility_hide)
        ).forEach { (state, label) ->
            FilterChip(
                selected = value == state,
                onClick = { onSelect(state) },
                label = { Text(label) },
                enabled = enabled
            )
        }
    }
}
