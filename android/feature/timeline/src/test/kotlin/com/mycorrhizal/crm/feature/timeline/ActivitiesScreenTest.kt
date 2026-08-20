package com.mycorrhizal.crm.feature.timeline

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ContactFlat
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// M19: the per-contact activities list's search/date filters, load-more,
// delete-confirmation (M17's rule), and participant chips, driving the real
// ActivitiesScreenContent.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ActivitiesScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        state: ActivitiesUiState,
        onEditActivity: (Int) -> Unit = {},
        onContactClick: (Int) -> Unit = {},
        onDelete: (Int) -> Unit = {},
        onSearchChange: (String) -> Unit = {},
        onFromDateChange: (String) -> Unit = {},
        onToDateChange: (String) -> Unit = {},
        onLoadMore: () -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ActivitiesScreenContent(
                    uiState = state,
                    onEditActivity = onEditActivity,
                    onContactClick = onContactClick,
                    onSearchChange = onSearchChange,
                    onFromDateChange = onFromDateChange,
                    onToDateChange = onToDateChange,
                    onLoadMore = onLoadMore,
                    onDelete = onDelete,
                )
            }
        }
    }

    @Test
    fun `shows empty state when no activities`() {
        setContent(ActivitiesUiState(contactId = 5, activities = emptyList()))
        composeTestRule.onNodeWithText("No activities yet").assertIsDisplayed()
    }

    @Test
    fun `shows a no-results message when filters are active`() {
        setContent(ActivitiesUiState(contactId = 5, activities = emptyList(), fromDate = "2026-08-01"))
        composeTestRule.onNodeWithText("No activities match your filters").assertIsDisplayed()
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
            ActivitiesUiState(contactId = 5, activities = listOf(Activity(id = 7, title = "Lunch", type = "meal"))),
            onEditActivity = { editedId = it },
        )
        composeTestRule.onNodeWithText("Lunch").performClick()
        assertEquals(7, editedId)
    }

    @Test
    fun `a participant chip navigates to that contact, not the activity edit`() {
        var editedId: Int? = null
        var contactedId: Int? = null
        setContent(
            ActivitiesUiState(
                contactId = 5,
                activities = listOf(
                    Activity(id = 1, title = "Coffee with Dana", contacts = listOf(ContactFlat(id = 5, firstname = "Dana"))),
                ),
            ),
            onEditActivity = { editedId = it },
            onContactClick = { contactedId = it },
        )
        composeTestRule.onNodeWithText("Dana").performClick()
        assertEquals(5, contactedId)
        assertNull(editedId)
    }

    @Test
    fun `typing in the search field invokes onSearchChange`() {
        var query = ""
        setContent(
            ActivitiesUiState(contactId = 5, activities = emptyList()),
            onSearchChange = { query = it },
        )
        composeTestRule.onNodeWithText("Search activities").performTextInput("coffee")
        assertEquals("coffee", query)
    }

    @Test
    fun `typing in the from date field invokes the callback`() {
        var from = ""
        setContent(
            ActivitiesUiState(contactId = 5, activities = emptyList()),
            onFromDateChange = { from = it },
        )
        composeTestRule.onNodeWithTag("timeline-from-date").performTextInput("2026-08-01")
        assertEquals("2026-08-01", from)
    }

    @Test
    fun `typing in the to date field invokes the callback`() {
        var to = ""
        setContent(
            ActivitiesUiState(contactId = 5, activities = emptyList()),
            onToDateChange = { to = it },
        )
        composeTestRule.onNodeWithTag("timeline-to-date").performTextInput("2026-08-10")
        assertEquals("2026-08-10", to)
    }

    @Test
    fun `delete asks first -- tapping delete shows a confirmation and does not call onDelete`() {
        var deletedId: Int? = null
        setContent(
            ActivitiesUiState(contactId = 5, activities = listOf(Activity(id = 7, title = "Lunch"))),
            onDelete = { deletedId = it },
        )
        composeTestRule.onNodeWithContentDescription("Delete Lunch").performClick()

        composeTestRule.onNodeWithText("Delete activity?").assertIsDisplayed()
        assertNull(deletedId)
    }

    @Test
    fun `cancel is inert -- dismissing the confirmation issues no call and leaves the item present`() {
        var deletedId: Int? = null
        setContent(
            ActivitiesUiState(contactId = 5, activities = listOf(Activity(id = 7, title = "Lunch"))),
            onDelete = { deletedId = it },
        )
        composeTestRule.onNodeWithContentDescription("Delete Lunch").performClick()
        composeTestRule.onNodeWithText("Cancel").performClick()

        assertNull(deletedId)
        composeTestRule.onNodeWithText("Lunch").assertIsDisplayed()
    }

    @Test
    fun `confirming the dialog calls onDelete with the right id`() {
        var deletedId: Int? = null
        setContent(
            ActivitiesUiState(contactId = 5, activities = listOf(Activity(id = 7, title = "Lunch"))),
            onDelete = { deletedId = it },
        )
        composeTestRule.onNodeWithContentDescription("Delete Lunch").performClick()
        composeTestRule.onNodeWithText("Delete").performClick()

        assertEquals(7, deletedId)
    }

    @Test
    fun `a load more button appears with a next cursor and calls back on tap`() {
        var loadMoreCalls = 0
        setContent(
            ActivitiesUiState(contactId = 5, activities = listOf(Activity(id = 7, title = "Lunch")), nextCursor = "cursor-2"),
            onLoadMore = { loadMoreCalls++ },
        )
        composeTestRule.onNodeWithText("Load more").performClick()
        assertEquals(1, loadMoreCalls)
    }
}
