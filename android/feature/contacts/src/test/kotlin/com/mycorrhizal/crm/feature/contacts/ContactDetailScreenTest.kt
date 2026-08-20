package com.mycorrhizal.crm.feature.contacts

import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.ui.semantics.SemanticsActions
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
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToNode
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.Anniversary
import com.mycorrhizal.crm.model.network.AnniversaryDate
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.ExternalIdentity
import com.mycorrhizal.crm.model.network.ExternalSystems
import com.mycorrhizal.crm.model.network.FieldConstraints
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.ImmichPersonSummary
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
                ContactDetailContent(contact = state.contact!!)
            }
        }
    }

    // --- Issue #212: header star toggle (web #173) ---------------------------

    @Test
    fun `favorite header star renders a labeled toggle`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White")),
                        isFavorite = true,
                    ),
                )
            }
        }
        composeTestRule.onNodeWithContentDescription("Unmark Dana White as favorite").assertIsDisplayed()
    }

    @Test
    fun `non-favorite header star renders a labeled toggle`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(
                        id = 5,
                        card = Card(name = Name(full = "Dana White")),
                        isFavorite = false,
                    ),
                )
            }
        }
        composeTestRule.onNodeWithContentDescription("Mark Dana White as favorite").assertIsDisplayed()
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
                        isFavorite = false,
                    ),
                    onToggleFavorite = { toggles++ },
                )
            }
        }

        composeTestRule.onNodeWithContentDescription("Mark Dana White as favorite").performClick()

        assertEquals(1, toggles)
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
            .performScrollToNode(hasText("Reminders"))
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
        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("Favorite number: 7"))
            .assertIsDisplayed()
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
        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("Milk options: oat; almond"))
            .assertIsDisplayed()
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

    // --- Issue #219: tapping the avatar opens the profile-photo upload flow ---

    @Test
    fun `tapping the avatar invokes the profile-photo callback`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        var tapped = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    onUploadProfilePicture = { tapped = true },
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
            .assert(SemanticsMatcher("has no click action") { node ->
                !node.config.contains(SemanticsActions.OnClick)
            })
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
                        ExternalIdentity(id = "i1", system = "paperless", externalId = "doc-1"),
                    ),
                    onDeleteExternalIdentity = { deleted = it.id },
                )
            }
        }

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("paperless"))
        composeTestRule.onNodeWithText("paperless").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Remove link paperless").performClick()
        assertEquals("i1", deleted)
    }

    @Test
    fun `the immich link renders its person name and photo count`() {
        val contact = ContactRecordResponse(id = 5, uid = "u5", card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    externalIdentities = listOf(
                        ExternalIdentity(id = "i1", entityId = "u5", system = ExternalSystems.IMMICH, externalId = "p1"),
                    ),
                    immichSummary = ImmichPersonSummary(personName = "Alice", photoCount = 7),
                )
            }
        }

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("Alice"))
        composeTestRule.onNodeWithText("Alice").assertIsDisplayed()
        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("7 photos"))
            .assertIsDisplayed()
    }

    @Test
    fun `external links section is absent when there are no identities`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact)
            }
        }

        composeTestRule.onNodeWithText("External Links").assertDoesNotExist()
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

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("External Links"))
        composeTestRule.onNodeWithText("External Links").assertIsDisplayed()
        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("Add Paperless link"))
            .assertIsDisplayed()
    }

    @Test
    fun `add link buttons are gated per system`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact, paperlessConfigured = true, seafileConfigured = false, nextcloudConfigured = false)
            }
        }

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("Add Paperless link"))
            .assertIsDisplayed()
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
                    onAddSeafileLink = { tapped = true },
                )
            }
        }

        composeTestRule.onNodeWithTag("contact-detail-list")
            .performScrollToNode(hasText("Add Seafile link"))
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

    @Test
    fun `a section caption is marked as a heading`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                phones = listOf(Phone(number = "+1-555-0100")),
            ),
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
        val contact = ContactRecordResponse(id = 5, uid = "uid-5", card = Card(name = Name(full = "Dana White")))
        val viewModel = mockk<ContactDetailViewModel>(relaxed = true)
        every { viewModel.uiState } returns MutableStateFlow(ContactDetailUiState(contact = contact))
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
        composeTestRule.onNode(hasText("Dana White").and(SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading)))
            .assertIsDisplayed()
    }
}
