package com.mycorrhizal.crm.feature.timeline

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// M19: the per-contact notes list's search/date filters, load-more, and
// delete-confirmation (M17's rule), driving the real NotesScreenContent.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class NotesScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        state: NotesUiState,
        onEditNote: (Int) -> Unit = {},
        onDelete: (Int) -> Unit = {},
        onSearchChange: (String) -> Unit = {},
        onFromDateChange: (String) -> Unit = {},
        onToDateChange: (String) -> Unit = {},
        onLoadMore: () -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                NotesScreenContent(
                    uiState = state,
                    onEditNote = onEditNote,
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
    fun `shows empty state when no notes`() {
        setContent(NotesUiState(contactId = 5, notes = emptyList()))
        composeTestRule.onNodeWithText("No notes yet").assertIsDisplayed()
    }

    @Test
    fun `shows a no-results message when filters are active`() {
        setContent(NotesUiState(contactId = 5, notes = emptyList(), searchQuery = "zzz"))
        composeTestRule.onNodeWithText("No notes match your filters").assertIsDisplayed()
    }

    @Test
    fun `shows note list items`() {
        setContent(
            NotesUiState(
                contactId = 5,
                notes = listOf(
                    Note(id = 3, content = "Loves climbing"),
                    Note(id = 4, content = "Met at conference"),
                ),
            ),
        )
        composeTestRule.onNodeWithText("Loves climbing").assertIsDisplayed()
        composeTestRule.onNodeWithText("Met at conference").assertIsDisplayed()
    }

    @Test
    fun `tapping a note invokes the edit callback`() {
        var editedId: Int? = null
        setContent(
            NotesUiState(contactId = 5, notes = listOf(Note(id = 3, content = "Loves climbing"))),
            onEditNote = { editedId = it },
        )
        composeTestRule.onNodeWithText("Loves climbing").performClick()
        assertEquals(3, editedId)
    }

    @Test
    fun `typing in the search field invokes onSearchChange`() {
        var query = ""
        setContent(
            NotesUiState(contactId = 5, notes = emptyList()),
            onSearchChange = { query = it },
        )
        composeTestRule.onNodeWithText("Search notes").performTextInput("climb")
        assertEquals("climb", query)
    }

    @Test
    fun `typing in the from date field invokes the callback`() {
        var from = ""
        setContent(
            NotesUiState(contactId = 5, notes = emptyList()),
            onFromDateChange = { from = it },
        )
        composeTestRule.onNodeWithTag("timeline-from-date").performTextInput("2026-08-01")
        assertEquals("2026-08-01", from)
    }

    @Test
    fun `typing in the to date field invokes the callback`() {
        var to = ""
        setContent(
            NotesUiState(contactId = 5, notes = emptyList()),
            onToDateChange = { to = it },
        )
        composeTestRule.onNodeWithTag("timeline-to-date").performTextInput("2026-08-10")
        assertEquals("2026-08-10", to)
    }

    @Test
    fun `delete asks first -- tapping delete shows a confirmation and does not call onDelete`() {
        var deletedId: Int? = null
        setContent(
            NotesUiState(contactId = 5, notes = listOf(Note(id = 3, content = "Loves climbing"))),
            onDelete = { deletedId = it },
        )
        composeTestRule.onNodeWithContentDescription("Delete Loves climbing").performClick()

        composeTestRule.onNodeWithText("Delete note?").assertIsDisplayed()
        // The confirmation is up; the repository call has NOT happened.
        assertNull(deletedId)
    }

    @Test
    fun `cancel is inert -- dismissing the confirmation issues no call and leaves the item present`() {
        var deletedId: Int? = null
        setContent(
            NotesUiState(contactId = 5, notes = listOf(Note(id = 3, content = "Loves climbing"))),
            onDelete = { deletedId = it },
        )
        composeTestRule.onNodeWithContentDescription("Delete Loves climbing").performClick()
        composeTestRule.onNodeWithText("Cancel").performClick()

        assertNull(deletedId)
        composeTestRule.onNodeWithText("Loves climbing").assertIsDisplayed()
    }

    @Test
    fun `confirming the dialog calls onDelete with the right id`() {
        var deletedId: Int? = null
        setContent(
            NotesUiState(contactId = 5, notes = listOf(Note(id = 3, content = "Loves climbing"))),
            onDelete = { deletedId = it },
        )
        composeTestRule.onNodeWithContentDescription("Delete Loves climbing").performClick()
        // The dialog's confirm TextButton ("Delete") is the only exact-match
        // text node once the dialog is up — the row affordance is an icon.
        composeTestRule.onNodeWithText("Delete").performClick()

        assertEquals(3, deletedId)
    }

    @Test
    fun `a load more button appears with a next cursor and calls back on tap`() {
        var loadMoreCalls = 0
        setContent(
            NotesUiState(contactId = 5, notes = listOf(Note(id = 3, content = "Loves climbing")), nextCursor = "cursor-2"),
            onLoadMore = { loadMoreCalls++ },
        )
        composeTestRule.onNodeWithText("Load more").performClick()
        assertEquals(1, loadMoreCalls)
    }
}
