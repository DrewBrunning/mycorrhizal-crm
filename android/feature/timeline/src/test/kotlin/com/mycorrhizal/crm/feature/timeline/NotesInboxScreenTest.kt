package com.mycorrhizal.crm.feature.timeline

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// M9 item 1: "reachability" proof for the Notes drawer entry — the real NotesInboxScreenContent
// (not a placeholder, not a test-only reimplementation) renders the unfiled-notes queue.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class NotesInboxScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        uiState: NotesInboxUiState,
        onNoteClick: (Int) -> Unit = {},
        onLoadMore: () -> Unit = {},
        darkTheme: Boolean = false,
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                NotesInboxScreenContent(
                    uiState = uiState,
                    onNoteClick = onNoteClick,
                    onLoadMore = onLoadMore,
                )
            }
        }
    }

    @Test
    fun `shows the empty state when there are no unfiled notes`() {
        setContent(NotesInboxUiState(isLoading = false, notes = emptyList()))
        composeTestRule.onNodeWithText("No notes yet").assertIsDisplayed()
    }

    @Test
    fun `renders real note content, not a placeholder`() {
        setContent(
            NotesInboxUiState(
                isLoading = false,
                notes = listOf(Note(id = 3, content = "Buy milk"), Note(id = 4, content = "Call mom")),
                total = 2,
            ),
        )
        composeTestRule.onNodeWithText("Buy milk").assertIsDisplayed()
        composeTestRule.onNodeWithText("Call mom").assertIsDisplayed()
    }

    @Test
    fun `shows the total unfiled count`() {
        setContent(NotesInboxUiState(isLoading = false, notes = listOf(Note(id = 3, content = "Buy milk")), total = 7))
        composeTestRule.onNodeWithText("7 unfiled").assertIsDisplayed()
    }

    @Test
    fun `tapping a note invokes the click callback`() {
        var clickedId: Int? = null
        setContent(
            NotesInboxUiState(isLoading = false, notes = listOf(Note(id = 3, content = "Buy milk"))),
            onNoteClick = { clickedId = it },
        )
        composeTestRule.onNodeWithText("Buy milk").performClick()
        assertEquals(3, clickedId)
    }

    @Test
    fun `a load more button appears with a next cursor and calls back on tap`() {
        var loadMoreCalls = 0
        setContent(
            NotesInboxUiState(isLoading = false, notes = listOf(Note(id = 3, content = "Buy milk")), nextCursor = "cursor-2"),
            onLoadMore = { loadMoreCalls++ },
        )
        composeTestRule.onNodeWithText("Load more").performClick()
        assertEquals(1, loadMoreCalls)
    }

    @Test
    fun `no load more button when there is no next cursor`() {
        setContent(NotesInboxUiState(isLoading = false, notes = listOf(Note(id = 3, content = "Buy milk")), nextCursor = null))
        assertEquals(0, composeTestRule.onAllNodesWithText("Load more").fetchSemanticsNodes().size)
    }

    @Test
    fun `shows a loading skeleton while loading`() {
        setContent(NotesInboxUiState(isLoading = true))
        composeTestRule.onNodeWithTag("notes-inbox-loading").assertIsDisplayed()
    }

    // --- Issue #214: Compose semantics a11y sweep (the axe-core analog) -----

    private fun populatedState() = NotesInboxUiState(
        isLoading = false,
        notes = listOf(Note(id = 3, content = "Buy milk"), Note(id = 4, content = "Call mom")),
        total = 2,
    )

    @Test
    fun `notes inbox has no accessibility violations (light)`() {
        setContent(populatedState(), darkTheme = false)

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `notes inbox has no accessibility violations (dark)`() {
        setContent(populatedState(), darkTheme = true)

        composeTestRule.assertAccessibleSemantics()
    }
}
