package com.mycorrhizal.crm.feature.contacts

import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertAny
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isFocused
import androidx.compose.ui.test.isToggleable
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onFirst
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToNode
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.Anniversary
import com.mycorrhizal.crm.model.network.AnniversaryDate
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.CadencePolicyInput
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactFieldKey
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ContactScoreFacet
import com.mycorrhizal.crm.model.network.ContactScoreResponse
import com.mycorrhizal.crm.model.network.ContactScoreTarget
import com.mycorrhizal.crm.model.network.DEFAULT_ENABLED_CONTACT_FIELDS
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.ExternalActivity
import com.mycorrhizal.crm.model.network.ExternalIdentity
import com.mycorrhizal.crm.model.network.ExternalSystems
import com.mycorrhizal.crm.model.network.FieldConstraints
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.GrammaticalGender
import com.mycorrhizal.crm.model.network.ImmichPersonSummary
import com.mycorrhizal.crm.model.network.LanguagePref
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.OnlineService
import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.model.network.Pronouns
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.network.Resource
import com.mycorrhizal.crm.model.network.SpeakToAs
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import kotlinx.coroutines.flow.MutableStateFlow
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
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
@OptIn(ExperimentalMaterial3Api::class)
class ContactDetailScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(state: ContactDetailUiState) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = state.contact!!,
                    dateFormat = state.dateFormat,
                    enabledFields = state.enabledFields
                )
            }
        }
    }

    /**
     * Scrolls the lazy list until a node matching [text] is composed (off-screen items aren't),
     * returning that scroll's node so callers can chain e.g. `.assertIsDisplayed()`.
     */
    private fun scrollTo(text: String) =
        composeTestRule.onNodeWithTag("contact-detail-list").performScrollToNode(hasText(text))

    /** Issue #1731: custom fields live behind a drill-down per group, not inline. Opens the row
     *  whose group label is [label] ("" = the ungrouped "Custom fields" row). */
    private fun openGroup(label: String = "") {
        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasTestTag("custom-field-group-$label"))
        composeTestRule.onNodeWithTag("custom-field-group-$label").performClick()
    }

    /** Scrolls the open drill-down's own list (not the detail list) to [text]. */
    private fun inGroup(text: String) = composeTestRule.onNodeWithTag(
        CUSTOM_FIELD_GROUP_LIST_TAG
    ).performScrollToNode(hasText(text))

    // --- Issue #212: header star toggle (web #173) ---------------------------

    @Test
    fun `favorite header star renders a labeled toggle`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White")),
                        isFavorite = true
                    )
                )
            }
        }
        composeTestRule.onNodeWithContentDescription(
            "Unmark Dana White as favorite"
        ).assertIsDisplayed()
    }

    // --- Issue #1922: TalkBack focus on the contact's name on entry -------

    @Test
    fun `the contact name receives focus on entry`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White"))
                    )
                )
            }
        }
        composeTestRule.waitForIdle()

        // The name is the large header title (the collapsed app-bar copy is not
        // part of ContactDetailContent, hence no ambiguous duplicate here).
        composeTestRule
            .onAllNodesWithText("Dana White", useUnmergedTree = true)
            .assertAny(isFocused())
    }

    @Test
    fun `non-favorite header star renders a labeled toggle`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White")),
                        isFavorite = false
                    )
                )
            }
        }
        composeTestRule.onNodeWithContentDescription(
            "Mark Dana White as favorite"
        ).assertIsDisplayed()
    }

    @Test
    fun `tapping the header star fires the toggle callback`() {
        var toggles = 0
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White")),
                        isFavorite = false
                    ),
                    onToggleFavorite = { toggles++ }
                )
            }
        }

        composeTestRule.onNodeWithContentDescription("Mark Dana White as favorite").performClick()

        assertEquals(1, toggles)
    }

    // --- T90 / issue #831: "You" badge + Mark as Me (web parity) ------------

    @Test
    fun `you badge shows when isMe is true`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White"))
                    ),
                    isMe = true
                )
            }
        }
        composeTestRule.onNodeWithTag("you-badge").assertIsDisplayed()
    }

    @Test
    fun `you badge is absent when isMe is false`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White"))
                    ),
                    isMe = false
                )
            }
        }
        composeTestRule.onNodeWithTag("you-badge").assertDoesNotExist()
    }

    // --- Issue #383 (ADR-0023): relationship health score badge -------------

    private fun scoreOf(band: String) = ContactScoreResponse(
        contactId = 5,
        score = 72,
        band = band,
        recency = ContactScoreFacet(value = 80.0, weight = 35.0, reason = "Last seen 5 days ago"),
        frequency = ContactScoreFacet(value = 60.0, weight = 20.0, reason = "Some cadence"),
        closeness = ContactScoreFacet(value = 85.0, weight = 20.0, reason = "Close relation"),
        reachOut = ContactScoreFacet(value = 100.0, weight = 15.0, reason = "No pending reach-out"),
        lastUpdated = ContactScoreFacet(value = 90.0, weight = 10.0, reason = "Updated recently")
    )

    @Test
    fun `health score badge is absent when score is null`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White"))
                    ),
                    score = null
                )
            }
        }
        composeTestRule.onNodeWithTag("health-score-badge").assertDoesNotExist()
    }

    @Test
    fun `health score badge renders with the score and a word-based band description`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White"))
                    ),
                    score = scoreOf("moss")
                )
            }
        }
        // The band's meaning must be spelled out in words (never color alone)
        // for TalkBack — see HealthScoreBadge's doc comment.
        composeTestRule.onNodeWithTag("health-score-badge").assertIsDisplayed()
        composeTestRule
            .onNodeWithContentDescription("Relationship health score 72 out of 100: Healthy")
            .assertIsDisplayed()
    }

    @Test
    fun `chanterelle and russula bands each get their own word-based description`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White"))
                    ),
                    score = scoreOf("chanterelle")
                )
            }
        }
        composeTestRule
            .onNodeWithContentDescription(
                "Relationship health score 72 out of 100: Needs attention"
            )
            .assertIsDisplayed()
    }

    @Test
    fun `russula band gets its own word-based description`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White"))
                    ),
                    score = scoreOf("russula")
                )
            }
        }
        composeTestRule
            .onNodeWithContentDescription("Relationship health score 72 out of 100: At risk")
            .assertIsDisplayed()
    }

    @Test
    fun `tapping the health score badge opens the facet breakdown dialog`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White"))
                    ),
                    score = scoreOf("moss")
                )
            }
        }

        composeTestRule.onNodeWithTag("health-score-badge").performClick()

        composeTestRule.onNodeWithText("Relationship health").assertIsDisplayed()
        composeTestRule.onNodeWithText("Last seen 5 days ago").assertIsDisplayed()
        composeTestRule.onNodeWithText("Some cadence").assertIsDisplayed()
        composeTestRule.onNodeWithText("Close relation").assertIsDisplayed()
        composeTestRule.onNodeWithText("No pending reach-out").assertIsDisplayed()
        composeTestRule.onNodeWithText("Updated recently").assertIsDisplayed()
    }

    // --- Issue #1734: ambient explanation in the score breakdown ---

    private fun openScoreDialog(score: ContactScoreResponse) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White"))
                    ),
                    score = score
                )
            }
        }
        composeTestRule.onNodeWithTag("health-score-badge").performClick()
    }

    @Test
    fun `an ambient household contact is explained as living with you`() {
        openScoreDialog(scoreOf("moss").copy(ambient = true, ambientReason = "household"))

        composeTestRule.onNodeWithTag("health-score-ambient").assertIsDisplayed()
        composeTestRule.onNodeWithText("Lives with you (household)").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Counts as in regular contact without logging activities"
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText("Marked as ambient").assertDoesNotExist()
    }

    @Test
    fun `an ambient flagged contact is explained as marked as ambient`() {
        openScoreDialog(scoreOf("moss").copy(ambient = true, ambientReason = "flag"))

        composeTestRule.onNodeWithText("Marked as ambient").assertIsDisplayed()
        composeTestRule.onNodeWithText("Lives with you (household)").assertDoesNotExist()
    }

    @Test
    fun `a non-ambient score shows no ambient explanation`() {
        openScoreDialog(scoreOf("moss"))

        composeTestRule.onNodeWithTag("health-score-ambient").assertDoesNotExist()
        composeTestRule.onNodeWithText("Marked as ambient").assertDoesNotExist()
        composeTestRule.onNodeWithText("Lives with you (household)").assertDoesNotExist()
    }

    @Test
    fun `closing the facet breakdown dialog dismisses it`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White"))
                    ),
                    score = scoreOf("moss")
                )
            }
        }

        composeTestRule.onNodeWithTag("health-score-badge").performClick()
        composeTestRule.onNodeWithText("Relationship health").assertIsDisplayed()

        composeTestRule.onNodeWithText("Close").performClick()

        composeTestRule.onNodeWithText("Relationship health").assertDoesNotExist()
    }

    // --- Issue #1798 (ADR 0023 amendment): target chip, picker, offer dialog ---

    private fun targetOf(tier: String = "close", status: String = "drifting") = ContactScoreTarget(
        tier = tier,
        status = status,
        value = 65.9,
        intervalDays = 30,
        intervalSource = "target_tier",
        reason = "Recency and frequency score 66 against a 30-day interval from the close target"
    )

    @Test
    fun `a score with a target shows the localized chip next to the badge`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White")),
                        crm = CRMEnvelope(targetCloseness = "close")
                    ),
                    score = scoreOf("moss").copy(target = targetOf())
                )
            }
        }

        composeTestRule.onNodeWithTag("target-closeness-chip").assertIsDisplayed()
        composeTestRule.onNodeWithText("Target: Close · Drifting").assertIsDisplayed()

        // The chip is a shortcut into the same score breakdown as the badge.
        composeTestRule.onNodeWithTag("target-closeness-chip").performClick()
        composeTestRule.onNodeWithText("Relationship health").assertIsDisplayed()
    }

    @Test
    fun `a score without a target shows no chip`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White"))
                    ),
                    score = scoreOf("moss").copy(target = null)
                )
            }
        }

        composeTestRule.onNodeWithTag("target-closeness-chip").assertDoesNotExist()
    }

    @Test
    fun `choosing a tier in the score dialog invokes the target callback`() {
        var picked: String? = "unset"
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White")),
                        crm = CRMEnvelope(targetCloseness = "close")
                    ),
                    score = scoreOf("moss").copy(target = targetOf()),
                    onSetTargetCloseness = { picked = it }
                )
            }
        }

        composeTestRule.onNodeWithTag("health-score-badge").performClick()
        // The label ("Target closeness") appears on both the section heading and
        // the field, so pin the picker itself rather than a duplicated text node.
        composeTestRule.onNodeWithTag("target-closeness-picker").assertIsDisplayed()
        composeTestRule.onNodeWithTag("target-closeness-picker").performClick()
        composeTestRule.onNodeWithText("Casual").performClick()

        assertEquals("casual", picked)
    }

    @Test
    fun `choosing no target in the picker invokes the callback with null`() {
        var picked: String? = "unset"
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White")),
                        crm = CRMEnvelope(targetCloseness = "close")
                    ),
                    score = scoreOf("moss").copy(target = targetOf()),
                    onSetTargetCloseness = { picked = it }
                )
            }
        }

        composeTestRule.onNodeWithTag("health-score-badge").performClick()
        composeTestRule.onNodeWithTag("target-closeness-picker").performClick()
        composeTestRule.onNodeWithText("No target").performClick()

        assertNull(picked)
    }

    @Test
    fun `the score dialog shows the target reason when one is set`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White"))
                    ),
                    score = scoreOf("moss").copy(target = targetOf())
                )
            }
        }

        composeTestRule.onNodeWithTag("health-score-badge").performClick()

        // The dialog's text column can clip below the facets; the reason must
        // still be composed and rendered into the dialog.
        composeTestRule
            .onNodeWithText(
                "Recency and frequency score 66 against a 30-day interval from the close target"
            )
            .assertExists()
    }

    @Test
    fun `a pending offer shows the dialog and accepting invokes its callback`() {
        var accepted = 0
        var dismissed = 0
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White"))
                    ),
                    cadencePolicyOffer = CadencePolicyInput(
                        entityId = "u5",
                        targetIntervalDays = 30
                    ),
                    onAcceptCadenceOffer = { accepted++ },
                    onDismissCadenceOffer = { dismissed++ }
                )
            }
        }

        composeTestRule.onNodeWithTag("target-closeness-offer").assertIsDisplayed()
        composeTestRule.onNodeWithText("Create a cadence?").assertIsDisplayed()
        composeTestRule
            .onNodeWithText("Stay in touch with Dana White every 30 days?")
            .assertIsDisplayed()

        composeTestRule.onNodeWithText("Create cadence").performClick()

        assertEquals(1, accepted)
        assertEquals(0, dismissed)
    }

    @Test
    fun `dismissing the offer invokes the dismiss callback`() {
        var accepted = 0
        var dismissed = 0
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White"))
                    ),
                    cadencePolicyOffer = CadencePolicyInput(
                        entityId = "u5",
                        targetIntervalDays = 30
                    ),
                    onAcceptCadenceOffer = { accepted++ },
                    onDismissCadenceOffer = { dismissed++ }
                )
            }
        }

        composeTestRule.onNodeWithText("Not now").performClick()

        assertEquals(0, accepted)
        assertEquals(1, dismissed)
    }

    @Test
    fun `renders the contact name and sections`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                emails = listOf(Email(address = "dana@example.com", label = "Work")),
                phones = listOf(Phone(number = "+1-555-0100"))
            ),
            crm = CRMEnvelope(circles = listOf("friends", "work"))
        )
        setContent(ContactDetailUiState(contact = contact))

        // The name is owned by the collapsing app bar (ContactDetailScreen).
        // The body renders the overview info first (web order), then the
        // timeline, then the management entry rows — scroll the list to them.
        val list = composeTestRule.onNodeWithTag("contact-detail-list")
        list.performScrollToNode(hasText("Activities · 0"))
        composeTestRule.onNodeWithText("Activities · 0").assertIsDisplayed()
        list.performScrollToNode(hasText("Notes · 0"))
        composeTestRule.onNodeWithText("Notes · 0").assertIsDisplayed()
        list.performScrollToNode(hasText("Reminders · 0"))
        composeTestRule.onNodeWithText("Reminders · 0").assertIsDisplayed()
    }

    @Test
    fun `renders empty name fallback when no name present`() {
        val contact = ContactRecordResponse(id = 5, card = Card())
        setContent(ContactDetailUiState(contact = contact))
        // No nickname/birthday/name in the body for a nameless contact — the
        // list still renders (timeline + management entries).
        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("Timeline"))
        composeTestRule.onNodeWithText("Timeline").assertIsDisplayed()
    }

    @Test
    fun `phone row renders call and copy actions`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                phones = listOf(Phone(number = "+1-555-0100"))
            )
        )
        setContent(ContactDetailUiState(contact = contact))

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasContentDescription("Call"))
            .assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription(
            "Copy phone number"
        ).performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `mobile phone renders an sms action`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                phones = listOf(Phone(number = "+1-555-0100", features = listOf("cell")))
            )
        )
        setContent(ContactDetailUiState(contact = contact))

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasContentDescription("Text"))
            .assertIsDisplayed()
    }

    @Test
    fun `landline phone does not render an sms action`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                phones = listOf(Phone(number = "+1-555-0100"))
            )
        )
        setContent(ContactDetailUiState(contact = contact))

        composeTestRule.onNodeWithContentDescription("Text").assertDoesNotExist()
    }

    @Test
    fun `phone with cell context renders an sms action`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                phones = listOf(Phone(number = "+1-555-0100", contexts = listOf("cell")))
            )
        )
        setContent(ContactDetailUiState(contact = contact))

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasContentDescription("Text"))
            .assertIsDisplayed()
    }

    @Test
    fun `phone typed as mobile via label renders an sms action`() {
        // CRM-created contacts carry the phone type in `label` (the backend's
        // buildPhones maps ContactPhone.Type -> Phone.Label) with empty
        // features/contexts — the web app's phoneHasToken covers this via
        // `r.type === token`; the Android client must too.
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                phones = listOf(Phone(number = "+1-555-0100", label = "mobile"))
            )
        )
        setContent(ContactDetailUiState(contact = contact))

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasContentDescription("Text"))
            .assertIsDisplayed()
    }

    @Test
    fun `email row renders compose and copy actions`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                emails = listOf(Email(address = "dana@example.com"))
            )
        )
        setContent(ContactDetailUiState(contact = contact))

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasContentDescription("Compose email"))
            .assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription(
            "Copy email"
        ).performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `address row renders map and copy actions`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                addresses = listOf(com.mycorrhizal.crm.model.network.Address(full = "123 Main St"))
            )
        )
        setContent(ContactDetailUiState(contact = contact))

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasContentDescription("Open in maps"))
            .assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription(
            "Copy address"
        ).performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `link section renders open and copy actions`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                links = listOf(Resource(uri = "https://example.com/profile", label = "Website"))
            )
        )
        setContent(
            ContactDetailUiState(
                contact = contact,
                enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS + ContactFieldKey.LINKS
            )
        )

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasContentDescription("Open link"))
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("Website").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription(
            "Copy link"
        ).performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `birthday renders using the eu format by default`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                anniversaries = listOf(
                    Anniversary(
                        kind = "birth",
                        date = AnniversaryDate(
                            partial = PartialDate(year = 1990, month = 6, day = 15)
                        )
                    )
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact)
            }
        }

        composeTestRule.onNodeWithText("Birthday: 15.06.1990").assertIsDisplayed()
    }

    @Test
    fun `birthday honors the user's date_format preference`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                anniversaries = listOf(
                    Anniversary(
                        kind = "birth",
                        date = AnniversaryDate(
                            partial = PartialDate(year = 1990, month = 6, day = 15)
                        )
                    )
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact, dateFormat = "us")
            }
        }

        composeTestRule.onNodeWithText("Birthday: 06/15/1990").assertIsDisplayed()
    }

    // --- Issue #1193: deceased badge + date of death (web ContactHeader parity) ---

    @Test
    fun `deceased badge is absent when no death anniversary is recorded`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact)
            }
        }

        composeTestRule.onNodeWithTag("deceased-badge").assertDoesNotExist()
    }

    @Test
    fun `deceased badge shows and date of death renders using the eu format by default`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                anniversaries = listOf(
                    Anniversary(
                        kind = "death",
                        date = AnniversaryDate(
                            partial = PartialDate(year = 2020, month = 5, day = 1)
                        )
                    )
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact)
            }
        }

        composeTestRule.onNodeWithTag("deceased-badge").assertIsDisplayed()
        composeTestRule.onNodeWithText("Died: 01.05.2020").assertIsDisplayed()
    }

    @Test
    fun `date of death honors the user's date_format preference`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                anniversaries = listOf(
                    Anniversary(
                        kind = "death",
                        date = AnniversaryDate(
                            partial = PartialDate(year = 2020, month = 5, day = 1)
                        )
                    )
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact, dateFormat = "us")
            }
        }

        composeTestRule.onNodeWithText("Died: 05/01/2020").assertIsDisplayed()
    }

    @Test
    fun `birth and death anniversaries render independently when both are recorded`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                anniversaries = listOf(
                    Anniversary(
                        kind = "birth",
                        date = AnniversaryDate(
                            partial = PartialDate(year = 1990, month = 6, day = 15)
                        )
                    ),
                    Anniversary(
                        kind = "death",
                        date = AnniversaryDate(
                            partial = PartialDate(year = 2020, month = 5, day = 1)
                        )
                    )
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact)
            }
        }

        composeTestRule.onNodeWithTag("deceased-badge").assertIsDisplayed()
        composeTestRule.onNodeWithText("Birthday: 15.06.1990").assertIsDisplayed()
        composeTestRule.onNodeWithText("Died: 01.05.2020").assertIsDisplayed()
    }

    // --- Issue #832: fields with no prior Android UI ---

    @Test
    fun `gender renders in the header alongside nickname and birthday`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(name = Name(full = "Dana White")),
            crm = CRMEnvelope(gender = "non-binary")
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact)
            }
        }
        composeTestRule.onNodeWithText("non-binary").assertIsDisplayed()
    }

    @Test
    fun `speak to as section shows pronouns and grammatical gender`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                speakToAs = SpeakToAs(
                    pronouns = listOf(Pronouns(pronouns = "they/them")),
                    grammaticalGenders = listOf(
                        GrammaticalGender(value = "common", language = "en")
                    )
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact)
            }
        }
        composeTestRule.onNodeWithText("Speak to as").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("they/them").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("common").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `speak to as section is absent when both lists are empty`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact)
            }
        }
        composeTestRule.onNodeWithText("Speak to as").assertDoesNotExist()
    }

    @Test
    fun `keywords render as a single joined row`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(name = Name(full = "Dana White"), keywords = listOf("hiking", "climbing"))
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS + ContactFieldKey.KEYWORDS
                )
            }
        }
        composeTestRule.onNodeWithText("Keywords").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("hiking, climbing").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `preferred languages render each entry's language`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                preferredLanguages = listOf(LanguagePref(language = "en"))
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    enabledFields =
                        DEFAULT_ENABLED_CONTACT_FIELDS + ContactFieldKey.PREFERRED_LANGUAGES
                )
            }
        }
        composeTestRule.onNodeWithText("Preferred languages").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("en").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `non-birth anniversaries become a drill-down row while the header birthday stays`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                anniversaries = listOf(
                    Anniversary(
                        kind = "birth",
                        date = AnniversaryDate(
                            partial = PartialDate(year = 1990, month = 6, day = 15)
                        )
                    ),
                    Anniversary(
                        kind = "wedding",
                        date = AnniversaryDate(
                            partial = PartialDate(year = 2020, month = 6, day = 1)
                        )
                    )
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS + ContactFieldKey.ANNIVERSARIES
                )
            }
        }
        composeTestRule.onNodeWithText("Birthday: 15.06.1990").assertIsDisplayed()
        // Issue #1728: the inline "Wedding: …" block is gone; the entry is
        // behind the drill-down row (count = non-birth entries only).
        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasTestTag("drill-down-anniversaries"))
        composeTestRule.onNodeWithTag("drill-down-anniversaries").assertIsDisplayed()
        composeTestRule.onNodeWithText("Anniversaries · 1").assertIsDisplayed()
        composeTestRule.onNodeWithText("Wedding: 01.06.2020").assertDoesNotExist()
    }

    @Test
    fun `anniversaries drill-down is hidden when its field is disabled`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                anniversaries = listOf(
                    Anniversary(
                        kind = "wedding",
                        date = AnniversaryDate(
                            partial = PartialDate(year = 2020, month = 6, day = 1)
                        )
                    )
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact)
            }
        }
        composeTestRule.onNodeWithTag("drill-down-anniversaries").assertDoesNotExist()
    }

    // --- Issue #832: detail-screen sections for fields that only ever had a form editor ---

    @Test
    fun `titles become a work drill-down row with a combined count`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                titles = listOf(
                    com.mycorrhizal.crm.model.network.Title(name = "Reporter", kind = "title")
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS + ContactFieldKey.TITLES
                )
            }
        }
        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasTestTag("drill-down-work"))
        composeTestRule.onNodeWithTag("drill-down-work").assertIsDisplayed()
        composeTestRule.onNodeWithText("Work · 1").assertIsDisplayed()
        composeTestRule.onNodeWithText("Reporter").assertDoesNotExist()
    }

    @Test
    fun `work drill-down is absent when every work field is disabled`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                titles = listOf(
                    com.mycorrhizal.crm.model.network.Title(name = "Reporter", kind = "title")
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    enabledFields =
                        DEFAULT_ENABLED_CONTACT_FIELDS - ContactFieldKey.WORK_INFORMATION
                )
            }
        }
        composeTestRule.onNodeWithTag("drill-down-work").assertDoesNotExist()
    }

    @Test
    fun `how we met and contact information stay inline while work information moves to the drill-down`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(name = Name(full = "Dana White")),
            crm = CRMEnvelope(
                howWeMet = "At the climbing gym",
                workInformation = "Full-stack at Acme",
                contactInformation = "Prefers email"
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact)
            }
        }
        composeTestRule.onNodeWithText("How we met").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("At the climbing gym").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Additional contact information"
        ).performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Prefers email").performScrollTo().assertIsDisplayed()
        // Issue #1728: work_information moved behind the Work drill-down.
        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasTestTag("drill-down-work"))
        composeTestRule.onNodeWithText("Work · 1").assertIsDisplayed()
        composeTestRule.onNodeWithText("Full-stack at Acme").assertDoesNotExist()
    }

    @Test
    fun `skills drill-down row shows the personal info count and opens the screen`() {
        var opened: Int? = null
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                personalInfo = listOf(
                    com.mycorrhizal.crm.model.network.PersonalInfo(
                        id = "pi-1",
                        kind = "expertise",
                        value = "Go",
                        level = "expert"
                    ),
                    com.mycorrhizal.crm.model.network.PersonalInfo(
                        id = "pi-2",
                        kind = "hobby",
                        value = "Climbing"
                    )
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    onViewSkills = { opened = it }
                )
            }
        }
        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasTestTag("drill-down-skills"))
        composeTestRule.onNodeWithText("Skills & interests · 2").assertIsDisplayed()
        composeTestRule.onNodeWithTag("drill-down-skills").performClick()
        assertEquals(5, opened)
        // Issue #1728: the inline "Expertise: Go" block is gone.
        composeTestRule.onNodeWithText("Expertise: Go").assertDoesNotExist()
    }

    @Test
    fun `online service row renders service name and handle`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                imppAddresses = listOf(
                    OnlineService(service = "Signal", uri = "6085142711")
                )
            )
        )
        setContent(
            ContactDetailUiState(
                contact = contact,
                enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS + ContactFieldKey.IMPP_ADDRESSES
            )
        )

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("Signal"))
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("6085142711").assertIsDisplayed()
    }

    @Test
    fun `custom field section is absent when there are no definitions`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact, fieldDefinitions = emptyList())
            }
        }
        composeTestRule.onNodeWithText("Custom fields").assertDoesNotExist()
    }

    @Test
    fun `a string and a number field render with their values`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(
                        FieldDefinition(id = "d1", label = "Coffee order", type = "string"),
                        FieldDefinition(id = "d2", label = "Favorite number", type = "number")
                    ),
                    fieldValuesByDefinitionId = mapOf("d1" to "Latte", "d2" to 7.0)
                )
            }
        }

        openGroup()
        inGroup("Coffee order").assertIsDisplayed()
        inGroup("Latte").assertIsDisplayed()
        inGroup("Favorite number").assertIsDisplayed()
        inGroup("7").assertIsDisplayed()
    }

    @Test
    fun `a definition with no value shows the placeholder, not a crash`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(
                        FieldDefinition(id = "d1", label = "Coffee order", type = "string")
                    ),
                    fieldValuesByDefinitionId = emptyMap()
                )
            }
        }

        openGroup()
        inGroup("Coffee order").assertIsDisplayed()
        inGroup("—").assertIsDisplayed()
    }

    @Test
    fun `a value whose definition is missing never renders anywhere`() {
        // T84: the render loop iterates definitions, not values, so an orphaned value (its
        // definition deleted since the value was set) is silently unreachable rather than
        // needing a special-case skip. Regression for the "degrades gracefully" test case.
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = emptyList(),
                    fieldValuesByDefinitionId = mapOf("deleted-def" to "orphaned value")
                )
            }
        }

        composeTestRule.onNodeWithText("orphaned value").assertDoesNotExist()
        composeTestRule.onNodeWithText("Custom fields").assertDoesNotExist()
    }

    @Test
    fun `a boolean and a multi-value field render per their type`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(
                        FieldDefinition(id = "d1", label = "VIP", type = "boolean"),
                        FieldDefinition(
                            id = "d2",
                            label = "Milk options",
                            type = "string",
                            constraints = FieldConstraints(multi = true)
                        )
                    ),
                    fieldValuesByDefinitionId = mapOf(
                        "d1" to true,
                        "d2" to listOf("oat", "almond")
                    )
                )
            }
        }

        openGroup()
        inGroup("VIP").assertIsDisplayed()
        inGroup("true").assertIsDisplayed()
        inGroup("Milk options").assertIsDisplayed()
        inGroup("oat; almond").assertIsDisplayed()
    }

    // --- Issue #1731: custom fields as per-group drill-downs ---

    private val groupedDefinitions = listOf(
        FieldDefinition(
            id = "d1",
            label = "Passport country",
            type = "string",
            group = "Travel profile"
        ),
        FieldDefinition(
            id = "d2",
            label = "Seat preference",
            type = "string",
            group = "Travel profile"
        ),
        FieldDefinition(id = "d3", label = "Shoe size", type = "number"),
        FieldDefinition(id = "d4", label = "Allergy", type = "string", group = "Health")
    )

    @Test
    fun `custom fields render as one drill-down row per group with filled counts and no inline fields`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = groupedDefinitions,
                    fieldValuesByDefinitionId = mapOf("d1" to "Canada", "d3" to 42.0)
                )
            }
        }

        scrollTo("Travel profile").assertIsDisplayed()
        scrollTo("1 of 2 filled").assertIsDisplayed()
        // Ungrouped row, titled "Custom fields" like the card that holds it.
        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasTestTag("custom-field-group-"))
        composeTestRule.onNodeWithTag("custom-field-group-").assertIsDisplayed()
        // A group with no filled value still appears so it can be filled.
        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasTestTag("custom-field-group-Health"))
        composeTestRule.onNodeWithTag("custom-field-group-Health").assertIsDisplayed()
        scrollTo("0 of 1 filled").assertIsDisplayed()
        // Nothing renders inline any more.
        composeTestRule.onNodeWithText("Passport country").assertDoesNotExist()
        composeTestRule.onNodeWithText("Canada").assertDoesNotExist()
    }

    @Test
    fun `opening a group shows only that group's fields, including empty ones, in order`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = groupedDefinitions,
                    fieldValuesByDefinitionId = mapOf("d1" to "Canada", "d3" to 42.0)
                )
            }
        }

        openGroup("Travel profile")

        inGroup("Passport country").assertIsDisplayed()
        inGroup("Canada").assertIsDisplayed()
        inGroup("Seat preference").assertIsDisplayed()
        composeTestRule.onNodeWithText("Shoe size").assertDoesNotExist()
        composeTestRule.onNodeWithText("Allergy").assertDoesNotExist()
    }

    @Test
    fun `a value edited inside a group drill-down saves against that definition and the back arrow closes it`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        var savedId: String? = null
        var savedValue: Any? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = groupedDefinitions,
                    fieldValuesByDefinitionId = mapOf("d1" to "Canada"),
                    onSaveFieldValue = { id, value ->
                        savedId = id
                        savedValue = value
                    }
                )
            }
        }

        openGroup("Travel profile")
        inGroup("Seat preference")
        composeTestRule.onNodeWithContentDescription("Edit Seat preference").performClick()
        composeTestRule.onNodeWithText("Value").performTextInput("Aisle")
        inGroup("Save")
        composeTestRule.onNodeWithText("Save").performClick()

        assertEquals("d2", savedId)
        assertEquals("Aisle", savedValue)

        composeTestRule.onNodeWithContentDescription("Back").performClick()
        composeTestRule.onNodeWithText("Seat preference").assertDoesNotExist()
    }

    @Test
    fun `groups that differ only by case are separate drill-down rows`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(
                        FieldDefinition(id = "a", label = "A", type = "string", group = "Travel"),
                        FieldDefinition(id = "b", label = "B", type = "string", group = "travel")
                    ),
                    fieldValuesByDefinitionId = mapOf("a" to "x", "b" to "y")
                )
            }
        }

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasTestTag("custom-field-group-Travel"))
        composeTestRule.onNodeWithTag("custom-field-group-Travel").assertIsDisplayed()
        composeTestRule.onNodeWithTag("custom-field-group-travel").assertIsDisplayed()
    }

    // --- Issue #830: per-contact custom-field value editing ---

    @Test
    fun `tapping the pencil reveals a text editor for a string field`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(
                        FieldDefinition(id = "d1", label = "Coffee order", type = "string")
                    ),
                    fieldValuesByDefinitionId = mapOf("d1" to "Latte")
                )
            }
        }

        openGroup()
        inGroup("Coffee order")
        composeTestRule.onNodeWithContentDescription("Edit Coffee order").performClick()

        inGroup("Value").assertIsDisplayed()
        inGroup("Save").assertIsDisplayed()
        inGroup("Cancel").assertIsDisplayed()
    }

    @Test
    @Config(sdk = [35], qualifiers = "w480dp-h1200dp")
    fun `a date custom field uses the shared picker and saves its wire value`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        var savedValue: Any? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(
                        FieldDefinition(id = "d1", label = "Renewal", type = "date")
                    ),
                    fieldValuesByDefinitionId = mapOf("d1" to "2026-08-10"),
                    onSaveFieldValue = { _, value -> savedValue = value }
                )
            }
        }

        openGroup()
        inGroup("Renewal")
        composeTestRule.onNodeWithContentDescription("Edit Renewal").performClick()
        // Rendered in the user's date format (EU by default) by the shared field.
        inGroup("10.08.2026")
        composeTestRule.onNodeWithTag("custom-field-date-d1").performClick()
        composeTestRule.onNodeWithText("Confirm").performClick()
        composeTestRule.onNodeWithText("Save").performClick()

        assertEquals("2026-08-10", savedValue)
    }

    @Test
    fun `editing and saving a string field invokes onSaveFieldValue with the typed wire value`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        var savedDefinitionId: String? = null
        var savedValue: Any? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(
                        FieldDefinition(id = "d1", label = "Coffee order", type = "string")
                    ),
                    fieldValuesByDefinitionId = mapOf("d1" to "Latte"),
                    onSaveFieldValue = { id, value ->
                        savedDefinitionId = id
                        savedValue = value
                    }
                )
            }
        }

        openGroup()
        inGroup("Coffee order")
        composeTestRule.onNodeWithContentDescription("Edit Coffee order").performClick()
        inGroup("Latte")
        composeTestRule.onNodeWithText("Latte").performTextReplacement("Macchiato")
        inGroup("Save")
        composeTestRule.onNodeWithText("Save").performClick()

        assertEquals("d1", savedDefinitionId)
        assertEquals("Macchiato", savedValue)
    }

    @Test
    fun `clearing a field's editor and saving passes null, not an empty string`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        var savedValue: Any? = "not yet called"
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(
                        FieldDefinition(id = "d1", label = "Coffee order", type = "string")
                    ),
                    fieldValuesByDefinitionId = mapOf("d1" to "Latte"),
                    onSaveFieldValue = { _, value -> savedValue = value }
                )
            }
        }

        openGroup()
        inGroup("Coffee order")
        composeTestRule.onNodeWithContentDescription("Edit Coffee order").performClick()
        inGroup("Latte")
        composeTestRule.onNodeWithText("Latte").performTextReplacement("")
        inGroup("Save")
        composeTestRule.onNodeWithText("Save").performClick()

        assertEquals(null, savedValue)
    }

    @Test
    fun `cancel discards the edit and reverts the displayed text`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        var saveCalled = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(
                        FieldDefinition(id = "d1", label = "Coffee order", type = "string")
                    ),
                    fieldValuesByDefinitionId = mapOf("d1" to "Latte"),
                    onSaveFieldValue = { _, _ -> saveCalled = true }
                )
            }
        }

        openGroup()
        inGroup("Coffee order")
        composeTestRule.onNodeWithContentDescription("Edit Coffee order").performClick()
        inGroup("Latte")
        composeTestRule.onNodeWithText("Latte").performTextInput("XYZ")
        inGroup("Cancel")
        composeTestRule.onNodeWithText("Cancel").performClick()

        inGroup("Latte").assertIsDisplayed()
        assertFalse(saveCalled)
    }

    @Test
    fun `saving equals true disables the Save button and shows a progress indicator`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(
                        FieldDefinition(id = "d1", label = "Coffee order", type = "string")
                    ),
                    fieldValuesByDefinitionId = mapOf("d1" to "Latte"),
                    savingFieldDefinitionId = "d1"
                )
            }
        }

        openGroup()
        inGroup("Coffee order")
        composeTestRule.onNodeWithContentDescription("Edit Coffee order").performClick()

        composeTestRule.onNodeWithText("Save").assertIsNotEnabled()
    }

    @Test
    fun `a boolean field's editor is a switch`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(
                        FieldDefinition(id = "d1", label = "VIP", type = "boolean")
                    ),
                    fieldValuesByDefinitionId = mapOf("d1" to true)
                )
            }
        }

        openGroup()
        inGroup("VIP")
        composeTestRule.onNodeWithContentDescription("Edit VIP").performClick()

        composeTestRule.onNode(isToggleable()).assertIsDisplayed()
    }

    @Test
    fun `an enum field's editor is a dropdown that saves the chosen option`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        var savedValue: Any? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(
                        FieldDefinition(
                            id = "d1",
                            label = "Milk",
                            type = "enum",
                            constraints = FieldConstraints(values = listOf("Oat", "Almond"))
                        )
                    ),
                    fieldValuesByDefinitionId = mapOf("d1" to "Oat"),
                    onSaveFieldValue = { _, value -> savedValue = value }
                )
            }
        }

        openGroup()
        inGroup("Milk")
        composeTestRule.onNodeWithContentDescription("Edit Milk").performClick()
        inGroup("Oat")
        composeTestRule.onNodeWithText("Oat").performClick()
        // The dropdown's options render in a Popup, outside the "contact-detail-list" scrollable
        // container, so they're found directly rather than via scrollTo.
        composeTestRule.onNodeWithText("Almond").performClick()
        inGroup("Save")
        composeTestRule.onNodeWithText("Save").performClick()

        assertEquals("Almond", savedValue)
    }

    @Test
    fun `a multi field's editor supports adding, editing and removing rows`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        var savedValue: Any? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(
                        FieldDefinition(
                            id = "d1",
                            label = "Milk options",
                            type = "string",
                            constraints = FieldConstraints(multi = true)
                        )
                    ),
                    fieldValuesByDefinitionId = mapOf("d1" to listOf("oat")),
                    onSaveFieldValue = { _, value -> savedValue = value }
                )
            }
        }

        openGroup()
        inGroup("Milk options")
        composeTestRule.onNodeWithContentDescription("Edit Milk options").performClick()
        inGroup("Add")
        composeTestRule.onNodeWithText("Add").performClick()
        // Every row's field shares the label "Value" — after Add there are two: the existing
        // "oat" row and the new empty one, in visual order, so index 1 is the new row.
        composeTestRule.onAllNodesWithText("Value")[1].performTextInput("almond")
        inGroup("Save")
        composeTestRule.onNodeWithText("Save").performClick()

        assertEquals(listOf("oat", "almond"), savedValue)
    }

    @Test
    fun `a multi field row can be removed, dropping it from the saved list`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        var savedValue: Any? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(
                        FieldDefinition(
                            id = "d1",
                            label = "Milk options",
                            type = "string",
                            constraints = FieldConstraints(multi = true)
                        )
                    ),
                    fieldValuesByDefinitionId = mapOf("d1" to listOf("oat", "almond")),
                    onSaveFieldValue = { _, value -> savedValue = value }
                )
            }
        }

        openGroup()
        inGroup("Milk options")
        composeTestRule.onNodeWithContentDescription("Edit Milk options").performClick()
        inGroup("oat")
        composeTestRule.onAllNodesWithContentDescription("Delete").onFirst().performClick()
        inGroup("Save")
        composeTestRule.onNodeWithText("Save").performClick()

        assertEquals(listOf("almond"), savedValue)
    }

    private fun assertScalarEditorRendersForType(type: String) {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(
                        FieldDefinition(id = "d1", label = "Field", type = type)
                    ),
                    fieldValuesByDefinitionId = mapOf("d1" to "value")
                )
            }
        }

        openGroup()
        inGroup("Field")
        composeTestRule.onNodeWithContentDescription("Edit Field").performClick()
        inGroup("Value")
        composeTestRule.onNodeWithText("Value").assertIsDisplayed()
    }

    @Test
    fun `a number field renders a text editor when opened`() = assertScalarEditorRendersForType(
        "number"
    )

    @Test
    fun `a text field renders a text editor when opened`() = assertScalarEditorRendersForType(
        "text"
    )

    @Test
    fun `an email field renders a text editor when opened`() = assertScalarEditorRendersForType(
        "email"
    )

    @Test
    fun `a phone field renders a text editor when opened`() = assertScalarEditorRendersForType(
        "phone"
    )

    @Test
    fun `a uri field renders a text editor when opened`() = assertScalarEditorRendersForType("uri")

    // --- M24: inline circle/tag editors ---

    @Test
    fun `circles render as removable chips and tags show the empty text`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        var removed: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    contactCircles = listOf(
                        com.mycorrhizal.crm.model.network.Circle(id = "c1", name = "friends")
                    ),
                    allCircles = listOf(
                        com.mycorrhizal.crm.model.network.Circle(id = "c1", name = "friends")
                    ),
                    contactTags = emptyList(),
                    onRemoveCircle = { removed = it.name }
                )
            }
        }

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("friends"))
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("No tags yet").assertIsDisplayed()
    }

    @Test
    fun `tapping a circle chip removes the membership`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        var removed: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    contactCircles = listOf(
                        com.mycorrhizal.crm.model.network.Circle(id = "c1", name = "friends")
                    ),
                    allCircles = listOf(
                        com.mycorrhizal.crm.model.network.Circle(id = "c1", name = "friends")
                    ),
                    onRemoveCircle = { removed = it.name }
                )
            }
        }

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("friends"))
        composeTestRule.onNodeWithText("friends").performClick()
        assertEquals("friends", removed)
    }

    @Test
    fun `the add menu lists circles the contact is not in`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        var added: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    contactCircles = listOf(
                        com.mycorrhizal.crm.model.network.Circle(id = "c1", name = "friends")
                    ),
                    allCircles = listOf(
                        com.mycorrhizal.crm.model.network.Circle(id = "c1", name = "friends"),
                        com.mycorrhizal.crm.model.network.Circle(id = "c2", name = "family")
                    ),
                    onAddCircle = { added = it.name }
                )
            }
        }

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("Add circle"))
        composeTestRule.onNodeWithText("Add circle").performClick()
        composeTestRule.onNodeWithText("family").performClick()
        assertEquals("family", added)
    }

    // --- Issue #219: tapping the avatar opens the profile-photo upload flow ---

    @Test
    fun `tapping the avatar invokes the profile-photo callback`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        var tapped = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    onUploadProfilePicture = { tapped = true }
                )
            }
        }

        composeTestRule.onNodeWithContentDescription("Photo of Dana White").performClick()
        assertTrue(tapped)
    }

    @Test
    fun `avatar without a photo callback is not clickable`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact)
            }
        }

        composeTestRule.onNodeWithContentDescription("Photo of Dana White")
            .assert(
                SemanticsMatcher("has no click action") { node ->
                    !node.config.contains(SemanticsActions.OnClick)
                }
            )
    }

    // --- Issue #220: External Links panel ---

    @Test
    fun `external links render as rows with a delete action`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        var deleted: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    externalIdentities = listOf(
                        ExternalIdentity(id = "i1", system = "paperless", externalId = "doc-1")
                    ),
                    onDeleteExternalIdentity = { deleted = it.id }
                )
            }
        }

        scrollTo("paperless")
        composeTestRule.onNodeWithText("paperless").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Remove link paperless").performClick()
        assertEquals("i1", deleted)
    }

    @Test
    fun `the immich link renders its person name and photo count`() {
        val contact = ContactRecordResponse(
            id = 5,
            uid = "u5",
            card = Card(name = Name(full = "Dana White"))
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    externalIdentities = listOf(
                        ExternalIdentity(
                            id = "i1",
                            entityId = "u5",
                            system = ExternalSystems.IMMICH,
                            externalId = "p1"
                        )
                    ),
                    immichSummary = ImmichPersonSummary(personName = "Alice", photoCount = 7)
                )
            }
        }

        scrollTo("Alice")
        composeTestRule.onNodeWithText("Alice").assertIsDisplayed()
        scrollTo("7 photos").assertIsDisplayed()
    }

    // --- Issue #836: Immich sync-now + external-activities timeline ---------

    @Test
    fun `the immich sync button fires the sync callback`() {
        val contact = ContactRecordResponse(
            id = 5,
            uid = "u5",
            card = Card(name = Name(full = "Dana White"))
        )
        var synced = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    externalIdentities = listOf(
                        ExternalIdentity(
                            id = "i1",
                            entityId = "u5",
                            system = ExternalSystems.IMMICH,
                            externalId = "p1"
                        )
                    ),
                    immichSummary = ImmichPersonSummary(personName = "Alice", photoCount = 7),
                    onSyncImmich = { synced = true }
                )
            }
        }

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasTestTag("immich-sync-button"))
        composeTestRule.onNodeWithTag("immich-sync-button").performClick()

        assertTrue(synced)
    }

    @Test
    fun `the immich sync button is disabled while syncing`() {
        val contact = ContactRecordResponse(
            id = 5,
            uid = "u5",
            card = Card(name = Name(full = "Dana White"))
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    externalIdentities = listOf(
                        ExternalIdentity(
                            id = "i1",
                            entityId = "u5",
                            system = ExternalSystems.IMMICH,
                            externalId = "p1"
                        )
                    ),
                    immichSummary = ImmichPersonSummary(personName = "Alice", photoCount = 7),
                    immichSyncing = true
                )
            }
        }

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasTestTag("immich-sync-button"))
        composeTestRule.onNodeWithTag("immich-sync-button").assertIsNotEnabled()
        // While syncing, the icon swaps for a progress indicator, so the
        // "Sync now" content description is gone too.
        composeTestRule.onNodeWithContentDescription("Sync now").assertDoesNotExist()
    }

    @Test
    fun `an external activity renders on the timeline`() {
        val contact = ContactRecordResponse(
            id = 5,
            uid = "u5",
            card = Card(name = Name(full = "Dana White"))
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    externalActivities = listOf(
                        ExternalActivity(
                            id = "a1",
                            entityId = "u5",
                            sourceSystem = "immich",
                            externalId = "asset-1",
                            type = "photo-appearance",
                            occurredAt = "2026-08-01T10:00:00Z",
                            payload = mapOf("person_name" to "Bob")
                        )
                    )
                )
            }
        }

        scrollTo("Photo appearance")
        composeTestRule.onNodeWithText("Photo appearance").assertIsDisplayed()
        scrollTo("Bob (via immich)").assertIsDisplayed()
    }

    @Test
    fun `external links section is absent when there are no identities`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact)
            }
        }

        composeTestRule.onNodeWithText("External links").assertDoesNotExist()
    }

    // --- Issue #236: "Add link" entry points ---

    @Test
    fun `external links section appears for a configured system with no links yet`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact, paperlessConfigured = true)
            }
        }

        scrollTo("External links")
        composeTestRule.onNodeWithText("External links").assertIsDisplayed()
        scrollTo("Add Paperless link").assertIsDisplayed()
    }

    @Test
    fun `add link buttons are gated per system`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    paperlessConfigured = true,
                    seafileConfigured = false,
                    nextcloudConfigured = false
                )
            }
        }

        scrollTo("Add Paperless link").assertIsDisplayed()
        composeTestRule.onNodeWithText("Add Seafile link").assertDoesNotExist()
        composeTestRule.onNodeWithText("Add Nextcloud link").assertDoesNotExist()
    }

    @Test
    fun `tapping an add link button invokes its callback`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        var tapped = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    seafileConfigured = true,
                    onAddSeafileLink = { tapped = true }
                )
            }
        }

        scrollTo("Add Seafile link")
        composeTestRule.onNodeWithText("Add Seafile link").performClick()
        assertTrue(tapped)
    }

    // --- M15: the share entry point lives in the header's action menu ---

    @Test
    fun `share contact in the action menu invokes onShareContact with the vcard uid`() {
        // M15 ticket test case 4: the share action must be reachable from a
        // contact's own header, not only from a shares list. This renders the
        // FULL ContactDetailScreen (not just ContactDetailContent) with a
        // mocked ViewModel so the top-bar action menu is present.
        val contact = ContactRecordResponse(
            id = 5,
            uid = "uid-5",
            card = Card(name = Name(full = "Dana White"))
        )
        val viewModel = mockk<ContactDetailViewModel>(relaxed = true)
        every { viewModel.uiState } returns MutableStateFlow(
            ContactDetailUiState(contact = contact)
        )
        every { viewModel.events } returns MutableStateFlow<ContactDetailEvent?>(null)

        var sharedUid: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailScreen(
                    onBack = {},
                    onShareContact = { sharedUid = it },
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.onNodeWithContentDescription("Contact actions").performClick()
        composeTestRule.onNodeWithText("Share contact").performClick()

        assertEquals("uid-5", sharedUid)
    }

    // --- v1.5.0 review / #1893: a refresh keeps the page on screen ---

    @Test
    fun `a reload with a contact already on screen keeps the content instead of the skeleton`() {
        // The ON_RESUME reload after a drill-down sets isLoading while the
        // previous record is still in state; the page must stay (with its
        // scroll position) rather than flash the skeleton.
        val contact = ContactRecordResponse(
            id = 5,
            uid = "uid-5",
            card = Card(name = Name(full = "Dana White"))
        )
        val viewModel = mockk<ContactDetailViewModel>(relaxed = true)
        every { viewModel.uiState } returns MutableStateFlow(
            ContactDetailUiState(contact = contact, isLoading = true)
        )
        every { viewModel.events } returns MutableStateFlow<ContactDetailEvent?>(null)

        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailScreen(
                    onBack = {},
                    onShareContact = {},
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.onNodeWithTag("contact-detail-list").assertExists()
        composeTestRule.onNodeWithContentDescription("Loading").assertDoesNotExist()
    }

    @Test
    fun `the first load with no contact yet shows the skeleton`() {
        val viewModel = mockk<ContactDetailViewModel>(relaxed = true)
        every { viewModel.uiState } returns MutableStateFlow(ContactDetailUiState(isLoading = true))
        every { viewModel.events } returns MutableStateFlow<ContactDetailEvent?>(null)

        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailScreen(
                    onBack = {},
                    onShareContact = {},
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.onNodeWithContentDescription("Loading").assertExists()
        composeTestRule.onNodeWithTag("contact-detail-list").assertDoesNotExist()
    }

    // --- T90 / issue #831: Mark as Me / Unmark as Me in the action menu (web parity) ---

    @Test
    fun `mark as me in the action menu invokes toggleMe`() {
        val contact = ContactRecordResponse(
            id = 5,
            uid = "uid-5",
            card = Card(name = Name(full = "Dana White"))
        )
        val viewModel = mockk<ContactDetailViewModel>(relaxed = true)
        every { viewModel.uiState } returns MutableStateFlow(
            ContactDetailUiState(contact = contact, selfContactVCardUid = null)
        )
        every { viewModel.events } returns MutableStateFlow<ContactDetailEvent?>(null)

        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailScreen(onBack = {}, onShareContact = {}, viewModel = viewModel)
            }
        }

        composeTestRule.onNodeWithContentDescription("Contact actions").performClick()
        composeTestRule.onNodeWithText("This is me").performClick()

        verify { viewModel.toggleMe() }
    }

    @Test
    fun `action menu shows unmark as me when this contact is already the self contact`() {
        val contact = ContactRecordResponse(
            id = 5,
            uid = "uid-5",
            card = Card(name = Name(full = "Dana White"))
        )
        val viewModel = mockk<ContactDetailViewModel>(relaxed = true)
        every { viewModel.uiState } returns MutableStateFlow(
            ContactDetailUiState(contact = contact, selfContactVCardUid = "uid-5")
        )
        every { viewModel.events } returns MutableStateFlow<ContactDetailEvent?>(null)

        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailScreen(onBack = {}, onShareContact = {}, viewModel = viewModel)
            }
        }

        composeTestRule.onNodeWithContentDescription("Contact actions").performClick()

        composeTestRule.onNodeWithText("This isn't me").assertIsDisplayed()
    }

    @Test
    fun `a section caption is marked as a heading`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                phones = listOf(Phone(number = "+1-555-0100"))
            )
        )
        setContent(ContactDetailUiState(contact = contact))

        // #208: section captions carried no heading semantics, so TalkBack's
        // heading navigation found nothing on this screen. One fix in
        // SectionCard covers every card's caption.
        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("Phone"))
        composeTestRule.onNodeWithText("Phone")
            .assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading))
    }

    @Test
    fun `the contact name is marked as a heading`() {
        val contact = ContactRecordResponse(
            id = 5,
            uid = "uid-5",
            card = Card(name = Name(full = "Dana White"))
        )
        val viewModel = mockk<ContactDetailViewModel>(relaxed = true)
        every { viewModel.uiState } returns MutableStateFlow(
            ContactDetailUiState(contact = contact)
        )
        every { viewModel.events } returns MutableStateFlow<ContactDetailEvent?>(null)

        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailScreen(onBack = {}, onShareContact = {}, viewModel = viewModel)
            }
        }

        // #208: the contact name is the de facto page heading but carried no
        // heading semantics. Two "Dana White" nodes exist (the collapsed
        // TopAppBar title, and the large centered name below it) -- only the
        // latter is the one this fix marks.
        composeTestRule.onNode(
            hasText("Dana White").and(SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading))
        )
            .assertIsDisplayed()
    }

    // --- Issue #1401: the timeline is a bounded preview + "View all" (web T78 parity) ---

    private fun photoAppearances(count: Int) = (1..count).map { i ->
        ExternalActivity(
            id = "a$i",
            entityId = "u5",
            sourceSystem = "immich",
            externalId = "asset-$i",
            type = "photo-appearance",
            occurredAt = "2026-08-01T10:%02d:00Z".format(i % 60),
            payload = mapOf("person_name" to "Bob")
        )
    }

    @Test
    fun `the timeline preview is capped at five events however many exist`() {
        val contact = ContactRecordResponse(
            id = 5,
            uid = "u5",
            card = Card(name = Name(full = "Dana White"))
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact, externalActivities = photoAppearances(100))
            }
        }

        scrollTo("Photo appearance")
        assertEquals(
            5,
            composeTestRule.onAllNodesWithText("Photo appearance").fetchSemanticsNodes().size
        )
    }

    @Test
    fun `the preview shows the five most recent events across notes activities and external activities`() {
        val contact = ContactRecordResponse(
            id = 5,
            uid = "u5",
            card = Card(name = Name(full = "Dana White")),
            notes = listOf(
                com.mycorrhizal.crm.model.network.Note(
                    id = 1,
                    content = "ancient note",
                    date = "2020-01-01T00:00:00Z"
                ),
                com.mycorrhizal.crm.model.network.Note(
                    id = 2,
                    content = "newest note",
                    date = "2026-09-20T00:00:00Z"
                )
            ),
            activities = listOf(
                com.mycorrhizal.crm.model.network.Activity(
                    id = 3,
                    title = "newest activity",
                    date = "2026-09-19T00:00:00Z"
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact, externalActivities = photoAppearances(5))
            }
        }

        scrollTo("newest note")
        composeTestRule.onNodeWithText("newest note").assertExists()
        composeTestRule.onNodeWithText("newest activity").assertExists()
        // 3 + 5 = 8 merged events; only the 5 newest remain: 2 recent notes/activities + 3 photos.
        composeTestRule.onNodeWithText("ancient note").assertDoesNotExist()
        assertEquals(
            3,
            composeTestRule.onAllNodesWithText("Photo appearance").fetchSemanticsNodes().size
        )
    }

    @Test
    fun `profile sections render above a large timeline without scrolling past it`() {
        val contact = ContactRecordResponse(
            id = 5,
            uid = "u5",
            card = Card(
                name = Name(full = "Dana White"),
                phones = listOf(Phone(number = "+1-555-0100")),
                emails = listOf(Email(address = "dana@example.com", label = "Work"))
            ),
            crm = CRMEnvelope(circles = listOf("friends"))
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact, externalActivities = photoAppearances(100))
            }
        }

        // No scrolling: the profile data is on the first screen, not buried under 100 timeline rows.
        composeTestRule.onNodeWithText("+1-555-0100").assertIsDisplayed()
        composeTestRule.onNodeWithText("dana@example.com", substring = true).assertIsDisplayed()
        // And the management rows below the timeline are one short scroll away, not 100 rows away.
        composeTestRule.onNodeWithTag(
            "contact-detail-list"
        ).performScrollToNode(hasText("Reminders · 0"))
        composeTestRule.onNodeWithText("Reminders · 0").assertIsDisplayed()
        assertTrue(
            composeTestRule.onAllNodesWithText("Photo appearance").fetchSemanticsNodes().size <= 5
        )
    }

    @Test
    fun `view all is shown even with no events and opens the explorer for this contact`() {
        val contact = ContactRecordResponse(
            id = 5,
            uid = "u5",
            card = Card(name = Name(full = "Dana White"))
        )
        var opened: Int? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact, onViewTimeline = { opened = it })
            }
        }

        scrollTo("View all")
        composeTestRule.onNodeWithText("View all").assertIsDisplayed().performClick()

        assertEquals(5, opened)
    }

    // Issue #1665 / ADR 0008: a 412 on delete shows the "not deleted" prompt on the detail screen.
    @Test
    fun `a delete conflict shows the prompt and routes both buttons to the view model`() {
        val contact = ContactRecordResponse(
            id = 5,
            uid = "uid-5",
            card = Card(name = Name(full = "Dana White"))
        )
        val viewModel = mockk<ContactDetailViewModel>(relaxed = true)
        every { viewModel.uiState } returns MutableStateFlow(
            ContactDetailUiState(contact = contact, conflict = true)
        )
        every { viewModel.events } returns MutableStateFlow<ContactDetailEvent?>(null)

        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailScreen(
                    onBack = {},
                    onShareContact = {},
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.onNodeWithText("Changed elsewhere").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "so it was not deleted",
            substring = true
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText("Load latest version").performClick()
        verify(exactly = 1) { viewModel.onConflictLoadLatest() }
        composeTestRule.onNodeWithText("Not now").performClick()
        verify(exactly = 1) { viewModel.onConflictDismissed() }
    }

    @Test
    fun `no conflict prompt on the detail screen without a conflict`() {
        val contact = ContactRecordResponse(
            id = 5,
            uid = "uid-5",
            card = Card(name = Name(full = "Dana White"))
        )
        val viewModel = mockk<ContactDetailViewModel>(relaxed = true)
        every { viewModel.uiState } returns MutableStateFlow(
            ContactDetailUiState(contact = contact)
        )
        every { viewModel.events } returns MutableStateFlow<ContactDetailEvent?>(null)

        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailScreen(
                    onBack = {},
                    onShareContact = {},
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.onNodeWithText("Changed elsewhere").assertDoesNotExist()
    }

    // --- Issue #1763: grouped drill-downs with icons and counts ---

    @Test
    fun `drill-down rows are grouped and carry their trailing counts`() {
        val contact = ContactRecordResponse(
            id = 5,
            uid = "u5",
            card = Card(name = Name(full = "Dana White")),
            activities = listOf(
                Activity(id = 1, title = "Coffee"),
                Activity(id = 2, title = "Walk")
            ),
            notes = listOf(Note(id = 3, content = "Met at the park")),
            reminders = listOf(Reminder(id = 4, message = "Call"))
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    lifeEventCount = 3,
                    giftCount = 2,
                    preferenceCount = 4,
                    agendaCount = 0,
                    relationshipCount = 5,
                    attachmentCount = 1,
                    cadenceCount = 1
                )
            }
        }

        scrollTo("History")
        composeTestRule.onNodeWithText("History").assertIsDisplayed()
        scrollTo("Activities · 2")
        composeTestRule.onNodeWithText("Activities · 2").assertIsDisplayed()
        scrollTo("Notes · 1")
        composeTestRule.onNodeWithText("Notes · 1").assertIsDisplayed()
        scrollTo("Life events · 3")
        composeTestRule.onNodeWithText("Life events · 3").assertIsDisplayed()

        scrollTo("Planning")
        composeTestRule.onNodeWithText("Planning").assertIsDisplayed()
        scrollTo("Reminders · 1")
        composeTestRule.onNodeWithText("Reminders · 1").assertIsDisplayed()
        scrollTo("Cadence · 1")
        composeTestRule.onNodeWithText("Cadence · 1").assertIsDisplayed()
        scrollTo("Conversation agenda · 0")
        composeTestRule.onNodeWithText("Conversation agenda · 0").assertIsDisplayed()
        scrollTo("Gifts · 2")
        composeTestRule.onNodeWithText("Gifts · 2").assertIsDisplayed()

        scrollTo("About")
        composeTestRule.onNodeWithText("About").assertIsDisplayed()
        scrollTo("Preferences · 4")
        composeTestRule.onNodeWithText("Preferences · 4").assertIsDisplayed()
        scrollTo("Relationships · 5")
        composeTestRule.onNodeWithText("Relationships · 5").assertIsDisplayed()
        scrollTo("Attachments · 1")
        composeTestRule.onNodeWithText("Attachments · 1").assertIsDisplayed()

        // Issue #1763: merge is an action, not a data section.
        composeTestRule.onNodeWithText("Merge contacts").assertDoesNotExist()
    }

    @Test
    fun `an unknown count renders the label without a number`() {
        val contact = ContactRecordResponse(
            id = 5,
            uid = "u5",
            card = Card(name = Name(full = "Dana White"))
        )
        composeTestRule.setContent {
            MycorrhizalTheme { ContactDetailContent(contact = contact) }
        }

        scrollTo("Life events")
        composeTestRule.onNodeWithText("Life events").assertIsDisplayed()
        composeTestRule.onNodeWithText("Life events · 0").assertDoesNotExist()
    }

    @Test
    fun `merge is reachable from the overflow menu`() {
        val contact = ContactRecordResponse(
            id = 5,
            uid = "uid-5",
            card = Card(name = Name(full = "Dana White"))
        )
        val viewModel = mockk<ContactDetailViewModel>(relaxed = true)
        every { viewModel.uiState } returns MutableStateFlow(
            ContactDetailUiState(contact = contact)
        )
        every { viewModel.events } returns MutableStateFlow<ContactDetailEvent?>(null)

        var mergedId: Int? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailScreen(onBack = {}, onMerge = { mergedId = it }, viewModel = viewModel)
            }
        }

        composeTestRule.onNodeWithContentDescription("Contact actions").performClick()
        composeTestRule.onNodeWithText("Merge contacts").performClick()

        assertEquals(5, mergedId)
    }

    @Test
    fun `export is a submenu offering both vCard versions`() {
        val contact = ContactRecordResponse(
            id = 5,
            uid = "uid-5",
            card = Card(name = Name(full = "Dana White"))
        )
        val viewModel = mockk<ContactDetailViewModel>(relaxed = true)
        every { viewModel.uiState } returns MutableStateFlow(
            ContactDetailUiState(contact = contact)
        )
        every { viewModel.events } returns MutableStateFlow<ContactDetailEvent?>(null)

        composeTestRule.setContent {
            MycorrhizalTheme { ContactDetailScreen(onBack = {}, viewModel = viewModel) }
        }

        composeTestRule.onNodeWithContentDescription("Contact actions").performClick()
        composeTestRule.onNodeWithText("Export").performClick()
        composeTestRule.onNodeWithText("Export vCard 4.0").assertIsDisplayed()
        composeTestRule.onNodeWithText("Export vCard 3.0").performClick()

        verify { viewModel.exportVcf(version = 3) }
    }
}
