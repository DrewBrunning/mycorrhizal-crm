package com.mycorrhizal.crm.feature.trash

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.AppVersion
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.model.network.TrashedContact
import com.mycorrhizal.crm.model.network.TrashedEntity
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.LocalServerVersion
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w480dp-h1600dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class TrashScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val ada = TrashedContact(
        contact = ContactSummary(id = 1, firstname = "Ada", lastname = "Test"),
        deletedAt = "2026-10-01T00:00:00Z",
        purgeAt = "2026-10-31T00:00:00Z"
    )
    private val bob = TrashedContact(
        contact = ContactSummary(id = 2, firstname = "Bob", lastname = "Test"),
        deletedAt = "2026-10-02T00:00:00Z",
        purgeAt = null
    )

    @Test
    fun `rows show the name and fire restore and delete-forever for that row`() {
        var restored: Int? = null
        var purged: Int? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                TrashList(
                    items = listOf(ada, bob),
                    retentionDays = 30,
                    enabled = true,
                    onRestore = { restored = it.contact.id },
                    onPurge = { purged = it.contact.id }
                )
            }
        }

        composeTestRule.onNodeWithText("Ada Test").assertIsDisplayed()
        composeTestRule.onNodeWithText("Bob Test").assertIsDisplayed()
        composeTestRule.onNodeWithTag("trash-restore-2").performClick()
        composeTestRule.onNodeWithTag("trash-delete-1").performClick()
        assertEquals(2, restored)
        assertEquals(1, purged)
    }

    @Test
    fun `retention zero says the contacts are kept until the trash is emptied`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                TrashList(
                    items = listOf(bob),
                    retentionDays = 0,
                    enabled = true,
                    onRestore = {},
                    onPurge = {}
                )
            }
        }

        composeTestRule.onNodeWithTag("trash-retention").assertIsDisplayed()
        composeTestRule.onNodeWithText("Deleted contacts are kept until you empty the trash.")
            .assertIsDisplayed()
    }

    @Test
    fun `list passes the accessibility sweep`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                TrashList(
                    items = listOf(ada, bob),
                    retentionDays = 30,
                    enabled = true,
                    onRestore = {},
                    onPurge = {}
                )
            }
        }

        composeTestRule.assertAccessibleSemantics()
    }

    private fun screenWith(
        state: TrashUiState,
        events: Channel<TrashUiEvent> = Channel(Channel.BUFFERED),
        onMenuClick: (() -> Unit)? = {},
        serverVersion: AppVersion? = null
    ): TrashViewModel {
        val vm = mockk<TrashViewModel>(relaxed = true)
        every { vm.uiState } returns MutableStateFlow(state)
        every { vm.events } returns events.receiveAsFlow()
        composeTestRule.setContent {
            CompositionLocalProvider(LocalServerVersion provides serverVersion) {
                MycorrhizalTheme { TrashScreen(onMenuClick = onMenuClick, viewModel = vm) }
            }
        }
        return vm
    }

    @Test
    fun `resuming the screen reloads the list`() {
        // The drawer restores a saved back-stack entry, so the view model outlives
        // leaving the screen; without a resume reload the list stays stale.
        val vm = screenWith(TrashUiState(items = listOf(ada)))

        verify { vm.reload() }
    }

    @Test
    fun `loading with no items shows the skeleton`() {
        screenWith(TrashUiState(isLoading = true))
        composeTestRule.onNodeWithTag("trash-loading").assertIsDisplayed()
        composeTestRule.onNodeWithTag("trash-empty").assertDoesNotExist()
    }

    @Test
    fun `a failed first load shows the error text`() {
        screenWith(TrashUiState(error = ApiError.Client(400, "boom")))
        composeTestRule.onNodeWithText("boom").assertIsDisplayed()
    }

    @Test
    fun `an empty trash shows the empty state with the retention window`() {
        screenWith(TrashUiState(retentionDays = 30))
        composeTestRule.onNodeWithText("Trash is empty").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Deleted contacts are kept for 30 days, then deleted forever."
        )
            .assertIsDisplayed()
    }

    @Test
    fun `menu button is shown and fires when a drawer handler is given`() {
        var clicked = false
        screenWith(TrashUiState(), onMenuClick = { clicked = true })
        composeTestRule.onNodeWithContentDescription("Menu").performClick()
        assertTrue(clicked)
    }

    @Test
    fun `menu button is hidden without a drawer handler`() {
        screenWith(TrashUiState(), onMenuClick = null)
        composeTestRule.onNodeWithContentDescription("Menu").assertDoesNotExist()
    }

    @Test
    fun `restore delegates to the view model`() {
        val vm = screenWith(TrashUiState(items = listOf(ada), retentionDays = 30))
        composeTestRule.onNodeWithTag("trash-restore-1").performClick()
        verify { vm.restore(ada) }
    }

    @Test
    fun `delete forever asks first and purges only after confirming`() {
        val vm = screenWith(TrashUiState(items = listOf(ada), retentionDays = 30))
        composeTestRule.onNodeWithTag("trash-delete-1").performClick()
        composeTestRule.onNodeWithText("Permanently delete Ada Test? This cannot be undone.")
            .assertIsDisplayed()
        verify(exactly = 0) { vm.purge(any()) }
        composeTestRule.onNodeWithTag("trash-purge-confirm").performClick()
        verify { vm.purge(ada) }
        composeTestRule.onNodeWithTag("trash-purge-confirm").assertDoesNotExist()
    }

    @Test
    fun `cancelling the purge dialog purges nothing`() {
        val vm = screenWith(TrashUiState(items = listOf(ada), retentionDays = 30))
        composeTestRule.onNodeWithTag("trash-delete-1").performClick()
        composeTestRule.onNodeWithText("Cancel").performClick()
        verify(exactly = 0) { vm.purge(any()) }
        composeTestRule.onNodeWithTag("trash-purge-confirm").assertDoesNotExist()
    }

    @Test
    fun `empty trash asks first and empties only after confirming`() {
        val vm = screenWith(TrashUiState(items = listOf(ada, bob), retentionDays = 30))
        composeTestRule.onNodeWithTag("trash-empty").performClick()
        composeTestRule.onNodeWithText(
            "Permanently delete all 2 contacts in the Trash? This cannot be undone."
        )
            .assertIsDisplayed()
        verify(exactly = 0) { vm.emptyTrash() }
        composeTestRule.onNodeWithTag("trash-empty-confirm").performClick()
        verify { vm.emptyTrash() }
        composeTestRule.onNodeWithTag("trash-empty-confirm").assertDoesNotExist()
    }

    @Test
    fun `cancelling the empty dialog empties nothing`() {
        val vm = screenWith(TrashUiState(items = listOf(ada), retentionDays = 30))
        composeTestRule.onNodeWithTag("trash-empty").performClick()
        composeTestRule.onNodeWithText("Cancel").performClick()
        verify(exactly = 0) { vm.emptyTrash() }
        composeTestRule.onNodeWithTag("trash-empty-confirm").assertDoesNotExist()
    }

    @Test
    fun `actions are disabled while a mutation is in flight`() {
        screenWith(TrashUiState(items = listOf(ada), retentionDays = 30, isWorking = true))
        composeTestRule.onNodeWithTag("trash-empty").assertIsNotEnabled()
        composeTestRule.onNodeWithTag("trash-restore-1").assertIsNotEnabled()
    }

    @Test
    fun `a refresh failure with items still listed surfaces as a snackbar`() {
        screenWith(
            TrashUiState(
                items = listOf(ada),
                retentionDays = 30,
                error = ApiError.Client(400, "refresh failed")
            )
        )
        composeTestRule.onNodeWithText("refresh failed").assertIsDisplayed()
    }

    @Test
    fun `events are announced as snackbars`() {
        val cases = listOf(
            TrashUiEvent.Restored("Ada", 0) to "Restored Ada.",
            TrashUiEvent.Restored("Ada", 1) to
                "Restored Ada, but 1 item could not be restored because what it linked to no longer exists.",
            TrashUiEvent.RestoreConflict("Ada") to
                "Can't restore Ada: another contact already uses this contact's ID. Merge or delete that contact first.",
            TrashUiEvent.Purged("Ada") to "Permanently deleted Ada.",
            TrashUiEvent.Emptied(2) to "Permanently deleted 2 contacts.",
            TrashUiEvent.Failed(ApiError.Client(400, "nope")) to "nope"
        )
        val events = Channel<TrashUiEvent>(Channel.BUFFERED)
        screenWith(TrashUiState(), events)
        for ((event, text) in cases) {
            events.trySend(event)
            composeTestRule.waitUntil(5_000) {
                composeTestRule.onAllNodesWithText(text).fetchSemanticsNodes().isNotEmpty()
            }
            composeTestRule.mainClock.advanceTimeBy(11_000)
        }
    }

    private val rex = TrashedEntity(
        entity = Entity(id = "e1", kind = EntityKind.PET, name = "Rex"),
        deletedAt = "2026-10-01T00:00:00Z",
        purgeAt = "2026-10-31T00:00:00Z",
        mergedInto = "e2",
        mergedIntoName = "Rexy"
    )
    private val acme = TrashedEntity(
        entity = Entity(id = "e3", kind = EntityKind.ORGANISATION, name = "Acme"),
        deletedAt = "2026-10-02T00:00:00Z"
    )

    @Test
    fun `entity rows show name kind and merge origin and restore only`() {
        var restored: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                TrashList(
                    items = listOf(ada),
                    entities = listOf(rex, acme),
                    retentionDays = 30,
                    enabled = true,
                    onRestore = {},
                    onPurge = {},
                    onRestoreEntity = { restored = it.entity.id }
                )
            }
        }

        composeTestRule.onNodeWithText("Rex").assertIsDisplayed()
        composeTestRule.onNodeWithText("Pet").assertIsDisplayed()
        composeTestRule.onNodeWithText("Organisation").assertIsDisplayed()
        composeTestRule.onNodeWithText("Merged into Rexy").assertIsDisplayed()
        composeTestRule.onNodeWithTag("trash-entity-restore-e1").performClick()
        assertEquals("e1", restored)
        // Entities have no purge endpoint: no Delete forever on those rows.
        composeTestRule.onNodeWithTag("trash-delete-e1").assertDoesNotExist()
        composeTestRule.onNodeWithTag("trash-entity-restore-e3").assertIsDisplayed()
    }

    @Test
    fun `entity restore is disabled while an action runs`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                TrashList(
                    items = emptyList(),
                    entities = listOf(acme),
                    retentionDays = 30,
                    enabled = false,
                    onRestore = {},
                    onPurge = {},
                    onRestoreEntity = {}
                )
            }
        }

        composeTestRule.onNodeWithTag("trash-entity-restore-e3").assertIsNotEnabled()
    }

    @Test
    fun `entities alone show the list, not the empty state, and hide empty trash`() {
        screenWith(TrashUiState(entities = listOf(acme)))

        composeTestRule.onNodeWithText("Acme").assertIsDisplayed()
        composeTestRule.onNodeWithText("Trash is empty").assertDoesNotExist()
        // Empty trash purges contacts only, so it is offered only when contacts are listed.
        composeTestRule.onNodeWithTag("trash-empty").assertDoesNotExist()
    }

    @Test
    fun `nothing deleted shows the empty state`() {
        screenWith(TrashUiState())

        composeTestRule.onNodeWithText("Trash is empty").assertIsDisplayed()
    }

    @Test
    fun `tapping entity restore calls the view model`() {
        val vm = screenWith(TrashUiState(entities = listOf(rex)))

        composeTestRule.onNodeWithTag("trash-entity-restore-e1").performClick()

        verify { vm.restoreEntity(rex) }
    }

    @Test
    fun `an entity restore announces success and the same-name warning`() {
        val events = Channel<TrashUiEvent>(Channel.BUFFERED)
        screenWith(TrashUiState(entities = listOf(rex)), events)

        events.trySend(TrashUiEvent.EntityRestored("Rex", sameNameLive = true))

        composeTestRule.waitUntil(5_000) {
            composeTestRule.onAllNodesWithText("Restored Rex.").fetchSemanticsNodes().isNotEmpty()
        }
        composeTestRule.onNodeWithText("Restored Rex.").assertIsDisplayed()
    }

    // Issue #2527: the entity trash needs a v1.7.0 server; the view model is told before loading.
    @Test
    fun `a v1_6_0 server starts the view model without the entity trash`() {
        val vm = screenWith(TrashUiState(items = listOf(ada)), serverVersion = AppVersion(1, 6, 0))
        composeTestRule.waitForIdle()
        verify { vm.start(false) }
        verify(exactly = 0) { vm.start(true) }
    }

    @Test
    fun `a v1_7_0 server starts the view model with the entity trash`() {
        val vm = screenWith(TrashUiState(items = listOf(ada)), serverVersion = AppVersion(1, 7, 0))
        composeTestRule.waitForIdle()
        verify { vm.start(true) }
    }

    @Test
    fun `an unknown server version starts the view model with the entity trash`() {
        val vm = screenWith(TrashUiState(items = listOf(ada)), serverVersion = null)
        composeTestRule.waitForIdle()
        verify { vm.start(true) }
    }

    private fun reasonCoded() = ApiError.Client(
        409,
        "RAW SERVER TEXT",
        reason = ErrorReason(ErrorReasonCode.ENTITY_NOT_FOUND, emptyMap())
    )

    @Test
    fun `a failed first load is worded from its reason code, never the server text`() {
        screenWith(TrashUiState(error = reasonCoded()))
        composeTestRule.onNodeWithText("Entity not found").assertIsDisplayed()
        composeTestRule.onNodeWithText("RAW SERVER TEXT").assertDoesNotExist()
    }

    @Test
    fun `a refresh failure with items listed is worded from its reason code`() {
        screenWith(TrashUiState(items = listOf(ada), retentionDays = 30, error = reasonCoded()))
        composeTestRule.onNodeWithText("Entity not found").assertIsDisplayed()
        composeTestRule.onNodeWithText("RAW SERVER TEXT").assertDoesNotExist()
    }

    @Test
    fun `a failed action event is worded from its reason code`() {
        val events = Channel<TrashUiEvent>(Channel.BUFFERED)
        screenWith(TrashUiState(), events)
        events.trySend(TrashUiEvent.Failed(reasonCoded()))
        composeTestRule.waitUntil(5_000) {
            composeTestRule.onAllNodesWithText("Entity not found").fetchSemanticsNodes()
                .isNotEmpty()
        }
        composeTestRule.onNodeWithText("RAW SERVER TEXT").assertDoesNotExist()
    }
}
