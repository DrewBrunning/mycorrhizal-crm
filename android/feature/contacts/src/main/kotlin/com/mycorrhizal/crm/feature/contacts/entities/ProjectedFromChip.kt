package com.mycorrhizal.crm.feature.contacts.entities

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.ui.R

/**
 * The source chip of a projected value (ADR 0043 decision 3): "from Acme", so the user can tell a
 * value shown from a linked entity from their own. [entityName] null falls back to a generic
 * "from a linked entity". Read-only by construction: it is text, not a control.
 */
@Composable
internal fun ProjectedFromChip(entityName: String?, modifier: Modifier = Modifier) {
    val text = if (entityName.isNullOrBlank()) {
        stringResource(R.string.contact_entity_links_projected_from_unknown)
    } else {
        stringResource(R.string.contact_entity_links_projected_from, entityName)
    }
    val description = if (entityName.isNullOrBlank()) {
        stringResource(R.string.contact_entity_links_projected_from_unknown)
    } else {
        stringResource(R.string.contact_entity_links_cd_from, entityName)
    }
    Surface(
        modifier = modifier.testTag(
            "projected-from-chip"
        ).semantics { contentDescription = description },
        color = MaterialTheme.colorScheme.secondaryContainer,
        contentColor = MaterialTheme.colorScheme.onSecondaryContainer,
        shape = MaterialTheme.shapes.small
    ) {
        Text(
            text = text,
            style = MaterialTheme.typography.labelMedium,
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp)
        )
    }
}

/**
 * A read-only list row for a projected ORG/ADR value: the value, the source chip, and the hint that
 * it is changed on the entity's page (or copied into the contact to edit it here). It carries no
 * edit or delete control.
 */
@Composable
internal fun ProjectedEntryRow(
    title: String,
    entityName: String?,
    tag: String,
    modifier: Modifier = Modifier,
    subtitle: String? = null
) {
    Surface(
        modifier = modifier.fillMaxWidth().padding(
            horizontal = 16.dp,
            vertical = 4.dp
        ).testTag(tag),
        tonalElevation = 1.dp
    ) {
        Column(Modifier.padding(horizontal = 12.dp, vertical = 8.dp)) {
            Text(text = title, style = MaterialTheme.typography.bodyLarge)
            if (!subtitle.isNullOrBlank()) {
                Text(
                    text = subtitle,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
            ProjectedFromChip(entityName, Modifier.padding(top = 4.dp))
            Text(
                text = stringResource(R.string.contact_entity_links_projected_hint),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 4.dp)
            )
        }
    }
}
