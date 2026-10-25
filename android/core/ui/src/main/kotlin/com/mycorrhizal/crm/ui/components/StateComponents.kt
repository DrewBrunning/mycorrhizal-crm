package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.ui.R

/**
 * Skeleton rows shown while a screen's data is loading. Pulsing is left to
 * later polish; static placeholders keep the tests deterministic.
 */
@Composable
fun LoadingSkeleton(rows: Int = 8, modifier: Modifier = Modifier) {
    val shimmerColor = MaterialTheme.colorScheme.surfaceVariant
    val baseColor = MaterialTheme.colorScheme.surface
    val brush = Brush.horizontalGradient(listOf(baseColor, shimmerColor, baseColor))
    val loadingLabel = stringResource(R.string.a11y_state_loading)
    // #203: nothing announced the loading state to TalkBack — the skeleton
    // boxes carry no text, so the screen reads as blank while loading. The
    // container gets the announcement (contentDescription + a polite live
    // region so it's spoken once it appears, not competing for focus); the
    // decorative boxes are cleared out of the tree so they don't add noise.
    Column(
        modifier = modifier
            .padding(16.dp)
            .semantics {
                liveRegion = LiveRegionMode.Polite
                contentDescription = loadingLabel
            }
    ) {
        repeat(rows) {
            Box(
                modifier = Modifier
                    .padding(vertical = 8.dp)
                    .fillMaxWidth()
                    .height(56.dp)
                    .clip(RoundedCornerShape(10.dp))
                    .background(brush)
                    .clearAndSetSemantics {}
            )
        }
    }
}

/**
 * The one empty state every list screen uses (issue #1760): an optional [icon],
 * a [title] following the "No <things> yet" pattern (no trailing period), at
 * most one sentence of [body] saying what the list is for, and — where the
 * screen can create the thing — the primary create action as a button
 * ([actionLabel] + [onAction], both or neither).
 *
 * Pass only [title] for a plain "nothing here" or a fatal-error message; the
 * layout degrades to a single centred line. Wording rules live in
 * `docs/android-app.md` ("Design system").
 */
@Composable
fun EmptyState(
    title: String,
    modifier: Modifier = Modifier,
    body: String? = null,
    icon: ImageVector? = null,
    actionLabel: String? = null,
    onAction: (() -> Unit)? = null
) {
    // #203: nothing announced an empty result to TalkBack — the title reads
    // fine once focused, but nothing spoke it when it replaced the loading
    // skeleton. The live region sits on the text block only, so the action
    // button stays an ordinary focusable control.
    Box(
        modifier = modifier
            .fillMaxWidth()
            .padding(32.dp),
        contentAlignment = Alignment.Center
    ) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            if (icon != null) {
                Icon(
                    imageVector = icon,
                    contentDescription = null,
                    modifier = Modifier.size(48.dp).padding(bottom = 8.dp),
                    tint = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
            Column(
                horizontalAlignment = Alignment.CenterHorizontally,
                modifier = Modifier.semantics(mergeDescendants = true) {
                    liveRegion = LiveRegionMode.Polite
                }
            ) {
                Text(
                    text = title,
                    style = if (body != null || onAction != null) {
                        MaterialTheme.typography.titleMedium
                    } else {
                        MaterialTheme.typography.bodyLarge
                    },
                    color = if (body != null || onAction != null) {
                        MaterialTheme.colorScheme.onSurface
                    } else {
                        MaterialTheme.colorScheme.onSurfaceVariant
                    },
                    textAlign = TextAlign.Center
                )
                if (body != null) {
                    Text(
                        text = body,
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        textAlign = TextAlign.Center,
                        modifier = Modifier.padding(top = 4.dp)
                    )
                }
            }
            if (actionLabel != null && onAction != null) {
                FilledTonalButton(onClick = onAction, modifier = Modifier.padding(top = 16.dp)) {
                    Text(actionLabel)
                }
            }
        }
    }
}
