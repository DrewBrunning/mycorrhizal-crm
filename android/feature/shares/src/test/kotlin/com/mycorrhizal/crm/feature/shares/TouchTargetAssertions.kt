package com.mycorrhizal.crm.feature.shares

import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertHeightIsAtLeast
import androidx.compose.ui.test.junit4.ComposeContentTestRule
import androidx.compose.ui.unit.dp
import org.junit.Assert.assertEquals

/**
 * Issues #2609/#2716: asserts that exactly [expectedCount] nodes match [matcher] and that every
 * one of them is MEASURED at least 48dp tall (the WCAG/Material touch-target floor). The count
 * guards against the vacuous pass where a refactor makes the matcher select nothing.
 */
internal fun ComposeContentTestRule.assertRowsAtLeast48dp(
    matcher: SemanticsMatcher,
    expectedCount: Int
) {
    val nodes = onAllNodes(matcher)
    assertEquals(expectedCount, nodes.fetchSemanticsNodes().size)
    for (index in 0 until expectedCount) {
        nodes[index].assertHeightIsAtLeast(48.dp)
    }
}
