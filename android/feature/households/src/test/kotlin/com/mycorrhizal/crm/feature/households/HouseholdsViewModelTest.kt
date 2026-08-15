package com.mycorrhizal.crm.feature.households

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ContactsPage
import com.mycorrhizal.crm.domain.repository.HouseholdDetail
import com.mycorrhizal.crm.domain.repository.HouseholdRepository
import com.mycorrhizal.crm.model.network.AddressHouseholdSuggestion
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.Household
import com.mycorrhizal.crm.model.network.HouseholdMember
import com.mycorrhizal.crm.model.network.HouseholdTypes
import com.mycorrhizal.crm.model.network.RelationshipEdge
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test

class HouseholdsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val householdRepository = mockk<HouseholdRepository>()
    private val contactRepository = mockk<ContactRepository>()

    private val suggestion = AddressHouseholdSuggestion(
        addressHash = "ah1",
        memberHash = "mh1",
        memberVCardUids = listOf("u1", "u2"),
    )

    @Before
    fun setUp() {
        // Name resolution is best-effort; tests that don't care get an empty map.
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())
    }

    @Test
    fun `loads households on init`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.list(any(), any()) } returns Result.success(
            listOf(
                Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                Household(id = "h2", name = "Flat", type = HouseholdTypes.ROOMMATES),
            ),
        )

        val vm = HouseholdsViewModel(householdRepository, contactRepository)
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

        val vm = HouseholdsViewModel(householdRepository, contactRepository)
        advanceUntilIdle()

        assertEquals("boom", vm.uiState.value.error)
    }

    @Test
    fun `create adds the household to the list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery { householdRepository.create("Home", HouseholdTypes.FAMILY_UNIT) } returns
            Result.success(Household(id = "h9", name = "Home", type = HouseholdTypes.FAMILY_UNIT))

        val vm = HouseholdsViewModel(householdRepository, contactRepository)
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

        val vm = HouseholdsViewModel(householdRepository, contactRepository)
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

        val vm = HouseholdsViewModel(householdRepository, contactRepository)
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

        val vm = HouseholdsViewModel(householdRepository, contactRepository)
        advanceUntilIdle()

        vm.delete("h1")
        advanceUntilIdle()

        assertEquals(listOf("Flat"), vm.uiState.value.households.map { it.name })
    }

    @Test
    fun `scan loads address suggestions into state`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery { householdRepository.suggestAddressHouseholds() } returns Result.success(listOf(suggestion))
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())

        val vm = HouseholdsViewModel(householdRepository, contactRepository)
        advanceUntilIdle()

        vm.scanAddressSuggestions()
        advanceUntilIdle()

        val state = vm.uiState.value
        assertTrue(state.suggestionsLoaded)
        assertFalse(state.suggestionsLoading)
        assertEquals(1, state.addressSuggestions.size)
        assertEquals(listOf("u1", "u2"), state.addressSuggestions[0].memberVCardUids)
    }

    @Test
    fun `scan with nothing to suggest renders an empty state`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery { householdRepository.suggestAddressHouseholds() } returns Result.success(emptyList())

        val vm = HouseholdsViewModel(householdRepository, contactRepository)
        advanceUntilIdle()

        vm.scanAddressSuggestions()
        advanceUntilIdle()

        val state = vm.uiState.value
        assertTrue(state.suggestionsLoaded)
        assertTrue(state.addressSuggestions.isEmpty())
    }

    @Test
    fun `scan failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery { householdRepository.suggestAddressHouseholds() } returns Result.failure(
            ApiError.Client(500, "boom"),
        )

        val vm = HouseholdsViewModel(householdRepository, contactRepository)
        advanceUntilIdle()

        vm.scanAddressSuggestions()
        advanceUntilIdle()

        assertEquals("boom", vm.uiState.value.error)
    }

    @Test
    fun `accept removes the suggestion and reloads households`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.list(any(), any()) } returnsMany listOf(
            Result.success(emptyList()),
            Result.success(listOf(Household(id = "h9", name = "Alice & Bob", type = HouseholdTypes.FAMILY_UNIT))),
        )
        coEvery { householdRepository.suggestAddressHouseholds() } returns Result.success(listOf(suggestion))
        coEvery { householdRepository.acceptAddressSuggestion(any()) } returns Result.success(
            Household(id = "h9", name = "Alice & Bob", type = HouseholdTypes.FAMILY_UNIT),
        )

        val vm = HouseholdsViewModel(householdRepository, contactRepository)
        advanceUntilIdle()
        vm.scanAddressSuggestions()
        advanceUntilIdle()

        vm.acceptSuggestion(suggestion)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertTrue(state.addressSuggestions.isEmpty())
        assertEquals(listOf("h9"), state.households.map { it.id })
        assertEquals(R.string.households_suggestion_accepted, state.infoRes)
    }

    @Test
    fun `accept failure surfaces the error and keeps the suggestion`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery { householdRepository.suggestAddressHouseholds() } returns Result.success(listOf(suggestion))
        coEvery { householdRepository.acceptAddressSuggestion(any()) } returns Result.failure(
            ApiError.Client(409, "already in a household"),
        )

        val vm = HouseholdsViewModel(householdRepository, contactRepository)
        advanceUntilIdle()
        vm.scanAddressSuggestions()
        advanceUntilIdle()

        vm.acceptSuggestion(suggestion)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals("already in a household", state.error)
        assertNull(state.pendingSuggestionKey)
        assertEquals(1, state.addressSuggestions.size)
    }

    @Test
    fun `dismiss removes the suggestion`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery { householdRepository.suggestAddressHouseholds() } returns Result.success(listOf(suggestion))
        coEvery { householdRepository.dismissAddressSuggestion(any()) } returns Result.success(Unit)

        val vm = HouseholdsViewModel(householdRepository, contactRepository)
        advanceUntilIdle()
        vm.scanAddressSuggestions()
        advanceUntilIdle()

        vm.dismissSuggestion(suggestion)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertTrue(state.addressSuggestions.isEmpty())
        assertEquals(R.string.households_suggestion_dismissed, state.infoRes)
    }

    @Test
    fun `scan resolves suggestion member names`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery { householdRepository.suggestAddressHouseholds() } returns Result.success(listOf(suggestion))
        coEvery { contactRepository.resolveByUid(listOf("u1", "u2")) } returns Result.success(
            mapOf(
                "u1" to ContactSummary(id = 1, uid = "u1", firstname = "Alice"),
                "u2" to ContactSummary(id = 2, uid = "u2", firstname = "Bob"),
            ),
        )

        val vm = HouseholdsViewModel(householdRepository, contactRepository)
        advanceUntilIdle()
        vm.scanAddressSuggestions()
        advanceUntilIdle()

        assertEquals(2, vm.uiState.value.contactsByUid.size)
        assertEquals("Alice", vm.uiState.value.contactsByUid["u1"]?.firstname)
    }
}

class HouseholdDetailViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val householdRepository = mockk<HouseholdRepository>()
    private val contactRepository = mockk<ContactRepository>()

    @Before
    fun setUp() {
        // Name resolution is best-effort; tests that don't care get an empty map.
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())
    }

    private fun viewModel(id: String = "h1"): HouseholdDetailViewModel =
        HouseholdDetailViewModel(
            householdRepository,
            contactRepository,
            SavedStateHandle(mapOf("householdId" to id)),
        )

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
    fun `load resolves member names via contact resolution`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
            HouseholdDetail(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = listOf(HouseholdMember(id = 1, householdId = "h1", memberVCardUid = "uid-1", role = "adult")),
            ),
        )
        coEvery { contactRepository.resolveByUid(listOf("uid-1")) } returns Result.success(
            mapOf("uid-1" to ContactSummary(id = 7, uid = "uid-1", firstname = "Alice")),
        )

        val vm = viewModel()
        advanceUntilIdle()

        assertEquals("Alice", vm.uiState.value.contactsByUid["uid-1"]?.firstname)
        assertEquals(7, vm.uiState.value.contactsByUid["uid-1"]?.id)
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
    fun `updateMemberRole updates the member in place`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
            HouseholdDetail(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = listOf(HouseholdMember(id = 1, householdId = "h1", memberVCardUid = "uid-1", role = "adult")),
            ),
        )
        coEvery { householdRepository.updateMember("h1", "uid-1", "child") } returns Result.success(Unit)

        val vm = viewModel()
        advanceUntilIdle()

        vm.updateMemberRole("uid-1", "child")
        advanceUntilIdle()

        assertEquals("child", vm.uiState.value.members[0].role)
        coVerify { householdRepository.updateMember("h1", "uid-1", "child") }
    }

    @Test
    fun `updateMemberRole with no role clears via an empty string not a null`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
            HouseholdDetail(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = listOf(HouseholdMember(id = 1, householdId = "h1", memberVCardUid = "uid-1", role = "adult")),
            ),
        )
        coEvery { householdRepository.updateMember("h1", "uid-1", "") } returns Result.success(Unit)

        val vm = viewModel()
        advanceUntilIdle()

        vm.updateMemberRole("uid-1", null)
        advanceUntilIdle()

        assertEquals("", vm.uiState.value.members[0].role)
        coVerify { householdRepository.updateMember("h1", "uid-1", "") }
    }

    @Test
    fun `updateMemberRole failure surfaces the error and resets the updating flag`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
            HouseholdDetail(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = listOf(HouseholdMember(id = 1, householdId = "h1", memberVCardUid = "uid-1", role = "adult")),
            ),
        )
        coEvery { householdRepository.updateMember("h1", "uid-1", "child") } returns Result.failure(
            ApiError.Client(500, "boom"),
        )

        val vm = viewModel()
        advanceUntilIdle()

        vm.updateMemberRole("uid-1", "child")
        advanceUntilIdle()

        assertEquals("boom", vm.uiState.value.error)
        assertNull(vm.uiState.value.updatingRoleUid)
        assertEquals("adult", vm.uiState.value.members[0].role)
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

    @Test
    fun `suggestRelationships surfaces the count when edges are created`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
            HouseholdDetail(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = listOf(
                    HouseholdMember(id = 1, householdId = "h1", memberVCardUid = "uid-1"),
                    HouseholdMember(id = 2, householdId = "h1", memberVCardUid = "uid-2"),
                ),
            ),
        )
        coEvery { householdRepository.suggestRelationships("h1") } returns Result.success(
            listOf(
                RelationshipEdge(
                    id = "e1",
                    sourceId = "uid-1",
                    targetId = "uid-2",
                    type = "spouse_of",
                    status = "suggested",
                ),
            ),
        )

        val vm = viewModel()
        advanceUntilIdle()

        vm.suggestRelationships()
        advanceUntilIdle()

        assertEquals(R.string.households_suggestions_generated, vm.uiState.value.infoRes)
        assertEquals(1, vm.uiState.value.infoCount)
    }

    @Test
    fun `suggestRelationships with nothing new surfaces the no-new-suggestions message`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
                HouseholdDetail(
                    household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                    members = listOf(
                        HouseholdMember(id = 1, householdId = "h1", memberVCardUid = "uid-1"),
                        HouseholdMember(id = 2, householdId = "h1", memberVCardUid = "uid-2"),
                    ),
                ),
            )
            coEvery { householdRepository.suggestRelationships("h1") } returns Result.success(emptyList())

            val vm = viewModel()
            advanceUntilIdle()

            vm.suggestRelationships()
            advanceUntilIdle()

            assertEquals(R.string.households_no_new_suggestions, vm.uiState.value.infoRes)
            assertNull(vm.uiState.value.infoCount)
        }

    @Test
    fun `suggestRelationships is a no-op with fewer than two members`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
            HouseholdDetail(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = listOf(HouseholdMember(id = 1, householdId = "h1", memberVCardUid = "uid-1")),
            ),
        )

        val vm = viewModel()
        advanceUntilIdle()

        vm.suggestRelationships()
        advanceUntilIdle()

        coVerify(exactly = 0) { householdRepository.suggestRelationships(any()) }
    }

    @Test
    fun `searchContacts returns results excluding existing members`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
            HouseholdDetail(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = listOf(HouseholdMember(id = 1, householdId = "h1", memberVCardUid = "uid-1")),
            ),
        )
        coEvery { contactRepository.listContacts(any(), any(), any()) } returns Result.success(
            ContactsPage(
                contacts = listOf(
                    ContactSummary(id = 1, uid = "uid-1", firstname = "Alice"),
                    ContactSummary(id = 2, uid = "uid-2", firstname = "Bob"),
                ),
                nextCursor = null,
                limit = 25,
                sync = null,
            ),
        )

        val vm = viewModel()
        advanceUntilIdle()

        vm.searchContacts("bo")
        advanceUntilIdle()

        val results = vm.uiState.value.memberSearchResults
        assertEquals(listOf("uid-2"), results.map { it.uid })
    }

    @Test
    fun `blank search does not query the repository`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
            HouseholdDetail(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = emptyList(),
            ),
        )

        val vm = viewModel()
        advanceUntilIdle()

        vm.searchContacts("   ")
        advanceUntilIdle()

        coVerify(exactly = 0) { contactRepository.listContacts(any(), any(), any()) }
    }

    @Test
    fun `search failure clears loading without surfacing an error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { householdRepository.getWithMembers("h1") } returns Result.success(
            HouseholdDetail(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = emptyList(),
            ),
        )
        coEvery { contactRepository.listContacts(any(), any(), any()) } returns Result.failure(
            ApiError.Network(java.io.IOException("offline")),
        )

        val vm = viewModel()
        advanceUntilIdle()

        vm.searchContacts("bo")
        advanceUntilIdle()

        assertFalse(vm.uiState.value.memberSearchLoading)
        assertTrue(vm.uiState.value.memberSearchResults.isEmpty())
        assertNull(vm.uiState.value.error)
    }
}
