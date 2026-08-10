package com.mycorrhizal.crm.feature.contacts

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.model.network.Resource
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
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

        composeTestRule.onNodeWithText("Dana White").assertIsDisplayed()
        composeTestRule.onNodeWithText("Email").assertIsDisplayed()
        composeTestRule.onNodeWithText("dana@example.com").assertIsDisplayed()
        composeTestRule.onNodeWithText("Phone").assertIsDisplayed()
        composeTestRule.onNodeWithText("+1-555-0100").assertIsDisplayed()
        composeTestRule.onNodeWithText("Circles").assertIsDisplayed()
    }

    @Test
    fun `renders empty name fallback when no name present`() {
        val contact = ContactRecordResponse(id = 5, card = Card())
        setContent(ContactDetailUiState(contact = contact))
        composeTestRule.onNodeWithText("Contact").assertIsDisplayed()
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

        composeTestRule.onNodeWithContentDescription("Call").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Copy phone number").assertIsDisplayed()
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

        composeTestRule.onNodeWithContentDescription("Text").assertIsDisplayed()
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
    fun `email row renders compose and copy actions`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                emails = listOf(Email(address = "dana@example.com")),
            ),
        )
        setContent(ContactDetailUiState(contact = contact))

        composeTestRule.onNodeWithContentDescription("Compose email").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Copy email").assertIsDisplayed()
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

        composeTestRule.onNodeWithText("123 Main St").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Open in maps").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Copy address").assertIsDisplayed()
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

        composeTestRule.onNodeWithText("Links").assertIsDisplayed()
        composeTestRule.onNodeWithText("Website").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Open link").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Copy link").assertIsDisplayed()
    }
}
