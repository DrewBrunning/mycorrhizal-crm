package com.mycorrhizal.crm.feature.contacts

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToNode
import com.mycorrhizal.crm.model.network.BriefingActivity
import com.mycorrhizal.crm.model.network.BriefingCadence
import com.mycorrhizal.crm.model.network.BriefingCadenceHealth
import com.mycorrhizal.crm.model.network.BriefingRelationship
import com.mycorrhizal.crm.model.network.BriefingUpcomingDate
import com.mycorrhizal.crm.model.network.ContactBriefing
import com.mycorrhizal.crm.model.network.ConversationAgenda
import com.mycorrhizal.crm.model.network.LifeEvent
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.RelationshipEdge
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.util.DateFormat
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Renders the prep-view content against real composables. The ViewModel-level
 * state machine and the wire parse are pinned elsewhere (PrepViewModelTest,
 * ApiClientTest, and the Playwright spec) — this pins that the *screen*
 * actually renders every section from a briefing, and that an empty briefing
 * renders its empty states rather than crashing (M11 test case 1's render
 * half).
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class PrepViewScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val listTag = "prep-view-list"

    private fun setContent(
        briefing: ContactBriefing,
        dateFormat: String = DateFormat.EU,
        onOpenContact: (Int) -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                PrepViewContent(
                    briefing = briefing,
                    dateFormat = dateFormat,
                    onOpenContact = onOpenContact,
                )
            }
        }
    }

    /** Scrolls the lazy list until a node matching [text] is composed (off-screen items aren't). */
    private fun scrollTo(text: String) {
        composeTestRule.onNodeWithTag(listTag).performScrollToNode(hasText(text, substring = true))
    }

    /** Scrolls the lazy list to a node, brings it fully into view, and asserts it displays. */
    private fun assertSection(text: String, substring: Boolean = false) {
        scrollTo(text)
        composeTestRule.onNodeWithText(text, substring = substring)
            .performScrollTo()
            .assertIsDisplayed()
    }

    @Test
    fun `a fully populated briefing renders every section`() {
        val briefing = ContactBriefing(
            contactId = 7,
            uid = "u7",
            name = "Alice Wonder",
            kind = "animal",
            lastActivity = BriefingActivity(
                id = 41,
                title = "Coffee",
                type = "visit",
                description = "Catch-up over coffee",
                date = "2026-08-10T14:00:00Z",
            ),
            recentNotesRaw = listOf(Note(id = 9, content = "Talks about her garden")),
            cadence = BriefingCadence(
                health = BriefingCadenceHealth(
                    hasQualifyingInteraction = true,
                    lastInteraction = "2026-08-10T14:00:00Z",
                    nextDue = "2026-09-09T14:00:00Z",
                    overdueBy = 0,
                ),
            ),
            openAgendaItemsRaw = listOf(ConversationAgenda(id = "ag-1", content = "Ask about the surgery")),
            relationshipsRaw = listOf(
                BriefingRelationship(
                    edge = RelationshipEdge(
                        id = "edge-1",
                        sourceId = "u7",
                        targetId = "u8",
                        type = "spouse_of",
                    ),
                    otherPartyContactId = 8,
                    otherPartyName = "Bob Marley",
                    otherPartyUid = "u8",
                    displayToken = "spouse_of",
                ),
            ),
            lifeEventsRaw = listOf(LifeEvent(id = "le-1", entityId = "u7", type = "graduated", description = "MSc")),
            upcomingRemindersRaw = listOf(Reminder(id = 3, message = "Send card", remindAt = "2026-08-20T09:00:00Z")),
            upcomingDatesRaw = listOf(BriefingUpcomingDate(label = "birthday", date = "--12-25", daysUntil = 5)),
        )

        setContent(briefing)

        // Header.
        composeTestRule.onNodeWithText("Alice Wonder").assertIsDisplayed()
        composeTestRule.onNodeWithText("Animal").assertIsDisplayed()

        // Cadence health card.
        assertSection("Relationship health")
        assertSection("On track")
        assertSection("Next due: 9 September 2026")

        // Agenda.
        assertSection("Things to bring up")
        assertSection("Ask about the surgery", substring = true)

        // Last interaction + recent notes.
        assertSection("Coffee (visit)")
        assertSection("Catch-up over coffee")
        assertSection("Talks about her garden", substring = true)

        // Relationships.
        assertSection("Bob Marley — spouse of")

        // Life events.
        assertSection("graduated — MSc", substring = true)

        // Upcoming reminders.
        assertSection("Send card", substring = true)

        // Upcoming dates: yearless --12-25 renders as "25 December" in eu format.
        assertSection("Birthday 25 December")
        assertSection("in 5 day(s)")
    }

    @Test
    fun `an empty briefing renders its empty states and hides empty sections`() {
        val briefing = ContactBriefing(contactId = 7, uid = "u7", name = "Prep Empty")

        setContent(briefing)

        composeTestRule.onNodeWithText("Prep Empty").assertIsDisplayed()
        composeTestRule.onNodeWithText("No interactions recorded yet").assertIsDisplayed()
        composeTestRule.onNodeWithText("Recent notes").assertDoesNotExist()

        // Sections with no data stay hidden rather than rendering empty shells.
        composeTestRule.onNodeWithText("Relationship health").assertDoesNotExist()
        composeTestRule.onNodeWithText("Things to bring up").assertDoesNotExist()
        composeTestRule.onNodeWithText("People around them").assertDoesNotExist()
        composeTestRule.onNodeWithText("Life events").assertDoesNotExist()
        composeTestRule.onNodeWithText("Upcoming reminders").assertDoesNotExist()
        composeTestRule.onNodeWithText("Upcoming dates").assertDoesNotExist()
    }

    @Test
    fun `a relationship row with a target contact navigates on tap`() {
        val briefing = ContactBriefing(
            contactId = 7,
            uid = "u7",
            name = "Alice Wonder",
            relationshipsRaw = listOf(
                BriefingRelationship(
                    edge = RelationshipEdge(
                        id = "edge-1",
                        sourceId = "u7",
                        targetId = "u8",
                        type = "spouse_of",
                    ),
                    otherPartyContactId = 8,
                    otherPartyName = "Bob Marley",
                    otherPartyUid = "u8",
                    displayToken = "spouse_of",
                ),
            ),
        )
        var openedContactId: Int? = null

        setContent(briefing, onOpenContact = { openedContactId = it })

        scrollTo("Bob Marley — spouse of")
        composeTestRule.onNodeWithText("Bob Marley — spouse of").performClick()

        assertEquals(8, openedContactId)
    }

    @Test
    fun `a relationship row without a target contact is not tappable`() {
        val briefing = ContactBriefing(
            contactId = 7,
            uid = "u7",
            name = "Alice Wonder",
            relationshipsRaw = listOf(
                BriefingRelationship(
                    edge = RelationshipEdge(
                        id = "edge-1",
                        sourceId = "u7",
                        targetId = "u8",
                        type = "friend_of",
                    ),
                    // No otherPartyContactId — the other endpoint is a thin/ghost
                    // contact with no CRM row to link to.
                    otherPartyName = "Ghost",
                    otherPartyUid = "u8",
                    displayToken = "friend_of",
                ),
            ),
        )
        var clicked = false

        setContent(briefing, onOpenContact = { clicked = true })

        scrollTo("Ghost — friend of")
        composeTestRule.onNodeWithText("Ghost — friend of").performClick()

        // The row is plain text; the tap must not fire navigation.
        assertEquals(false, clicked)
    }
}
