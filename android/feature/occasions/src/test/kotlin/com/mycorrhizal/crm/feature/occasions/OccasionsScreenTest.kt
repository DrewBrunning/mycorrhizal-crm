package com.mycorrhizal.crm.feature.occasions

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertHeightIsAtLeast
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertWidthIsAtLeast
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.domain.repository.OccasionEventRepository
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.model.network.OccasionEvent
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
import java.time.Instant
import java.util.Locale
import java.util.TimeZone
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

private const val OCCASIONS_DESCRIPTION =
    "Occasions are the standing things you do for people, like a yearly card or gift, " +
        "and one-off events with an invitee list. Active obligations create reminders " +
        "automatically; reminders are the nudge, not a second occasion. To log something " +
        "that already happened, use Activities."

private const val EVENT_DIALOG_HINT =
    "A one-off event you are planning. Add invitees after saving, then record what each " +
        "person told you."

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class OccasionsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val repository = mockk<OccasionEventRepository>()
    private val originalZone = TimeZone.getDefault()
    private val originalLocale = Locale.getDefault()

    @Before
    fun pinZoneAndLocale() {
        TimeZone.setDefault(TimeZone.getTimeZone("UTC"))
        Locale.setDefault(Locale.UK)
    }

    @After
    fun restoreZoneAndLocale() {
        TimeZone.setDefault(originalZone)
        Locale.setDefault(originalLocale)
    }

    private fun event(id: String = "e1", title: String = "Summer BBQ") = OccasionEvent(
        id = id,
        title = title,
        startsAt = "2026-07-04T15:00:00Z",
        location = "The park"
    )

    private fun render(events: List<OccasionEvent>) {
        coEvery { repository.list() } returns Result.success(events)
        val viewModel = OccasionsViewModel(repository)
        composeTestRule.setContent {
            MycorrhizalTheme {
                OccasionsScreen(onMenuClick = {}, onOpenAttendees = {}, viewModel = viewModel)
            }
        }
        composeTestRule.waitForIdle()
    }

    @Test
    fun `renders the list with the formatted start`() {
        render(listOf(event()))
        composeTestRule.onNodeWithText("Occasions").assertIsDisplayed()
        composeTestRule.onNodeWithText("Summer BBQ").assertIsDisplayed()
        composeTestRule.onNodeWithText("04.07.2026 15:00").assertIsDisplayed()
        composeTestRule.onNodeWithText("The park").assertIsDisplayed()
    }

    @Test
    fun `a failed load shows the server message with Retry that reloads the events`() {
        coEvery { repository.list() } returnsMany listOf(
            Result.failure(ApiError.Network(java.io.IOException("offline"))),
            Result.success(listOf(event()))
        )
        val viewModel = OccasionsViewModel(repository)
        composeTestRule.setContent {
            MycorrhizalTheme {
                OccasionsScreen(onMenuClick = {}, onOpenAttendees = {}, viewModel = viewModel)
            }
        }
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText(
            "Can't reach your server. Check your connection and try again."
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText("No events yet").assertDoesNotExist()

        composeTestRule.onNodeWithText("Retry").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Summer BBQ").assertIsDisplayed()
        composeTestRule.onNodeWithText("Retry").assertDoesNotExist()
    }

    @Test
    fun `shows the empty state when there are no events`() {
        render(emptyList())
        composeTestRule
            .onNodeWithText("No events yet")
            .assertIsDisplayed()
    }

    @Test
    fun `shows the occasions explanation with events`() {
        render(listOf(event()))
        composeTestRule.onNodeWithText(OCCASIONS_DESCRIPTION).assertIsDisplayed()
    }

    @Test
    fun `shows the occasions explanation with the empty state`() {
        render(emptyList())
        composeTestRule.onNodeWithText(OCCASIONS_DESCRIPTION).assertIsDisplayed()
    }

    @Test
    fun `the create dialog explains that an event is a one-off plan`() {
        render(emptyList())
        composeTestRule.onNodeWithContentDescription("New event").performClick()
        composeTestRule.onNodeWithText(EVENT_DIALOG_HINT).assertIsDisplayed()
    }

    // Issue #1760: the empty state carries the same create action as the FAB.
    @Test
    fun `the empty state action opens the create dialog and explains events`() {
        render(emptyList())
        composeTestRule.onNodeWithText(
            "Create one to plan an event, invite people, and track RSVPs."
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText("New event").performClick()
        composeTestRule.onNodeWithText("Create an event").assertIsDisplayed()
    }

    @Test
    fun `a failed load is reported in the snackbar`() {
        coEvery { repository.list() } returns Result.failure(ApiError.Client(500, "boom"))
        val viewModel = OccasionsViewModel(repository)
        composeTestRule.setContent {
            MycorrhizalTheme {
                OccasionsScreen(onMenuClick = {}, onOpenAttendees = {}, viewModel = viewModel)
            }
        }
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("boom").assertIsDisplayed()
    }

    @Test
    fun `the add button opens the create dialog`() {
        render(emptyList())
        composeTestRule.onNodeWithContentDescription("New event").performClick()
        composeTestRule.onNodeWithText("Create an event").assertIsDisplayed()
    }

    @Test
    fun `the edit action opens a prefilled dialog`() {
        render(listOf(event()))
        composeTestRule.onNodeWithContentDescription("Edit Summer BBQ").performClick()
        composeTestRule.onNodeWithText("Edit event").assertIsDisplayed()
        // The row and the prefilled title field both carry the title.
        composeTestRule.onAllNodesWithText("Summer BBQ").assertCountEquals(2)
    }

    @Test
    fun `the start field opens the date picker and then the time picker`() {
        render(emptyList())
        composeTestRule.onNodeWithContentDescription("New event").performClick()
        composeTestRule.onNodeWithContentDescription("Starts").performClick()
        composeTestRule.onNodeWithText("Confirm").performClick()
        composeTestRule.onNodeWithText("Time").assertIsDisplayed()
    }

    @Test
    fun `each event row names its event in the edit and delete actions`() {
        render(listOf(event("e1", "Summer BBQ"), event("e2", "Winter party")))
        listOf("Summer BBQ", "Winter party").forEach { title ->
            composeTestRule.onNodeWithContentDescription("Edit $title")
                .assertIsDisplayed().assertHeightIsAtLeast(48.dp).assertWidthIsAtLeast(48.dp)
            composeTestRule.onNodeWithContentDescription("Delete $title")
                .assertIsDisplayed().assertHeightIsAtLeast(48.dp).assertWidthIsAtLeast(48.dp)
        }
        composeTestRule.onAllNodesWithContentDescription("Edit").assertCountEquals(0)
        composeTestRule.onAllNodesWithContentDescription("Delete").assertCountEquals(0)
    }

    @Test
    fun `the delete action opens a confirmation`() {
        render(listOf(event()))
        composeTestRule.onNodeWithContentDescription("Delete Summer BBQ").performClick()
        composeTestRule.onNodeWithText("Delete event").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Delete “Summer BBQ”? This cannot be undone."
        ).assertIsDisplayed()
    }

    // ADR 0024 parts 4-5 (issue #2521): the host's derived-lists slot scrolls with the events,
    // both with events and when there are none.
    private fun renderWithSlot(events: List<OccasionEvent>) {
        coEvery { repository.list() } returns Result.success(events)
        val viewModel = OccasionsViewModel(repository)
        composeTestRule.setContent {
            MycorrhizalTheme {
                OccasionsScreen(
                    onMenuClick = {},
                    onOpenAttendees = {},
                    derivedLists = { androidx.compose.material3.Text("Slot content") },
                    viewModel = viewModel
                )
            }
        }
        composeTestRule.waitForIdle()
    }

    @Test
    fun `the derived lists slot is shown above the events`() {
        renderWithSlot(listOf(event()))
        composeTestRule.onNodeWithText("Slot content").assertIsDisplayed()
        composeTestRule.onNodeWithText("Summer BBQ").assertIsDisplayed()
    }

    @Test
    fun `the derived lists slot is shown with the empty state`() {
        renderWithSlot(emptyList())
        composeTestRule.onNodeWithText("Slot content").assertIsDisplayed()
        composeTestRule.onNodeWithText("No events yet").assertIsDisplayed()
    }

    @Test
    fun `a failed load is worded from its reason code, never the server text`() {
        coEvery { repository.list() } returns Result.failure(
            ApiError.Client(
                409,
                "RAW SERVER TEXT",
                reason = ErrorReason(ErrorReasonCode.ENTITY_NOT_FOUND, emptyMap())
            )
        )
        val viewModel = OccasionsViewModel(repository)
        composeTestRule.setContent {
            MycorrhizalTheme {
                OccasionsScreen(onMenuClick = {}, onOpenAttendees = {}, viewModel = viewModel)
            }
        }
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Organisation, place or pet not found").assertIsDisplayed()
        composeTestRule.onNodeWithText("RAW SERVER TEXT").assertDoesNotExist()
    }

    private fun at(id: String, startsAt: String, endsAt: String? = null) =
        OccasionEvent(id = id, title = id, startsAt = startsAt, endsAt = endsAt)

    @Test
    fun `upcoming events come first soonest first, then past events most recent first`() {
        val now = Instant.parse("2026-11-13T12:00:00Z")
        val ordered = orderOccasionEvents(
            listOf(
                at("past-old", "2026-01-01T10:00:00Z"),
                at("later", "2027-03-01T10:00:00Z"),
                at("past-recent", "2026-11-01T10:00:00Z"),
                at("soon", "2026-11-20T10:00:00Z")
            ),
            now
        )
        assertEquals(listOf("soon", "later", "past-recent", "past-old"), ordered.map { it.id })
    }

    @Test
    fun `an event still running counts as upcoming and an unparseable start sorts last`() {
        val now = Instant.parse("2026-11-13T12:00:00Z")
        val ordered = orderOccasionEvents(
            listOf(
                at("garbage", "not a date"),
                at("finished", "2026-11-13T08:00:00Z", endsAt = "2026-11-13T11:00:00Z"),
                at("running", "2026-11-13T09:00:00Z", endsAt = "2026-11-13T18:00:00Z"),
                at("tomorrow", "2026-11-14T09:00:00Z")
            ),
            now
        )
        assertEquals(listOf("running", "tomorrow", "finished", "garbage"), ordered.map { it.id })
    }

    @Test
    fun `the list renders upcoming events before past ones regardless of server order`() {
        render(
            listOf(
                at("Past gala", "2001-05-01T10:00:00Z"),
                at("Far future", "2099-06-01T10:00:00Z"),
                at("Near future", "2099-01-01T10:00:00Z")
            )
        )
        fun top(title: String) = composeTestRule.onNodeWithText(title)
            .fetchSemanticsNode().boundsInRoot.top
        assertTrue(top("Near future") < top("Far future"))
        assertTrue(top("Far future") < top("Past gala"))
    }
}
