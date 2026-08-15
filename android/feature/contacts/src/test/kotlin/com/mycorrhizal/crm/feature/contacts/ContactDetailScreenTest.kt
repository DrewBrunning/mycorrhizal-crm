package com.mycorrhizal.crm.feature.contacts

import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToNode
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.Anniversary
import com.mycorrhizal.crm.model.network.AnniversaryDate
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.FieldConstraints
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.OnlineService
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.network.Resource
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.flow.MutableStateFlow
import org.junit.Assert.assertEquals
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
                ContactDetailContent(contact = state.contact!!)
            }
        }
    }

    @Test
    fun `renders the contact name and sections`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                emails = listOf(Email(address = "dana@example.com", label = "Work")),
                phones = listOf(Phone(number = "+1-555-0100")),
            ),
            crm = CRMEnvelope(circles = listOf("friends", "work")),
        )
        setContent(ContactDetailUiState(contact = contact))

        // The name is owned by the collapsing app bar (ContactDetailScreen).
        // The body renders the overview info first (web order), then the
        // timeline, then the management entry rows — scroll the list to them.
        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("Activities"))
        composeTestRule.onNodeWithText("Activities").assertIsDisplayed()
        composeTestRule.onNodeWithText("Notes").assertIsDisplayed()
        composeTestRule.onNodeWithText("Reminders").assertIsDisplayed()
    }

    @Test
    fun `renders empty name fallback when no name present`() {
        val contact = ContactRecordResponse(id = 5, card = Card())
        setContent(ContactDetailUiState(contact = contact))
        // No nickname/birthday/name in the body for a nameless contact — the
        // list still renders (timeline + management entries).
        composeTestRule.onNodeWithText("Timeline").assertIsDisplayed()
    }

    @Test
    fun `phone row renders call and copy actions`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                phones = listOf(Phone(number = "+1-555-0100")),
            ),
        )
        setContent(ContactDetailUiState(contact = contact))

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasContentDescription("Call"))
            .assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Copy phone number").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `mobile phone renders an sms action`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                phones = listOf(Phone(number = "+1-555-0100", features = listOf("cell"))),
            ),
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
                phones = listOf(Phone(number = "+1-555-0100")),
            ),
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
                phones = listOf(Phone(number = "+1-555-0100", contexts = listOf("cell"))),
            ),
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
                phones = listOf(Phone(number = "+1-555-0100", label = "mobile")),
            ),
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
                emails = listOf(Email(address = "dana@example.com")),
            ),
        )
        setContent(ContactDetailUiState(contact = contact))

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasContentDescription("Compose email"))
            .assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Copy email").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `address row renders map and copy actions`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                addresses = listOf(com.mycorrhizal.crm.model.network.Address(full = "123 Main St")),
            ),
        )
        setContent(ContactDetailUiState(contact = contact))

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasContentDescription("Open in maps"))
            .assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Copy address").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `link section renders open and copy actions`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                links = listOf(Resource(uri = "https://example.com/profile", label = "Website")),
            ),
        )
        setContent(ContactDetailUiState(contact = contact))

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasContentDescription("Open link"))
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("Website").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Copy link").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `birthday renders using the eu format by default`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                anniversaries = listOf(
                    Anniversary(kind = "birth", date = AnniversaryDate(partial = PartialDate(year = 1990, month = 6, day = 15))),
                ),
            ),
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact)
            }
        }

        composeTestRule.onNodeWithText("Birthday: 15 June 1990").assertIsDisplayed()
    }

    @Test
    fun `birthday honors the user's date_format preference`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                anniversaries = listOf(
                    Anniversary(kind = "birth", date = AnniversaryDate(partial = PartialDate(year = 1990, month = 6, day = 15))),
                ),
            ),
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact, dateFormat = "us")
            }
        }

        composeTestRule.onNodeWithText("Birthday: June 15, 1990").assertIsDisplayed()
    }

    @Test
    fun `online service row renders service name and handle`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                imppAddresses = listOf(
                    OnlineService(service = "Signal", uri = "6085142711"),
                ),
            ),
        )
        setContent(ContactDetailUiState(contact = contact))

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
                        FieldDefinition(id = "d2", label = "Favorite number", type = "number"),
                    ),
                    fieldValuesByDefinitionId = mapOf("d1" to "Latte", "d2" to 7.0),
                )
            }
        }

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("Coffee order: Latte"))
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("Favorite number: 7").assertIsDisplayed()
    }

    @Test
    fun `a definition with no value shows the placeholder, not a crash`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(FieldDefinition(id = "d1", label = "Coffee order", type = "string")),
                    fieldValuesByDefinitionId = emptyMap(),
                )
            }
        }

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("Coffee order: —"))
            .assertIsDisplayed()
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
                    fieldValuesByDefinitionId = mapOf("deleted-def" to "orphaned value"),
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
                            constraints = FieldConstraints(multi = true),
                        ),
                    ),
                    fieldValuesByDefinitionId = mapOf("d1" to true, "d2" to listOf("oat", "almond")),
                )
            }
        }

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("VIP: true"))
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("Milk options: oat; almond").assertIsDisplayed()
    }

    // --- M24: inline circle/tag editors ---

    @Test
    fun `circles render as removable chips and tags show the empty text`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        var removed: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    contactCircles = listOf(com.mycorrhizal.crm.model.network.Circle(id = "c1", name = "friends")),
                    allCircles = listOf(com.mycorrhizal.crm.model.network.Circle(id = "c1", name = "friends")),
                    contactTags = emptyList(),
                    onRemoveCircle = { removed = it.name },
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
                    contactCircles = listOf(com.mycorrhizal.crm.model.network.Circle(id = "c1", name = "friends")),
                    allCircles = listOf(com.mycorrhizal.crm.model.network.Circle(id = "c1", name = "friends")),
                    onRemoveCircle = { removed = it.name },
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
                    contactCircles = listOf(com.mycorrhizal.crm.model.network.Circle(id = "c1", name = "friends")),
                    allCircles = listOf(
                        com.mycorrhizal.crm.model.network.Circle(id = "c1", name = "friends"),
                        com.mycorrhizal.crm.model.network.Circle(id = "c2", name = "family"),
                    ),
                    onAddCircle = { added = it.name },
                )
            }
        }

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("Add circle"))
        composeTestRule.onNodeWithText("Add circle").performClick()
        composeTestRule.onNodeWithText("family").performClick()
        assertEquals("family", added)
    }

    // --- M15: the share entry point lives in the header's action menu ---

    @Test
    fun `share contact in the action menu invokes onShareContact with the vcard uid`() {
        // M15 ticket test case 4: the share action must be reachable from a
        // contact's own header, not only from a shares list. This renders the
        // FULL ContactDetailScreen (not just ContactDetailContent) with a
        // mocked ViewModel so the top-bar action menu is present.
        val contact = ContactRecordResponse(id = 5, uid = "uid-5", card = Card(name = Name(full = "Dana White")))
        val viewModel = mockk<ContactDetailViewModel>(relaxed = true)
        every { viewModel.uiState } returns MutableStateFlow(ContactDetailUiState(contact = contact))
        every { viewModel.events } returns MutableStateFlow<ContactDetailEvent?>(null)

        var sharedUid: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailScreen(
                    onBack = {},
                    onShareContact = { sharedUid = it },
                    viewModel = viewModel,
                )
            }
        }

        composeTestRule.onNodeWithContentDescription("Contact actions").performClick()
        composeTestRule.onNodeWithText("Share contact").performClick()

        assertEquals("uid-5", sharedUid)
    }
}
