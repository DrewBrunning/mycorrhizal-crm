package com.mycorrhizal.crm.feature.imports

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.ExpandMore
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.SourceImportIssue
import com.mycorrhizal.crm.ui.R

/**
 * Issue #2524 (web: `SourceImportLossReport.tsx`): the order categories are shown in, most
 * consequential first. Mirrors web's `CATEGORY_ORDER`; keep the two in sync.
 */
private val LOSS_CATEGORY_ORDER = listOf(
    "unsupported",
    "invalid",
    "lossy",
    "transformed",
    "skipped"
)

internal data class LossGroup(val category: String, val issues: List<SourceImportIssue>)

/**
 * Groups [issues] by category in [LOSS_CATEGORY_ORDER], keeping input order inside a group and
 * omitting empty groups. Unlike web, a category this client does not know is appended after the
 * known ones instead of being dropped, so a newer server can never hide a loss.
 */
internal fun groupLossReport(issues: List<SourceImportIssue>): List<LossGroup> {
    val byCategory = issues.groupBy { it.category }
    val known = LOSS_CATEGORY_ORDER.filter { it in byCategory }
    val unknown = byCategory.keys.filterNot { it in LOSS_CATEGORY_ORDER }
    return (known + unknown).map { LossGroup(it, byCategory.getValue(it)) }
}

@Composable
private fun lossCategoryLabel(category: String): String = stringResource(
    when (category) {
        "unsupported" -> R.string.import_loss_category_unsupported
        "invalid" -> R.string.import_loss_category_invalid
        "lossy" -> R.string.import_loss_category_lossy
        "transformed" -> R.string.import_loss_category_transformed
        "skipped" -> R.string.import_loss_category_skipped
        else -> R.string.import_loss_category_other
    }
)

/**
 * The pre-confirm loss report: [headerText] (the screen's count line, tagged `<tagPrefix>-loss`)
 * followed by one collapsible section per category listing record, field and message. The list is
 * height-capped and scrolls so an expanded report cannot push the review rows off screen.
 */
@Composable
internal fun SourceImportLossReport(
    issues: List<SourceImportIssue>,
    headerText: String,
    tagPrefix: String,
    modifier: Modifier = Modifier
) {
    Column(modifier = modifier.padding(horizontal = 16.dp, vertical = 8.dp)) {
        Text(
            text = headerText,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.error,
            modifier = Modifier.testTag("$tagPrefix-loss")
        )
        Column(
            modifier = Modifier.heightIn(max = 280.dp).verticalScroll(rememberScrollState())
        ) {
            groupLossReport(issues).forEach { group ->
                LossGroupSection(group = group, tagPrefix = tagPrefix)
            }
        }
    }
}

@Composable
private fun LossGroupSection(group: LossGroup, tagPrefix: String) {
    var expanded by rememberSaveable(group.category) { mutableStateOf(false) }
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        modifier = Modifier
            .fillMaxWidth()
            .clickable(role = Role.Button) { expanded = !expanded }
            .padding(vertical = 12.dp)
            .testTag("$tagPrefix-loss-group-${group.category}")
    ) {
        Text(
            text = lossCategoryLabel(group.category),
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.weight(1f)
        )
        Text(text = group.issues.size.toString(), style = MaterialTheme.typography.labelMedium)
        Icon(
            imageVector = if (expanded) Icons.Filled.ExpandLess else Icons.Filled.ExpandMore,
            contentDescription = null
        )
    }
    if (expanded) {
        group.issues.forEachIndexed { index, issue ->
            Column(
                modifier = Modifier
                    .padding(start = 8.dp, bottom = 8.dp)
                    .testTag("$tagPrefix-loss-item-${group.category}-$index")
            ) {
                Text(text = issue.message, style = MaterialTheme.typography.bodySmall)
                Text(
                    text = if (issue.field.isEmpty()) issue.record else "${issue.record} · ${issue.field}",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
        }
    }
}
