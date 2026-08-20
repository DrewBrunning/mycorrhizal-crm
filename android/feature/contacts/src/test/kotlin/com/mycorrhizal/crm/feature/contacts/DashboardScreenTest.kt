package com.mycorrhizal.crm.feature.contacts

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
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
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.model.network.Birthday
import com.mycorrhizal.crm.model.network.CadenceHealth
import com.mycorrhizal.crm.model.network.CadencePolicy
import com.mycorrhizal.crm.model.network.DashboardRandomContact
import com.mycorrhizal.crm.model.network.DashboardReminder
import com.mycorrhizal.crm.model.network.DashboardResponse
import com.mycorrhizal.crm.model.network.OverdueCadence
import com.mycorrhizal.crm.model.network.ReachOutSuggestion
import com.mycorrhizal.crm.model.util.DateFormat
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.every
import io.mockk.mockk
import java.time.LocalDate
import kotlinx.coroutines.flow.flowOf
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
        // Issue #212: the favorites quick-access block, rendered first.
        favorites = listOf(DashboardRandomContact(id = 9, firstname = "Zebra", lastname = "Smith", nickname = "Z")),
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
        randomContacts = listOf(DashboardRandomContact(id = 4, firstname = "Rex", lastname = "Jones", nickname = "Rex")),
        overdueCadences = listOf(
            OverdueCadence(
                policy = CadencePolicy(id = "c1", entityId = "u3"),
                health = CadenceHealth(overdueBy = 3, nextDue = "2026-08-01T00:00:00Z"),
                contactId = 3L,
                contactName = "Carol Davis",
            ),
        ),
        // Issue #177: pending event-driven reach-out suggestions.
        reachOutSuggestions = listOf(
            ReachOutSuggestion(
                id = "s1", kind = "organization", oldValue = "OldCo", newValue = "NewCo",
                contactId = 5L, contactName = "Dana Prince",
            ),
        ),
    )

    private fun setContent(
        state: DashboardUiState,
        dateFormat: String = DateFormat.EU,
        onOpenContact: (Int) -> Unit = {},
        onCompleteReminder: (id: Int, skip: Boolean) -> Unit = { _, _ -> },
        onDismissReachOutSuggestion: (id: String) -> Unit = {},
        darkTheme: Boolean = false,
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                DashboardContent(
                    state = state,
                    dateFormat = dateFormat,
                    onOpenContact = onOpenContact,
                    onCompleteReminder = onCompleteReminder,
                    onDismissReachOutSuggestion = onDismissReachOutSuggestion,
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
    fun `a populated dashboard renders all widget sections`() {
        setContent(populatedState())

        // Favorites (issue #212): first section, nickname-preferred name.
        composeTestRule.onNodeWithText("Favorites").assertIsDisplayed()
        composeTestRule.onNodeWithText("Z Smith").assertIsDisplayed()

        // Overdue cadences.
        scrollTo("Overdue Relationships")
        composeTestRule.onNodeWithText("Carol Davis").assertIsDisplayed()
        composeTestRule.onNodeWithText("3 days overdue").assertIsDisplayed()

        // Reach-out suggestions (issue #177).
        scrollTo("Reasons to Reach Out")
        composeTestRule.onNodeWithText("Dana Prince").assertIsDisplayed()
        composeTestRule.onNodeWithText("OldCo → NewCo").assertIsDisplayed()
        composeTestRule.onNodeWithText("New employer").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Dismiss suggestion").assertIsDisplayed()

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
    fun `a favorite that also shows up in random contacts does not crash on a duplicate lazy key`() {
        // All five widget sections share one LazyColumn (DashboardContent),
        // so a key must be unique across sections, not just within one. A
        // contact can legitimately be both favorited and drawn into the
        // random "stay in touch" pick -- if both sections key by the raw
        // contact id, Compose throws "Key \"9\" was already used" the moment
        // that happens, which crashed the app on login in practice.
        val state = populatedState().copy(
            favorites = listOf(DashboardRandomContact(id = 9, firstname = "Zebra", lastname = "Smith", nickname = "Z")),
            randomContacts = listOf(DashboardRandomContact(id = 9, firstname = "Zebra", lastname = "Smith", nickname = "Z")),
        )

        setContent(state)

        composeTestRule.onNodeWithText("Z Smith").assertIsDisplayed()
    }

    @Test
    fun `empty widgets show their empty text and the overdue section stays hidden`() {
        setContent(DashboardUiState())

        composeTestRule.onNodeWithText("No favorites yet").assertIsDisplayed()
        composeTestRule.onNodeWithText("No upcoming birthdays").assertIsDisplayed()
        composeTestRule.onNodeWithText("No upcoming reminders").assertIsDisplayed()
        composeTestRule.onNodeWithText("No contacts available").assertIsDisplayed()
        // Web parity: an all-clear dashboard hides the overdue section entirely.
        composeTestRule.onNodeWithText("Overdue Relationships").assertDoesNotExist()
        // Same treatment for reach-out suggestions (issue #177).
        composeTestRule.onNodeWithText("Reasons to Reach Out").assertDoesNotExist()
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
    fun `tapping a favorites card opens the contact`() {
        var opened: Int? = null
        setContent(populatedState(), onOpenContact = { opened = it })

        composeTestRule.onNodeWithText("Z Smith").performClick()

        assertEquals(9, opened)
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
    fun `tapping a reach-out suggestion card opens the contact`() {
        var opened: Int? = null
        setContent(populatedState(), onOpenContact = { opened = it })

        scrollTo("Reasons to Reach Out")
        composeTestRule.onNodeWithText("Dana Prince").performClick()

        assertEquals(5, opened)
    }

    @Test
    fun `dismissing a reach-out suggestion calls the callback and does not navigate`() {
        var opened: Int? = null
        var dismissed: String? = null
        setContent(populatedState(), onOpenContact = { opened = it }, onDismissReachOutSuggestion = { dismissed = it })

        scrollToContentDescription("Dismiss suggestion")
        composeTestRule.onNodeWithContentDescription("Dismiss suggestion").performClick()

        assertEquals("s1", dismissed)
        assertEquals(null, opened)
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

    // --- Issue #214: Compose semantics a11y sweep (the axe-core analog) -----
    //
    // Deliberately mounts the real DashboardScreen (Scaffold + TopAppBar +
    // hamburger included) via a mocked-repository DashboardViewModel — the
    // same construction DashboardViewModelTest uses — rather than
    // DashboardContent above: the sweep needs the whole top-level screen,
    // chrome included, and it's the chrome (the menu IconButton) that
    // surfaced this ticket's touch-target finding.

    private fun fullDashboard() = populatedState().let { state ->
        DashboardResponse(
            birthdays = state.birthdays,
            randomContacts = state.randomContacts,
            upcomingReminders = state.upcomingReminders,
            overdue = state.overdueCadences,
            favorites = state.favorites,
        )
    }

    private fun setScreenContent(darkTheme: Boolean) {
        val apiClient = mockk<ApiClient>()
        val authRepository = mockk<AuthRepository>()
        every { authRepository.observeSession() } returns flowOf(SessionState())
        coEvery { apiClient.getDashboard() } returns Result.success(fullDashboard())
        val viewModel = DashboardViewModel(apiClient, authRepository)

        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                DashboardScreen(onOpenContact = {}, onMenuClick = {}, viewModel = viewModel)
            }
        }
    }

    @Test
    fun `dashboard has no accessibility violations (light)`() {
        setScreenContent(darkTheme = false)

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `dashboard has no accessibility violations (dark)`() {
        setScreenContent(darkTheme = true)

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `section headers are marked as headings`() {
        setContent(populatedState())

        // #208: section titles carried no heading semantics, so TalkBack's
        // heading navigation found nothing on the dashboard.
        scrollTo("Upcoming birthdays")
        composeTestRule.onNodeWithText("Upcoming birthdays")
            .assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading))
        scrollTo("Overdue Relationships")
        composeTestRule.onNodeWithText("Overdue Relationships")
            .assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading))
        scrollTo("Reasons to Reach Out")
        composeTestRule.onNodeWithText("Reasons to Reach Out")
            .assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading))
    }
}
