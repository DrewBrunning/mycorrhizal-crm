package com.mycorrhizal.crm.feature.shares

import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import com.mycorrhizal.crm.model.network.ShareGrantNotifications
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test

class ShareGrantBadgeViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<ShareGrantRepository>()

    @Test
    fun `exposes the pending count`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.notifications() } returns
            Result.success(ShareGrantNotifications(pendingCount = 3))
        val vm = ShareGrantBadgeViewModel(repository)
        advanceUntilIdle()
        assertEquals(3, vm.pendingCount.value)
    }

    @Test
    fun `refresh follows the server and a failure hides the badge`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { repository.notifications() } returnsMany listOf(
            Result.success(ShareGrantNotifications(pendingCount = 2)),
            Result.success(ShareGrantNotifications(pendingCount = 0)),
            Result.success(ShareGrantNotifications(pendingCount = 5)),
            Result.failure(ApiError.Client(404, "old server"))
        )
        val vm = ShareGrantBadgeViewModel(repository)
        advanceUntilIdle()
        assertEquals(2, vm.pendingCount.value)
        vm.refresh()
        advanceUntilIdle()
        assertEquals(0, vm.pendingCount.value)
        vm.refresh()
        advanceUntilIdle()
        assertEquals(5, vm.pendingCount.value)
        vm.refresh()
        advanceUntilIdle()
        assertEquals(0, vm.pendingCount.value)
    }

    @Test
    fun `a share-grant change re-reads the count without the drawer reopening`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { repository.notifications() } returnsMany listOf(
            Result.success(ShareGrantNotifications(pendingCount = 3)),
            Result.success(ShareGrantNotifications(pendingCount = 2))
        )
        val changes = ShareGrantChanges()
        val vm = ShareGrantBadgeViewModel(repository, changes)
        advanceUntilIdle()
        assertEquals(3, vm.pendingCount.value)

        changes.notifyChanged()
        advanceUntilIdle()

        assertEquals(2, vm.pendingCount.value)
    }
}
