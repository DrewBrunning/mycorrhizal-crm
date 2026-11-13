package com.mycorrhizal.crm.feature.contacts.entities

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityContactLink
import com.mycorrhizal.crm.model.network.EntityInput
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.EntityMergeResponse
import com.mycorrhizal.crm.model.network.EntitySuggestionEntity
import com.mycorrhizal.crm.model.network.EntitySuggestionPair
import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.network.TemporalRange
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
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
class EntityScreensTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val acme = Entity(
        id = "e1",
        kind = EntityKind.ORGANISATION,
        name = "Acme",
        revision = 3,
        core = mapOf("url" to "https://acme.example", "organization" to mapOf("name" to "Acme")),
        linkCount = 3,
        projectionCount = 2
    )

    // --- list ------------------------------------------------------------------------------

    private fun setList(
        state: EntitiesUiState,
        onOpen: (String) -> Unit = {},
        onKind: (String) -> Unit = {},
        onFind: () -> Unit = {},
        onDismiss: (EntitySuggestionPair) -> Unit = {},
        onMerge: (EntitySuggestionPair) -> Unit = {},
        onConfirmMerge: (Boolean, Set<String>) -> Unit = { _, _ -> },
        onDismissMerge: () -> Unit = {}
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                EntitiesContent(
                    state = state,
                    onSelectKind = onKind,
                    onQueryChange = {},
                    onOpenEntity = onOpen,
                    onLoadMore = {},
                    onRetry = {},
                    onAdd = {},
                    onFindDuplicates = onFind,
                    onDismissSuggestion = onDismiss,
                    onMergeSuggestion = onMerge,
                    onConfirmMerge = onConfirmMerge,
                    onDismissMerge = onDismissMerge
                )
            }
        }
    }

    @Test
    fun `the list shows the kind tabs and one row per entity and opens a row`() {
        var opened: String? = null
        setList(
            EntitiesUiState(entities = listOf(acme, acme.copy(id = "e2", name = "Globex"))),
            { opened = it }
        )
        composeTestRule.onNodeWithTag("entities-tab-organisation").assertIsDisplayed()
        composeTestRule.onNodeWithTag("entities-tab-place").assertIsDisplayed()
        composeTestRule.onNodeWithTag("entities-tab-pet").assertIsDisplayed()
        composeTestRule.onAllNodesWithText("https://acme.example").assertCountEquals(2)
        composeTestRule.onNodeWithTag("entity-row-e2").performClick()
        assertEquals("e2", opened)
    }

    @Test
    fun `selecting a tab reports the kind`() {
        var kind: String? = null
        setList(EntitiesUiState(entities = listOf(acme)), onKind = { kind = it })
        composeTestRule.onNodeWithTag("entities-tab-pet").performClick()
        assertEquals(EntityKind.PET, kind)
    }

    @Test
    fun `an empty kind offers to add one and a failed load offers a retry`() {
        setList(EntitiesUiState(kind = EntityKind.PET))
        composeTestRule.onNodeWithText("No pets yet.").assertIsDisplayed()
        composeTestRule.onNodeWithText("Add pet").assertIsDisplayed()
    }

    private val cabinPair = EntitySuggestionPair(
        reason = "normalized_name",
        a = EntitySuggestionEntity(id = "p1", kind = EntityKind.PLACE, name = "Cabin"),
        b = EntitySuggestionEntity(id = "p2", kind = EntityKind.PLACE, name = "cabin.")
    )

    @Test
    fun `the duplicates card offers a scan and reports it`() {
        var found = 0
        setList(EntitiesUiState(entities = listOf(acme)), onFind = { found++ })
        composeTestRule.onNodeWithText("Possible duplicates").assertIsDisplayed()
        composeTestRule.onNodeWithText("Find duplicates").performClick()
        assertEquals(1, found)
        composeTestRule.onNodeWithText("No duplicates found.").assertDoesNotExist()
    }

    @Test
    fun `a scan with no pairs says so`() {
        setList(EntitiesUiState(entities = listOf(acme), suggestions = emptyList()))
        composeTestRule.onNodeWithText("No duplicates found.").assertIsDisplayed()
    }

    @Test
    fun `a failed scan shows its message and still offers the scan`() {
        setList(
            EntitiesUiState(
                entities = listOf(acme),
                suggestionsError = ApiError.Network(java.io.IOException("offline"))
            )
        )
        composeTestRule.onNodeWithText("Couldn't look for duplicates.").assertIsDisplayed()
        composeTestRule.onNodeWithText("Find duplicates").assertIsDisplayed()
    }

    @Test
    fun `a pair shows its names and reason, opens either side and can be dismissed`() {
        var opened: String? = null
        var dismissed: EntitySuggestionPair? = null
        setList(
            EntitiesUiState(
                kind = EntityKind.PLACE,
                entities = listOf(acme.copy(kind = EntityKind.PLACE)),
                suggestions = listOf(cabinPair)
            ),
            onOpen = { opened = it },
            onDismiss = { dismissed = it }
        )
        composeTestRule.onNodeWithText("Cabin and cabin.").assertIsDisplayed()
        composeTestRule.onNodeWithText("Very similar name").assertIsDisplayed()
        composeTestRule.onNodeWithTag("suggestion-open-b-p2").performClick()
        assertEquals("p2", opened)
        composeTestRule.onNodeWithTag("suggestion-open-a-p1").performClick()
        assertEquals("p1", opened)
        composeTestRule.onNodeWithText("Not a duplicate").performClick()
        assertEquals(cabinPair, dismissed)
    }

    @Test
    fun `a pair has a Merge button that reports the pair`() {
        var merged: EntitySuggestionPair? = null
        setList(
            EntitiesUiState(
                kind = EntityKind.PLACE,
                entities = listOf(acme.copy(kind = EntityKind.PLACE)),
                suggestions = listOf(cabinPair)
            ),
            onMerge = { merged = it }
        )
        composeTestRule.onNodeWithTag("suggestion-merge-p1").performClick()
        assertEquals(cabinPair, merged)
    }

    private val survivorA = Entity(id = "p1", kind = EntityKind.PLACE, name = "Cabin")
    private val loserB = Entity(
        id = "p2",
        kind = EntityKind.PLACE,
        name = "cabin.",
        core = mapOf("notes" to "by the lake")
    )

    private fun mergeState(error: ApiError? = null, merging: Boolean = false) = EntitiesUiState(
        kind = EntityKind.PLACE,
        suggestions = listOf(cabinPair),
        mergePair = cabinPair,
        mergeA = survivorA,
        mergeB = loserB,
        mergeError = error,
        isMerging = merging
    )

    @Test
    fun `the merge dialog opens pre-filled, keeps the first and passes the choices on`() {
        var confirmed: Pair<Boolean, Set<String>>? = null
        setList(mergeState(), onConfirmMerge = { keep, fill -> confirmed = keep to fill })
        composeTestRule.onNodeWithTag("entity-merge-dialog").assertIsDisplayed()
        composeTestRule.onNodeWithText("Merge cabin.").assertIsDisplayed()
        // Nothing to pick: the pair is the choice, so confirm is live straight away.
        composeTestRule.onAllNodesWithTag("entity-merge-search").assertCountEquals(0)
        composeTestRule.onNodeWithTag("entity-merge-confirm").assertIsEnabled()
        composeTestRule.onNodeWithText(
            "cabin. will be deleted and its contacts move to Cabin. " +
                "Nothing is merged unless you confirm here."
        ).assertIsDisplayed()
        composeTestRule.onNodeWithTag("entity-merge-fill-notes").assertIsOff()
        composeTestRule.onNodeWithTag("entity-merge-fill-notes").performScrollTo().performClick()
        composeTestRule.onNodeWithTag("entity-merge-confirm").performClick()
        assertEquals(false to setOf("notes"), confirmed)
    }

    @Test
    fun `the swap switch keeps the other side and clears ticked fields`() {
        var confirmed: Pair<Boolean, Set<String>>? = null
        setList(mergeState(), onConfirmMerge = { keep, fill -> confirmed = keep to fill })
        composeTestRule.onNodeWithTag("entity-merge-swap").performScrollTo().performClick()
        composeTestRule.onNodeWithText(
            "Cabin will be deleted and its contacts move to cabin.. " +
                "Nothing is merged unless you confirm here."
        ).assertIsDisplayed()
        composeTestRule.onNodeWithTag("entity-merge-confirm").performClick()
        assertEquals(true to emptySet<String>(), confirmed)
    }

    @Test
    fun `a failed merge shows its error in the dialog`() {
        setList(mergeState(error = ApiError.Network(java.io.IOException("offline"))))
        composeTestRule.onNodeWithTag("entity-merge-error").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `confirm is disabled while the merge runs`() {
        setList(mergeState(merging = true))
        composeTestRule.onNodeWithTag("entity-merge-confirm").assertIsNotEnabled()
    }

    @Test
    fun `cancel reports the dismissal`() {
        var dismissed = 0
        setList(mergeState(), onDismissMerge = { dismissed++ })
        composeTestRule.onNodeWithText("Cancel").performClick()
        assertEquals(1, dismissed)
    }

    @Test
    fun `no dialog shows until both records of the pair have loaded`() {
        setList(mergeState().copy(mergeB = null))
        composeTestRule.onAllNodesWithTag("entity-merge-dialog").assertCountEquals(0)
    }

    @Test
    fun `the list has no accessibility violations`() {
        setList(EntitiesUiState(entities = listOf(acme)))
        composeTestRule.assertAccessibleSemantics()
    }

    // --- detail ----------------------------------------------------------------------------

    private fun setDetail(
        state: EntityDetailUiState,
        onDelete: (Boolean) -> Unit = {},
        onRestore: () -> Unit = {},
        onMerge: (Entity, Boolean, Set<String>) -> Unit = { _, _, _ -> },
        onSearchMerge: (String) -> Unit = {},
        onOpenContact: (Int) -> Unit = {}
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                EntityDetailContent(
                    state = state,
                    onBack = {},
                    onOpenContact = onOpenContact,
                    onOpenEntity = {},
                    onRetry = {},
                    onSave = { _, _ -> },
                    onFormDismissed = {},
                    onSaveField = { _, _ -> },
                    onDelete = onDelete,
                    onRestore = onRestore,
                    onRestoreMerged = {},
                    onSearchMerge = onSearchMerge,
                    onMerge = onMerge,
                    onMergeDismissed = {}
                )
            }
        }
    }

    @Test
    fun `the people list shows role, period and primary, and opens the contact`() {
        var opened = -1
        setDetail(
            EntityDetailUiState(
                entity = acme,
                isLoading = false,
                people = listOf(
                    EntityContactLink(
                        linkId = "l1",
                        contactId = 7,
                        firstname = "Ada",
                        lastname = "Lovelace",
                        role = "employee",
                        roleLabel = "Head chef",
                        isPrimary = true,
                        period = TemporalRange(start = PartialDate(year = 2019))
                    )
                )
            ),
            onOpenContact = { opened = it }
        )
        composeTestRule.onNodeWithText("Employee · Head chef · 2019 –").assertIsDisplayed()
        composeTestRule.onNodeWithText("Primary").assertIsDisplayed()
        composeTestRule.onNodeWithTag("entity-person-l1").performClick()
        assertEquals(7, opened)
    }

    @Test
    fun `delete says how many contacts lose it and the adopt-all box starts unticked`() {
        var adoptAll: Boolean? = null
        setDetail(
            EntityDetailUiState(entity = acme, isLoading = false),
            onDelete = { adoptAll = it }
        )
        composeTestRule.onNodeWithTag("entity-delete").performClick()
        composeTestRule.onNodeWithText(
            "Acme is the primary organisation of 2 contacts; they will lose that organisation."
        )
            .assertIsDisplayed()
        composeTestRule.onNodeWithTag("entity-delete-adopt").assertIsOff()
        composeTestRule.onNodeWithTag("entity-delete-confirm").performClick()
        assertEquals(false, adoptAll)
    }

    @Test
    fun `ticking adopt-all is passed to delete`() {
        var adoptAll: Boolean? = null
        setDetail(
            EntityDetailUiState(entity = acme, isLoading = false),
            onDelete = { adoptAll = it }
        )
        composeTestRule.onNodeWithTag("entity-delete").performClick()
        composeTestRule.onNodeWithTag("entity-delete-adopt").performClick()
        composeTestRule.onNodeWithTag("entity-delete-adopt").assertIsOn()
        composeTestRule.onNodeWithTag("entity-delete-confirm").performClick()
        assertEquals(true, adoptAll)
    }

    @Test
    fun `adopt-all is disabled while the people list failed to load`() {
        setDetail(
            EntityDetailUiState(entity = acme, isLoading = false, peopleFailed = true)
        )
        composeTestRule.onNodeWithTag("entity-delete").performClick()
        composeTestRule.onNodeWithTag("entity-delete-adopt").assertIsNotEnabled()
    }

    @Test
    fun `a pet's delete never offers adopt`() {
        setDetail(
            EntityDetailUiState(
                entity = acme.copy(kind = EntityKind.PET, projectionCount = 0, linkCount = 1),
                isLoading = false
            )
        )
        composeTestRule.onNodeWithTag("entity-delete").performClick()
        composeTestRule.onAllNodesWithTag("entity-delete-adopt").assertCountEquals(0)
    }

    @Test
    fun `after a delete the page offers Restore, and only then`() {
        var restored = false
        setDetail(
            EntityDetailUiState(entity = acme, isLoading = false, deleted = true),
            onRestore = { restored = true }
        )
        composeTestRule.onNodeWithText("Acme was deleted").assertIsDisplayed()
        composeTestRule.onNodeWithTag("entity-restore").performClick()
        assertTrue(restored)
    }

    @Test
    fun `a normal page has no Restore`() {
        setDetail(EntityDetailUiState(entity = acme, isLoading = false))
        composeTestRule.onAllNodesWithTag("entity-restore").assertCountEquals(0)
    }

    @Test
    fun `a missing entity says so`() {
        setDetail(EntityDetailUiState(isLoading = false, notFound = true))
        composeTestRule.onNodeWithTag("entity-not-found").assertIsDisplayed()
    }

    @Test
    fun `merge needs an explicit pick, starts with nothing filled and passes the choices on`() {
        var picked: Triple<String, Boolean, Set<String>>? = null
        val other = Entity(
            id = "e2",
            kind = EntityKind.ORGANISATION,
            name = "Acme Inc",
            core = mapOf("notes" to "hello")
        )
        setDetail(
            EntityDetailUiState(entity = acme, isLoading = false, mergeCandidates = listOf(other)),
            onMerge = { o, keep, fill -> picked = Triple(o.id, keep, fill) }
        )
        composeTestRule.onNodeWithTag("entity-merge").performClick()
        composeTestRule.onNodeWithTag("entity-merge-confirm").assertIsNotEnabled()
        composeTestRule.onNodeWithTag("entity-merge-candidate-e2").performClick()
        composeTestRule.onNodeWithTag("entity-merge-confirm").assertIsEnabled()
        // This page's Acme is merged away into the pick. Acme's url is offered (the survivor has
        // none) but stays unticked, so nothing is filled unless asked for.
        composeTestRule.onNodeWithTag("entity-merge-fill-url").assertIsOff()
        composeTestRule.onNodeWithTag("entity-merge-confirm").performClick()
        assertEquals(Triple("e2", false, emptySet<String>()), picked)
    }

    @Test
    fun `a fillable field is offered unticked and only sent when ticked`() {
        var picked: Triple<String, Boolean, Set<String>>? = null
        val other = Entity(id = "e2", kind = EntityKind.ORGANISATION, name = "Acme Inc")
        val withNotes = acme.copy(core = mapOf("notes" to "hello"))
        setDetail(
            EntityDetailUiState(
                entity = withNotes,
                isLoading = false,
                mergeCandidates = listOf(other)
            ),
            onMerge = { o, keep, fill -> picked = Triple(o.id, keep, fill) }
        )
        composeTestRule.onNodeWithTag("entity-merge").performClick()
        composeTestRule.onNodeWithTag("entity-merge-candidate-e2").performClick()
        composeTestRule.onNodeWithTag("entity-merge-fill-notes").assertIsOff()
        composeTestRule.onNodeWithTag("entity-merge-fill-notes").performScrollTo().performClick()
        composeTestRule.onNodeWithTag("entity-merge-fill-notes").assertIsOn()
        composeTestRule.onNodeWithTag("entity-merge-confirm").performClick()
        assertEquals(Triple("e2", false, setOf("notes")), picked)
    }

    @Test
    fun `a merged-away page shows the undo and the survivor`() {
        val survivor = Entity(id = "e2", name = "Acme Inc")
        setDetail(
            EntityDetailUiState(
                entity = acme,
                isLoading = false,
                mergeOutcome = MergeOutcome(
                    acme,
                    survivor,
                    thisWasMergedAway = true,
                    response = EntityMergeResponse()
                )
            )
        )
        composeTestRule.onNodeWithTag("entity-merge-undo").assertIsDisplayed()
        composeTestRule.onNodeWithTag("entity-open-survivor").assertIsDisplayed()
    }

    // --- form ------------------------------------------------------------------------------

    private fun setForm(kind: String, entity: Entity? = null, onSave: (EntityInput) -> Unit = {}) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                EntityFormDialog(
                    kind = kind,
                    entity = entity,
                    saving = false,
                    error = null,
                    onSave = onSave,
                    onDismiss = {}
                )
            }
        }
    }

    @Test
    fun `a blank name is refused before saving`() {
        var saved: EntityInput? = null
        setForm(EntityKind.ORGANISATION, onSave = { saved = it })
        composeTestRule.onNodeWithTag("entity-form-save").performClick()
        composeTestRule.onNodeWithText("Enter a name.").assertIsDisplayed()
        assertEquals(null, saved)
    }

    @Test
    fun `an organisation saves its name and core`() {
        var saved: EntityInput? = null
        setForm(EntityKind.ORGANISATION, onSave = { saved = it })
        composeTestRule.onNodeWithTag("entity-form-name").performTextInput("Acme")
        composeTestRule.onNodeWithTag("entity-form-url").performTextInput("https://a.example")
        composeTestRule.onNodeWithTag("entity-form-save").performClick()
        assertEquals("Acme", saved?.name)
        assertEquals(EntityKind.ORGANISATION, saved?.kind)
        assertEquals("https://a.example", saved?.core?.get("url"))
    }

    @Test
    fun `a pet date that is not a date is refused`() {
        var saved: EntityInput? = null
        setForm(EntityKind.PET, onSave = { saved = it })
        composeTestRule.onNodeWithTag("entity-form-name").performTextInput("Rex")
        composeTestRule.onNodeWithTag("entity-form-birth").performTextInput("2020-13")
        composeTestRule.onNodeWithTag("entity-form-save").performClick()
        composeTestRule.onNodeWithText("Enter Birth as YYYY, YYYY-MM or YYYY-MM-DD.")
            .performScrollTo().assertIsDisplayed()
        assertEquals(null, saved)
    }

    @Test
    fun `editing keeps the stored core keys the form does not show`() {
        var saved: EntityInput? = null
        val place = Entity(
            id = "p1",
            kind = EntityKind.PLACE,
            name = "Cabin",
            core = mapOf("address" to mapOf("coordinates" to "geo:1,2"))
        )
        setForm(EntityKind.PLACE, place, onSave = { saved = it })
        composeTestRule.onNodeWithTag("entity-form-city").performTextInput("Oslo")
        composeTestRule.onNodeWithTag("entity-form-save").performClick()
        val input = checkNotNull(saved)
        val address = input.core?.get("address") as Map<*, *>
        assertEquals("geo:1,2", address["coordinates"])
        assertNotNull(address["components"])
        assertEquals(EntityKind.PLACE, input.kind)
    }
}
