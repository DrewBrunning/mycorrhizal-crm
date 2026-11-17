package com.mycorrhizal.crm.feature.shares

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
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

/** Issue #2609: the touch-target floor of every option row (WCAG 2.5.8 / Material 48dp). */
internal val MinOptionRowHeight = 48.dp

/** Issue #2609: the gap between a radio/checkbox glyph and its label. */
internal val OptionControlGap = 12.dp

/** How many section names a row lists before it collapses the rest into "and N more". */
private const val SECTIONS_SHOWN = 3

/**
 * Issue #2643: "<prefix>: Emails, Phones, Addresses and 2 more" for a grant's sections, or null
 * when it has none (a note, activity or circle grant, or a contact grant with no sections).
 */
@Composable
internal fun sectionsSummary(sections: List<String>, @StringRes prefixRes: Int): String? {
    if (sections.isEmpty()) return null
    val shown = sections.take(SECTIONS_SHOWN)
        .map { stringResource(sectionLabelRes(it)) }
        .joinToString(", ")
    val rest = sections.size - SECTIONS_SHOWN
    val list = if (rest > 0) {
        pluralStringResource(R.plurals.shares_sections_more, rest, shown, rest)
    } else {
        shown
    }
    return stringResource(prefixRes, list)
}

/**
 * Issue #2627: the owner-visible status as a chip, so Active / Pending / Declined / Revoked /
 * Expired read at a glance instead of as one more grey caption. The label is the text, never
 * colour alone.
 */
@Composable
internal fun GrantStatusChip(status: String, modifier: Modifier = Modifier) {
    val colors = MaterialTheme.colorScheme
    val (container, content) = when (status) {
        ShareGrantStatuses.ACTIVE -> colors.tertiaryContainer to colors.onTertiaryContainer

        ShareGrantStatuses.DECLINED,
        ShareGrantStatuses.REVOKED -> colors.errorContainer to colors.onErrorContainer

        ShareGrantStatuses.EXPIRED -> colors.surfaceVariant to colors.onSurfaceVariant

        else -> colors.secondaryContainer to colors.onSecondaryContainer
    }
    Surface(
        color = container,
        contentColor = content,
        shape = MaterialTheme.shapes.small,
        modifier = modifier.semantics(mergeDescendants = true) {}
    ) {
        Text(
            text = stringResource(grantStatusLabelRes(status)),
            style = MaterialTheme.typography.labelMedium,
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp)
        )
    }
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

/** Renders a JSContact partial date in the user's date preference (read view and preview). */
@Composable
internal fun rememberGrantDateFormatter(): (Int?, Int?, Int?) -> String {
    val dateFormat = LocalDateFormat.current
    return remember(dateFormat) {
        { year, month, day ->
            when {
                year != null && month != null && day != null ->
                    DateFormat.formatFull(year, month, day, dateFormat)

                month != null && day != null -> DateFormat.formatYearless(month, day, dateFormat)

                else -> listOfNotNull(year, month, day).joinToString("-")
            }
        }
    }
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
