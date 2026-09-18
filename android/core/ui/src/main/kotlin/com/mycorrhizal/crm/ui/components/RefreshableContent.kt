package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.layout.BoxScope
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier

/**
 * Wraps a screen's scrollable content in a Material3 pull-to-refresh container.
 *
 * [isRefreshing] drives the indicator while a refresh is in flight. Callers must
 * keep the existing content visible during a refresh (only show the initial-load
 * skeleton when the list is empty) so pulling down doesn't blank the screen.
 * [onRefresh] runs when the user pulls down.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RefreshableContent(
    isRefreshing: Boolean,
    onRefresh: () -> Unit,
    modifier: Modifier = Modifier,
    content: @Composable BoxScope.() -> Unit,
) {
    PullToRefreshBox(
        isRefreshing = isRefreshing,
        onRefresh = onRefresh,
        modifier = modifier,
        content = content,
    )
}
