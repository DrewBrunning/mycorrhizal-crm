package com.mycorrhizal.crm.feature.timeline

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ContactFlat
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// M9 item 1: "reachability" proof for the Activities drawer entry — the real
// ActivitiesInboxScreenContent (not a placeholder) renders every contact's activities.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ActivitiesInboxScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        uiState: ActivitiesInboxUiState,
        onActivityClick: (Int) -> Unit = {},
        onContactClick: (Int) -> Unit = {},
        onLoadMore: () -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ActivitiesInboxScreenContent(
                    uiState = uiState,
                    onActivityClick = onActivityClick,
                    onContactClick = onContactClick,
                    onLoadMore = onLoadMore,
                )
            }
        }
    }

    @Test
    fun `shows the empty state when there are no activities`() {
        setContent(ActivitiesInboxUiState(isLoading = false, activities = emptyList()))
        composeTestRule.onNodeWithText("No activities yet").assertIsDisplayed()
    }

    @Test
    fun `renders real activity content, not a placeholder`() {
        setContent(
            ActivitiesInboxUiState(
                isLoading = false,
                activities = listOf(Activity(id = 1, title = "Coffee with Dana"), Activity(id = 2, title = "Call with Carol")),
            ),
        )
        composeTestRule.onNodeWithText("Coffee with Dana").assertIsDisplayed()
        composeTestRule.onNodeWithText("Call with Carol").assertIsDisplayed()
    }

    @Test
    fun `tapping an activity row invokes the click callback`() {
        var clickedId: Int? = null
        setContent(
            ActivitiesInboxUiState(isLoading = false, activities = listOf(Activity(id = 1, title = "Coffee with Dana"))),
            onActivityClick = { clickedId = it },
        )
        composeTestRule.onNodeWithText("Coffee with Dana").performClick()
        assertEquals(1, clickedId)
    }

    @Test
    fun `a contact chip navigates to that contact, not the activity`() {
        var clickedActivityId: Int? = null
        var clickedContactId: Int? = null
        setContent(
            ActivitiesInboxUiState(
                isLoading = false,
                activities = listOf(
                    Activity(id = 1, title = "Coffee with Dana", contacts = listOf(ContactFlat(id = 5, firstname = "Dana"))),
                ),
            ),
            onActivityClick = { clickedActivityId = it },
            onContactClick = { clickedContactId = it },
        )
        composeTestRule.onNodeWithText("Dana").performClick()
        assertEquals(5, clickedContactId)
        assertEquals(null, clickedActivityId)
    }

    @Test
    fun `a load more button appears with a next cursor and calls back on tap`() {
        var loadMoreCalls = 0
        setContent(
            ActivitiesInboxUiState(isLoading = false, activities = listOf(Activity(id = 1, title = "Coffee")), nextCursor = "cursor-2"),
            onLoadMore = { loadMoreCalls++ },
        )
        composeTestRule.onNodeWithText("Load more").performClick()
        assertEquals(1, loadMoreCalls)
    }

    @Test
    fun `shows a loading skeleton while loading`() {
        setContent(ActivitiesInboxUiState(isLoading = true))
        composeTestRule.onNodeWithTag("activities-inbox-loading").assertIsDisplayed()
    }
}
