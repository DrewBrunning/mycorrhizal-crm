package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.CardNote
import com.mycorrhizal.crm.ui.R

/**
 * Issue #832: `Card.notes[]` — a multiline text row per note, plus a
 * read-only `author`/`created` caption when import populated them (never
 * editable — there is no UI to set those on either platform). Direct port
 * of web's `CardNotesEditor.tsx`. Deliberately NOT built on
 * [MultiValueEditor]: that composable's value field is hardcoded single-line
 * and always renders a type/pref row, neither of which applies here.
 */
@Composable
fun CardNotesEditor(
    items: List<CardNote>,
    onChange: (List<CardNote>) -> Unit,
    label: String,
    modifier: Modifier = Modifier,
) {
    Column(modifier = modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(
            text = label,
            style = MaterialTheme.typography.labelLarge.copy(fontFamily = com.mycorrhizal.crm.ui.theme.MycorrhizalFonts.mono),
            color = MaterialTheme.colorScheme.primary,
            modifier = Modifier.padding(top = 8.dp),
        )
        items.forEachIndexed { index, note ->
            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Row(
                    horizontalArrangement = Arrangement.spacedBy(4.dp),
                    verticalAlignment = Alignment.Top,
                ) {
                    OutlinedTextField(
                        value = note.note.orEmpty(),
                        onValueChange = { value ->
                            onChange(items.mapIndexed { i, it -> if (i == index) it.copy(note = value) else it })
                        },
                        label = { Text(stringResource(R.string.contact_value_n, index + 1)) },
                        minLines = 2,
                        modifier = Modifier.weight(1f),
                    )
                    IconButton(
                        onClick = { onChange(items.filterIndexed { i, _ -> i != index }) },
                        enabled = items.size > 1,
                    ) {
                        Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.contact_remove))
                    }
                }
                val authorCaption = noteAuthorCaption(note)
                if (authorCaption != null) {
                    Text(
                        text = authorCaption,
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
        IconButton(onClick = { onChange(items + CardNote(note = "")) }) {
            Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.contact_add))
        }
    }
}

/** The read-only "— Name, 2026-01-01" caption for an import-populated note, or null. */
private fun noteAuthorCaption(note: CardNote): String? {
    val name = note.author?.name?.takeIf { it.isNotBlank() } ?: note.author?.uri?.takeIf { it.isNotBlank() }
    val created = note.created?.utc?.takeIf { it.isNotBlank() }
    return when {
        name != null && created != null -> "— $name, $created"
        name != null -> "— $name"
        created != null -> "— $created"
        else -> null
    }
}
