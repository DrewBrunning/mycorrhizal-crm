package com.mycorrhizal.crm.feature.contacts.entities

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToNode
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.domain.repository.EntityRepository
import com.mycorrhizal.crm.model.network.EntityFromContactsSkipReason
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.EntitySuggestionsResponse
import com.mycorrhizal.crm.model.network.EntityValueGroup
import com.mycorrhizal.crm.model.network.EntityValueGroupContact
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** #2513 (web #2301): the create-entities-from-contacts screen content. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class EntitiesFromContactsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun contact(id: Int, first: String, primary: Boolean = false) = EntityValueGroupContact(
        contactId = id,
        firstname = first,
        lastname = "Lovelace",
        valueName = "ACME",
        hasPrimary = primary
    )

    private val acme = EntityValueGroup(
        key = "k-acme",
        name = "Acme",
        reason = "exact_name",
        variants = listOf("Acme", "ACME Ltd"),
        contacts = listOf(contact(7, "Ada", primary = true), contact(8, "Bo"))
    )
    private val globex = EntityValueGroup(
        key = "k-globex",
        name = "Globex",
        contacts = listOf(contact(10, "Di"))
    )

    private class Calls {
        val kinds = mutableListOf<String>()
        val selected = mutableListOf<Pair<String, Boolean>>()
        val names = mutableListOf<Pair<String, String>>()
        val primaries = mutableListOf<Pair<String, Boolean>>()
        val contacts = mutableListOf<Triple<String, Int, Boolean>>()
        var created = 0
        var retried = 0
        var loadedMore = 0
        var dismissed = 0
    }

    private fun set(state: EntitiesFromContactsUiState, calls: Calls = Calls()): Calls {
        composeTestRule.setContent {
            MycorrhizalTheme {
                EntitiesFromContactsContent(
                    state = state,
                    actions = EntitiesFromContactsActions(
                        onSelectKind = { calls.kinds += it },
                        onSelected = { k, v -> calls.selected += k to v },
                        onName = { k, v -> calls.names += k to v },
                        onPrimary = { k, v -> calls.primaries += k to v },
                        onContact = { k, id, v -> calls.contacts += Triple(k, id, v) },
                        onCreate = { calls.created++ },
                        onRetry = { calls.retried++ },
                        onLoadMore = { calls.loadedMore++ },
                        onDismissResult = { calls.dismissed++ }
                    )
                )
            }
        }
        return calls
    }

    private fun loaded(
        vararg groups: EntityValueGroup,
        edits: Map<String, FromContactsGroupEdit> = emptyMap(),
        kind: String = EntityKind.ORGANISATION
    ) = EntitiesFromContactsUiState(
        kind = kind,
        groups = groups.toList(),
        edits = edits,
        loaded = true
    )

    @Test
    fun `each group shows its name, contact count, spellings and who already has a primary`() {
        set(loaded(acme, globex))
        composeTestRule.onNodeWithText("Acme — 2 contacts").assertIsDisplayed()
        composeTestRule.onNodeWithText("Globex — 1 contact").assertIsDisplayed()
        composeTestRule.onNodeWithText("Also written as: Acme, ACME Ltd").assertIsDisplayed()
        composeTestRule.onNodeWithText("1 of them already has a primary organisation.")
            .assertIsDisplayed()
        composeTestRule.onNodeWithTag("from-contacts-include-k-acme").assertIsOff()
        // The editor only appears once a group is ticked.
        composeTestRule.onAllNodesWithTag("from-contacts-name-k-acme").assertCountEquals(0)
        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `places word the primary hint for places`() {
        set(loaded(acme, kind = EntityKind.PLACE))
        composeTestRule.onNodeWithText("1 of them already has a primary place.")
            .assertIsDisplayed()
    }

    @Test
    fun `ticking a group and switching tabs are reported`() {
        val calls = set(loaded(acme))
        composeTestRule.onNodeWithTag("from-contacts-include-k-acme").performClick()
        composeTestRule.onNodeWithTag("from-contacts-tab-place").performClick()
        assertEquals(listOf("k-acme" to true), calls.selected)
        assertEquals(listOf(EntityKind.PLACE), calls.kinds)
    }

    @Test
    fun `a ticked group shows name, role, primary and a tickbox per contact`() {
        val calls = set(
            loaded(
                acme,
                edits = mapOf(
                    "k-acme" to FromContactsGroupEdit(
                        selected = true,
                        name = "Acme",
                        role = "employee",
                        excluded = setOf(8)
                    )
                )
            )
        )
        composeTestRule.onNodeWithTag("from-contacts-name-k-acme").performScrollTo()
        composeTestRule.onNodeWithTag("from-contacts-name-k-acme").performTextInput("X")
        assertEquals("k-acme", calls.names.single().first)

        // The open menu is a popup window Robolectric does not drive reliably; the choice
        // reaching the view model is covered there. Here: the closed field is a labelled,
        // announced drop-down showing the current role.
        composeTestRule.onNodeWithTag("from-contacts-role-k-acme").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Role for Acme, Employee").assertIsDisplayed()

        composeTestRule.onNodeWithTag("from-contacts-primary-k-acme").performScrollTo()
        composeTestRule.onNodeWithTag("from-contacts-primary-k-acme").assertIsOff()
        composeTestRule.onNodeWithTag("from-contacts-primary-k-acme").performClick()
        assertEquals(listOf("k-acme" to true), calls.primaries)

        composeTestRule.onNodeWithTag("from-contacts-contact-k-acme-7").performScrollTo()
        composeTestRule.onNodeWithTag("from-contacts-contact-k-acme-7").assertIsOn()
        composeTestRule.onNodeWithTag("from-contacts-contact-k-acme-8").assertIsOff()
        composeTestRule.onNodeWithTag("from-contacts-contact-k-acme-7").performClick()
        composeTestRule.onNodeWithTag("from-contacts-contact-k-acme-8").performScrollTo()
        composeTestRule.onNodeWithTag("from-contacts-contact-k-acme-8").performClick()
        assertEquals(
            listOf(Triple("k-acme", 7, false), Triple("k-acme", 8, true)),
            calls.contacts
        )
    }

    @Test
    fun `the create button counts what is chosen and is disabled with nothing chosen`() {
        set(loaded(acme))
        composeTestRule.onNodeWithTag("from-contacts-create").assertIsNotEnabled()
        composeTestRule.onNodeWithText("Create 0 organisations").assertIsDisplayed()
    }

    @Test
    fun `creating reports the click and is disabled while a create is running`() {
        val chosen = mapOf("k-acme" to FromContactsGroupEdit(selected = true, name = "Acme"))
        val calls = set(loaded(acme, globex, edits = chosen))
        composeTestRule.onNodeWithText("Create 1 organisation").assertIsDisplayed()
        composeTestRule.onNodeWithTag("from-contacts-create").assertIsEnabled().performClick()
        assertEquals(1, calls.created)
    }

    @Test
    fun `a running create disables the button`() {
        val chosen = mapOf("k-acme" to FromContactsGroupEdit(selected = true, name = "Acme"))
        set(loaded(acme, edits = chosen).copy(isCreating = true))
        composeTestRule.onNodeWithTag("from-contacts-create").assertIsNotEnabled()
    }

    @Test
    fun `nothing left to create shows the empty message`() {
        set(loaded())
        composeTestRule.onNodeWithText("Nothing left to create from your contacts.")
            .assertIsDisplayed()
    }

    @Test
    fun `a failed load shows the error with a retry and no empty message`() {
        val calls = set(
            EntitiesFromContactsUiState(loadError = ApiError.Network(java.io.IOException("x")))
        )
        composeTestRule.onNodeWithText("Couldn't look through your contacts.").assertIsDisplayed()
        composeTestRule.onAllNodesWithTag("from-contacts-empty").assertCountEquals(0)
        composeTestRule.onNodeWithText("Retry").performClick()
        assertEquals(1, calls.retried)
    }

    @Test
    fun `a failed create shows the error and keeps the groups`() {
        set(
            loaded(acme).copy(
                createError = ApiError.Network(java.io.IOException("offline"))
            )
        )
        composeTestRule.onNodeWithText("Couldn't create the entities.").assertIsDisplayed()
        composeTestRule.onNodeWithTag("from-contacts-error").assertIsDisplayed()
        composeTestRule.onNodeWithText("Acme — 2 contacts").assertIsDisplayed()
    }

    @Test
    fun `the results summary lists what was created and each skipped item`() {
        val calls = set(
            loaded(globex).copy(
                result = FromContactsResult(
                    kind = EntityKind.ORGANISATION,
                    createdNames = listOf("Acme", "Initech"),
                    skipped = listOf(
                        FromContactsSkipItem(
                            "Globex",
                            null,
                            EntityFromContactsSkipReason.GROUP_NOT_FOUND
                        ),
                        FromContactsSkipItem(
                            "Acme",
                            "Bo Lovelace",
                            EntityFromContactsSkipReason.CONTACT_NOT_IN_GROUP
                        ),
                        FromContactsSkipItem(
                            "Acme",
                            null,
                            EntityFromContactsSkipReason.CONTACT_NOT_IN_GROUP
                        ),
                        FromContactsSkipItem(
                            "Bad",
                            null,
                            EntityFromContactsSkipReason.NAME_INVALID
                        ),
                        FromContactsSkipItem(
                            "Worse",
                            null,
                            EntityFromContactsSkipReason.CORE_INVALID
                        ),
                        FromContactsSkipItem("Odd", null, "a_future_reason")
                    )
                )
            )
        )
        composeTestRule.onNodeWithTag("from-contacts-result").assertIsDisplayed()
        composeTestRule.onNodeWithText("Created 2 organisations.").assertIsDisplayed()
        composeTestRule.onNodeWithText("Acme, Initech").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Globex: skipped. It may already exist or no longer match your contacts."
        )
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("Acme: Bo Lovelace no longer belongs to it.")
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("Acme: a contact no longer belongs to it.")
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("Bad: the name is not valid.").assertIsDisplayed()
        composeTestRule.onNodeWithText("Worse: its details are not valid.").assertIsDisplayed()
        composeTestRule.onNodeWithText("Odd: it was not applied.").assertIsDisplayed()
        composeTestRule.onNodeWithTag(
            "from-contacts-result-dismiss"
        ).performScrollTo().performClick()
        assertEquals(1, calls.dismissed)
    }

    @Test
    fun `a summary with nothing created still lists the skips`() {
        set(
            loaded().copy(
                result = FromContactsResult(
                    kind = EntityKind.PLACE,
                    createdNames = emptyList(),
                    skipped = listOf(
                        FromContactsSkipItem(
                            "Cabin",
                            null,
                            EntityFromContactsSkipReason.GROUP_NOT_FOUND
                        )
                    )
                )
            )
        )
        composeTestRule.onNodeWithText("Nothing was created.").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Cabin: skipped. It may already exist or no longer match your contacts."
        )
            .assertIsDisplayed()
    }

    @Test
    fun `a further page is requested from the load-more row`() {
        val calls = set(loaded(acme).copy(nextCursor = "c2"))
        composeTestRule.onNodeWithTag("from-contacts-load-more").performScrollTo()
        assertEquals(true, calls.loadedMore >= 1)
    }

    @Test
    fun `Retry after a failed load more keeps the ticked group and shows the next page`() {
        val repository = mockk<EntityRepository>()
        fun page(vararg g: EntityValueGroup, next: String? = null) = Result.success(
            EntitySuggestionsResponse(
                mode = "contact_values",
                groups = g.toList(),
                nextCursor = next
            )
        )
        coEvery { repository.valueGroups(EntityKind.ORGANISATION, null) } returns
            page(acme, next = "c2")
        coEvery { repository.valueGroups(EntityKind.ORGANISATION, "c2") } returnsMany listOf(
            Result.failure(ApiError.Network(java.io.IOException("offline"))),
            page(globex)
        )
        val viewModel = EntitiesFromContactsViewModel(repository)
        composeTestRule.setContent {
            MycorrhizalTheme { EntitiesFromContactsScreen(onBack = {}, viewModel = viewModel) }
        }
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithTag("from-contacts-include-k-acme").performClick()
        composeTestRule.onNodeWithTag("from-contacts-include-k-acme").assertIsOn()
        composeTestRule.onNodeWithTag("from-contacts-list")
            .performScrollToNode(hasTestTag("from-contacts-load-error"))
        composeTestRule.onNodeWithTag("from-contacts-load-error").assertIsDisplayed()

        composeTestRule.onNodeWithText("Retry").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithTag("from-contacts-list")
            .performScrollToNode(hasTestTag("from-contacts-group-k-globex"))
        composeTestRule.onNodeWithTag("from-contacts-group-k-globex").assertIsDisplayed()
        composeTestRule.onNodeWithTag("from-contacts-list")
            .performScrollToNode(hasTestTag("from-contacts-include-k-acme"))
        composeTestRule.onNodeWithTag("from-contacts-include-k-acme").assertIsOn()
    }
}
