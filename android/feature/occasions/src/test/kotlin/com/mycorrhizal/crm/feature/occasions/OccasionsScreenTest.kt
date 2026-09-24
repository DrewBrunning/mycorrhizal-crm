package com.mycorrhizal.crm.feature.occasions

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.domain.repository.OccasionEventRepository
import com.mycorrhizal.crm.model.network.OccasionEvent
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class OccasionsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val repository = mockk<OccasionEventRepository>()

    private fun event(id: String = "e1", title: String = "Summer BBQ") =
        OccasionEvent(id = id, title = title, startsAt = "2026-07-04T15:00:00Z", location = "The park")

    private fun render(events: List<OccasionEvent>) {
        coEvery { repository.list() } returns Result.success(events)
        val viewModel = OccasionsViewModel(repository)
        composeTestRule.setContent {
            MycorrhizalTheme {
                OccasionsScreen(onBack = {}, onOpenAttendees = {}, viewModel = viewModel)
            }
        }
        composeTestRule.waitForIdle()
    }

    @Test
    fun `renders the list with the formatted start`() {
        render(listOf(event()))
        composeTestRule.onNodeWithText("Occasions").assertIsDisplayed()
        composeTestRule.onNodeWithText("Summer BBQ").assertIsDisplayed()
        composeTestRule.onNodeWithText("2026-07-04 15:00").assertIsDisplayed()
        composeTestRule.onNodeWithText("The park").assertIsDisplayed()
    }

    @Test
    fun `shows the empty state when there are no events`() {
        render(emptyList())
        composeTestRule
            .onNodeWithText("No events yet. Create one to start planning who to invite.")
            .assertIsDisplayed()
    }

    @Test
    fun `the add button opens the create dialog`() {
        render(emptyList())
        composeTestRule.onNodeWithContentDescription("New event").performClick()
        composeTestRule.onNodeWithText("Create an event").assertIsDisplayed()
    }

    @Test
    fun `the edit action opens a prefilled dialog`() {
        render(listOf(event()))
        composeTestRule.onNodeWithContentDescription("Edit").performClick()
        composeTestRule.onNodeWithText("Edit event").assertIsDisplayed()
        // The row and the prefilled title field both carry the title.
        composeTestRule.onAllNodesWithText("Summer BBQ").assertCountEquals(2)
    }

    @Test
    fun `the delete action opens a confirmation`() {
        render(listOf(event()))
        composeTestRule.onNodeWithContentDescription("Delete").performClick()
        composeTestRule.onNodeWithText("Delete event").assertIsDisplayed()
        composeTestRule.onNodeWithText("Delete “Summer BBQ”? This cannot be undone.").assertIsDisplayed()
    }
}
