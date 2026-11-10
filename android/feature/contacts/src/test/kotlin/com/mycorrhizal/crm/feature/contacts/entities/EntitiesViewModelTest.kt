package com.mycorrhizal.crm.feature.contacts.entities

import com.mycorrhizal.crm.domain.repository.EntityRepository
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.EntityListResponse
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test

class EntitiesViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<EntityRepository>()

    private fun page(vararg names: String, next: String? = null, kind: String = EntityKind.ORGANISATION) =
        Result.success(
            EntityListResponse(
                entities = names.map { Entity(id = "id-$it", name = it, kind = kind) },
                nextCursor = next
            )
        )

    @Test
    fun `loads the organisations tab first`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list(EntityKind.ORGANISATION, null, null, any()) } returns page("Acme")
        val vm = EntitiesViewModel(repository)
        advanceUntilIdle()
        assertEquals(EntityKind.ORGANISATION, vm.uiState.value.kind)
        assertEquals(listOf("Acme"), vm.uiState.value.entities.map { it.name })
        assertFalse(vm.uiState.value.isLoading)
    }

    @Test
    fun `selecting a kind replaces the list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list(EntityKind.ORGANISATION, null, null, any()) } returns page("Acme")
        coEvery { repository.list(EntityKind.PLACE, null, null, any()) } returns
            page("Cabin", kind = EntityKind.PLACE)
        val vm = EntitiesViewModel(repository)
        advanceUntilIdle()
        vm.selectKind(EntityKind.PLACE)
        advanceUntilIdle()
        assertEquals(EntityKind.PLACE, vm.uiState.value.kind)
        assertEquals(listOf("Cabin"), vm.uiState.value.entities.map { it.name })
    }

    @Test
    fun `a slow response for the previous kind never overwrites the current one`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val slow = CompletableDeferred<Result<EntityListResponse>>()
            coEvery { repository.list(EntityKind.ORGANISATION, null, null, any()) } coAnswers { slow.await() }
            coEvery { repository.list(EntityKind.PET, null, null, any()) } returns
                page("Rex", kind = EntityKind.PET)
            val vm = EntitiesViewModel(repository)
            advanceUntilIdle()
            vm.selectKind(EntityKind.PET)
            advanceUntilIdle()
            slow.complete(page("Acme"))
            advanceUntilIdle()
            assertEquals(listOf("Rex"), vm.uiState.value.entities.map { it.name })
        }

    @Test
    fun `typing searches once after the debounce with the final text`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list(any(), any(), any(), any()) } returns page("Acme")
            val vm = EntitiesViewModel(repository)
            advanceUntilIdle()
            vm.onQueryChange("a")
            vm.onQueryChange("ac")
            advanceTimeBy(100)
            coVerify(exactly = 0) { repository.list(any(), "ac", any(), any()) }
            advanceUntilIdle()
            coVerify(exactly = 1) { repository.list(EntityKind.ORGANISATION, "ac", null, any()) }
            coVerify(exactly = 0) { repository.list(any(), "a", any(), any()) }
        }

    @Test
    fun `load more appends the next page and stops when there is none`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list(EntityKind.ORGANISATION, null, null, any()) } returns
                page("A", next = "c2")
            coEvery { repository.list(EntityKind.ORGANISATION, null, "c2", any()) } returns page("B")
            val vm = EntitiesViewModel(repository)
            advanceUntilIdle()
            vm.loadMore()
            advanceUntilIdle()
            assertEquals(listOf("A", "B"), vm.uiState.value.entities.map { it.name })
            assertNull(vm.uiState.value.nextCursor)
            vm.loadMore()
            advanceUntilIdle()
            coVerify(exactly = 2) { repository.list(any(), any(), any(), any()) }
        }

    @Test
    fun `a failure is kept for display and cleared by a retry`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list(any(), any(), any(), any()) } returns
                Result.failure(ApiError.Unknown(RuntimeException("x")))
            val vm = EntitiesViewModel(repository)
            advanceUntilIdle()
            assertNotNull(vm.uiState.value.error)
            coEvery { repository.list(any(), any(), any(), any()) } returns page("Acme")
            vm.refresh()
            advanceUntilIdle()
            assertNull(vm.uiState.value.error)
            assertEquals(1, vm.uiState.value.entities.size)
        }
}
