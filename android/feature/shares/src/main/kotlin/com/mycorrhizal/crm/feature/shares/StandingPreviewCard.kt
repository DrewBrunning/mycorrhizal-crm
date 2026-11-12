package com.mycorrhizal.crm.feature.shares

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.localizedMessage

/**
 * Issue #2519: "What they will see" for a standing contact share (web's `StandingShareForm`
 * preview). It renders the owner's own export of the [selectedSections] through the same
 * allowlist projection the recipient's read view uses, so the two cannot drift. Read-only; there
 * is no include-sensitive control, because a standing share never exposes private or secret items.
 */
@Composable
internal fun StandingPreviewCard(
    preview: StandingPreview,
    selectedSections: Set<String>,
    modifier: Modifier = Modifier
) {
    OutlinedCard(
        modifier = modifier.fillMaxWidth(),
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant)
    ) {
        Column(modifier = Modifier.padding(12.dp)) {
            Text(
                text = stringResource(R.string.shares_standing_preview_title),
                style = MaterialTheme.typography.titleSmall
            )
            GrantCaption(stringResource(R.string.shares_standing_preview_hint))
            PreviewBody(preview, selectedSections, Modifier.padding(top = 8.dp))
        }
    }
}

@Composable
private fun PreviewBody(
    preview: StandingPreview,
    selectedSections: Set<String>,
    modifier: Modifier = Modifier
) {
    when (preview) {
        // Nothing selected (or no contact to read): there is nothing the recipient could see.
        StandingPreview.Idle -> EmptyLine(modifier)

        StandingPreview.Loading -> Text(
            text = stringResource(R.string.shares_standing_preview_loading),
            style = MaterialTheme.typography.bodyMedium,
            modifier = modifier
        )

        is StandingPreview.Error -> Column(modifier = modifier) {
            Text(
                text = stringResource(R.string.shares_standing_preview_error),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.error
            )
            GrantCaption(preview.error.localizedMessage())
        }

        is StandingPreview.Ready -> {
            val formatter = rememberGrantDateFormatter()
            val view = remember(preview.card, selectedSections, formatter) {
                projectSharedContact(preview.card, selectedSections, formatter)
            }
            Column(modifier = modifier) {
                if (view.displayName.isNotBlank()) {
                    Text(text = view.displayName, style = MaterialTheme.typography.titleMedium)
                }
                view.sections.forEach { section ->
                    LabelledLines(stringResource(sectionLabelRes(section.token)), section.lines)
                }
                if (view.sections.isEmpty()) EmptyLine()
            }
        }
    }
}

@Composable
private fun EmptyLine(modifier: Modifier = Modifier) {
    Text(
        text = stringResource(R.string.shares_standing_preview_empty),
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = modifier
    )
}
