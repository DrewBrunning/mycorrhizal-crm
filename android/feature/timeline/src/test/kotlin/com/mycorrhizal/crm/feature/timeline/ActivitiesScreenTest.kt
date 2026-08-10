package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Scaffold
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
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
class ActivitiesScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        state: ActivitiesUiState,
        onEditActivity: (Int) -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                Scaffold { padding ->
                    Box(modifier = Modifier.padding(padding)) {
                        when {
                            state.isLoading -> LoadingSkeleton()
                            state.activities.isEmpty() && state.error == null ->
                                EmptyState("No activities yet")
                            else -> {
                                LazyColumn {
                                    items(state.activities) { activity ->
                                        ActivityListItem(
                                            activity = activity,
                                            onClick = { onEditActivity(activity.id) },
                                        )
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    @Test
    fun `shows empty state when no activities`() {
        setContent(ActivitiesUiState(contactId = 5, activities = emptyList()))
        composeTestRule.onNodeWithText("No activities yet").assertIsDisplayed()
    }

    @Test
    fun `shows activity list items`() {
        setContent(
            ActivitiesUiState(
                contactId = 5,
                activities = listOf(
                    Activity(id = 1, title = "Coffee with Dana", type = "visit"),
                    Activity(id = 2, title = "Phone call", type = "call"),
                ),
            ),
        )
        composeTestRule.onNodeWithText("Coffee with Dana").assertIsDisplayed()
        composeTestRule.onNodeWithText("Phone call").assertIsDisplayed()
    }

    @Test
    fun `tapping an activity invokes the edit callback`() {
        var editedId: Int? = null
        setContent(
            ActivitiesUiState(
                contactId = 5,
                activities = listOf(Activity(id = 7, title = "Lunch", type = "meal")),
            ),
            onEditActivity = { editedId = it },
        )
        composeTestRule.onNodeWithText("Lunch").performClick()
        assertEquals(7, editedId)
    }
}
