package com.mycorrhizal.crm.feature.contacts.entities

import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertHasClickAction
import androidx.compose.ui.test.assertHeightIsAtLeast
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotSelected
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.Address
import com.mycorrhizal.crm.model.network.ContactEntityLink
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.Organization
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
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
        composeTestRule.onNodeWithText("Remove").performClick()
        composeTestRule.onNodeWithText(
            "Remove the link to Acme? The organisation, place or pet itself is kept."
        ).assertIsDisplayed()
        // The confirm names the action the dialog is about, not a destructive "Delete".
        composeTestRule.onNode(
            hasTestTag("contact-entity-link-remove-confirm") and hasText("Remove")
        ).assertIsDisplayed()
        composeTestRule.onAllNodesWithText("Delete").assertCountEquals(0)
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
    fun `the entity name is a button of at least 48dp that opens the entity`() {
        var opened: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactEntityLinksContent(
                    state = ContactEntityLinksUiState(isLoading = false, links = listOf(acmeLink)),
                    onBack = {},
                    onOpenEntity = { opened = it },
                    onLink = { _, _ -> },
                    onSetPrimary = { _, _ -> },
                    onRemove = {},
                    onAdopt = {},
                    onLinkOwn = { _, _, _, _ -> },
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
        val name = composeTestRule.onNodeWithTag("contact-entity-link-open-l1")
        name.assertHeightIsAtLeast(48.dp).assertHasClickAction()
        name.assert(
            SemanticsMatcher.expectValue(SemanticsProperties.Role, Role.Button)
        )
        // Link styling: a chevron next to the name, not decoration without meaning.
        composeTestRule.onNodeWithTag("contact-entity-link-chevron-l1", useUnmergedTree = true)
            .assertExists()
        name.performClick()
        assertEquals("e1", opened)
    }

    @Test
    fun `captions on a link are at least 12sp`() {
        setContent(ContactEntityLinksUiState(isLoading = false, links = listOf(acmeLink)))
        for (text in listOf("Organisation \u00b7 Employee", "Shown as organisation")) {
            val size = composeTestRule.onNodeWithText(text, substring = true)
                .fetchSemanticsNode().textSizeSp()
            assertTrue("$text is ${size}sp", size >= 12f)
        }
    }

    @Test
    fun `the link dialog option rows and the primary switch rows are at least 48dp`() {
        setContent(ContactEntityLinksUiState(isLoading = false, links = listOf(acmeLink)))
        composeTestRule.onNodeWithTag("contact-entity-link-primary-l1").assertHeightIsAtLeast(48.dp)
        composeTestRule.onNodeWithTag("contact-entity-link-add").performClick()
        composeTestRule.onNodeWithTag("contact-entity-link-entity-search").performTextInput("Zed")
        composeTestRule.onNodeWithTag("contact-entity-link-create", useUnmergedTree = false)
            .assertHeightIsAtLeast(48.dp)
    }

    @Test
    fun `the own-value dialog option rows are at least 48dp`() {
        setContent(
            ContactEntityLinksUiState(
                isLoading = false,
                ownOrganizations = listOf(Organization(id = "o1", name = "Globex")),
                ownMatch = Entity(id = "e9", name = "Globex")
            )
        )
        composeTestRule.onNodeWithTag("contact-entity-link-from-own").performClick()
        composeTestRule.onNodeWithTag("contact-entity-own-create-new").assertHeightIsAtLeast(48.dp)
        composeTestRule.onNodeWithTag("contact-entity-own-use-existing")
            .assertHeightIsAtLeast(48.dp)
    }

    private fun androidx.compose.ui.semantics.SemanticsNode.textSizeSp(): Float {
        val results = mutableListOf<androidx.compose.ui.text.TextLayoutResult>()
        config[androidx.compose.ui.semantics.SemanticsActions.GetTextLayoutResult]
            .action?.invoke(results)
        return results.first().layoutInput.style.fontSize.value
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
