package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.size
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconButtonColors
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

/** Material touch-target guidance / WCAG 2.5.8 Target Size (Minimum). */
private val MIN_TOUCH_TARGET = 48.dp

/**
 * Issue #214/#215: a drop-in [IconButton] replacement that forces the
 * touch-target-affecting size to the 48dp minimum instead of Material3's own
 * 40dp default, which `core:testing`'s a11y sweep
 * (`AccessibilitySemantics.kt`) flags on every icon-only clickable.
 *
 * `Modifier.minimumInteractiveComponentSize()` (which `IconButton` already
 * applies internally) reserves layout *space* around a small visual but does
 * not change what accessibility services see as the node's bounds — only a
 * real outer size constraint does, since `IconButton`'s inner
 * `Modifier.size(StateLayerSize)` (40dp) yields to a tighter incoming
 * constraint from the caller's own `modifier`.
 *
 * Every screen's TopAppBar navigation/action icon and list-row edit/delete
 * icons should use this instead of a bare `IconButton`, so the fix lives in
 * one place instead of being re-applied — and potentially missed — at each
 * call site individually.
 */
@Composable
fun AccessibleIconButton(
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    colors: IconButtonColors = IconButtonDefaults.iconButtonColors(),
    interactionSource: MutableInteractionSource? = null,
    content: @Composable () -> Unit,
) {
    IconButton(
        onClick = onClick,
        modifier = modifier.size(MIN_TOUCH_TARGET),
        enabled = enabled,
        colors = colors,
        interactionSource = interactionSource,
        content = content,
    )
}
