package com.mycorrhizal.crm.feature.shares

import com.mycorrhizal.crm.domain.repository.ContactShareRepository
import com.mycorrhizal.crm.model.network.ContactShare
import com.mycorrhizal.crm.model.network.ContactShareStatuses
import com.mycorrhizal.crm.model.network.ContactSharesPage
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.model.network.RowImportAction
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class ContactSharesViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<ContactShareRepository>()

    private fun incomingShare(id: String = "s1", name: String = "Dana White") = ContactShare(
        id = id,
        fromUserId = 7,
        toUserId = 1,
        contactDisplayName = name,
        status = ContactShareStatuses.PENDING,
    )

    private fun outgoingShare(id: String = "s3", name: String = "Casey Tran") = ContactShare(
        id = id,
        fromUserId = 1,
        toUserId = 9,
        contactDisplayName = name,
        status = ContactShareStatuses.PENDING,
    )

    private fun stubLists(
        incoming: List<ContactShare> = emptyList(),
        outgoing: List<ContactShare> = emptyList(),
    ) {
        coEvery { repository.listIncoming(any(), any()) } returns Result.success(
            ContactSharesPage(contactShares = incoming, usernames = mapOf("7" to "dana")),
        )
        coEvery { repository.listOutgoing(any(), any()) } returns Result.success(
            ContactSharesPage(contactShares = outgoing, usernames = mapOf("9" to "casey")),
        )
    }

    @Test
    fun `loads incoming and outgoing shares on init`() = runTest(mainDispatcherRule.testDispatcher) {
        stubLists(incoming = listOf(incomingShare()), outgoing = listOf(outgoingShare()))

        val vm = ContactSharesViewModel(repository)
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isLoading)
        assertEquals(1, vm.uiState.value.incoming.size)
        assertEquals("Dana White", vm.uiState.value.incoming[0].contactDisplayName)
        assertEquals(1, vm.uiState.value.outgoing.size)
        assertEquals("dana", vm.uiState.value.usernames["7"])
        assertEquals("casey", vm.uiState.value.usernames["9"])
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `load failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.listIncoming(any(), any()) } returns Result.failure(ApiError.Client(500, "boom"))
        coEvery { repository.listOutgoing(any(), any()) } returns Result.failure(ApiError.Client(500, "boom"))

        val vm = ContactSharesViewModel(repository)
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isLoading)
        assertTrue(vm.uiState.value.error != null)
    }

    @Test
    fun `a single failed list surfaces the error while the other list still renders`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.listIncoming(any(), any()) } returns Result.failure(ApiError.Client(500, "boom"))
            coEvery { repository.listOutgoing(any(), any()) } returns Result.success(
                ContactSharesPage(contactShares = listOf(outgoingShare())),
            )

            val vm = ContactSharesViewModel(repository)
            advanceUntilIdle()

            assertFalse(vm.uiState.value.isLoading)
            // The failed list must not masquerade as "no incoming shares".
            assertTrue(vm.uiState.value.error != null)
            assertEquals(0, vm.uiState.value.incoming.size)
            assertEquals(1, vm.uiState.value.outgoing.size)
        }

    @Test
    fun `selectTab switches the tab`() = runTest(mainDispatcherRule.testDispatcher) {
        stubLists()
        val vm = ContactSharesViewModel(repository)
        advanceUntilIdle()

        vm.selectTab(SharesTab.OUTGOING)

        assertEquals(SharesTab.OUTGOING, vm.uiState.value.selectedTab)
    }

    // --- Accept flow (M15: accept is preview-then-confirm, never a one-tap button) ---

    @Test
    fun `openAccept fetches the preview and suggests the row default action`() = runTest(mainDispatcherRule.testDispatcher) {
        stubLists(incoming = listOf(incomingShare()))
        coEvery { repository.accept("s1") } returns Result.success(
            ImportPreviewResponse(
                sessionId = "import-1",
                rows = listOf(
                    ImportRowPreview(
                        rowIndex = 0,
                        parsedContact = mapOf("firstname" to "Dana"),
                        suggestedAction = "update",
                    ),
                ),
                totalRows = 1,
                validRows = 1,
                duplicateCount = 0,
                errorCount = 0,
            ),
        )

        val vm = ContactSharesViewModel(repository)
        advanceUntilIdle()

        vm.openAccept(incomingShare())
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals("s1", state.acceptingShare?.id)
        assertFalse(state.previewLoading)
        assertEquals("import-1", state.preview?.sessionId)
        // The row's suggested_action (update/skip) becomes the default choice.
        assertEquals("update", state.confirmAction)
    }

    @Test
    fun `confirmAccept confirms with the chosen action and removes the share from incoming`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubLists(incoming = listOf(incomingShare()))
            coEvery { repository.accept("s1") } returns Result.success(
                ImportPreviewResponse(
                    sessionId = "import-1",
                    rows = listOf(ImportRowPreview(rowIndex = 0, suggestedAction = "add")),
                    totalRows = 1,
                ),
            )
            coEvery { repository.confirm(any(), any(), any()) } returns Result.success(
                ImportResult(totalProcessed = 1, created = 1),
            )
            // After a successful confirm, the incoming list is refreshed — the
            // accepted share no longer shows as pending (the accepted contact now
            // lives locally).
            coEvery { repository.listIncoming(any(), any()) } returns Result.success(ContactSharesPage())

            val vm = ContactSharesViewModel(repository)
            advanceUntilIdle()

            vm.openAccept(incomingShare())
            advanceUntilIdle()

            vm.setConfirmAction("add")
            vm.confirmAccept()
            advanceUntilIdle()

            coVerify { repository.confirm("s1", "import-1", listOf(RowImportAction(rowIndex = 0, action = "add"))) }
            assertNull(vm.uiState.value.acceptingShare)
            assertTrue(vm.uiState.value.incoming.isEmpty())
        }

    // --- Decline flow (M15: the repository is NOT called until the user confirms) ---

    @Test
    fun `requestDecline does not call the repository`() = runTest(mainDispatcherRule.testDispatcher) {
        stubLists(incoming = listOf(incomingShare()))

        val vm = ContactSharesViewModel(repository)
        advanceUntilIdle()

        vm.requestDecline(incomingShare())
        advanceUntilIdle()

        assertEquals("s1", vm.uiState.value.declinePendingId)
        coVerify(exactly = 0) { repository.decline(any()) }
    }

    @Test
    fun `confirmDecline calls the repository and removes the share from incoming`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubLists(incoming = listOf(incomingShare()))
            coEvery { repository.decline("s1") } returns Result.success(Unit)
            // After declining, the pending share leaves the inbox.
            coEvery { repository.listIncoming(any(), any()) } returns Result.success(ContactSharesPage())

            val vm = ContactSharesViewModel(repository)
            advanceUntilIdle()

            vm.requestDecline(incomingShare())
            advanceUntilIdle()
            vm.confirmDecline()
            advanceUntilIdle()

            coVerify(exactly = 1) { repository.decline("s1") }
            assertNull(vm.uiState.value.declinePendingId)
            assertTrue(vm.uiState.value.incoming.isEmpty())
        }

    @Test
    fun `cancelDecline leaves the share untouched`() = runTest(mainDispatcherRule.testDispatcher) {
        stubLists(incoming = listOf(incomingShare()))

        val vm = ContactSharesViewModel(repository)
        advanceUntilIdle()

        vm.requestDecline(incomingShare())
        advanceUntilIdle()
        vm.cancelDecline()
        advanceUntilIdle()

        assertNull(vm.uiState.value.declinePendingId)
        coVerify(exactly = 0) { repository.decline(any()) }
        assertEquals(1, vm.uiState.value.incoming.size)
    }
}
