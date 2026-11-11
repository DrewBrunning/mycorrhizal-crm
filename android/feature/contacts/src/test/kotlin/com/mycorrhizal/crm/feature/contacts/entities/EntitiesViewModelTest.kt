package com.mycorrhizal.crm.feature.contacts.entities

import com.mycorrhizal.crm.domain.repository.EntityRepository
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityInput
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.EntityListResponse
import com.mycorrhizal.crm.model.network.EntitySuggestionEntity
import com.mycorrhizal.crm.model.network.EntitySuggestionPair
import com.mycorrhizal.crm.model.network.EntitySuggestionsResponse
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
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class EntitiesViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<EntityRepository>()

    private fun page(
        vararg names: String,
        next: String? = null,
        kind: String = EntityKind.ORGANISATION
    ) = Result.success(
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
            coEvery {
                repository.list(
                    EntityKind.ORGANISATION,
                    null,
                    null,
                    any()
                )
            } coAnswers { slow.await() }
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
            coEvery {
                repository.list(
                    EntityKind.ORGANISATION,
                    null,
                    "c2",
                    any()
                )
            } returns page("B")
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

    @Test
    fun `creating an entity of the shown kind adds it and reports done`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery {
                repository.list(
                    EntityKind.ORGANISATION,
                    null,
                    null,
                    any()
                )
            } returns page("Acme")
            val created = Entity(id = "e2", name = "Globex", kind = EntityKind.ORGANISATION)
            coEvery {
                repository.create(
                    EntityInput(kind = EntityKind.ORGANISATION, name = "Globex")
                )
            } returns
                Result.success(created)
            val vm = EntitiesViewModel(repository)
            advanceUntilIdle()
            var saved: Entity? = null
            vm.create(EntityInput(kind = EntityKind.ORGANISATION, name = "Globex")) { saved = it }
            advanceUntilIdle()
            assertEquals("e2", saved?.id)
            assertEquals(listOf("Acme", "Globex"), vm.uiState.value.entities.map { it.name })
            assertFalse(vm.uiState.value.isSaving)
        }

    @Test
    fun `a failed create keeps the form open with the error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list(any(), any(), any(), any()) } returns page("Acme")
            coEvery { repository.create(any()) } returns
                Result.failure(ApiError.Unknown(RuntimeException("x")))
            val vm = EntitiesViewModel(repository)
            advanceUntilIdle()
            var saved = false
            vm.create(EntityInput(kind = EntityKind.ORGANISATION, name = "Globex")) { saved = true }
            advanceUntilIdle()
            assertFalse(saved)
            assertNotNull(vm.uiState.value.formError)
            vm.onFormDismissed()
            assertNull(vm.uiState.value.formError)
        }

    @Test
    fun `coming back to the screen re-reads the list, but not before the first load`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list(any(), any(), any(), any()) } returns page("Acme")
            val vm = EntitiesViewModel(repository)
            vm.onResumed()
            advanceUntilIdle()
            coVerify(exactly = 1) { repository.list(any(), any(), any(), any()) }
            vm.onResumed()
            advanceUntilIdle()
            coVerify(exactly = 2) { repository.list(any(), any(), any(), any()) }
        }
    // --- duplicate suggestions (#2499) -----------------------------------------------------

    private fun pair(a: String, b: String, reason: String = "exact_name") = EntitySuggestionPair(
        reason = reason,
        a = EntitySuggestionEntity(id = "id-$a", kind = EntityKind.ORGANISATION, name = a),
        b = EntitySuggestionEntity(id = "id-$b", kind = EntityKind.ORGANISATION, name = b)
    )

    private fun suggestionsOf(vararg pairs: EntitySuggestionPair) =
        Result.success(EntitySuggestionsResponse(mode = "pairs", pairs = pairs.toList()))

    @Test
    fun `nothing is scanned until asked, then the pairs of the shown kind load`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list(EntityKind.ORGANISATION, null, null, any()) } returns
                page("Acme", "ACME")
            coEvery { repository.suggestions(EntityKind.ORGANISATION) } returns
                suggestionsOf(pair("Acme", "ACME"))
            val vm = EntitiesViewModel(repository)
            advanceUntilIdle()
            assertNull(vm.uiState.value.suggestions)
            coVerify(exactly = 0) { repository.suggestions(any()) }

            vm.findDuplicates()
            advanceUntilIdle()

            assertEquals(1, vm.uiState.value.suggestions?.size)
            assertFalse(vm.uiState.value.suggestionsLoading)
            assertNull(vm.uiState.value.suggestionsError)
        }

    @Test
    fun `a failed scan keeps the error and no list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list(any(), any(), any(), any()) } returns page("Acme")
        coEvery { repository.suggestions(any()) } returns
            Result.failure(ApiError.Network(java.io.IOException("offline")))
        val vm = EntitiesViewModel(repository)
        advanceUntilIdle()
        vm.findDuplicates()
        advanceUntilIdle()
        assertNull(vm.uiState.value.suggestions)
        assertNotNull(vm.uiState.value.suggestionsError)
        assertFalse(vm.uiState.value.suggestionsLoading)
    }

    @Test
    fun `dismissing hides the pair for good and a failure keeps it`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val keep = pair("Cabin", "Cabin")
            val hide = pair("Acme", "ACME")
            coEvery { repository.list(any(), any(), any(), any()) } returns page("Acme")
            coEvery { repository.suggestions(any()) } returns suggestionsOf(hide, keep)
            coEvery { repository.dismissSuggestion("id-Acme", "id-ACME") } returns
                Result.success(Unit)
            coEvery { repository.dismissSuggestion("id-Cabin", "id-Cabin") } returns
                Result.failure(ApiError.Network(java.io.IOException("offline")))
            val vm = EntitiesViewModel(repository)
            advanceUntilIdle()
            vm.findDuplicates()
            advanceUntilIdle()

            vm.dismissSuggestion(keep)
            advanceUntilIdle()
            assertEquals(2, vm.uiState.value.suggestions?.size)
            assertNotNull(vm.uiState.value.suggestionsError)

            vm.dismissSuggestion(hide)
            advanceUntilIdle()
            assertEquals(listOf(keep), vm.uiState.value.suggestions)
            assertNull(vm.uiState.value.suggestionsError)
        }

    @Test
    fun `switching kind drops the scanned pairs and a slow scan never lands on the new kind`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val slow = CompletableDeferred<Result<EntitySuggestionsResponse>>()
            coEvery { repository.list(any(), any(), any(), any()) } returns page("Acme")
            coEvery { repository.suggestions(EntityKind.ORGANISATION) } coAnswers { slow.await() }
            val vm = EntitiesViewModel(repository)
            advanceUntilIdle()
            vm.findDuplicates()
            advanceUntilIdle()
            assertTrue(vm.uiState.value.suggestionsLoading)

            vm.selectKind(EntityKind.PLACE)
            advanceUntilIdle()
            assertFalse(vm.uiState.value.suggestionsLoading)
            slow.complete(suggestionsOf(pair("Acme", "ACME")))
            advanceUntilIdle()

            assertNull(vm.uiState.value.suggestions)
        }
}
