package com.mycorrhizal.crm.feature.contacts

import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.hasTestTag
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
import com.mycorrhizal.crm.model.network.DataDecayHealth
import com.mycorrhizal.crm.model.network.DataDecayPolicy
import com.mycorrhizal.crm.model.network.OverdueCadence
import com.mycorrhizal.crm.model.network.OverdueDataDecayPolicy
import com.mycorrhizal.crm.model.network.ReachOutKind
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
import org.junit.Assert.assertTrue
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
        // Issue #352: contacts whose info is due for re-verification.
        dataDecayOverdue = listOf(
            OverdueDataDecayPolicy(
                policy = DataDecayPolicy(id = "d1", entityId = "u6"),
                health = DataDecayHealth(overdueBy = 40, nextDue = "2026-08-01T00:00:00Z"),
                contactId = 6L,
                contactName = "Eve Stale",
            ),
        ),
    )

    private fun setContent(
        state: DashboardUiState,
        dateFormat: String = DateFormat.EU,
        onOpenContact: (Int) -> Unit = {},
        onCompleteReminder: (id: Int, skip: Boolean) -> Unit = { _, _ -> },
        onDismissReachOutSuggestion: (id: String) -> Unit = {},
        onVerifyDataDecay: (id: String) -> Unit = {},
        onBackUpNow: () -> Unit = {},
        onDismissBackupReminder: () -> Unit = {},
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
                    onVerifyDataDecay = onVerifyDataDecay,
                    onBackUpNow = onBackUpNow,
                    onDismissBackupReminder = onDismissBackupReminder,
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
        scrollTo("Overdue relationships")
        composeTestRule.onNodeWithText("Carol Davis").assertIsDisplayed()
        composeTestRule.onNodeWithText("3 days overdue").assertIsDisplayed()

        // Data decay (issue #352).
        scrollTo("Info needs a check")
        composeTestRule.onNodeWithText("Eve Stale").assertIsDisplayed()
        composeTestRule.onNodeWithText("40 days overdue").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Confirm still current").assertIsDisplayed()

        // Reach-out suggestions (issue #177).
        scrollTo("Reasons to reach out")
        composeTestRule.onNodeWithText("Dana Prince").assertIsDisplayed()
        composeTestRule.onNodeWithText("OldCo → NewCo").assertIsDisplayed()
        composeTestRule.onNodeWithText("New employer").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Dismiss suggestion").assertIsDisplayed()

        // Birthdays: age rendered from the full date, in the eu date format.
        scrollTo("Alice Wonder")
        val age = LocalDate.now().year - 1990
        composeTestRule.onNodeWithText("Alice Wonder").assertIsDisplayed()
        composeTestRule.onNodeWithText("25.12. ($age years old)").assertIsDisplayed()

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
        composeTestRule.onNodeWithText("Overdue relationships").assertDoesNotExist()
        // Same treatment for reach-out suggestions (issue #177).
        composeTestRule.onNodeWithText("Reasons to reach out").assertDoesNotExist()
        // Same treatment for data decay (issue #352).
        composeTestRule.onNodeWithText("Info needs a check").assertDoesNotExist()
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

        scrollTo("Overdue relationships")
        composeTestRule.onNodeWithText("Carol Davis").performClick()

        assertEquals(3, opened)
    }

    @Test
    fun `tapping a reach-out suggestion card opens the contact`() {
        var opened: Int? = null
        setContent(populatedState(), onOpenContact = { opened = it })

        scrollTo("Reasons to reach out")
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

    // Issue #1798: a target_behind suggestion localizes the tier and formats the
    // last qualifying interaction instead of the org/title/address from→to text.
    @Test
    fun `a target_behind suggestion renders its label and formatted last-contact date`() {
        val suggestion = ReachOutSuggestion(
            id = "s2",
            kind = ReachOutKind.TARGET_BEHIND,
            oldValue = "close",
            newValue = "2026-08-01T10:00:00Z",
            contactId = 5L,
            contactName = "Dana Prince",
        )
        setContent(populatedState().copy(reachOutSuggestions = listOf(suggestion)))

        scrollTo("Reasons to reach out")

        composeTestRule.onNodeWithText("Behind target").assertIsDisplayed()
        val lastContact = DateFormat.formatTimestamp("2026-08-01T10:00:00Z", DateFormat.EU)
        composeTestRule
            .onNodeWithText("Behind your Close target · last contact $lastContact")
            .assertIsDisplayed()
    }

    @Test
    fun `tapping a data decay card opens the contact`() {
        var opened: Int? = null
        setContent(populatedState(), onOpenContact = { opened = it })

        scrollTo("Info needs a check")
        composeTestRule.onNodeWithText("Eve Stale").performClick()

        assertEquals(6, opened)
    }

    @Test
    fun `confirming a data decay policy calls the callback and does not navigate`() {
        var opened: Int? = null
        var verified: String? = null
        setContent(populatedState(), onOpenContact = { opened = it }, onVerifyDataDecay = { verified = it })

        scrollToContentDescription("Confirm still current")
        composeTestRule.onNodeWithContentDescription("Confirm still current").performClick()

        assertEquals("d1", verified)
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
        val viewModel = DashboardViewModel(
            apiClient,
            authRepository,
            mockk<com.mycorrhizal.crm.domain.repository.BundleBackupRepository> {
                every { observeStatus() } returns flowOf(com.mycorrhizal.crm.domain.backup.BundleBackupStatus())
            },
        )

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
        scrollTo("Overdue relationships")
        composeTestRule.onNodeWithText("Overdue relationships")
            .assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading))
        scrollTo("Reasons to reach out")
        composeTestRule.onNodeWithText("Reasons to reach out")
            .assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading))
    }

    // --- Issue #1264: local-profile backup reminder banner -----------------------

    @Test
    fun `the backup banner is absent by default`() {
        setContent(populatedState())

        composeTestRule.onNodeWithTag("backup-reminder-banner").assertDoesNotExist()
    }

    @Test
    fun `the backup banner renders with its title and both actions`() {
        setContent(populatedState().copy(showBackupReminder = true))

        composeTestRule.onNodeWithTag("backup-reminder-banner").assertIsDisplayed()
        composeTestRule.onNodeWithText("Back up your data").assertIsDisplayed()
        composeTestRule.onNodeWithText("Back up now").assertIsDisplayed()
        composeTestRule.onNodeWithText("Dismiss").assertIsDisplayed()
    }

    @Test
    fun `back up now and dismiss invoke their callbacks`() {
        var backedUp = 0
        var dismissed = 0
        setContent(
            populatedState().copy(showBackupReminder = true),
            onBackUpNow = { backedUp++ },
            onDismissBackupReminder = { dismissed++ },
        )

        composeTestRule.onNodeWithText("Back up now").performClick()
        composeTestRule.onNodeWithText("Dismiss").performClick()

        assertEquals(1, backedUp)
        assertEquals(1, dismissed)
    }

    // Issue #1824: "Showing N of M" under a block the server capped.

    private fun scrollToTag(tag: String) {
        composeTestRule.onNodeWithTag(listTag).performScrollToNode(hasTestTag(tag))
    }

    @Test
    fun `every capped block shows a showing-of-total caption when truncated`() {
        // populatedState() has one row per block, so a larger total is truncation.
        setContent(
            populatedState().copy(
                totals = DashboardTotals(
                    birthdays = 40, upcomingReminders = 60, overdue = 26,
                    reachOutSuggestions = 30, dataDecayOverdue = 27,
                ),
            ),
        )

        scrollToTag("dashboard-truncated-cadence")
        composeTestRule.onNodeWithTag("dashboard-truncated-cadence").assertIsDisplayed().assert(hasText("Showing 1 of 26"))
        scrollToTag("dashboard-truncated-data-decay")
        composeTestRule.onNodeWithTag("dashboard-truncated-data-decay").assertIsDisplayed().assert(hasText("Showing 1 of 27"))
        scrollToTag("dashboard-truncated-reach-out")
        composeTestRule.onNodeWithTag("dashboard-truncated-reach-out").assertIsDisplayed().assert(hasText("Showing 1 of 30"))
        scrollToTag("dashboard-truncated-birthday")
        composeTestRule.onNodeWithTag("dashboard-truncated-birthday").assertIsDisplayed().assert(hasText("Showing 1 of 40"))
        scrollToTag("dashboard-truncated-reminder")
        composeTestRule.onNodeWithTag("dashboard-truncated-reminder").assertIsDisplayed().assert(hasText("Showing 1 of 60"))
    }

    @Test
    fun `the caption is plain text with no link because no list screen exists to open`() {
        setContent(populatedState().copy(totals = DashboardTotals(upcomingReminders = 60)))

        scrollToTag("dashboard-truncated-reminder")
        composeTestRule.onNodeWithTag("dashboard-truncated-reminder")
            .assert(SemanticsMatcher.keyNotDefined(SemanticsActions.OnClick))
    }

    @Test
    fun `no caption when a block is not truncated or its total is absent`() {
        // total == shown: nothing hidden. Absent total (older server): unknown.
        setContent(
            populatedState().copy(
                totals = DashboardTotals(birthdays = 1, upcomingReminders = null, overdue = 1),
            ),
        )

        scrollTo("Call Dana")
        listOf("birthday", "reminder", "cadence", "data-decay", "reach-out").forEach {
            composeTestRule.onNodeWithTag("dashboard-truncated-$it").assertDoesNotExist()
        }
    }

    @Test
    fun `truncation is only reported when the total exceeds the shown rows`() {
        assertTrue(isDashboardTruncated(shown = 25, total = 60))
        assertFalse(isDashboardTruncated(shown = 25, total = 25))
        assertFalse(isDashboardTruncated(shown = 24, total = null))
    }
}
