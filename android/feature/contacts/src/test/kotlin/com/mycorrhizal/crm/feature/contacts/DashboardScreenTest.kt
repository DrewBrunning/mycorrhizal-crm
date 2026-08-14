package com.mycorrhizal.crm.feature.contacts

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToNode
import com.mycorrhizal.crm.model.network.Birthday
import com.mycorrhizal.crm.model.network.CadenceHealth
import com.mycorrhizal.crm.model.network.CadencePolicy
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.DashboardReminder
import com.mycorrhizal.crm.model.network.OverdueCadence
import com.mycorrhizal.crm.model.util.DateFormat
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import java.time.LocalDate
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Renders the dashboard's four widget sections from a real [DashboardUiState]
 * against real composables. The ViewModel state machine and the wire parse
 * are pinned elsewhere (DashboardViewModelTest, ApiClientTest) — this pins
 * that the *screen* renders every widget, shows the right empty states,
 * navigates on card taps, and wires complete/skip (including the skip
 * confirmation dialog) to the callbacks.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class DashboardScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val listTag = "dashboard-list"

    private fun populatedState() = DashboardUiState(
        birthdays = listOf(
            Birthday(name = "Alice Wonder", birthday = "1990-12-25", contactId = 1L),
        ),
        upcomingReminders = listOf(
            DashboardReminder(
                id = 7,
                message = "Call Dana",
                remindAt = "2026-08-15T09:00:00Z",
                recurrence = "weekly",
                byMail = true,
                contactId = 3,
                contactName = "Bobby Smith",
            ),
        ),
        randomContacts = listOf(ContactSummary(id = 4, firstname = "Rex", lastname = "Jones", nickname = "Rex")),
        overdueCadences = listOf(
            OverdueCadence(
                policy = CadencePolicy(id = "c1", entityId = "u3"),
                health = CadenceHealth(overdueBy = 3, nextDue = "2026-08-01T00:00:00Z"),
                contactId = 3L,
                contactName = "Carol Davis",
            ),
        ),
    )

    private fun setContent(
        state: DashboardUiState,
        dateFormat: String = DateFormat.EU,
        onOpenContact: (Int) -> Unit = {},
        onCompleteReminder: (id: Int, skip: Boolean) -> Unit = { _, _ -> },
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                DashboardContent(
                    state = state,
                    dateFormat = dateFormat,
                    onOpenContact = onOpenContact,
                    onCompleteReminder = onCompleteReminder,
                )
            }
        }
    }

    /** Scrolls the lazy list until a node matching [text] is composed (off-screen items aren't). */
    private fun scrollTo(text: String) {
        composeTestRule.onNodeWithTag(listTag).performScrollToNode(hasText(text, substring = true))
    }

    /** Scrolls to a content-description node (for the icon-button actions). */
    private fun scrollToContentDescription(description: String) {
        composeTestRule.onNodeWithTag(listTag).performScrollToNode(hasContentDescription(description))
    }

    @Test
    fun `a populated dashboard renders all four widgets`() {
        setContent(populatedState())

        // Overdue cadences.
        scrollTo("Overdue Relationships")
        composeTestRule.onNodeWithText("Carol Davis").assertIsDisplayed()
        composeTestRule.onNodeWithText("3 days overdue").assertIsDisplayed()

        // Birthdays: age rendered from the full date, in the eu date format.
        scrollTo("Alice Wonder")
        val age = LocalDate.now().year - 1990
        composeTestRule.onNodeWithText("Alice Wonder").assertIsDisplayed()
        composeTestRule.onNodeWithText("25 December ($age years old)").assertIsDisplayed()

        // Upcoming reminders: message, embedded contact name, recurrence + by-mail chips.
        scrollTo("Call Dana")
        composeTestRule.onNodeWithText("Call Dana").assertIsDisplayed()
        composeTestRule.onNodeWithText("Bobby Smith").assertIsDisplayed()
        composeTestRule.onNodeWithText("Weekly").assertIsDisplayed()
        composeTestRule.onNodeWithText("Email").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Complete reminder").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Skip reminder").assertIsDisplayed()

        // Stay in touch: nickname-preferred name.
        scrollTo("Rex Jones")
        composeTestRule.onNodeWithText("Rex Jones").assertIsDisplayed()
    }

    @Test
    fun `empty widgets show their empty text and the overdue section stays hidden`() {
        setContent(DashboardUiState())

        composeTestRule.onNodeWithText("No upcoming birthdays").assertIsDisplayed()
        composeTestRule.onNodeWithText("No upcoming reminders").assertIsDisplayed()
        composeTestRule.onNodeWithText("No contacts available").assertIsDisplayed()
        // Web parity: an all-clear dashboard hides the overdue section entirely.
        composeTestRule.onNodeWithText("Overdue Relationships").assertDoesNotExist()
    }

    @Test
    fun `tapping a birthday card opens the contact`() {
        var opened: Int? = null
        setContent(populatedState(), onOpenContact = { opened = it })

        scrollTo("Alice Wonder")
        composeTestRule.onNodeWithText("Alice Wonder").performClick()

        assertEquals(1, opened)
    }

    @Test
    fun `tapping an overdue card opens the contact`() {
        var opened: Int? = null
        setContent(populatedState(), onOpenContact = { opened = it })

        scrollTo("Overdue Relationships")
        composeTestRule.onNodeWithText("Carol Davis").performClick()

        assertEquals(3, opened)
    }

    @Test
    fun `tapping a stay-in-touch card opens the contact`() {
        var opened: Int? = null
        setContent(populatedState(), onOpenContact = { opened = it })

        scrollTo("Rex Jones")
        composeTestRule.onNodeWithText("Rex Jones").performClick()

        assertEquals(4, opened)
    }

    @Test
    fun `tapping a reminder card opens the contact`() {
        var opened: Int? = null
        setContent(populatedState(), onOpenContact = { opened = it })

        scrollTo("Call Dana")
        composeTestRule.onNodeWithText("Call Dana").performClick()

        assertEquals(3, opened)
    }

    @Test
    fun `completing a reminder calls complete without skip`() {
        var completed: Pair<Int, Boolean>? = null
        setContent(populatedState(), onCompleteReminder = { id, skip -> completed = id to skip })

        scrollToContentDescription("Complete reminder")
        composeTestRule.onNodeWithContentDescription("Complete reminder").performClick()

        assertEquals(7 to false, completed)
    }

    @Test
    fun `skipping a reminder confirms before skipping`() {
        var completed: Pair<Int, Boolean>? = null
        setContent(populatedState(), onCompleteReminder = { id, skip -> completed = id to skip })

        scrollToContentDescription("Skip reminder")
        composeTestRule.onNodeWithContentDescription("Skip reminder").performClick()

        // The confirmation dialog is up; nothing has been called yet.
        composeTestRule.onNodeWithText("Skip this reminder?", substring = true).assertIsDisplayed()
        assertEquals(null, completed)

        composeTestRule.onNodeWithText("Skip").performClick()

        assertEquals(7 to true, completed)
    }

    @Test
    fun `cancelling the skip dialog does not skip`() {
        var completed = false
        setContent(populatedState(), onCompleteReminder = { _, _ -> completed = true })

        scrollToContentDescription("Skip reminder")
        composeTestRule.onNodeWithContentDescription("Skip reminder").performClick()

        composeTestRule.onNodeWithText("Cancel").performClick()

        assertFalse(completed)
    }
}
