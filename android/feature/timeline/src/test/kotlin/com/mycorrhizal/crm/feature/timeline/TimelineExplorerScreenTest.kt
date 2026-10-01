package com.mycorrhizal.crm.feature.timeline

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ExternalActivity
import com.mycorrhizal.crm.model.network.Gift
import com.mycorrhizal.crm.model.network.GiftStatuses
import com.mycorrhizal.crm.model.network.LifeEvent
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.ReminderCompletion
import com.mycorrhizal.crm.model.network.TimelineBuckets
import com.mycorrhizal.crm.model.network.TimelineTypes
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// Issue #1401: the timeline explorer's lazy rows, filters, Load more and row actions, driving the
// real TimelineExplorerContent.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class TimelineExplorerScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun show(
        state: TimelineExplorerUiState,
        onEditActivity: (Int) -> Unit = {},
        onEditNote: (Int) -> Unit = {},
        onTypesChange: (Set<String>) -> Unit = {},
        onBucketChange: (String) -> Unit = {},
        onLoadMore: () -> Unit = {},
        onRetry: () -> Unit = {},
        onUndoCompletion: (Int) -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                TimelineExplorerContent(
                    state = state,
                    onEditActivity = onEditActivity,
                    onEditNote = onEditNote,
                    onTypesChange = onTypesChange,
                    onBucketChange = onBucketChange,
                    onLoadMore = onLoadMore,
                    onRetry = onRetry,
                    onUndoCompletion = onUndoCompletion,
                )
            }
        }
    }

    private fun notes(count: Int) = (1..count).map { TimelineItem.NoteItem(Note(id = it, content = "note $it")) }

    @Test
    fun `rows are composed lazily - a large page does not compose every event`() {
        show(TimelineExplorerUiState(contactId = 5, items = notes(200)))

        composeTestRule.onNodeWithText("note 1").assertIsDisplayed()
        val composed = composeTestRule.onAllNodesWithText("note ", substring = true).fetchSemanticsNodes().size
        assertTrue("expected a lazily-composed window, got $composed of 200 rows", composed in 1..60)
    }

    @Test
    fun `load more shows when there is a next cursor and fires the callback`() {
        var loads = 0
        show(
            TimelineExplorerUiState(contactId = 5, items = notes(2), nextCursor = "cur-1"),
            onLoadMore = { loads++ },
        )

        composeTestRule.onNodeWithText("Load more").assertIsDisplayed().performClick()

        assertEquals(1, loads)
    }

    @Test
    fun `load more is absent on the last page`() {
        show(TimelineExplorerUiState(contactId = 5, items = notes(2), nextCursor = null))

        composeTestRule.onNodeWithText("Load more").assertDoesNotExist()
    }

    @Test
    fun `load more is disabled while the next page is loading`() {
        show(TimelineExplorerUiState(contactId = 5, items = notes(2), nextCursor = "cur-1", isLoadingMore = true))

        composeTestRule.onNodeWithText("Load more").assertIsNotEnabled()
    }

    @Test
    fun `selecting a type chip reports the selection and deselecting removes it`() {
        var selection: Set<String>? = null
        show(
            TimelineExplorerUiState(contactId = 5, items = notes(1), types = setOf(TimelineTypes.NOTE)),
            onTypesChange = { selection = it },
        )

        composeTestRule.onNodeWithText("Activity").performClick()
        assertEquals(setOf(TimelineTypes.NOTE, TimelineTypes.ACTIVITY), selection)

        composeTestRule.onNodeWithText("Note").performClick()
        assertEquals(emptySet<String>(), selection)
    }

    @Test
    fun `the all types chip clears the type filter`() {
        var selection: Set<String>? = null
        show(
            TimelineExplorerUiState(contactId = 5, items = notes(1), types = setOf(TimelineTypes.GIFT)),
            onTypesChange = { selection = it },
        )

        composeTestRule.onNodeWithText("All types").performClick()

        assertEquals(emptySet<String>(), selection)
    }

    @Test
    fun `picking a recency chip reports the bucket`() {
        var bucket: String? = null
        show(TimelineExplorerUiState(contactId = 5, items = notes(1)), onBucketChange = { bucket = it })

        composeTestRule.onNodeWithText("Last 7 days").performClick()

        assertEquals(TimelineBuckets.LAST_7_DAYS, bucket)
    }

    @Test
    fun `an empty timeline shows the empty message`() {
        show(TimelineExplorerUiState(contactId = 5))

        composeTestRule.onNodeWithText("No timeline events").assertIsDisplayed()
    }

    @Test
    fun `a failed first load shows the error with a retry that refetches`() {
        var retries = 0
        show(TimelineExplorerUiState(contactId = 5, error = "Unexpected server response"), onRetry = { retries++ })

        composeTestRule.onNodeWithText("Unexpected server response").assertIsDisplayed()
        composeTestRule.onNodeWithText("Retry").assertIsEnabled().performClick()

        assertEquals(1, retries)
    }

    @Test
    fun `tapping a note or activity row routes to its edit form`() {
        var editedNote: Int? = null
        var editedActivity: Int? = null
        show(
            TimelineExplorerUiState(
                contactId = 5,
                items = listOf(
                    TimelineItem.NoteItem(Note(id = 4, content = "Loves climbing")),
                    TimelineItem.ActivityItem(Activity(id = 8, title = "Coffee with Dana")),
                ),
            ),
            onEditNote = { editedNote = it },
            onEditActivity = { editedActivity = it },
        )

        composeTestRule.onNodeWithText("Loves climbing").performClick()
        composeTestRule.onNodeWithText("Coffee with Dana").performClick()

        assertEquals(4, editedNote)
        assertEquals(8, editedActivity)
    }

    @Test
    fun `undoing a completion confirms first and then reports the id`() {
        var undone: Int? = null
        show(
            TimelineExplorerUiState(
                contactId = 5,
                items = listOf(TimelineItem.CompletionItem(ReminderCompletion(id = 9, contactId = 5, message = "Call Dana"))),
            ),
            onUndoCompletion = { undone = it },
        )

        composeTestRule.onNodeWithContentDescription("Undo completion").performClick()
        assertEquals(null, undone)
        composeTestRule.onNodeWithText("Undo completed reminder?").assertIsDisplayed()
        composeTestRule.onNodeWithText("Undo").performClick()

        assertEquals(9, undone)
    }

    @Test
    fun `life events external activities and gifts render in the explorer`() {
        show(
            TimelineExplorerUiState(
                contactId = 5,
                items = listOf(
                    TimelineItem.LifeEventItem(LifeEvent(id = "le1", type = "moved", description = "Moved to Oslo"), "2026-08-20T00:00:00Z"),
                    TimelineItem.ExternalActivityItem(
                        ExternalActivity(id = "ea1", sourceSystem = "immich", type = "photo-appearance", payload = mapOf("person_name" to "Alice")),
                    ),
                    TimelineItem.GiftItem(Gift(id = "g1", description = "A book", status = GiftStatuses.GIVEN), "2026-08-01T00:00:00Z"),
                ),
            ),
        )

        composeTestRule.onNodeWithText("Moved to Oslo").assertIsDisplayed()
        composeTestRule.onNodeWithText("Photo appearance").assertIsDisplayed()
        composeTestRule.onNodeWithText("A book").assertIsDisplayed()
        composeTestRule.onNodeWithText("Given").assertIsDisplayed()
    }
}
