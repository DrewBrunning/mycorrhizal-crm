package com.mycorrhizal.crm.feature.contacts

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class AddToActivitiesDialogTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun show(
        state: AddToActivitiesUiState,
        onToggle: (Int) -> Unit = {},
        onSubmit: () -> Unit = {}
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                AddToActivitiesDialogContent(
                    state = state,
                    onQueryChange = {},
                    onToggle = onToggle,
                    onSubmit = onSubmit,
                    onDismiss = {}
                )
            }
        }
    }

    private val activities = listOf(
        Activity(id = 1, title = "Coffee"),
        Activity(id = 2, title = "Lunch")
    )

    @Test
    fun `submit is disabled until something is selected`() {
        show(AddToActivitiesUiState(activities = activities, isLoading = false))

        composeTestRule.onNodeWithTag("add-to-activities-submit").assertIsNotEnabled()
    }

    @Test
    fun `tapping a row toggles it and submit fires when selected`() {
        var toggled: Int? = null
        var submits = 0
        show(
            AddToActivitiesUiState(
                activities = activities,
                isLoading = false,
                selected = setOf(1)
            ),
            onToggle = { toggled = it },
            onSubmit = { submits++ }
        )

        composeTestRule.onNodeWithText("Lunch").performClick()
        composeTestRule.onNodeWithTag("add-to-activities-submit").assertIsEnabled().performClick()

        assertEquals(2, toggled)
        assertEquals(1, submits)
    }

    @Test
    fun `not found titles stay listed in the open dialog`() {
        show(
            AddToActivitiesUiState(
                activities = activities,
                isLoading = false,
                failedTitles = listOf("Lunch"),
                selected = setOf(2)
            )
        )

        composeTestRule.onNodeWithText("Not added (activity not found):").assertIsDisplayed()
    }
}
