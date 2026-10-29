package com.mycorrhizal.crm.feature.trash

import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.TrashRepository
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.TrashListResponse
import com.mycorrhizal.crm.model.network.TrashRestoreResponse
import com.mycorrhizal.crm.model.network.TrashRestoreSkip
import com.mycorrhizal.crm.model.network.TrashedContact
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class TrashViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val trash = mockk<TrashRepository>()
    private val contacts = mockk<ContactRepository>()

    private fun row(id: Int, name: String) = TrashedContact(
        contact = ContactSummary(id = id, firstname = name, lastname = "Test"),
        deletedAt = "2026-10-01T00:00:00Z",
        purgeAt = "2026-10-31T00:00:00Z"
    )

    private fun stubList(vararg rows: TrashedContact, retention: Int = 30) {
        coEvery { trash.list(any(), any()) } returns Result.success(
            TrashListResponse(
                contacts = rows.toList(),
                total = rows.size,
                retentionDays = retention
            )
        )
    }

    private fun vm(): TrashViewModel {
        coEvery { contacts.syncContacts() } returns Result.success(Unit)
        return TrashViewModel(trash, contacts)
    }

    @Test
    fun `load surfaces the rows and the retention`() = runTest(mainDispatcherRule.testDispatcher) {
        stubList(row(1, "Ada"), row(2, "Bob"), retention = 0)

        val model = vm()
        advanceUntilIdle()

        assertEquals(listOf(1, 2), model.uiState.value.items.map { it.contact.id })
        assertEquals(0, model.uiState.value.retentionDays)
        assertFalse(model.uiState.value.isLoading)
        assertNull(model.uiState.value.error)
    }

    @Test
    fun `a load failure sets the error and keeps no rows`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { trash.list(any(), any()) } returns Result.failure(ApiError.Server(500, "boom"))

        val model = vm()
        advanceUntilIdle()

        assertNotNull(model.uiState.value.error)
        assertTrue(model.uiState.value.items.isEmpty())
    }

    @Test
    fun `restore removes the row, syncs contacts and reports success`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubList(row(1, "Ada"), row(2, "Bob"))
        coEvery { trash.restore(2) } returns Result.success(
            TrashRestoreResponse(contact = ContactSummary(id = 2))
        )
        val model = vm()
        advanceUntilIdle()

        model.restore(model.uiState.value.items[1])
        advanceUntilIdle()

        assertEquals(listOf(1), model.uiState.value.items.map { it.contact.id })
        coVerify { contacts.syncContacts() }
        assertEquals(TrashUiEvent.Restored("Bob Test", lostRows = 0), model.events.first())
    }

    @Test
    fun `restore reports how many rows could not be brought back`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubList(row(2, "Bob"))
        coEvery { trash.restore(2) } returns Result.success(
            TrashRestoreResponse(
                contact = ContactSummary(id = 2),
                skipped = listOf(
                    TrashRestoreSkip("relationship_edges", "x", 2),
                    TrashRestoreSkip("circle_members", "x", 1)
                )
            )
        )
        val model = vm()
        advanceUntilIdle()

        model.restore(model.uiState.value.items.single())
        advanceUntilIdle()

        assertEquals(TrashUiEvent.Restored("Bob Test", lostRows = 3), model.events.first())
    }

    @Test
    fun `a 409 restore keeps the row and reports the conflict`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubList(row(1, "Ada"))
        coEvery { trash.restore(1) } returns Result.failure(ApiError.Client(409, "uid in use"))
        val model = vm()
        advanceUntilIdle()

        model.restore(model.uiState.value.items.single())
        advanceUntilIdle()

        assertEquals(1, model.uiState.value.items.size)
        assertEquals(TrashUiEvent.RestoreConflict("Ada Test"), model.events.first())
    }

    @Test
    fun `purge removes the row`() = runTest(mainDispatcherRule.testDispatcher) {
        stubList(row(1, "Ada"), row(2, "Bob"))
        coEvery { trash.purge(1) } returns Result.success(Unit)
        val model = vm()
        advanceUntilIdle()

        model.purge(model.uiState.value.items[0])
        advanceUntilIdle()

        assertEquals(listOf(2), model.uiState.value.items.map { it.contact.id })
        assertEquals(TrashUiEvent.Purged("Ada Test"), model.events.first())
    }

    @Test
    fun `a failed purge keeps the row and reports the error`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubList(row(1, "Ada"))
        coEvery { trash.purge(1) } returns Result.failure(ApiError.Server(500, "boom"))
        val model = vm()
        advanceUntilIdle()

        model.purge(model.uiState.value.items.single())
        advanceUntilIdle()

        assertEquals(1, model.uiState.value.items.size)
        assertTrue(model.events.first() is TrashUiEvent.Failed)
    }

    @Test
    fun `empty trash clears every row`() = runTest(mainDispatcherRule.testDispatcher) {
        stubList(row(1, "Ada"), row(2, "Bob"))
        coEvery { trash.empty() } returns Result.success(2)
        val model = vm()
        advanceUntilIdle()

        model.emptyTrash()
        advanceUntilIdle()

        assertTrue(model.uiState.value.items.isEmpty())
        assertEquals(TrashUiEvent.Emptied(2), model.events.first())
    }

    @Test
    fun `a second action while one is running is ignored`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubList(row(1, "Ada"))
        coEvery { trash.purge(1) } returns Result.success(Unit)
        val model = vm()
        advanceUntilIdle()
        val item = model.uiState.value.items.single()

        model.purge(item)
        model.purge(item)
        advanceUntilIdle()

        coVerify(exactly = 1) { trash.purge(1) }
    }
}
