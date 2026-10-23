package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.FloatingActionButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

/**
 * Bottom inset that keeps the last row of a [BrandFab]-bearing list scrollable
 * clear of the FAB: the 56dp button, its 16dp margin, and 16dp of breathing
 * room. Scaffold already supplies the navigation-bar inset, so this is
 * additive to the content padding rather than a replacement for it.
 */
val FabListBottomPadding = 88.dp

/** Convenience contentPadding for a list whose FAB floats over its rows. */
val FabListContentPadding = PaddingValues(bottom = FabListBottomPadding)

/**
 * Brand FAB: a circular button in the mycelium green with a light (bone)
 * icon — the tappable "create" affordance across the app, matching the web's
 * contained-action color. M3's default FAB is `primaryContainer` (parchment)
 * with dark content, which reads as non-tappable here.
 */
@Composable
fun BrandFab(
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    content: @Composable () -> Unit,
) {
    FloatingActionButton(
        onClick = onClick,
        modifier = modifier,
        containerColor = MaterialTheme.colorScheme.primary,
        contentColor = MaterialTheme.colorScheme.onPrimary,
        shape = androidx.compose.foundation.shape.CircleShape,
        elevation = FloatingActionButtonDefaults.elevation(
            defaultElevation = 4.dp,
            pressedElevation = 8.dp,
        ),
    ) {
        content()
    }
}
