package com.mycorrhizal.crm.feature.households

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.HouseholdDetail
import com.mycorrhizal.crm.domain.repository.HouseholdRepository
import com.mycorrhizal.crm.model.network.Household
import com.mycorrhizal.crm.model.network.HouseholdMember
import com.mycorrhizal.crm.model.network.HouseholdTypes
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

class HouseholdsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val householdRepository = mockk<HouseholdRepository>()

    @Test
    fun `loads households on init`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.list(any(), any()) } returns Result.success(
            listOf(
                Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                Household(id = "h2", name = "Flat", type = HouseholdTypes.ROOMMATES),
            ),
        )

        val vm = HouseholdsViewModel(householdRepository)
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isLoading)
        assertEquals(2, vm.uiState.value.households.size)
        assertEquals("Home", vm.uiState.value.households[0].name)
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `load failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.list(any(), any()) } returns Result.failure(
            ApiError.Client(500, "boom"),
        )

        val vm = HouseholdsViewModel(householdRepository)
        advanceUntilIdle()

        assertEquals("boom", vm.uiState.value.error)
    }

    @Test
    fun `create adds the household to the list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery { householdRepository.create("Home", HouseholdTypes.FAMILY_UNIT) } returns
            Result.success(Household(id = "h9", name = "Home", type = HouseholdTypes.FAMILY_UNIT))

        val vm = HouseholdsViewModel(householdRepository)
        advanceUntilIdle()

        var done = false
        vm.create("Home", HouseholdTypes.FAMILY_UNIT) { done = true }
        advanceUntilIdle()

        assertTrue(done)
        assertEquals(listOf("Home"), vm.uiState.value.households.map { it.name })
    }

    @Test
    fun `blank name does not create`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.list(any(), any()) } returns Result.success(emptyList())

        val vm = HouseholdsViewModel(householdRepository)
        advanceUntilIdle()

        vm.create("   ", HouseholdTypes.FAMILY_UNIT)
        advanceUntilIdle()

        coVerify(exactly = 0) { householdRepository.create(any(), any()) }
    }

    @Test
    fun `rename updates the household in place`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.list(any(), any()) } returns Result.success(
            listOf(Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT)),
        )
        coEvery { householdRepository.update("h1", "Main Home", HouseholdTypes.FAMILY_UNIT) } returns
            Result.success(Household(id = "h1", name = "Main Home", type = HouseholdTypes.FAMILY_UNIT))

        val vm = HouseholdsViewModel(householdRepository)
        advanceUntilIdle()

        vm.rename("h1", "Main Home", HouseholdTypes.FAMILY_UNIT)
        advanceUntilIdle()

        assertEquals(listOf("Main Home"), vm.uiState.value.households.map { it.name })
    }

    @Test
    fun `delete removes the household`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.list(any(), any()) } returns Result.success(
            listOf(
                Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                Household(id = "h2", name = "Flat", type = HouseholdTypes.ROOMMATES),
            ),
        )
        coEvery { householdRepository.delete("h1") } returns Result.success(Unit)

        val vm = HouseholdsViewModel(householdRepository)
        advanceUntilIdle()

        vm.delete("h1")
        advanceUntilIdle()

        assertEquals(listOf("Flat"), vm.uiState.value.households.map { it.name })
    }
}

class HouseholdDetailViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val householdRepository = mockk<HouseholdRepository>()

    private fun viewModel(id: String = "h1"): HouseholdDetailViewModel =
        HouseholdDetailViewModel(householdRepository, SavedStateHandle(mapOf("householdId" to id)))

    @Test
    fun `loads household and members on init`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
            HouseholdDetail(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = listOf(HouseholdMember(id = 1, householdId = "h1", memberVCardUid = "uid-1", role = "adult")),
            ),
        )

        val vm = viewModel()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isLoading)
        assertEquals("Home", vm.uiState.value.household?.name)
        assertEquals(1, vm.uiState.value.members.size)
        assertEquals("uid-1", vm.uiState.value.members[0].memberVCardUid)
        assertEquals("adult", vm.uiState.value.members[0].role)
    }

    @Test
    fun `addMember appends to the members list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
            HouseholdDetail(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = emptyList(),
            ),
        )
        coEvery { householdRepository.addMember("h1", "uid-2", null) } returns Result.success(
            HouseholdMember(id = 2, householdId = "h1", memberVCardUid = "uid-2"),
        )

        val vm = viewModel()
        advanceUntilIdle()

        vm.addMember("uid-2")
        advanceUntilIdle()

        assertEquals(listOf("uid-2"), vm.uiState.value.members.map { it.memberVCardUid })
    }

    @Test
    fun `removeMember drops the member`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
            HouseholdDetail(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = listOf(
                    HouseholdMember(id = 1, householdId = "h1", memberVCardUid = "uid-1"),
                    HouseholdMember(id = 2, householdId = "h1", memberVCardUid = "uid-2"),
                ),
            ),
        )
        coEvery { householdRepository.removeMember("h1", "uid-1") } returns Result.success(Unit)

        val vm = viewModel()
        advanceUntilIdle()

        vm.removeMember("uid-1")
        advanceUntilIdle()

        assertEquals(listOf("uid-2"), vm.uiState.value.members.map { it.memberVCardUid })
    }
}
