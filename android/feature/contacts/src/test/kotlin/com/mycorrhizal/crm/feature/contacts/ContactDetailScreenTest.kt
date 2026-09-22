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
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onFirst
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.isToggleable
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToNode
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
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
import io.mockk.verify
import kotlinx.coroutines.flow.MutableStateFlow
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
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

    /**
     * Scrolls the lazy list until a node matching [text] is composed (off-screen items aren't),
     * returning that scroll's node so callers can chain e.g. `.assertIsDisplayed()`.
     */
    private fun scrollTo(text: String) =
        composeTestRule.onNodeWithTag("contact-detail-list").performScrollToNode(hasText(text))

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

    // --- T90 / issue #831: "You" badge + Mark as Me (web parity) ------------

    @Test
    fun `you badge shows when isMe is true`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White"))),
                    isMe = true,
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
                    contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White"))),
                    isMe = false,
                )
            }
        }
        composeTestRule.onNodeWithTag("you-badge").assertDoesNotExist()
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

        scrollTo("Coffee order").assertIsDisplayed()
        scrollTo("Latte").assertIsDisplayed()
        scrollTo("Favorite number").assertIsDisplayed()
        scrollTo("7").assertIsDisplayed()
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

        scrollTo("Coffee order").assertIsDisplayed()
        scrollTo("—").assertIsDisplayed()
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

        scrollTo("VIP").assertIsDisplayed()
        scrollTo("true").assertIsDisplayed()
        scrollTo("Milk options").assertIsDisplayed()
        scrollTo("oat; almond").assertIsDisplayed()
    }

    // --- Issue #830: per-contact custom-field value editing ---

    @Test
    fun `tapping the pencil reveals a text editor for a string field`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(FieldDefinition(id = "d1", label = "Coffee order", type = "string")),
                    fieldValuesByDefinitionId = mapOf("d1" to "Latte"),
                )
            }
        }

        scrollTo("Coffee order")
        composeTestRule.onNodeWithContentDescription("Edit Coffee order").performClick()

        scrollTo("Value").assertIsDisplayed()
        scrollTo("Save").assertIsDisplayed()
        scrollTo("Cancel").assertIsDisplayed()
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
                    fieldDefinitions = listOf(FieldDefinition(id = "d1", label = "Coffee order", type = "string")),
                    fieldValuesByDefinitionId = mapOf("d1" to "Latte"),
                    onSaveFieldValue = { id, value -> savedDefinitionId = id; savedValue = value },
                )
            }
        }

        scrollTo("Coffee order")
        composeTestRule.onNodeWithContentDescription("Edit Coffee order").performClick()
        scrollTo("Latte")
        composeTestRule.onNodeWithText("Latte").performTextReplacement("Macchiato")
        scrollTo("Save")
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
                    fieldDefinitions = listOf(FieldDefinition(id = "d1", label = "Coffee order", type = "string")),
                    fieldValuesByDefinitionId = mapOf("d1" to "Latte"),
                    onSaveFieldValue = { _, value -> savedValue = value },
                )
            }
        }

        scrollTo("Coffee order")
        composeTestRule.onNodeWithContentDescription("Edit Coffee order").performClick()
        scrollTo("Latte")
        composeTestRule.onNodeWithText("Latte").performTextReplacement("")
        scrollTo("Save")
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
                    fieldDefinitions = listOf(FieldDefinition(id = "d1", label = "Coffee order", type = "string")),
                    fieldValuesByDefinitionId = mapOf("d1" to "Latte"),
                    onSaveFieldValue = { _, _ -> saveCalled = true },
                )
            }
        }

        scrollTo("Coffee order")
        composeTestRule.onNodeWithContentDescription("Edit Coffee order").performClick()
        scrollTo("Latte")
        composeTestRule.onNodeWithText("Latte").performTextInput("XYZ")
        scrollTo("Cancel")
        composeTestRule.onNodeWithText("Cancel").performClick()

        scrollTo("Latte").assertIsDisplayed()
        assertFalse(saveCalled)
    }

    @Test
    fun `saving equals true disables the Save button and shows a progress indicator`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(FieldDefinition(id = "d1", label = "Coffee order", type = "string")),
                    fieldValuesByDefinitionId = mapOf("d1" to "Latte"),
                    savingFieldDefinitionId = "d1",
                )
            }
        }

        scrollTo("Coffee order")
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
                    fieldDefinitions = listOf(FieldDefinition(id = "d1", label = "VIP", type = "boolean")),
                    fieldValuesByDefinitionId = mapOf("d1" to true),
                )
            }
        }

        scrollTo("VIP")
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
                            constraints = FieldConstraints(values = listOf("Oat", "Almond")),
                        ),
                    ),
                    fieldValuesByDefinitionId = mapOf("d1" to "Oat"),
                    onSaveFieldValue = { _, value -> savedValue = value },
                )
            }
        }

        scrollTo("Milk")
        composeTestRule.onNodeWithContentDescription("Edit Milk").performClick()
        scrollTo("Oat")
        composeTestRule.onNodeWithText("Oat").performClick()
        // The dropdown's options render in a Popup, outside the "contact-detail-list" scrollable
        // container, so they're found directly rather than via scrollTo.
        composeTestRule.onNodeWithText("Almond").performClick()
        scrollTo("Save")
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
                            constraints = FieldConstraints(multi = true),
                        ),
                    ),
                    fieldValuesByDefinitionId = mapOf("d1" to listOf("oat")),
                    onSaveFieldValue = { _, value -> savedValue = value },
                )
            }
        }

        scrollTo("Milk options")
        composeTestRule.onNodeWithContentDescription("Edit Milk options").performClick()
        scrollTo("Add")
        composeTestRule.onNodeWithText("Add").performClick()
        // Every row's field shares the label "Value" — after Add there are two: the existing
        // "oat" row and the new empty one, in visual order, so index 1 is the new row.
        composeTestRule.onAllNodesWithText("Value")[1].performTextInput("almond")
        scrollTo("Save")
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
                            constraints = FieldConstraints(multi = true),
                        ),
                    ),
                    fieldValuesByDefinitionId = mapOf("d1" to listOf("oat", "almond")),
                    onSaveFieldValue = { _, value -> savedValue = value },
                )
            }
        }

        scrollTo("Milk options")
        composeTestRule.onNodeWithContentDescription("Edit Milk options").performClick()
        scrollTo("oat")
        composeTestRule.onAllNodesWithContentDescription("Delete").onFirst().performClick()
        scrollTo("Save")
        composeTestRule.onNodeWithText("Save").performClick()

        assertEquals(listOf("almond"), savedValue)
    }

    private fun assertScalarEditorRendersForType(type: String) {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(
                    contact = contact,
                    fieldDefinitions = listOf(FieldDefinition(id = "d1", label = "Field", type = type)),
                    fieldValuesByDefinitionId = mapOf("d1" to "value"),
                )
            }
        }

        scrollTo("Field")
        composeTestRule.onNodeWithContentDescription("Edit Field").performClick()
        scrollTo("Value")
        composeTestRule.onNodeWithText("Value").assertIsDisplayed()
    }

    @Test
    fun `a number field renders a text editor when opened`() = assertScalarEditorRendersForType("number")

    @Test
    fun `a text field renders a text editor when opened`() = assertScalarEditorRendersForType("text")

    @Test
    fun `an email field renders a text editor when opened`() = assertScalarEditorRendersForType("email")

    @Test
    fun `a phone field renders a text editor when opened`() = assertScalarEditorRendersForType("phone")

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

        scrollTo("paperless")
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

        scrollTo("Alice")
        composeTestRule.onNodeWithText("Alice").assertIsDisplayed()
        scrollTo("7 photos").assertIsDisplayed()
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

        scrollTo("External Links")
        composeTestRule.onNodeWithText("External Links").assertIsDisplayed()
        scrollTo("Add Paperless link").assertIsDisplayed()
    }

    @Test
    fun `add link buttons are gated per system`() {
        val contact = ContactRecordResponse(id = 5, card = Card(name = Name(full = "Dana White")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = contact, paperlessConfigured = true, seafileConfigured = false, nextcloudConfigured = false)
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
                    onAddSeafileLink = { tapped = true },
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

    // --- T90 / issue #831: Mark as Me / Unmark as Me in the action menu (web parity) ---

    @Test
    fun `mark as me in the action menu invokes toggleMe`() {
        val contact = ContactRecordResponse(id = 5, uid = "uid-5", card = Card(name = Name(full = "Dana White")))
        val viewModel = mockk<ContactDetailViewModel>(relaxed = true)
        every { viewModel.uiState } returns MutableStateFlow(
            ContactDetailUiState(contact = contact, selfContactVCardUid = null),
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
        val contact = ContactRecordResponse(id = 5, uid = "uid-5", card = Card(name = Name(full = "Dana White")))
        val viewModel = mockk<ContactDetailViewModel>(relaxed = true)
        every { viewModel.uiState } returns MutableStateFlow(
            ContactDetailUiState(contact = contact, selfContactVCardUid = "uid-5"),
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
