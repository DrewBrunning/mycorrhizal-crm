package com.mycorrhizal.crm.feature.occasions

import com.mycorrhizal.crm.domain.repository.OccasionObligationRepository
import com.mycorrhizal.crm.model.network.GiftShoppingItem
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class OccasionListsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<OccasionObligationRepository>()

    private fun item(id: String = "o1", days: Int = 6, status: String = "needed") =
        GiftShoppingItem(
            contactId = 3,
            contactName = "Dana",
            obligationId = id,
            label = "Birthday gift",
            date = "2026-12-18",
            daysUntil = days,
            status = status
        )

    @Test
    fun `shopping list loads the 30 day window first`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.giftShoppingList(30) } returns Result.success(listOf(item()))

            val vm = GiftShoppingListViewModel(repository)
            advanceUntilIdle()

            assertEquals(30, vm.uiState.value.days)
            assertEquals(listOf("o1"), vm.uiState.value.items.map { it.obligationId })
            assertFalse(vm.uiState.value.isLoading)
        }

    @Test
    fun `choosing 90 days reloads that window and replaces the rows`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.giftShoppingList(30) } returns Result.success(listOf(item("a")))
            coEvery { repository.giftShoppingList(90) } returns
                Result.success(listOf(item("a"), item("b", days = 60)))
            val vm = GiftShoppingListViewModel(repository)
            advanceUntilIdle()

            vm.setDays(90)
            advanceUntilIdle()

            assertEquals(90, vm.uiState.value.days)
            assertEquals(listOf("a", "b"), vm.uiState.value.items.map { it.obligationId })
        }

    @Test
    fun `choosing the window already shown does not refetch`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.giftShoppingList(30) } returns Result.success(emptyList())
            val vm = GiftShoppingListViewModel(repository)
            advanceUntilIdle()

            vm.setDays(30)
            advanceUntilIdle()

            coVerify(exactly = 1) { repository.giftShoppingList(30) }
        }

    @Test
    fun `an unsupported window is ignored`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.giftShoppingList(30) } returns Result.success(emptyList())
        val vm = GiftShoppingListViewModel(repository)
        advanceUntilIdle()

        vm.setDays(7)
        advanceUntilIdle()

        assertEquals(30, vm.uiState.value.days)
        coVerify(exactly = 0) { repository.giftShoppingList(7) }
    }

    @Test
    fun `a failed load shows the error with no rows and retry clears it`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.giftShoppingList(30) } returnsMany listOf(
                Result.failure(ApiError.Client(500, "boom")),
                Result.success(listOf(item()))
            )
            val vm = GiftShoppingListViewModel(repository)
            advanceUntilIdle()
            assertTrue(vm.uiState.value.error is ApiError.Client)
            assertTrue(vm.uiState.value.items.isEmpty())

            vm.load()
            advanceUntilIdle()

            assertNull(vm.uiState.value.error)
            assertEquals(1, vm.uiState.value.items.size)
        }

    @Test
    fun `a failure after a window switch drops the previous window's rows`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.giftShoppingList(30) } returns Result.success(listOf(item()))
            coEvery { repository.giftShoppingList(90) } returns
                Result.failure(ApiError.Client(500, "boom"))
            val vm = GiftShoppingListViewModel(repository)
            advanceUntilIdle()

            vm.setDays(90)
            advanceUntilIdle()

            // 30-day rows under a 90-day heading would be wrong data.
            assertTrue(vm.uiState.value.items.isEmpty())
            assertTrue(vm.uiState.value.error is ApiError.Client)
        }

    @Test
    fun `card list download exposes the csv bytes once`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.cardListCsv("card") } returns Result.success("a,b".toByteArray())
            val vm = OccasionCardListViewModel(repository)

            vm.download()
            advanceUntilIdle()

            assertArrayEquals("a,b".toByteArray(), vm.uiState.value.csv)
            assertFalse(vm.uiState.value.isDownloading)

            vm.onCsvHandled()
            assertNull(vm.uiState.value.csv)
        }

    @Test
    fun `card list failure surfaces the error and leaves no csv`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.cardListCsv("card") } returns
                Result.failure(ApiError.Client(500, "boom"))
            val vm = OccasionCardListViewModel(repository)

            vm.download()
            advanceUntilIdle()

            assertTrue(vm.uiState.value.error is ApiError.Client)
            assertNull(vm.uiState.value.csv)
            assertFalse(vm.uiState.value.isDownloading)

            vm.onErrorShown()
            assertNull(vm.uiState.value.error)
        }

    @Test
    fun `a second download while one is running is ignored`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.cardListCsv("card") } returns Result.success("x".toByteArray())
            val vm = OccasionCardListViewModel(repository)

            vm.download()
            vm.download()
            advanceUntilIdle()

            coVerify(exactly = 1) { repository.cardListCsv("card") }
        }
}
