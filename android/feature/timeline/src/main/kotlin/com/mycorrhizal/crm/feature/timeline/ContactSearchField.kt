package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.ui.R

/**
 * Debounced contact-search field shared by the note and activity forms'
 * pickers: a query box (with a loading spinner) and a result list that calls
 * [onPick] when a row is tapped. Mirrors the RelationshipsScreen picker.
 */
@Composable
fun ContactSearchField(
    query: String,
    results: List<ContactSummary>,
    loading: Boolean,
    onQueryChange: (String) -> Unit,
    onPick: (ContactSummary) -> Unit,
    labelRes: Int,
    modifier: Modifier = Modifier,
) {
    Column(modifier = modifier) {
        OutlinedTextField(
            value = query,
            onValueChange = onQueryChange,
            label = { Text(stringResource(labelRes)) },
            singleLine = true,
            trailingIcon = {
                if (loading) {
                    CircularProgressIndicator(modifier = Modifier.padding(4.dp), strokeWidth = 2.dp)
                }
            },
            modifier = Modifier.fillMaxWidth(),
        )
        when {
            query.isBlank() -> Text(
                text = stringResource(R.string.timeline_type_to_search),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            results.isEmpty() && !loading -> Text(
                text = stringResource(R.string.timeline_no_contacts_found),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            else -> LazyColumn(modifier = Modifier.heightIn(max = 200.dp)) {
                items(results, key = { it.id }) { contact ->
                    Text(
                        text = contact.displayName,
                        style = MaterialTheme.typography.bodyMedium,
                        modifier = Modifier
                            .fillMaxWidth()
                            .clickable { onPick(contact) }
                            .padding(vertical = 8.dp),
                    )
                }
            }
        }
    }
}
