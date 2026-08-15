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
 *
 * [hiddenMatchCount] (default 0) renders an extra caption when the caller
 * narrowed the server's deliberately-broad results client-side (the merge
 * picker's T101/T112 strict-name filter) — a count of rows the server matched
 * on other fields, so silently-hidden results are disclosed rather than
 * dropped without a trace. Note/activity pickers don't set it.
 */
@Composable
fun ContactSearchField(
    query: String,
    results: List<ContactSummary>,
    loading: Boolean,
    onQueryChange: (String) -> Unit,
    onPick: (ContactSummary) -> Unit,
    labelRes: Int,
    hiddenMatchCount: Int = 0,
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
            // "No contacts found" only when the server genuinely returned nothing.
            // If the caller's strict filter dropped every result, the caption
            // below explains it instead ("0 shown, N matched on other fields") —
            // the web merge picker's T101 behaviour.
            !loading && results.isEmpty() && hiddenMatchCount == 0 -> Text(
                text = stringResource(R.string.timeline_no_contacts_found),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            else -> {
                if (results.isNotEmpty()) {
                    LazyColumn(modifier = Modifier.heightIn(max = 200.dp)) {
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
                // T112: disclose rows the strict-name filter dropped (the server
                // may have matched them on email/phone/address). Only set by the
                // merge picker; note/activity pickers leave the default 0.
                if (hiddenMatchCount > 0) {
                    Text(
                        text = stringResource(
                            R.string.merge_hidden_matches,
                            results.size,
                            hiddenMatchCount,
                        ),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}
