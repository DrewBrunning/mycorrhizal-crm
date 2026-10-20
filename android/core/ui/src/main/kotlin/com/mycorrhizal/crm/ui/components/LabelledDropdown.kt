package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.MenuAnchorType
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp

/**
 * One choice of a [LabelledDropdown]. [sectionHeader], when set, draws a small
 * heading above this entry (the preference-category menu groups by section).
 */
data class DropdownOption<T>(
    val value: T,
    val label: String,
    val sectionHeader: String? = null,
)

/**
 * Issue #1773: the one select control for forms. A read-only outlined field with a
 * visible [label] and a trailing arrow, opening an `ExposedDropdownMenu` — the
 * relationship dialog's shape. It replaces the bare full-width `OutlinedButton` +
 * `DropdownMenu` pickers that showed only the current value ("Idea", "Normal")
 * with no field name and no arrow.
 *
 * TalkBack hears "<label>, <value>, drop-down list": the anchor's content
 * description carries both, and its role is [Role.DropdownList].
 *
 * [valueText] is what the closed field shows (the selected option's label, or a
 * placeholder such as "None"). A disabled field shows the same text greyed out.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun <T> LabelledDropdown(
    label: String,
    valueText: String,
    options: List<DropdownOption<T>>,
    onSelect: (T) -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
) {
    var expanded by remember { mutableStateOf(false) }
    ExposedDropdownMenuBox(
        expanded = expanded && enabled,
        onExpandedChange = { expanded = it && enabled },
        modifier = Modifier.fillMaxWidth(),
    ) {
        OutlinedTextField(
            value = valueText,
            onValueChange = {},
            readOnly = true,
            enabled = enabled,
            singleLine = true,
            label = { Text(label) },
            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = expanded && enabled) },
            modifier = modifier
                .fillMaxWidth()
                // Typed anchor (the no-argument overload is deprecated): read-only, so
                // tap, Enter/Space and D-pad centre open the menu, and a disabled
                // field is not expandable from the keyboard either.
                .menuAnchor(MenuAnchorType.PrimaryNotEditable, enabled)
                .semantics {
                    role = Role.DropdownList
                    contentDescription = "$label, $valueText"
                },
        )
        ExposedDropdownMenu(expanded = expanded && enabled, onDismissRequest = { expanded = false }) {
            options.forEach { option ->
                option.sectionHeader?.let { header ->
                    Text(
                        text = header,
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(horizontal = 12.dp, vertical = 6.dp),
                    )
                }
                DropdownMenuItem(
                    text = { Text(option.label, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                    onClick = {
                        expanded = false
                        onSelect(option.value)
                    },
                )
            }
        }
    }
}
