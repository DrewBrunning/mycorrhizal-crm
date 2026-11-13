package com.mycorrhizal.crm.feature.shares

import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import com.mycorrhizal.crm.model.network.IncomingShareGrant
import com.mycorrhizal.crm.model.network.IncomingShareGrantsPage
import com.mycorrhizal.crm.model.network.ShareBlock
import com.mycorrhizal.crm.model.network.ShareGrantNotifications
import com.mycorrhizal.crm.model.network.ShareGrantStatuses
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.flow.launchIn
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class SharedWithMeViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<ShareGrantRepository>()

    private val pending = IncomingShareGrant(
        id = "g1",
        fromUsername = "dana",
        entityType = "contact",
        label = "Dana White",
        status = ShareGrantStatuses.PENDING
    )
    private val active = IncomingShareGrant(
        id = "g2",
        fromUsername = "dana",
        entityType = "note",
        label = "Note",
        status = ShareGrantStatuses.ACTIVE
    )

    private fun stubLoad(
        grants: List<IncomingShareGrant> = listOf(pending, active),
        blocks: List<ShareBlock> = emptyList()
    ) {
        coEvery { repository.listIncoming(any(), any()) } returns
            Result.success(IncomingShareGrantsPage(shareGrants = grants))
        coEvery { repository.blocks() } returns Result.success(blocks)
    }

    @Test
    fun `loads incoming grants and blocked senders on init`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(blocks = listOf(ShareBlock(7, "mallory")))
        val vm = SharedWithMeViewModel(repository)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals(listOf("g1", "g2"), state.grants.map { it.id })
        assertEquals("mallory", state.blocks.single().username)
        assertFalse(state.isLoading)
        assertNull(state.loadError)
    }

    @Test
    fun `a failed load shows a retryable error and keeps no stale rows`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad()
        val vm = SharedWithMeViewModel(repository)
        advanceUntilIdle()
        assertEquals(2, vm.uiState.value.grants.size)

        coEvery { repository.listIncoming(any(), any()) } returns
            Result.failure(ApiError.Client(404, "gone"))
        vm.load()
        advanceUntilIdle()

        assertTrue(vm.uiState.value.grants.isEmpty())
        assertNotNull(vm.uiState.value.loadError)

        stubLoad()
        vm.load()
        advanceUntilIdle()
        assertNull(vm.uiState.value.loadError)
        assertEquals(2, vm.uiState.value.grants.size)
    }

    @Test
    fun `accept calls the repository once and reloads`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad()
        coEvery { repository.accept("g1") } returns Result.success(pending.copy(status = "active"))
        val vm = SharedWithMeViewModel(repository)
        advanceUntilIdle()

        vm.accept(pending)
        advanceUntilIdle()

        coVerify(exactly = 1) { repository.accept("g1") }
        coVerify(exactly = 2) { repository.listIncoming(any(), any()) }
        assertFalse(vm.uiState.value.busy)
    }

    @Test
    fun `accepting a grant drops the drawer badge count without leaving the screen`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad()
        coEvery { repository.accept("g1") } returns Result.success(pending.copy(status = "active"))
        coEvery { repository.notifications() } returnsMany listOf(
            Result.success(ShareGrantNotifications(pendingCount = 1)),
            Result.success(ShareGrantNotifications(pendingCount = 0))
        )
        // The same singleton the app injects into both view models.
        val changes = ShareGrantChanges()
        val badge = ShareGrantBadgeViewModel(repository, changes)
        val vm = SharedWithMeViewModel(repository, changes)
        advanceUntilIdle()
        assertEquals(1, badge.pendingCount.value)

        vm.accept(pending)
        advanceUntilIdle()

        assertEquals(0, badge.pendingCount.value)
    }

    @Test
    fun `every recipient action signals a share-grant change, a failed one included`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad()
        coEvery { repository.accept(any()) } returns Result.success(pending)
        coEvery { repository.decline(any(), any()) } returns Result.success(pending)
        coEvery { repository.leave(any()) } returns Result.failure(ApiError.Client(409, "x"))
        coEvery { repository.unblock(any()) } returns Result.success(Unit)
        val changes = ShareGrantChanges()
        var signals = 0
        changes.changes.onEach { signals++ }.launchIn(backgroundScope)
        val vm = SharedWithMeViewModel(repository, changes)
        advanceUntilIdle()

        vm.accept(pending)
        advanceUntilIdle()
        assertEquals(1, signals)
        vm.requestDecline(pending)
        vm.confirmPrompt()
        advanceUntilIdle()
        assertEquals(2, signals)
        vm.requestBlock(pending)
        vm.confirmPrompt()
        advanceUntilIdle()
        assertEquals(3, signals)
        vm.requestLeave(active)
        vm.confirmPrompt()
        advanceUntilIdle()
        assertEquals(4, signals)
        vm.unblock(ShareBlock(7, "mallory"))
        advanceUntilIdle()
        assertEquals(5, signals)
    }

    @Test
    fun `a pending grant is never accepted without an explicit accept`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad()
        val vm = SharedWithMeViewModel(repository)
        advanceUntilIdle()
        coVerify(exactly = 0) { repository.accept(any()) }
    }

    @Test
    fun `decline and block and leave are confirmation gated`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad()
        coEvery { repository.decline(any(), any()) } returns
            Result.success(pending.copy(status = "declined"))
        coEvery { repository.leave(any()) } returns Result.success(active.copy(status = "declined"))
        val vm = SharedWithMeViewModel(repository)
        advanceUntilIdle()

        vm.requestDecline(pending)
        vm.requestBlock(pending)
        vm.requestLeave(active)
        advanceUntilIdle()
        coVerify(exactly = 0) { repository.decline(any(), any()) }
        coVerify(exactly = 0) { repository.leave(any()) }
        // Only one prompt at a time: the first request wins until it is resolved.
        assertTrue(vm.uiState.value.prompt is SharedWithMePrompt.Decline)

        vm.dismissPrompt()
        assertNull(vm.uiState.value.prompt)
        coVerify(exactly = 0) { repository.decline(any(), any()) }
    }

    @Test
    fun `confirming a decline calls decline without block`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad()
        coEvery { repository.decline("g1", false) } returns
            Result.success(pending.copy(status = "declined"))
        val vm = SharedWithMeViewModel(repository)
        advanceUntilIdle()

        vm.requestDecline(pending)
        vm.confirmPrompt()
        advanceUntilIdle()

        coVerify(exactly = 1) { repository.decline("g1", false) }
        assertNull(vm.uiState.value.prompt)
    }

    @Test
    fun `confirming a block declines with block true`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad()
        coEvery { repository.decline("g1", true) } returns
            Result.success(pending.copy(status = "declined"))
        val vm = SharedWithMeViewModel(repository)
        advanceUntilIdle()

        vm.requestBlock(pending)
        vm.confirmPrompt()
        advanceUntilIdle()

        coVerify(exactly = 1) { repository.decline("g1", true) }
    }

    @Test
    fun `confirming a leave calls leave`() = runTest(mainDispatcherRule.testDispatcher) {
        stubLoad()
        coEvery { repository.leave("g2") } returns Result.success(active.copy(status = "declined"))
        val vm = SharedWithMeViewModel(repository)
        advanceUntilIdle()

        vm.requestLeave(active)
        vm.confirmPrompt()
        advanceUntilIdle()

        coVerify(exactly = 1) { repository.leave("g2") }
    }

    @Test
    fun `a failed action surfaces the error, closes the prompt and reloads`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad()
        coEvery { repository.accept("g1") } returns Result.failure(ApiError.Client(409, "x"))
        val vm = SharedWithMeViewModel(repository)
        advanceUntilIdle()

        vm.accept(pending)
        advanceUntilIdle()

        assertNotNull(vm.uiState.value.actionError)
        vm.onActionErrorShown()
        assertNull(vm.uiState.value.actionError)
        // The row may have changed under us (revoked, expired): refresh rather than keep a stale one.
        coVerify(exactly = 2) { repository.listIncoming(any(), any()) }
    }

    @Test
    fun `unblock removes the block and reloads`() = runTest(mainDispatcherRule.testDispatcher) {
        stubLoad(blocks = listOf(ShareBlock(7, "mallory")))
        coEvery { repository.unblock(7) } returns Result.success(Unit)
        val vm = SharedWithMeViewModel(repository)
        advanceUntilIdle()

        vm.unblock(ShareBlock(7, "mallory"))
        advanceUntilIdle()

        coVerify(exactly = 1) { repository.unblock(7) }
        coVerify(exactly = 2) { repository.blocks() }
    }

    @Test
    fun `a failed blocks load is an error without dropping the grants list`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { repository.listIncoming(any(), any()) } returns
            Result.success(IncomingShareGrantsPage(shareGrants = listOf(pending)))
        coEvery { repository.blocks() } returns Result.failure(ApiError.Client(500, "x"))
        val vm = SharedWithMeViewModel(repository)
        advanceUntilIdle()

        assertEquals(1, vm.uiState.value.grants.size)
        assertTrue(vm.uiState.value.blocks.isEmpty())
        assertNotNull(vm.uiState.value.loadError)
    }
}
