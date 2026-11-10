package com.mycorrhizal.crm.feature.shares

import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import com.mycorrhizal.crm.model.network.OutgoingShareGrant
import com.mycorrhizal.crm.model.network.OutgoingShareGrantsPage
import com.mycorrhizal.crm.model.network.ShareGrantPatch
import com.mycorrhizal.crm.model.network.ShareGrantStatuses
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import java.time.Instant
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class SharedByMeViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<ShareGrantRepository>()

    private val contactGrant = OutgoingShareGrant(
        id = "g1",
        toUsername = "casey",
        entityType = "contact",
        label = "Dana White",
        sections = listOf("emails", "phones", "notes"),
        status = ShareGrantStatuses.ACTIVE,
        expiresAt = "2027-01-01T00:00:00Z",
        revision = 3
    )
    private val circleGrant = OutgoingShareGrant(
        id = "g2",
        toUsername = "casey",
        entityType = "circle",
        label = "Family",
        status = ShareGrantStatuses.PENDING,
        memberCount = 4,
        revision = 1
    )
    private val revoked = contactGrant.copy(id = "g3", status = ShareGrantStatuses.REVOKED)

    private fun stubLoad(vararg grants: OutgoingShareGrant) {
        coEvery { repository.listOutgoing(any(), any()) } returns
            Result.success(OutgoingShareGrantsPage(shareGrants = grants.toList()))
    }

    private fun newVm(): SharedByMeViewModel = SharedByMeViewModel(repository).also {
        it.now = { Instant.parse("2026-11-10T00:00:00Z") }
    }

    @Test
    fun `loads outgoing grants`() = runTest(mainDispatcherRule.testDispatcher) {
        stubLoad(contactGrant, circleGrant)
        val vm = newVm()
        advanceUntilIdle()
        assertEquals(listOf("g1", "g2"), vm.uiState.value.grants.map { it.id })
        assertEquals(4, vm.uiState.value.grants[1].memberCount)
    }

    @Test
    fun `a failed load shows a retryable error and no rows`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(contactGrant)
        val vm = newVm()
        advanceUntilIdle()
        coEvery { repository.listOutgoing(any(), any()) } returns
            Result.failure(ApiError.Server(500, "x"))
        vm.load()
        advanceUntilIdle()
        assertTrue(vm.uiState.value.grants.isEmpty())
        assertNotNull(vm.uiState.value.loadError)
    }

    @Test
    fun `revoke is confirmation gated`() = runTest(mainDispatcherRule.testDispatcher) {
        stubLoad(contactGrant)
        coEvery { repository.revoke("g1") } returns
            Result.success(contactGrant.copy(status = ShareGrantStatuses.REVOKED))
        val vm = newVm()
        advanceUntilIdle()

        vm.requestRevoke(contactGrant)
        coVerify(exactly = 0) { repository.revoke(any()) }
        assertTrue(vm.uiState.value.prompt is SharedByMePrompt.Revoke)

        vm.dismissPrompt()
        coVerify(exactly = 0) { repository.revoke(any()) }

        vm.requestRevoke(contactGrant)
        vm.confirmPrompt()
        advanceUntilIdle()
        coVerify(exactly = 1) { repository.revoke("g1") }
        assertNull(vm.uiState.value.prompt)
        coVerify(exactly = 2) { repository.listOutgoing(any(), any()) }
    }

    @Test
    fun `reconfirm is confirmation gated and calls reconfirm`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val withheld = contactGrant.copy(reconfirmSections = listOf("addresses"))
        stubLoad(withheld)
        coEvery { repository.reconfirm("g1") } returns Result.success(contactGrant)
        val vm = newVm()
        advanceUntilIdle()

        vm.requestReconfirm(withheld)
        coVerify(exactly = 0) { repository.reconfirm(any()) }
        vm.confirmPrompt()
        advanceUntilIdle()
        coVerify(exactly = 1) { repository.reconfirm("g1") }
    }

    @Test
    fun `a failed revoke surfaces the error and reloads`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(contactGrant)
        coEvery { repository.revoke("g1") } returns Result.failure(ApiError.Client(404, "x"))
        val vm = newVm()
        advanceUntilIdle()
        vm.requestRevoke(contactGrant)
        vm.confirmPrompt()
        advanceUntilIdle()
        assertNotNull(vm.uiState.value.actionError)
        coVerify(exactly = 2) { repository.listOutgoing(any(), any()) }
        vm.onActionErrorShown()
        assertNull(vm.uiState.value.actionError)
    }

    @Test
    fun `edit can only narrow the sections and never add one`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(contactGrant)
        val vm = newVm()
        advanceUntilIdle()

        vm.openEdit(contactGrant)
        assertEquals(setOf("emails", "phones", "notes"), vm.uiState.value.edit?.sections)
        vm.toggleEditSection("phones", false)
        vm.toggleEditSection("addresses", true)
        assertEquals(setOf("emails", "notes"), vm.uiState.value.edit?.sections)
    }

    @Test
    fun `the last section cannot be removed from a contact grant`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(contactGrant)
        val vm = newVm()
        advanceUntilIdle()
        vm.openEdit(contactGrant)
        vm.toggleEditSection("emails", false)
        vm.toggleEditSection("phones", false)
        vm.toggleEditSection("notes", false)
        assertEquals(setOf("notes"), vm.uiState.value.edit?.sections)
    }

    @Test
    fun `saving sends only what changed with the revision`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(contactGrant)
        val patch = slot<ShareGrantPatch>()
        coEvery { repository.update("g1", capture(patch), 3) } returns Result.success(contactGrant)
        val vm = newVm()
        advanceUntilIdle()

        vm.openEdit(contactGrant)
        vm.toggleEditSection("phones", false)
        vm.saveEdit()
        advanceUntilIdle()

        assertEquals(listOf("emails", "notes"), patch.captured.sections)
        assertNull(patch.captured.expiresAt)
        assertFalse(patch.captured.clearExpiry)
        assertNull(vm.uiState.value.edit)
    }

    @Test
    fun `the expiry can be set from now or cleared`() = runTest(mainDispatcherRule.testDispatcher) {
        stubLoad(contactGrant)
        val patches = mutableListOf<ShareGrantPatch>()
        coEvery {
            repository.update(
                "g1",
                capture(patches),
                3
            )
        } returns Result.success(contactGrant)
        val vm = newVm()
        advanceUntilIdle()

        vm.openEdit(contactGrant)
        vm.setEditExpiry(ExpiryChoice.Days(30))
        vm.saveEdit()
        advanceUntilIdle()
        assertEquals("2026-12-10T00:00:00Z", patches[0].expiresAt)
        assertNull(patches[0].sections)

        vm.openEdit(contactGrant)
        vm.setEditExpiry(ExpiryChoice.None)
        vm.saveEdit()
        advanceUntilIdle()
        assertTrue(patches[1].clearExpiry)
        assertNull(patches[1].expiresAt)
    }

    @Test
    fun `saving with no change does not call the server`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(contactGrant)
        val vm = newVm()
        advanceUntilIdle()
        vm.openEdit(contactGrant)
        vm.saveEdit()
        advanceUntilIdle()
        coVerify(exactly = 0) { repository.update(any(), any(), any()) }
        assertNull(vm.uiState.value.edit)
    }

    @Test
    fun `clearing an expiry that is not set is not a change`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(circleGrant)
        val vm = newVm()
        advanceUntilIdle()
        vm.openEdit(circleGrant)
        vm.setEditExpiry(ExpiryChoice.None)
        vm.saveEdit()
        advanceUntilIdle()
        coVerify(exactly = 0) { repository.update(any(), any(), any()) }
    }

    @Test
    fun `a rejected save shows the error and reloads the row`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(contactGrant)
        coEvery { repository.update(any(), any(), any()) } returns
            Result.failure(ApiError.Client(409, "x"))
        val vm = newVm()
        advanceUntilIdle()
        vm.openEdit(contactGrant)
        vm.toggleEditSection("phones", false)
        vm.saveEdit()
        advanceUntilIdle()
        assertNotNull(vm.uiState.value.actionError)
        assertNull(vm.uiState.value.edit)
        coVerify(exactly = 2) { repository.listOutgoing(any(), any()) }
    }

    @Test
    fun `a finished grant has no actions`() = runTest(mainDispatcherRule.testDispatcher) {
        stubLoad(revoked)
        val vm = newVm()
        advanceUntilIdle()
        vm.requestRevoke(revoked)
        vm.openEdit(revoked)
        assertNull(vm.uiState.value.prompt)
        assertNull(vm.uiState.value.edit)
        assertFalse(revoked.isLive)
        assertTrue(contactGrant.isLive)
        assertTrue(circleGrant.isLive)
    }
}
