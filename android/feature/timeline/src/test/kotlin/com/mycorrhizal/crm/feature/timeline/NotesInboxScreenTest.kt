package com.mycorrhizal.crm.feature.timeline

import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.domain.repository.NotesScope
import com.mycorrhizal.crm.domain.repository.UnfiledNotesPage
import com.mycorrhizal.crm.model.network.ContactFlat
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.NoteImage
import com.mycorrhizal.crm.network.ApiError
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
        onErrorShown: () -> Unit = {},
        onScopeChange: (NotesScope) -> Unit = {},
        onContactClick: (Int) -> Unit = {},
        darkTheme: Boolean = false
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                NotesInboxScreenContent(
                    uiState = uiState,
                    onNoteClick = onNoteClick,
                    onLoadMore = onLoadMore,
                    onCreateNote = onCreateNote,
                    onErrorShown = onErrorShown,
                    onScopeChange = onScopeChange,
                    onContactClick = onContactClick
                )
            }
        }
    }

    @Test
    fun `the empty state says the unassigned set is empty, never No notes yet`() {
        // Issue #1757: a user whose notes are all on contacts saw "No notes yet" here.
        setContent(
            NotesInboxUiState(isLoading = false, notes = emptyList(), scope = NotesScope.UNFILED)
        )
        composeTestRule.onNodeWithText("No unassigned notes yet").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Notes you capture without a contact land here."
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText("No notes yet").assertDoesNotExist()
    }

    @Test
    fun `the title names the unassigned set`() {
        setContent(
            NotesInboxUiState(isLoading = false, notes = emptyList(), scope = NotesScope.UNFILED)
        )
        composeTestRule.onNodeWithText("Unassigned notes").assertIsDisplayed()
    }

    @Test
    fun `the empty state offers New note and the FAB does the same`() {
        var created = 0
        setContent(
            NotesInboxUiState(isLoading = false, notes = emptyList()),
            onCreateNote = { created++ }
        )

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
            NotesInboxUiState(
                isLoading = false,
                notes = listOf(Note(id = 3, content = "Buy milk"))
            ),
            onCreateNote = { created++ }
        )
        composeTestRule.onNodeWithText("No unassigned notes yet").assertDoesNotExist()
        composeTestRule.onNodeWithContentDescription("New note").performClick()
        assertEquals(1, created)
    }

    @Test
    fun `an empty list with an error renders the error message`() {
        setContent(
            NotesInboxUiState(
                isLoading = false,
                notes = emptyList(),
                error = ApiError.Client(400, "boom")
            )
        )
        composeTestRule.onNodeWithText("boom").assertIsDisplayed()
    }

    // Issue #2106: the typed error is worded in the UI language at composition time.
    @Test
    @Config(qualifiers = "de")
    fun `an empty list with a client-worded error renders it in German`() {
        setContent(
            NotesInboxUiState(
                isLoading = false,
                notes = emptyList(),
                error = ApiError.Client(404, "Note not found")
            )
        )
        composeTestRule.onNodeWithText("Nicht gefunden").assertIsDisplayed()
    }

    @Test
    fun `renders real note content, not a placeholder`() {
        setContent(
            NotesInboxUiState(
                isLoading = false,
                notes = listOf(
                    Note(id = 3, content = "Buy milk"),
                    Note(id = 4, content = "Call mom")
                ),
                total = 2
            )
        )
        composeTestRule.onNodeWithText("Buy milk").assertIsDisplayed()
        composeTestRule.onNodeWithText("Call mom").assertIsDisplayed()
    }

    @Test
    fun `shows the total count`() {
        setContent(
            NotesInboxUiState(
                isLoading = false,
                notes = listOf(Note(id = 3, content = "Buy milk")),
                total = 7
            )
        )
        composeTestRule.onNodeWithText("7 total").assertIsDisplayed()
    }

    @Test
    fun `tapping a note invokes the click callback`() {
        var clickedId: Int? = null
        setContent(
            NotesInboxUiState(
                isLoading = false,
                notes = listOf(Note(id = 3, content = "Buy milk"))
            ),
            onNoteClick = { clickedId = it }
        )
        composeTestRule.onNodeWithText("Buy milk").performClick()
        assertEquals(3, clickedId)
    }

    @Test
    fun `a load more button appears with a next cursor and calls back on tap`() {
        var loadMoreCalls = 0
        setContent(
            NotesInboxUiState(
                isLoading = false,
                notes = listOf(Note(id = 3, content = "Buy milk")),
                nextCursor = "cursor-2"
            ),
            onLoadMore = { loadMoreCalls++ }
        )
        composeTestRule.onNodeWithText("Load more").performClick()
        assertEquals(1, loadMoreCalls)
    }

    @Test
    fun `no load more button when there is no next cursor`() {
        setContent(
            NotesInboxUiState(
                isLoading = false,
                notes = listOf(Note(id = 3, content = "Buy milk")),
                nextCursor = null
            )
        )
        assertEquals(0, composeTestRule.onAllNodesWithText("Load more").fetchSemanticsNodes().size)
    }

    @Test
    fun `a populated list with an error shows the snackbar and reports it shown`() {
        var errorShown = 0
        setContent(
            NotesInboxUiState(
                isLoading = false,
                notes = listOf(Note(id = 3, content = "Buy milk")),
                error = ApiError.Client(400, "boom")
            ),
            onErrorShown = { errorShown++ }
        )

        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("boom").assertIsDisplayed()
        composeTestRule.mainClock.advanceTimeBy(5_000)
        composeTestRule.waitForIdle()
        assertEquals(1, errorShown)
    }

    @Test
    fun `a note with a date shows the formatted date`() {
        setContent(
            NotesInboxUiState(
                isLoading = false,
                notes = listOf(Note(id = 3, content = "Buy milk", date = "2026-01-02"))
            )
        )

        composeTestRule.onNodeWithText("02.01.2026").assertIsDisplayed()
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
            .assert(
                SemanticsMatcher.expectValue(SemanticsProperties.LiveRegion, LiveRegionMode.Polite)
            )
    }

    // --- Issue #214: Compose semantics a11y sweep (the axe-core analog) -----

    private fun populatedState() = NotesInboxUiState(
        isLoading = false,
        notes = listOf(Note(id = 3, content = "Buy milk"), Note(id = 4, content = "Call mom")),
        total = 2
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
        coEvery { repository.listNotes(NotesScope.ALL) } returns Result.success(
            UnfiledNotesPage(
                notes = listOf(Note(id = 3, content = "Buy milk")),
                nextCursor = null,
                total = 1
            )
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

    // Issue #2134
    @Test
    fun `an inbox note row renders Markdown instead of showing the raw markers`() {
        setContent(NotesInboxUiState(notes = listOf(Note(id = 3, content = "_Call_ back"))))

        composeTestRule.onNodeWithText("Call back").assertIsDisplayed()
    }

    // --- Issue #2224: all notes by default, an All notes / Unfiled filter, the contact per note ---

    @Test
    fun `the default view is titled Notes and its empty state is not the unassigned one`() {
        setContent(NotesInboxUiState(isLoading = false, notes = emptyList()))

        composeTestRule.onNodeWithText("Notes").assertIsDisplayed()
        composeTestRule.onNodeWithText("No notes yet").assertIsDisplayed()
        composeTestRule.onNodeWithText("No unassigned notes yet").assertDoesNotExist()
    }

    @Test
    fun `a filed note shows its contact name and tapping it opens the contact`() {
        var opened = -1
        setContent(
            NotesInboxUiState(
                isLoading = false,
                notes = listOf(
                    Note(
                        id = 8,
                        content = "Met at the fair",
                        contactId = 5,
                        contact = ContactFlat(id = 5, firstname = "Ada", lastname = "Lovelace")
                    ),
                    Note(id = 9, content = "Loose thought")
                ),
                total = 2
            ),
            onContactClick = { opened = it }
        )

        composeTestRule.onNodeWithText("Ada Lovelace").performClick()
        assertEquals(5, opened)
        // The unfiled note has no contact chip.
        composeTestRule.onAllNodesWithText("#0").assertCountEquals(0)
    }

    @Test
    fun `the filter offers All notes and Unfiled and reports the choice`() {
        val chosen = mutableListOf<NotesScope>()
        setContent(
            NotesInboxUiState(isLoading = false, notes = emptyList()),
            onScopeChange = { chosen += it }
        )

        composeTestRule.onNodeWithText("All notes").assertIsDisplayed()
        composeTestRule.onNodeWithText("Unfiled").performClick()
        assertEquals(listOf(NotesScope.UNFILED), chosen)
    }

    @Test
    fun `switching the filter on the real screen changes the list and the count`() {
        val repository = mockk<NoteRepository>()
        coEvery { repository.listNotes(NotesScope.ALL) } returns Result.success(
            UnfiledNotesPage(
                notes = listOf(
                    Note(
                        id = 1,
                        content = "Filed note",
                        contact = ContactFlat(id = 5, firstname = "Ada")
                    ),
                    Note(id = 2, content = "Loose note")
                ),
                nextCursor = null,
                total = 2
            )
        )
        coEvery { repository.listNotes(NotesScope.UNFILED) } returns Result.success(
            UnfiledNotesPage(
                notes = listOf(Note(id = 2, content = "Loose note")),
                nextCursor = null,
                total = 1
            )
        )
        val viewModel = NotesInboxViewModel(repository)

        composeTestRule.setContent {
            MycorrhizalTheme { NotesInboxScreen(onNoteClick = {}, viewModel = viewModel) }
        }
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Filed note").assertIsDisplayed()
        composeTestRule.onNodeWithText("2 total").assertIsDisplayed()

        composeTestRule.onNodeWithText("Unfiled").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Filed note").assertDoesNotExist()
        composeTestRule.onNodeWithText("Loose note").assertIsDisplayed()
        composeTestRule.onNodeWithText("1 total").assertIsDisplayed()
        composeTestRule.onNodeWithText("Unassigned notes").assertIsDisplayed()
    }

    // v1.6.0 parity with web's /notes page: a row shows its photos, badged when
    // private or secret.
    @Test
    fun `a note row shows its photos with the sensitivity badge`() {
        setContent(
            NotesInboxUiState(
                isLoading = false,
                notes = listOf(Note(id = 3, content = "Buy milk")),
                imagesByNote = mapOf(
                    3 to listOf(NoteImage(id = 9, noteId = 3, sensitivity = "private"))
                )
            )
        )

        composeTestRule.onNodeWithContentDescription("Note photo").assertIsDisplayed()
        composeTestRule.onNodeWithText("Private").assertIsDisplayed()
    }
}
