package com.mycorrhizal.crm.feature.settings

import com.mycorrhizal.crm.domain.repository.FieldDefinitionRepository
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import kotlinx.coroutines.CompletableDeferred
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

/** Issue #830. Mirrors `TagsViewModelTest`'s shape. */
class FieldDefinitionsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<FieldDefinitionRepository>()

    @Test
    fun `loads definitions on init`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.success(
            listOf(FieldDefinition(id = "d1", label = "Coffee order"), FieldDefinition(id = "d2", label = "T-shirt size")),
        )

        val vm = FieldDefinitionsViewModel(repository)
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isLoading)
        assertEquals(2, vm.uiState.value.definitions.size)
        assertEquals("Coffee order", vm.uiState.value.definitions[0].label)
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `load failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.failure(ApiError.Client(500, "boom"))

        val vm = FieldDefinitionsViewModel(repository)
        advanceUntilIdle()

        assertEquals("boom", vm.uiState.value.error)
    }

    @Test
    fun `delete removes the definition from the list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.success(
            listOf(FieldDefinition(id = "d1", label = "Coffee order"), FieldDefinition(id = "d2", label = "T-shirt size")),
        )
        coEvery { repository.delete("d1") } returns Result.success(Unit)

        val vm = FieldDefinitionsViewModel(repository)
        advanceUntilIdle()

        vm.delete("d1")
        advanceUntilIdle()

        assertEquals(listOf("T-shirt size"), vm.uiState.value.definitions.map { it.label })
    }

    @Test
    fun `delete failure surfaces the error and keeps the definition`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.success(listOf(FieldDefinition(id = "d1", label = "Coffee order")))
        coEvery { repository.delete("d1") } returns Result.failure(ApiError.Client(500, "boom"))

        val vm = FieldDefinitionsViewModel(repository)
        advanceUntilIdle()

        vm.delete("d1")
        advanceUntilIdle()

        assertEquals("boom", vm.uiState.value.error)
        assertEquals(listOf("d1"), vm.uiState.value.definitions.map { it.id })
    }

    @Test
    fun `a second delete call is ignored while one is already in flight`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.success(
            listOf(FieldDefinition(id = "d1", label = "Coffee order"), FieldDefinition(id = "d2", label = "T-shirt size")),
        )
        val gate = CompletableDeferred<Unit>()
        coEvery { repository.delete("d1") } coAnswers {
            gate.await()
            Result.success(Unit)
        }

        val vm = FieldDefinitionsViewModel(repository)
        advanceUntilIdle()

        vm.delete("d1")
        advanceUntilIdle() // deletingId flips to "d1" and the coroutine suspends on the gate
        assertTrue(vm.uiState.value.deletingId == "d1")

        vm.delete("d2") // a second call while the first is still in flight must be a no-op

        gate.complete(Unit)
        advanceUntilIdle()

        coVerify(exactly = 0) { repository.delete("d2") }
        assertEquals(listOf("T-shirt size"), vm.uiState.value.definitions.map { it.label })
    }
}
