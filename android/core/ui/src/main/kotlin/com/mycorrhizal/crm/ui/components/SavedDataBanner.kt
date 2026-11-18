package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.ui.R

/**
 * Issue #2629: the one banner a cached screen shows when its refresh failed and the rows on
 * screen come from the offline mirror ("Showing saved data" + Retry). It is announced politely
 * when it appears. A screen shows it from a flag set where the cache was served (the list
 * ViewModel's `showingSavedData`, the contact record's `servedFromCache`), never from a guess.
 */
@Composable
fun SavedDataBanner(onRetry: () -> Unit, modifier: Modifier = Modifier) {
    Surface(
        color = MaterialTheme.colorScheme.secondaryContainer,
        contentColor = MaterialTheme.colorScheme.onSecondaryContainer,
        modifier = modifier
            .fillMaxWidth()
            .testTag("saved-data-banner")
            .semantics { liveRegion = LiveRegionMode.Polite }
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween,
            modifier = Modifier.padding(start = 16.dp, end = 8.dp)
        ) {
            Text(
                text = stringResource(R.string.saved_data_banner_message),
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.weight(1f)
            )
            TextButton(onClick = onRetry) {
                Text(stringResource(R.string.action_retry))
            }
        }
    }
}
