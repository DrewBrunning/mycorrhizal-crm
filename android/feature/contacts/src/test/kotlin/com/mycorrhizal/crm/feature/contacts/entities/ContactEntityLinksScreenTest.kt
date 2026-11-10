package com.mycorrhizal.crm.feature.contacts.entities

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotSelected
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.Address
import com.mycorrhizal.crm.model.network.ContactEntityLink
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.Organization
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
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
class ContactEntityLinksScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val acmeLink = ContactEntityLink(
        id = "l1",
        entityId = "e1",
        entityName = "Acme",
        kind = EntityKind.ORGANISATION,
        role = "employee",
        isPrimary = true,
        projectsAs = "ORG",
        revision = 2
    )
    private val rexLink = ContactEntityLink(
        id = "l2",
        entityId = "e2",
        entityName = "Rex",
        kind = EntityKind.PET,
        role = "owner",
        revision = 1
    )

    private class Calls {
        val primary = mutableListOf<Pair<String, Boolean>>()
        val removed = mutableListOf<String>()
        val adopted = mutableListOf<String>()
        val linkedOwn = mutableListOf<Triple<OwnValue, String?, String>>()
    }

    private fun setContent(state: ContactEntityLinksUiState, calls: Calls = Calls()) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactEntityLinksContent(
                    state = state,
                    onBack = {},
                    onOpenEntity = {},
                    onLink = { _, _ -> },
                    onSetPrimary = { l, p -> calls.primary += l.id to p },
                    onRemove = { calls.removed += it.id },
                    onAdopt = { calls.adopted += it.id },
                    onLinkOwn = { v, e, r, _ -> calls.linkedOwn += Triple(v, e, r) },
                    onFindMatch = {},
                    onSearch = { _, _ -> },
                    onClearSearch = {},
                    onFormDismissed = {},
                    onErrorShown = {},
                    onConflictLoadLatest = {},
                    onConflictDismissed = {},
                    onRetry = {}
                )
            }
        }
    }

    @Test
    fun `an empty section says so and offers both ways to link`() {
        setContent(ContactEntityLinksUiState(isLoading = false))
        composeTestRule.onNodeWithTag("contact-entity-links-empty").assertIsDisplayed()
        composeTestRule.onNodeWithTag("contact-entity-link-add").assertIsDisplayed()
        composeTestRule.onNodeWithTag("contact-entity-link-from-own").assertIsDisplayed()
    }

    @Test
    fun `a projecting link shows what it is shown as and a primary switch`() {
        val calls = Calls()
        setContent(ContactEntityLinksUiState(isLoading = false, links = listOf(acmeLink)), calls)
        composeTestRule.onNodeWithText("Shown as organisation").assertIsDisplayed()
        composeTestRule.onNodeWithTag("contact-entity-link-primary-l1").assertIsOn()
        composeTestRule.onNodeWithTag("contact-entity-link-primary-l1").performClick()
        assertEquals(listOf("l1" to false), calls.primary)
    }

    @Test
    fun `a pet link has no primary switch and no adopt because it never projects`() {
        setContent(ContactEntityLinksUiState(isLoading = false, links = listOf(rexLink)))
        composeTestRule.onAllNodesWithTag("contact-entity-link-primary-l2").assertCountEquals(0)
        composeTestRule.onAllNodesWithTag("contact-entity-link-adopt-l2").assertCountEquals(0)
    }

    @Test
    fun `adopt is offered only for a link that currently projects`() {
        val calls = Calls()
        val notProjecting = acmeLink.copy(id = "l3", isPrimary = false, projectsAs = "")
        setContent(
            ContactEntityLinksUiState(isLoading = false, links = listOf(acmeLink, notProjecting)),
            calls
        )
        composeTestRule.onAllNodesWithTag("contact-entity-link-adopt-l3").assertCountEquals(0)
        composeTestRule.onNodeWithTag("contact-entity-link-adopt-l1").performClick()
        assertEquals(listOf("l1"), calls.adopted)
    }

    @Test
    fun `a link that is not current says so`() {
        setContent(
            ContactEntityLinksUiState(
                isLoading = false,
                links = listOf(acmeLink.copy(current = false, projectsAs = ""))
            )
        )
        composeTestRule.onNodeWithText("Not current").assertIsDisplayed()
    }

    @Test
    fun `removing a link asks first and keeps the entity`() {
        val calls = Calls()
        setContent(ContactEntityLinksUiState(isLoading = false, links = listOf(acmeLink)), calls)
        composeTestRule.onNodeWithText("Delete").performClick()
        composeTestRule.onNodeWithText(
            "Remove the link to Acme? The entity itself is kept."
        ).assertIsDisplayed()
        assertEquals(emptyList<String>(), calls.removed)
        composeTestRule.onNodeWithTag("contact-entity-link-remove-confirm").performClick()
        assertEquals(listOf("l1"), calls.removed)
    }

    @Test
    fun `linking an own value defaults to creating a new entity and sends the chosen role`() {
        val calls = Calls()
        setContent(
            ContactEntityLinksUiState(
                isLoading = false,
                ownOrganizations = listOf(Organization(id = "o1", name = "Globex")),
                ownAddresses = listOf(Address(id = "a1", full = "1 High St")),
                ownMatch = Entity(id = "e9", name = "Globex")
            ),
            calls
        )
        composeTestRule.onNodeWithTag("contact-entity-link-from-own").performClick()
        composeTestRule.onNodeWithText(
            "Create a new organisation from this value"
        ).assertIsDisplayed()
        composeTestRule.onNodeWithTag("contact-entity-own-create-new").assertIsSelected()
        composeTestRule.onNodeWithTag("contact-entity-own-use-existing").assertIsNotSelected()
        composeTestRule.onNodeWithTag("contact-entity-own-link").performClick()
        val call = calls.linkedOwn.single()
        assertEquals("o1", call.first.id)
        assertEquals(null, call.second)
        assertEquals("employee", call.third)
    }

    @Test
    fun `an existing match is offered but never pre-selected, and choosing it sends its id`() {
        val calls = Calls()
        setContent(
            ContactEntityLinksUiState(
                isLoading = false,
                ownOrganizations = listOf(Organization(id = "o1", name = "Globex")),
                ownMatch = Entity(id = "e9", name = "Globex")
            ),
            calls
        )
        composeTestRule.onNodeWithTag("contact-entity-link-from-own").performClick()
        composeTestRule.onNodeWithTag("contact-entity-own-use-existing").performClick()
        composeTestRule.onNodeWithTag("contact-entity-own-link").performClick()
        assertEquals("e9", calls.linkedOwn.single().second)
    }

    @Test
    fun `the section has no accessibility violations`() {
        setContent(ContactEntityLinksUiState(isLoading = false, links = listOf(acmeLink, rexLink)))
        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `the projected chip names its entity and is only text`() {
        composeTestRule.setContent {
            MycorrhizalTheme { ProjectedEntryRow(title = "Acme", entityName = "Acme", tag = "row") }
        }
        composeTestRule.onNodeWithText("from Acme").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Shown from the linked entity. Change it on the entity's page, or copy it into this contact to edit it here."
        ).assertIsDisplayed()
        composeTestRule.onAllNodesWithTag("organization-edit-x").assertCountEquals(0)
    }

    @Test
    fun `an unknown entity falls back to a generic chip`() {
        composeTestRule.setContent { MycorrhizalTheme { ProjectedFromChip(entityName = null) } }
        composeTestRule.onNodeWithText("from a linked entity").assertIsDisplayed()
    }
}
