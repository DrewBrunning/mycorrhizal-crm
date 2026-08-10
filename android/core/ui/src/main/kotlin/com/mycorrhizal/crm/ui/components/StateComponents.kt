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
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp

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
    Column(modifier = modifier.padding(16.dp)) {
        repeat(rows) {
            Box(
                modifier = Modifier
                    .padding(vertical = 8.dp)
                    .fillMaxWidth()
                    .height(56.dp)
                    .clip(RoundedCornerShape(10.dp))
                    .background(brush),
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
    Box(
        modifier = modifier.fillMaxWidth().padding(32.dp),
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
