package com.mycorrhizal.crm.feature.timeline

import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.domain.repository.UnfiledNotesPage
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
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
        onCreateNote: () -> Unit = {},
        darkTheme: Boolean = false,
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                NotesInboxScreenContent(
                    uiState = uiState,
                    onNoteClick = onNoteClick,
                    onLoadMore = onLoadMore,
                    onCreateNote = onCreateNote,
                )
            }
        }
    }

    @Test
    fun `the empty state says the unassigned set is empty, never No notes yet`() {
        // Issue #1757: a user whose notes are all on contacts saw "No notes yet" here.
        setContent(NotesInboxUiState(isLoading = false, notes = emptyList()))
        composeTestRule.onNodeWithText("No unassigned notes yet").assertIsDisplayed()
        composeTestRule.onNodeWithText("Notes you capture without a contact land here.").assertIsDisplayed()
        composeTestRule.onNodeWithText("No notes yet").assertDoesNotExist()
    }

    @Test
    fun `the title names the unassigned set`() {
        setContent(NotesInboxUiState(isLoading = false, notes = emptyList()))
        composeTestRule.onNodeWithText("Unassigned notes").assertIsDisplayed()
    }

    @Test
    fun `the empty state offers New note and the FAB does the same`() {
        var created = 0
        setContent(NotesInboxUiState(isLoading = false, notes = emptyList()), onCreateNote = { created++ })

        // The empty-state button and the FAB both carry the "New note" label.
        val buttons = composeTestRule.onAllNodesWithText("New note")
        buttons[0].performClick()
        composeTestRule.onNodeWithContentDescription("New note").performClick()
        assertEquals(2, created)
    }

    @Test
    fun `a populated list has the FAB but no empty state`() {
        var created = 0
        setContent(
            NotesInboxUiState(isLoading = false, notes = listOf(Note(id = 3, content = "Buy milk"))),
            onCreateNote = { created++ },
        )
        composeTestRule.onNodeWithText("No unassigned notes yet").assertDoesNotExist()
        composeTestRule.onNodeWithContentDescription("New note").performClick()
        assertEquals(1, created)
    }

    @Test
    fun `an empty list with an error renders the error message`() {
        setContent(NotesInboxUiState(isLoading = false, notes = emptyList(), error = "boom"))
        composeTestRule.onNodeWithText("boom").assertIsDisplayed()
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
    fun `shows the total count`() {
        setContent(NotesInboxUiState(isLoading = false, notes = listOf(Note(id = 3, content = "Buy milk")), total = 7))
        composeTestRule.onNodeWithText("7 total").assertIsDisplayed()
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

    @Test
    fun `the loading skeleton announces itself as a polite live region`() {
        // #203: nothing announced the loading state to TalkBack -- the
        // skeleton boxes carried no text, so the screen read as blank while
        // loading.
        setContent(NotesInboxUiState(isLoading = true))
        composeTestRule.onNodeWithTag("notes-inbox-loading")
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.LiveRegion, LiveRegionMode.Polite))
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

    // --- Top-level NotesInboxScreen against a real ViewModel -----------------
    //
    // `onRefresh = viewModel::load` (line 65) is only evaluated when the real
    // top-level screen wires a real ViewModel; the content tests above pass
    // their own callbacks. Construct the VM directly (mockk repository, no Hilt
    // container) as NotesInboxViewModelTest does.
    @Test
    fun `top-level screen renders the view model's unfiled notes`() {
        val repository = mockk<NoteRepository>()
        coEvery { repository.listUnfiled() } returns Result.success(
            UnfiledNotesPage(
                notes = listOf(Note(id = 3, content = "Buy milk")),
                nextCursor = null,
                total = 1,
            ),
        )
        val viewModel = NotesInboxViewModel(repository)

        composeTestRule.setContent {
            MycorrhizalTheme {
                NotesInboxScreen(onNoteClick = {}, viewModel = viewModel)
            }
        }
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Buy milk").assertIsDisplayed()
    }
}
