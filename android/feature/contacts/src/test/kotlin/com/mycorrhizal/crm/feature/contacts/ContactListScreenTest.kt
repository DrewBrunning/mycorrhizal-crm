package com.mycorrhizal.crm.feature.contacts

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToIndex
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.SearchActivityHit
import com.mycorrhizal.crm.model.network.SearchNoteHit
import com.mycorrhizal.crm.model.network.SearchResult
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ContactListScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        uiState: ContactListUiState,
        onContactClick: (Int) -> Unit = {},
        onSearchQueryChange: (String) -> Unit = {},
        onLoadMore: () -> Unit = {},
        onCircleFilterChange: (String?) -> Unit = {},
        onIncludeArchivedChange: (Boolean) -> Unit = {},
        onToggleSelection: (Int) -> Unit = {},
        onToggleSelectAll: () -> Unit = {},
        onRunBulkAction: (String, String?, String?) -> Unit = { _, _, _ -> },
        onMenuClick: (() -> Unit)? = {},
        darkTheme: Boolean = false,
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                ContactListScreenContent(
                    uiState = uiState,
                    onContactClick = onContactClick,
                    onSearchQueryChange = onSearchQueryChange,
                    onLoadMore = onLoadMore,
                    onCircleFilterChange = onCircleFilterChange,
                    onIncludeArchivedChange = onIncludeArchivedChange,
                    onToggleSelection = onToggleSelection,
                    onToggleSelectAll = onToggleSelectAll,
                    onRunBulkAction = onRunBulkAction,
                    onMenuClick = onMenuClick,
                )
            }
        }
    }

    private fun manyContacts(count: Int): List<ContactSummary> =
        (1..count).map { ContactSummary(id = it, fn = "Contact $it", firstname = "Contact $it") }

    @Test
    fun `shows skeleton while loading`() {
        setContent(ContactListUiState(isLoading = true))
        composeTestRule.onNodeWithTag("contact-list-loading").assertIsDisplayed()
    }

    @Test
    fun `shows empty state when no contacts`() {
        setContent(ContactListUiState(isLoading = false, contacts = emptyList()))
        composeTestRule.onNodeWithText("No contacts yet").assertIsDisplayed()
    }

    @Test
    fun `shows contact list items`() {
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = listOf(
                    ContactSummary(id = 1, fn = "Alice", firstname = "Alice"),
                    ContactSummary(id = 2, fn = "Bob", firstname = "Bob"),
                ),
            ),
        )
        composeTestRule.onNodeWithText("Alice").assertIsDisplayed()
        composeTestRule.onNodeWithText("Bob").assertIsDisplayed()
    }

    @Test
    fun `tapping a contact navigates to its detail`() {
        var navigatedId: Int? = null
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = listOf(ContactSummary(id = 1, fn = "Alice", firstname = "Alice")),
            ),
            onContactClick = { navigatedId = it },
        )
        composeTestRule.onNodeWithText("Alice").performClick()
        assertEquals(1, navigatedId)
    }

    @Test
    fun `typing in the search field forwards the query`() {
        var query: String? = null
        setContent(
            ContactListUiState(isLoading = false, contacts = emptyList()),
            onSearchQueryChange = { query = it },
        )
        composeTestRule.onNodeWithText("Search contacts").performTextInput("ali")
        assertEquals("ali", query)
    }

    @Test
    fun `no cross-entity section renders when searchResult is null`() {
        setContent(
            ContactListUiState(isLoading = false, contacts = emptyList(), searchResult = null),
        )
        composeTestRule.onNodeWithText("matches in notes and activities", substring = true).assertDoesNotExist()
    }

    @Test
    fun `no cross-entity section renders for an empty result with no resolved relation`() {
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = emptyList(),
                searchResult = SearchResult(),
            ),
        )
        composeTestRule.onNodeWithText("matches in notes and activities", substring = true).assertDoesNotExist()
    }

    @Test
    fun `cross-entity header shows the total count and stays collapsed until tapped`() {
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = emptyList(),
                searchResult = SearchResult(
                    notes = listOf(SearchNoteHit(id = 1, content = "called mom", contactId = 5, contactName = "Dana")),
                    activities = listOf(SearchActivityHit(id = 2, title = "Coffee with mom")),
                ),
            ),
        )

        composeTestRule.onNodeWithText("2 matches in notes and activities").assertIsDisplayed()
        // Collapsed by default: the note/activity content itself isn't shown yet.
        composeTestRule.onNodeWithText("called mom").assertDoesNotExist()
        composeTestRule.onNodeWithText("Coffee with mom").assertDoesNotExist()

        composeTestRule.onNodeWithText("2 matches in notes and activities").performClick()

        composeTestRule.onNodeWithText("called mom").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Coffee with mom").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `a note with a contact shows its chip and navigates on tap`() {
        var navigatedId: Int? = null
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = emptyList(),
                searchResult = SearchResult(
                    notes = listOf(SearchNoteHit(id = 1, content = "called Dana", contactId = 5, contactName = "Dana White")),
                ),
            ),
            onContactClick = { navigatedId = it },
        )

        composeTestRule.onNodeWithText("1 matches in notes and activities").performClick()
        composeTestRule.onNodeWithText("Dana White").performScrollTo().performClick()

        assertEquals(5, navigatedId)
    }

    @Test
    fun `an unfiled note shows the unfiled chip instead of a contact`() {
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = emptyList(),
                searchResult = SearchResult(
                    notes = listOf(SearchNoteHit(id = 1, content = "a stray note", contactId = null, contactName = null)),
                ),
            ),
        )

        composeTestRule.onNodeWithText("1 matches in notes and activities").performClick()
        composeTestRule.onNodeWithText("Unfiled").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `a resolved relation banner shows even with zero hits`() {
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = emptyList(),
                searchResult = SearchResult(resolvedRelation = "parent_of"),
            ),
        )
        composeTestRule.onNodeWithText("Matched relationship: parent_of")
            .performScrollTo()
            .assertIsDisplayed()
    }

    // M9 item 3: ContactListViewModel.loadNextPage() was implemented and unit-tested but had
    // no call site — scrolling near the end of a paginated list must trigger it.
    //
    // This composes EMPTY first and only then delivers the page, because that is the real app's
    // order (the ViewModel's initial state is always ContactListUiState() with no contacts).
    // An already-populated first composition passes even when the scroll trigger captures a
    // stale uiState and is dead on a real device — which is exactly the bug this shape caught.
    @Test
    fun `scrolling near the end loads the next page when contacts arrive after first composition`() {
        var loadMoreCalls = 0
        var state by mutableStateOf(ContactListUiState(isLoading = true))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactListScreenContent(
                    uiState = state,
                    onSearchQueryChange = {},
                    onContactClick = {},
                    onLoadMore = { loadMoreCalls++ },
                )
            }
        }

        state = ContactListUiState(
            isLoading = false,
            contacts = manyContacts(30),
            pagination = PaginationState(nextCursor = "cursor-2", hasMore = true),
        )
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithTag("contact-list").performScrollToIndex(29)
        composeTestRule.waitForIdle()

        assertTrue("expected loadNextPage to be wired to scroll-near-end", loadMoreCalls > 0)
    }

    @Test
    fun `scrolling to the end does not load more once the last page is reached`() {
        var loadMoreCalls = 0
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = manyContacts(30),
                pagination = PaginationState(nextCursor = null, hasMore = false),
            ),
            onLoadMore = { loadMoreCalls++ },
        )

        composeTestRule.onNodeWithTag("contact-list").performScrollToIndex(29)
        composeTestRule.waitForIdle()

        assertEquals(0, loadMoreCalls)
    }

    // --- M23: circle filter + archived toggle ---------------------------------

    @Test
    fun `circle filter dropdown lists the loaded circles and selecting one filters`() {
        var filter: String? = "unset"
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = listOf(ContactSummary(id = 1, fn = "Alice", firstname = "Alice")),
                circles = listOf(Circle(id = "c-1", name = "Book club"), Circle(id = "c-2", name = "Family")),
            ),
            onCircleFilterChange = { filter = it },
        )

        composeTestRule.onNodeWithTag("circle-filter").performClick()
        composeTestRule.onNodeWithText("Book club").performClick()

        assertEquals("Book club", filter)
    }

    @Test
    fun `circle filter shows all circles by default`() {
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = emptyList(),
                circles = listOf(Circle(id = "c-1", name = "Book club")),
            ),
        )

        composeTestRule.onNodeWithText("All circles").assertIsDisplayed()
    }

    @Test
    fun `archived toggle fires the callback`() {
        var toggled: Boolean? = null
        setContent(
            ContactListUiState(isLoading = false, contacts = emptyList()),
            onIncludeArchivedChange = { toggled = it },
        )

        composeTestRule.onNodeWithTag("archived-toggle").performClick()

        assertEquals(true, toggled)
    }

    // --- M23: inline bulk selection ------------------------------------------

    @Test
    fun `entering select mode shows the selection title and row taps toggle`() {
        var toggledId: Int? = null
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = listOf(ContactSummary(id = 1, uid = "u1", fn = "Alice", firstname = "Alice")),
            ),
            onToggleSelection = { toggledId = it },
        )

        composeTestRule.onNodeWithTag("enter-select-mode").performClick()
        composeTestRule.onNodeWithText("0 selected").assertIsDisplayed()
        composeTestRule.onNodeWithText("Alice").performClick()

        assertEquals(1, toggledId)
    }

    @Test
    fun `select mode announces the row's checked state, not just its label`() {
        // The row's own checked/selected state must reach accessibility
        // services (Role.Checkbox + ToggleableState), not just its Checkbox
        // glyph — a decorative Checkbox (onCheckedChange = null) contributes
        // no semantics of its own, so the row itself must carry it.
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = listOf(
                    ContactSummary(id = 1, uid = "u1", fn = "Alice", firstname = "Alice"),
                    ContactSummary(id = 2, uid = "u2", fn = "Bob", firstname = "Bob"),
                ),
                selected = setOf(1),
            ),
        )

        composeTestRule.onNodeWithTag("enter-select-mode").performClick()

        composeTestRule.onNodeWithText("Alice").assertIsOn()
        composeTestRule.onNodeWithText("Bob").assertIsOff()
    }

    @Test
    fun `select all in select mode fires the callback`() {
        var selectAllCalls = 0
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = listOf(ContactSummary(id = 1, fn = "Alice", firstname = "Alice")),
            ),
            onToggleSelectAll = { selectAllCalls++ },
        )

        composeTestRule.onNodeWithTag("enter-select-mode").performClick()
        composeTestRule.onNodeWithTag("select-all").performClick()

        assertEquals(1, selectAllCalls)
    }

    @Test
    fun `a selected row in select mode runs a bulk action after confirming`() {
        var ran: Triple<String, String?, String?>? = null
        setContent(
            ContactListUiState(
                isLoading = false,
                contacts = listOf(ContactSummary(id = 1, uid = "u1", fn = "Alice", firstname = "Alice")),
                selected = setOf(1),
            ),
            onRunBulkAction = { action, circleId, tagId -> ran = Triple(action, circleId, tagId) },
        )

        composeTestRule.onNodeWithTag("enter-select-mode").performClick()
        composeTestRule.onNodeWithTag("bulk-archive").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Confirm bulk action?").assertIsDisplayed()
        composeTestRule.onNodeWithText("Confirm").performClick()

        assertEquals("archive", ran?.first)
        assertEquals(null, ran?.second)
        assertEquals(null, ran?.third)
    }

    // --- Issue #150: nullable onMenuClick hides the hamburger -----------------

    @Test
    fun `shows the menu button when a drawer opener is provided`() {
        setContent(ContactListUiState(isLoading = false, contacts = emptyList()), onMenuClick = {})

        composeTestRule.onNodeWithContentDescription("Menu").assertIsDisplayed()
    }

    @Test
    fun `hides the menu button when there is no drawer to open`() {
        setContent(ContactListUiState(isLoading = false, contacts = emptyList()), onMenuClick = null)

        composeTestRule.onNodeWithContentDescription("Menu").assertDoesNotExist()
    }

    // --- Issue #214: Compose semantics a11y sweep (the axe-core analog) -----

    private fun populatedListState() = ContactListUiState(
        isLoading = false,
        contacts = listOf(
            ContactSummary(id = 1, uid = "u1", fn = "Alice Johnson", firstname = "Alice", primaryEmail = "alice@example.com"),
            ContactSummary(id = 2, uid = "u2", fn = "Bob Smith", firstname = "Bob", primaryPhone = "+15551230000"),
        ),
    )

    @Test
    fun `contact list has no accessibility violations (light)`() {
        setContent(populatedListState(), darkTheme = false)

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `contact list in multi-select mode has no accessibility violations`() {
        // Multi-select mode is where the row's long-press action and the
        // per-row Checkbox actually render — exercise it, not just the
        // default list state.
        setContent(populatedListState(), darkTheme = false)
        composeTestRule.onNodeWithTag("enter-select-mode").performClick()

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `contact list has no accessibility violations (dark)`() {
        setContent(populatedListState(), darkTheme = true)

        composeTestRule.assertAccessibleSemantics()
    }
}
