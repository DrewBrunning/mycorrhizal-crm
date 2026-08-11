package com.mycorrhizal.crm.feature.circles

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.CircleMember
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
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

class CirclesViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val circleRepository = mockk<CircleRepository>()

    @Test
    fun `loads circles on init`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { circleRepository.list(any(), any()) } returns Result.success(
            listOf(
                Circle(id = "c1", name = "Friends"),
                Circle(id = "c2", name = "Family"),
            ),
        )

        val vm = CirclesViewModel(circleRepository)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.isLoading)
        assertEquals(2, state.circles.size)
        assertEquals("Friends", state.circles[0].name)
        assertNull(state.error)
    }

    @Test
    fun `load failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { circleRepository.list(any(), any()) } returns Result.failure(
            ApiError.Client(500, "boom"),
        )

        val vm = CirclesViewModel(circleRepository)
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isLoading)
        assertEquals("boom", vm.uiState.value.error)
    }

    @Test
    fun `create adds the circle to the list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { circleRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery { circleRepository.create("Friends") } returns Result.success(Circle(id = "c9", name = "Friends"))

        val vm = CirclesViewModel(circleRepository)
        advanceUntilIdle()

        var done = false
        vm.create("Friends") { done = true }
        advanceUntilIdle()

        assertTrue(done)
        assertEquals(listOf("Friends"), vm.uiState.value.circles.map { it.name })
    }

    @Test
    fun `blank name does not create`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { circleRepository.list(any(), any()) } returns Result.success(emptyList())

        val vm = CirclesViewModel(circleRepository)
        advanceUntilIdle()

        vm.create("   ")
        advanceUntilIdle()

        coVerify(exactly = 0) { circleRepository.create(any()) }
    }

    @Test
    fun `rename updates the circle in place`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { circleRepository.list(any(), any()) } returns Result.success(
            listOf(Circle(id = "c1", name = "Friends")),
        )
        coEvery { circleRepository.rename("c1", "Close Friends") } returns
            Result.success(Circle(id = "c1", name = "Close Friends"))

        val vm = CirclesViewModel(circleRepository)
        advanceUntilIdle()

        vm.rename("c1", "Close Friends")
        advanceUntilIdle()

        assertEquals(listOf("Close Friends"), vm.uiState.value.circles.map { it.name })
    }

    @Test
    fun `delete removes the circle`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { circleRepository.list(any(), any()) } returns Result.success(
            listOf(Circle(id = "c1", name = "Friends"), Circle(id = "c2", name = "Family")),
        )
        coEvery { circleRepository.delete("c1") } returns Result.success(Unit)

        val vm = CirclesViewModel(circleRepository)
        advanceUntilIdle()

        vm.delete("c1")
        advanceUntilIdle()

        assertEquals(listOf("Family"), vm.uiState.value.circles.map { it.name })
    }
}

class CircleDetailViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val circleRepository = mockk<CircleRepository>()

    private fun viewModel(id: String = "c1"): CircleDetailViewModel =
        CircleDetailViewModel(circleRepository, SavedStateHandle(mapOf("circleId" to id)))

    @Test
    fun `loads circle and members on init`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { circleRepository.getWithMembers("c1") } returns Result.success(
            com.mycorrhizal.crm.domain.repository.CircleDetail(
                circle = Circle(id = "c1", name = "Friends"),
                members = listOf(CircleMember(id = 1, circleId = "c1", memberVCardUid = "uid-1")),
            ),
        )

        val vm = viewModel()
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.isLoading)
        assertEquals("Friends", state.circle?.name)
        assertEquals(1, state.members.size)
        assertEquals("uid-1", state.members[0].memberVCardUid)
    }

    @Test
    fun `addMember appends to the members list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { circleRepository.getWithMembers("c1") } returns Result.success(
            com.mycorrhizal.crm.domain.repository.CircleDetail(
                circle = Circle(id = "c1", name = "Friends"),
                members = emptyList(),
            ),
        )
        coEvery { circleRepository.addMember("c1", "uid-2") } returns Result.success(
            CircleMember(id = 2, circleId = "c1", memberVCardUid = "uid-2"),
        )

        val vm = viewModel()
        advanceUntilIdle()

        vm.addMember("uid-2")
        advanceUntilIdle()

        assertEquals(listOf("uid-2"), vm.uiState.value.members.map { it.memberVCardUid })
    }

    @Test
    fun `removeMember drops the member`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { circleRepository.getWithMembers("c1") } returns Result.success(
            com.mycorrhizal.crm.domain.repository.CircleDetail(
                circle = Circle(id = "c1", name = "Friends"),
                members = listOf(
                    CircleMember(id = 1, circleId = "c1", memberVCardUid = "uid-1"),
                    CircleMember(id = 2, circleId = "c1", memberVCardUid = "uid-2"),
                ),
            ),
        )
        coEvery { circleRepository.removeMember("c1", "uid-1") } returns Result.success(Unit)

        val vm = viewModel()
        advanceUntilIdle()

        vm.removeMember("uid-1")
        advanceUntilIdle()

        assertEquals(listOf("uid-2"), vm.uiState.value.members.map { it.memberVCardUid })
    }
}
