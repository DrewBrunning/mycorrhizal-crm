package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
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
fun LoadingSkeleton(
    rows: Int = 8,
    modifier: Modifier = Modifier,
) {
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
            },
    ) {
        repeat(rows) {
            Box(
                modifier = Modifier
                    .padding(vertical = 8.dp)
                    .fillMaxWidth()
                    .height(56.dp)
                    .clip(RoundedCornerShape(10.dp))
                    .background(brush)
                    .clearAndSetSemantics {},
            )
        }
    }
}

@Composable
fun EmptyState(
    message: String,
    modifier: Modifier = Modifier,
    icon: (@Composable () -> Unit)? = null,
) {
    // #203: nothing announced an empty result to TalkBack — the message Text
    // reads fine once focused, but nothing spoke it when it replaced the
    // loading skeleton.
    Box(
        modifier = modifier
            .fillMaxWidth()
            .padding(32.dp)
            .semantics { liveRegion = LiveRegionMode.Polite },
        contentAlignment = Alignment.Center,
    ) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            icon?.invoke()
            Text(
                text = message,
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center,
            )
        }
    }
}
