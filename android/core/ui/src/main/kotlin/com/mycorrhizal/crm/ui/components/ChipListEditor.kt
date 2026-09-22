package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.InputChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.ui.R

/**
 * Issue #832: a bare `List<String>` editor (e.g. `Card.keywords`) — a chip
 * per entry with a delete affordance, plus a text field + add button/Enter
 * to append. Direct port of web's `KeywordsEditor.tsx`. Deliberately NOT
 * built on [MultiValueEditor]: that composable hard-renders a type dropdown
 * per row, which has no meaning for a bare string list.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun ChipListEditor(
    items: List<String>,
    onChange: (List<String>) -> Unit,
    label: String,
    modifier: Modifier = Modifier,
) {
    var draft by remember { mutableStateOf("") }

    fun commitDraft() {
        val trimmed = draft.trim()
        if (trimmed.isNotEmpty() && trimmed !in items) {
            onChange(items + trimmed)
        }
        draft = ""
    }

    Column(modifier = modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(
            text = label,
            style = MaterialTheme.typography.labelLarge.copy(fontFamily = com.mycorrhizal.crm.ui.theme.MycorrhizalFonts.mono),
            color = MaterialTheme.colorScheme.primary,
            modifier = Modifier.padding(top = 8.dp),
        )
        if (items.isNotEmpty()) {
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
                items.forEachIndexed { index, item ->
                    InputChip(
                        selected = false,
                        onClick = { onChange(items.filterIndexed { i, _ -> i != index }) },
                        label = { Text(item) },
                        trailingIcon = {
                            Icon(
                                Icons.Outlined.Close,
                                contentDescription = stringResource(R.string.contact_remove),
                            )
                        },
                    )
                }
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
            OutlinedTextField(
                value = draft,
                onValueChange = { draft = it },
                label = { Text(stringResource(R.string.contact_add)) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                keyboardActions = KeyboardActions(onDone = { commitDraft() }),
                modifier = Modifier.weight(1f),
            )
            IconButton(onClick = { commitDraft() }) {
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.contact_add))
            }
        }
    }
}
