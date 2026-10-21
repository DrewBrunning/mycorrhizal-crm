package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.focusable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.MenuAnchorType
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.disabled
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp

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
    val open = expanded && enabled
    ExposedDropdownMenuBox(
        expanded = open,
        onExpandedChange = { expanded = it && enabled },
        modifier = Modifier.fillMaxWidth(),
    ) {
        // Review of #1773: the anchor is NOT an OutlinedTextField. A read-only text
        // field is still an EditText to accessibility services, and the description
        // set on its modifier landed on a second, non-actionable node, so TalkBack
        // met "Status, Idea" and then a separate "Idea, edit box" (seen in a
        // uiautomator dump on the emulator). Same build as ContactListScreen's circle
        // filter (#209): one merged Box node carrying the menu-anchor click, the
        // drop-down role and the "<label>, <value>" description, with the outlined
        // chrome drawn by OutlinedTextFieldDefaults.DecorationBox and the value by a
        // plain Text, which also ellipsises a long value instead of clipping it.
        Box(
            modifier = modifier
                .fillMaxWidth()
                // OutlinedTextField reserves this room for its floating label; the
                // decoration box does not, so a dropdown first in a scrolling dialog
                // had its label clipped (seen on the emulator in New preference).
                .padding(top = 8.dp)
                // Typed anchor (the no-argument overload is deprecated): read-only, so
                // tap, Enter/Space and D-pad centre open the menu, and a disabled
                // field is not expandable from the keyboard either.
                .menuAnchor(MenuAnchorType.PrimaryNotEditable, enabled)
                // The text field used to supply focus; a Box needs it explicitly
                // for keyboard / D-pad users to reach the anchor at all.
                .focusable(enabled)
                .semantics(mergeDescendants = true) {
                    role = Role.DropdownList
                    contentDescription = "$label, $valueText"
                    if (!enabled) disabled()
                },
            // The decoration box sizes to its content; stretch it to the field width
            // (seen on the emulator: the outline hugged "Idea" instead of the row).
            propagateMinConstraints = true,
        ) {
            OutlinedTextFieldDefaults.DecorationBox(
                value = valueText,
                innerTextField = {
                    Text(
                        text = valueText,
                        style = MaterialTheme.typography.bodyLarge,
                        color = if (enabled) {
                            MaterialTheme.colorScheme.onSurface
                        } else {
                            MaterialTheme.colorScheme.onSurface.copy(alpha = DISABLED_ALPHA)
                        },
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                },
                enabled = enabled,
                singleLine = true,
                visualTransformation = VisualTransformation.None,
                interactionSource = remember { MutableInteractionSource() },
                label = { Text(label) },
                trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = open) },
            )
        }
        ExposedDropdownMenu(expanded = open, onDismissRequest = { expanded = false }) {
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

/** Material's disabled-content opacity. */
private const val DISABLED_ALPHA = 0.38f
