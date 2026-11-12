package com.mycorrhizal.crm.feature.contacts.entities

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.domain.repository.EntityRepository
import com.mycorrhizal.crm.model.network.EntityListResponse
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** #2513: the entities screen's entry point to the create-from-contacts helper. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class EntitiesScreenEntryTest {

    @get:Rule(order = 0)
    val mainDispatcherRule = MainDispatcherRule()

    @get:Rule(order = 1)
    val composeTestRule = createComposeRule()

    // The view model's flow is collected on the (standard) test main dispatcher: run it, then let
    // Compose recompose.
    private fun settle() {
        mainDispatcherRule.testDispatcher.scheduler.advanceUntilIdle()
        composeTestRule.waitForIdle()
    }

    private fun show(onOpen: () -> Unit) {
        val repository = mockk<EntityRepository>()
        coEvery { repository.list(any(), any(), any(), any()) } returns
            Result.success(EntityListResponse())
        val vm = EntitiesViewModel(repository)
        composeTestRule.setContent {
            MycorrhizalTheme {
                EntitiesScreen(
                    onBack = {},
                    onOpenEntity = {},
                    onCreateFromContacts = onOpen,
                    viewModel = vm
                )
            }
        }
    }

    @Test
    fun `organisations offer the helper in the top bar and it opens it`() {
        var opened = 0
        show { opened++ }
        composeTestRule.onNodeWithTag("entities-from-contacts").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Create from my contacts…").performClick()
        assertEquals(1, opened)
    }

    @Test
    fun `places offer it and pets do not`() {
        show {}
        composeTestRule.onNodeWithTag("entities-tab-place").performClick()
        settle()
        composeTestRule.onNodeWithTag("entities-from-contacts").assertIsDisplayed()
        composeTestRule.onNodeWithTag("entities-tab-pet").performClick()
        settle()
        composeTestRule.onAllNodesWithTag("entities-from-contacts").assertCountEquals(0)
    }
}
