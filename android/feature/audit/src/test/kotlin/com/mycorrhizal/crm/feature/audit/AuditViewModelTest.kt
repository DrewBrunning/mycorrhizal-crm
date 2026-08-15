package com.mycorrhizal.crm.feature.audit

import com.mycorrhizal.crm.domain.repository.AuditRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.AuditEvent
import com.mycorrhizal.crm.model.network.AuditEntityTypes
import com.mycorrhizal.crm.model.network.AuditEventsResponse
import com.mycorrhizal.crm.model.network.AuditOperations
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import java.io.IOException

class AuditViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val auditRepository = mockk<AuditRepository>()
    private val contactRepository = mockk<ContactRepository>()

    private fun viewModel(): AuditViewModel = AuditViewModel(auditRepository, contactRepository)

    private fun event(
        id: Long = 1,
        type: String = AuditEntityTypes.CONTACT,
        entityId: String = "uid-1",
        operation: String = AuditOperations.UPDATE,
    ) = AuditEvent(id = id, createdAt = "2026-08-14T10:00:00Z", entityType = type, entityId = entityId, operation = operation)

    private fun stubList(response: AuditEventsResponse) {
        coEvery {
            auditRepository.list(entityType = any(), entityId = any(), limit = any())
        } returns Result.success(response)
    }

    private fun stubResolve(byUid: Map<String, ContactSummary>) {
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(byUid)
    }

    @Test
    fun `load fetches the default window and surfaces the events`() = runTest(mainDispatcherRule.testDispatcher) {
        stubList(AuditEventsResponse(auditEvents = listOf(event(1), event(2, operation = AuditOperations.DELETE))))
        stubResolve(emptyMap())

        val vm = viewModel()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isLoading)
        assertEquals(2, vm.uiState.value.events.size)
        assertNull(vm.uiState.value.error)
        coVerify {
            auditRepository.list(entityType = null, entityId = null, limit = 100)
        }
    }

    @Test
    fun `load failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { auditRepository.list(entityType = any(), entityId = any(), limit = any()) } returns
            Result.failure(ApiError.Client(500, "boom"))

        val vm = viewModel()
        advanceUntilIdle()

        assertEquals("boom", vm.uiState.value.error)
    }

    @Test
    fun `applying an entity type sends the filter and resets the window`() = runTest(mainDispatcherRule.testDispatcher) {
        stubList(AuditEventsResponse(auditEvents = emptyList()))
        stubResolve(emptyMap())

        val vm = viewModel()
        advanceUntilIdle()
        coEvery {
            auditRepository.list(entityType = "contact", entityId = null, limit = 100)
        } returns Result.success(
            AuditEventsResponse(auditEvents = listOf(event(3, type = AuditEntityTypes.CONTACT))),
        )

        vm.applyEntityType(AuditEntityTypes.CONTACT)
        advanceUntilIdle()

        assertEquals(AuditEntityTypes.CONTACT, vm.uiState.value.entityType)
        assertEquals(1, vm.uiState.value.events.size)
        coVerify {
            auditRepository.list(entityType = AuditEntityTypes.CONTACT, entityId = null, limit = 100)
        }
    }

    @Test
    fun `entity id is debounced before it becomes a filter`() = runTest(mainDispatcherRule.testDispatcher) {
        stubList(AuditEventsResponse(auditEvents = emptyList()))
        stubResolve(emptyMap())
        coEvery {
            auditRepository.list(entityType = null, entityId = "uid-9", limit = 100)
        } returns Result.success(
            AuditEventsResponse(auditEvents = listOf(event(4, entityId = "uid-9"))),
        )

        val vm = viewModel()
        advanceUntilIdle()

        vm.onEntityIdChange("uid-9")
        // Not yet applied: still in the debounce window, no second request fired.
        advanceTimeBy(200)
        assertTrue(vm.uiState.value.entityId.isEmpty())
        coVerify(exactly = 1) {
            auditRepository.list(entityType = null, entityId = any(), limit = 100)
        }

        advanceTimeBy(200)
        advanceUntilIdle()
        assertEquals("uid-9", vm.uiState.value.entityId)
        coVerify {
            auditRepository.list(entityType = null, entityId = "uid-9", limit = 100)
        }
    }

    @Test
    fun `clear filters resets both filters to the default window`() = runTest(mainDispatcherRule.testDispatcher) {
        stubList(AuditEventsResponse(auditEvents = emptyList()))
        stubResolve(emptyMap())

        val vm = viewModel()
        advanceUntilIdle()
        vm.applyEntityType(AuditEntityTypes.NOTE)
        advanceUntilIdle()

        coEvery {
            auditRepository.list(entityType = null, entityId = null, limit = 100)
        } returns Result.success(AuditEventsResponse(auditEvents = emptyList()))

        vm.clearFilters()
        advanceUntilIdle()

        assertNull(vm.uiState.value.entityType)
        assertEquals("", vm.uiState.value.entityId)
        assertFalse(vm.uiState.value.hasActiveFilters)
    }

    @Test
    fun `load more grows the window by one step`() = runTest(mainDispatcherRule.testDispatcher) {
        // A full default window (100 rows) means "there might be more".
        val full = (1L..100L).map { event(id = it) }
        stubList(AuditEventsResponse(auditEvents = full))
        stubResolve(emptyMap())

        val vm = viewModel()
        advanceUntilIdle()
        assertTrue(vm.uiState.value.canLoadMore)

        vm.loadMore()
        advanceUntilIdle()

        assertEquals(200, vm.uiState.value.limit)
        coVerify {
            auditRepository.list(entityType = null, entityId = null, limit = 200)
        }
    }

    @Test
    fun `a partial window offers no load more`() = runTest(mainDispatcherRule.testDispatcher) {
        stubList(AuditEventsResponse(auditEvents = listOf(event(1), event(2))))
        stubResolve(emptyMap())

        val vm = viewModel()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.canLoadMore)
    }

    @Test
    fun `contact uids resolve to summaries for the linkable cells`() = runTest(mainDispatcherRule.testDispatcher) {
        stubList(
            AuditEventsResponse(
                auditEvents = listOf(
                    event(1, entityId = "uid-1"),
                    event(2, type = AuditEntityTypes.NOTE, entityId = "9"),
                ),
            ),
        )
        stubResolve(mapOf("uid-1" to ContactSummary(id = 5, uid = "uid-1", firstname = "Dana", lastname = "White")))

        val vm = viewModel()
        advanceUntilIdle()

        val contact = vm.uiState.value.contactsByUid["uid-1"]
        assertEquals(5, contact?.id)
        assertEquals("Dana White", contact?.displayName)
    }

    @Test
    fun `a resolve failure degrades to an empty map`() = runTest(mainDispatcherRule.testDispatcher) {
        stubList(AuditEventsResponse(auditEvents = listOf(event(1, entityId = "uid-1"))))
        coEvery { contactRepository.resolveByUid(any()) } returns Result.failure(ApiError.Network(IOException("offline")))

        val vm = viewModel()
        advanceUntilIdle()

        assertTrue(vm.uiState.value.contactsByUid.isEmpty())
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `undo posts then refreshes the list`() = runTest(mainDispatcherRule.testDispatcher) {
        stubList(AuditEventsResponse(auditEvents = listOf(event(1))))
        stubResolve(emptyMap())
        coEvery { auditRepository.undo(1L) } returns Result.success(Unit)

        val vm = viewModel()
        advanceUntilIdle()

        vm.undo(1L)
        advanceUntilIdle()

        coVerify { auditRepository.undo(1L) }
        // The undo success + the post-undo refresh: two list calls total.
        coVerify(exactly = 2) {
            auditRepository.list(entityType = null, entityId = null, limit = 100)
        }
        val emitted = vm.events.first()
        assertTrue(emitted is AuditUiEvent.UndoSucceeded)
    }

    @Test
    fun `undo failure on 410 is flagged retention-gone`() = runTest(mainDispatcherRule.testDispatcher) {
        stubList(AuditEventsResponse(auditEvents = listOf(event(1))))
        stubResolve(emptyMap())
        coEvery { auditRepository.undo(1L) } returns Result.failure(ApiError.Client(410, "gone"))

        val vm = viewModel()
        advanceUntilIdle()

        vm.undo(1L)
        advanceUntilIdle()

        val emitted = vm.events.first() as AuditUiEvent.UndoFailed
        assertTrue(emitted.isRetentionGone)
        coVerify(exactly = 1) {
            auditRepository.list(entityType = null, entityId = null, limit = 100)
        }
    }

    @Test
    fun `undo failure on another code carries the server message`() = runTest(mainDispatcherRule.testDispatcher) {
        stubList(AuditEventsResponse(auditEvents = listOf(event(1))))
        stubResolve(emptyMap())
        coEvery { auditRepository.undo(1L) } returns Result.failure(ApiError.Client(400, "unsupported"))

        val vm = viewModel()
        advanceUntilIdle()

        vm.undo(1L)
        advanceUntilIdle()

        val emitted = vm.events.first() as AuditUiEvent.UndoFailed
        assertFalse(emitted.isRetentionGone)
        assertEquals("unsupported", emitted.message)
    }
}
