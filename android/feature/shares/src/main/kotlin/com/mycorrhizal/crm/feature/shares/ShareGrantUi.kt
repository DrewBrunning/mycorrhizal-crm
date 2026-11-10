package com.mycorrhizal.crm.feature.shares

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.ShareGrantEntityTypes
import com.mycorrhizal.crm.model.network.ShareGrantStatuses
import com.mycorrhizal.crm.model.util.DateFormat
import com.mycorrhizal.crm.ui.LocalDateFormat
import com.mycorrhizal.crm.ui.R

/** The human name of a grant's entity type. An unknown type (a newer server) reads as a generic share. */
@StringRes
internal fun entityTypeLabelRes(entityType: String): Int = when (entityType) {
    ShareGrantEntityTypes.CONTACT -> R.string.shares_entity_contact
    ShareGrantEntityTypes.NOTE -> R.string.shares_entity_note
    ShareGrantEntityTypes.ACTIVITY -> R.string.shares_entity_activity
    ShareGrantEntityTypes.CIRCLE -> R.string.shares_entity_circle
    else -> R.string.shares_title
}

/** The label of an owner-visible grant status. */
@StringRes
internal fun grantStatusLabelRes(status: String): Int = when (status) {
    ShareGrantStatuses.ACTIVE -> R.string.shares_status_active
    ShareGrantStatuses.DECLINED -> R.string.shares_status_declined
    ShareGrantStatuses.REVOKED -> R.string.shares_status_revoked
    ShareGrantStatuses.EXPIRED -> R.string.shares_status_expired
    else -> R.string.shares_status_pending
}

/** The expiry presets of the standing-share editor, in days. */
internal val EXPIRY_PRESET_DAYS = listOf(7, 30, 90, 365)

/** A caption-sized line used by the grant rows. */
@Composable
internal fun GrantCaption(text: String, modifier: Modifier = Modifier) {
    Text(
        text = text,
        style = MaterialTheme.typography.labelMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = modifier
    )
}

/** "Expires <date>" for a grant with an expiry, or null. */
@Composable
internal fun expiryText(expiresAt: String?): String? {
    val dateFormat = LocalDateFormat.current
    val date = DateFormat.formatTimestamp(expiresAt, dateFormat)
    return if (date.isBlank()) null else stringResource(R.string.shares_grant_expires, date)
}

/** A titled block of lines; used by the read view. */
@Composable
internal fun LabelledLines(title: String, lines: List<String>, modifier: Modifier = Modifier) {
    Column(modifier = modifier.padding(vertical = 8.dp)) {
        Text(text = title, style = MaterialTheme.typography.titleSmall)
        lines.forEach { line ->
            Text(
                text = line,
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.padding(top = 2.dp)
            )
        }
    }
}
