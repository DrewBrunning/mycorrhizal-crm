package com.mycorrhizal.crm.feature.shares

import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
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
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runCurrent
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
    private val contacts = mockk<ContactRepository>()

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

    private fun newVm(): SharedByMeViewModel = SharedByMeViewModel(repository, contacts).also {
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
    fun `loadMore appends the next page and follows its cursor`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val older = contactGrant.copy(id = "g4", label = "Older")
        coEvery { repository.listOutgoing(any(), any()) } returnsMany listOf(
            Result.success(
                OutgoingShareGrantsPage(shareGrants = listOf(contactGrant), nextCursor = "c1")
            ),
            Result.success(OutgoingShareGrantsPage(shareGrants = listOf(older), nextCursor = ""))
        )
        val vm = newVm()
        advanceUntilIdle()
        assertEquals("c1", vm.uiState.value.nextCursor)

        vm.loadMore()
        advanceUntilIdle()

        assertEquals(listOf("g1", "g4"), vm.uiState.value.grants.map { it.id })
        assertNull(vm.uiState.value.nextCursor)
        assertFalse(vm.uiState.value.loadingMore)
        coVerify(exactly = 1) { repository.listOutgoing("c1", 100) }
    }

    @Test
    fun `a failed loadMore keeps the loaded rows and exposes a retry error`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { repository.listOutgoing(any(), any()) } returnsMany listOf(
            Result.success(
                OutgoingShareGrantsPage(shareGrants = listOf(contactGrant), nextCursor = "c1")
            ),
            Result.failure(ApiError.Server(500, "boom"))
        )
        val vm = newVm()
        advanceUntilIdle()

        vm.loadMore()
        advanceUntilIdle()

        assertEquals(listOf("g1"), vm.uiState.value.grants.map { it.id })
        assertNotNull(vm.uiState.value.loadMoreError)
        assertFalse(vm.uiState.value.loadingMore)
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
    fun `a revoke that finishes while a load is in flight still ends with the revoked list`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val firstLoad = CompletableDeferred<Unit>()
            var calls = 0
            coEvery { repository.listOutgoing(any(), any()) } coAnswers {
                if (++calls == 1) {
                    firstLoad.await()
                    Result.success(OutgoingShareGrantsPage(shareGrants = listOf(contactGrant)))
                } else {
                    Result.success(
                        OutgoingShareGrantsPage(
                            shareGrants = listOf(
                                contactGrant.copy(status = ShareGrantStatuses.REVOKED)
                            )
                        )
                    )
                }
            }
            coEvery { repository.revoke("g1") } returns
                Result.success(contactGrant.copy(status = ShareGrantStatuses.REVOKED))
            val vm = newVm()
            runCurrent()
            assertTrue(vm.uiState.value.isLoading)

            vm.requestRevoke(contactGrant)
            vm.confirmPrompt()
            runCurrent()
            firstLoad.complete(Unit)
            advanceUntilIdle()

            assertEquals(
                listOf(ShareGrantStatuses.REVOKED),
                vm.uiState.value.grants.map { it.status }
            )
            assertFalse(vm.uiState.value.isLoading)
            coVerify(exactly = 2) { repository.listOutgoing(any(), any()) }
        }

    @Test
    fun `a load tap while loading does not queue a second request`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val firstLoad = CompletableDeferred<Unit>()
            coEvery { repository.listOutgoing(any(), any()) } coAnswers {
                firstLoad.await()
                Result.success(OutgoingShareGrantsPage(shareGrants = listOf(contactGrant)))
            }
            val vm = newVm()
            runCurrent()

            vm.load()
            firstLoad.complete(Unit)
            advanceUntilIdle()

            coVerify(exactly = 1) { repository.listOutgoing(any(), any()) }
        }

    @Test
    fun `reconfirm is confirmation gated and calls reconfirm`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val withheld = contactGrant.copy(reconfirmSections = listOf("addresses"))
        stubLoad(withheld)
        coEvery { repository.reconfirm("g1") } returns Result.success(contactGrant)
        coEvery { repository.previewContact(any(), any()) } returns Result.success(emptyMap())
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

    // --- Issue #2627: remove an ended grant ---

    private val declined = contactGrant.copy(id = "g4", status = ShareGrantStatuses.DECLINED)
    private val expired = contactGrant.copy(id = "g5", status = ShareGrantStatuses.EXPIRED)

    @Test
    fun `only an ended grant can be removed`() = runTest(mainDispatcherRule.testDispatcher) {
        stubLoad(contactGrant, circleGrant, revoked, declined, expired)
        val vm = newVm()
        advanceUntilIdle()

        vm.requestRemove(contactGrant)
        assertNull(vm.uiState.value.prompt)
        vm.requestRemove(circleGrant)
        assertNull(vm.uiState.value.prompt)

        listOf(revoked, declined, expired).forEach { grant ->
            vm.requestRemove(grant)
            assertEquals(SharedByMePrompt.Remove(grant), vm.uiState.value.prompt)
            vm.dismissPrompt()
        }
        assertTrue(revoked.isEnded && declined.isEnded && expired.isEnded)
        assertFalse(contactGrant.isEnded || circleGrant.isEnded)
        coVerify(exactly = 0) { repository.delete(any()) }
    }

    @Test
    fun `remove is confirmation gated, deletes the row and reloads`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(contactGrant, declined)
        coEvery { repository.delete("g4") } returns Result.success(Unit)
        val vm = newVm()
        advanceUntilIdle()

        vm.requestRemove(declined)
        coVerify(exactly = 0) { repository.delete(any()) }
        stubLoad(contactGrant)
        vm.confirmPrompt()
        advanceUntilIdle()

        coVerify(exactly = 1) { repository.delete("g4") }
        coVerify(exactly = 0) { repository.revoke(any()) }
        assertNull(vm.uiState.value.prompt)
        assertNull(vm.uiState.value.actionError)
        assertEquals(listOf("g1"), vm.uiState.value.grants.map { it.id })
        coVerify(exactly = 2) { repository.listOutgoing(any(), any()) }
    }

    @Test
    fun `a removed row leaves the list before the reload answers`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val reload = CompletableDeferred<Unit>()
        var calls = 0
        coEvery { repository.listOutgoing(any(), any()) } coAnswers {
            if (++calls > 1) reload.await()
            Result.success(OutgoingShareGrantsPage(shareGrants = listOf(contactGrant, declined)))
        }
        coEvery { repository.delete("g4") } returns Result.success(Unit)
        val vm = newVm()
        advanceUntilIdle()

        vm.requestRemove(declined)
        vm.confirmPrompt()
        runCurrent()

        assertTrue(vm.uiState.value.isLoading)
        assertEquals(listOf("g1"), vm.uiState.value.grants.map { it.id })
        reload.complete(Unit)
        advanceUntilIdle()
    }

    @Test
    fun `a 404 after remove means it is already gone, not an error`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(declined)
        coEvery { repository.delete("g4") } returns Result.failure(
            ApiError.Client(
                404,
                "Share grant not found",
                reason = ErrorReason(ErrorReasonCode.SHARE_GRANT_NOT_FOUND, emptyMap())
            )
        )
        val vm = newVm()
        advanceUntilIdle()

        stubLoad()
        vm.requestRemove(declined)
        vm.confirmPrompt()
        advanceUntilIdle()

        assertNull(vm.uiState.value.actionError)
        assertTrue(vm.uiState.value.grants.isEmpty())
        assertNull(vm.uiState.value.prompt)
    }

    @Test
    fun `a 409 not-ended keeps the row, reports the reason and reloads`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(declined)
        coEvery { repository.delete("g4") } returns Result.failure(
            ApiError.Client(
                409,
                "RAW",
                reason = ErrorReason(ErrorReasonCode.SHARE_GRANT_NOT_ENDED, emptyMap())
            )
        )
        val vm = newVm()
        advanceUntilIdle()

        vm.requestRemove(declined)
        vm.confirmPrompt()
        advanceUntilIdle()

        val error = vm.uiState.value.actionError as ApiError.Client
        assertEquals(ErrorReasonCode.SHARE_GRANT_NOT_ENDED, error.reason?.code)
        assertEquals(listOf("g4"), vm.uiState.value.grants.map { it.id })
        coVerify(exactly = 2) { repository.listOutgoing(any(), any()) }
    }

    @Test
    fun `a failed remove for another reason keeps the row`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(declined)
        coEvery { repository.delete("g4") } returns Result.failure(ApiError.Server(500, "x"))
        val vm = newVm()
        advanceUntilIdle()

        vm.requestRemove(declined)
        vm.confirmPrompt()
        advanceUntilIdle()

        assertNotNull(vm.uiState.value.actionError)
        assertEquals(listOf("g4"), vm.uiState.value.grants.map { it.id })
    }

    // --- Issue #2659: re-confirm review ---

    private val withheldGrant = contactGrant.copy(
        entityId = "uid-1",
        reconfirmSections = listOf("phones", "emails")
    )

    @Test
    fun `requesting a reconfirm previews the withheld sections and counts their items`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(withheldGrant)
        coEvery { repository.previewContact("uid-1", listOf("emails", "phones")) } returns
            Result.success(
                mapOf(
                    "emails" to mapOf("e1" to mapOf("address" to "a@x.org")),
                    "phones" to mapOf(
                        "p1" to mapOf("number" to "1"),
                        "p2" to mapOf("number" to "2")
                    )
                )
            )
        val vm = newVm()
        advanceUntilIdle()

        vm.requestReconfirm(withheldGrant)
        advanceUntilIdle()

        assertEquals(mapOf("emails" to 1, "phones" to 2), vm.uiState.value.reconfirmCounts)
    }

    @Test
    fun `a failed preview leaves the counts empty and keeps the prompt`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(withheldGrant)
        coEvery { repository.previewContact(any(), any()) } returns
            Result.failure(ApiError.Server(503, "down"))
        val vm = newVm()
        advanceUntilIdle()

        vm.requestReconfirm(withheldGrant)
        advanceUntilIdle()

        assertTrue(vm.uiState.value.reconfirmCounts.isEmpty())
        assertNotNull(vm.uiState.value.prompt)
        assertNull(vm.uiState.value.actionError)
    }

    @Test
    fun `dismissing the prompt clears the preview counts`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(withheldGrant)
        coEvery { repository.previewContact(any(), any()) } returns
            Result.success(mapOf("emails" to mapOf("e1" to mapOf("address" to "a@x.org"))))
        val vm = newVm()
        advanceUntilIdle()
        vm.requestReconfirm(withheldGrant)
        advanceUntilIdle()

        vm.dismissPrompt()

        assertTrue(vm.uiState.value.reconfirmCounts.isEmpty())
        assertNull(vm.uiState.value.prompt)
    }

    @Test
    fun `open contact resolves the grant's contact uid to its id once`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(withheldGrant)
        coEvery { contacts.resolveByUid(listOf("uid-1")) } returns
            Result.success(mapOf("uid-1" to ContactSummary(id = 7, uid = "uid-1")))
        val vm = newVm()
        advanceUntilIdle()

        vm.openContact(withheldGrant)
        advanceUntilIdle()
        assertEquals(7, vm.uiState.value.openContactId)

        vm.onContactOpened()
        assertNull(vm.uiState.value.openContactId)
    }

    @Test
    fun `open contact for an unresolvable uid reports not found, never navigates`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(withheldGrant)
        coEvery { contacts.resolveByUid(any()) } returns Result.success(emptyMap())
        val vm = newVm()
        advanceUntilIdle()

        vm.openContact(withheldGrant)
        advanceUntilIdle()

        assertNull(vm.uiState.value.openContactId)
        assertTrue(vm.uiState.value.actionError is ApiError.Client)
    }

    @Test
    fun `open contact surfaces a resolver failure as an action error`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(withheldGrant)
        coEvery { contacts.resolveByUid(any()) } returns Result.failure(ApiError.Server(503, "x"))
        val vm = newVm()
        advanceUntilIdle()

        vm.openContact(withheldGrant)
        advanceUntilIdle()

        assertNull(vm.uiState.value.openContactId)
        assertNotNull(vm.uiState.value.actionError)
    }
}
