package com.mycorrhizal.crm.feature.circles

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.CircleDetail
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.CircleMember
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
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
import org.junit.Before
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
                Circle(id = "c2", name = "Family")
            )
        )

        val vm = CirclesViewModel(circleRepository)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.isLoading)
        assertEquals(2, state.circles.size)
        // The list is presented in name order (#2444), not the server's cursor order.
        assertEquals(listOf("Family", "Friends"), state.circles.map { it.name })
        assertNull(state.error)
    }

    @Test
    fun `load failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { circleRepository.list(any(), any()) } returns Result.failure(
            ApiError.Client(500, "boom")
        )

        val vm = CirclesViewModel(circleRepository)
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isLoading)
        assertEquals("boom", vm.uiState.value.error?.message)
    }

    @Test
    fun `create adds the circle to the list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { circleRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery {
            circleRepository.create(
                "Friends"
            )
        } returns Result.success(Circle(id = "c9", name = "Friends"))

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
            listOf(Circle(id = "c1", name = "Friends"))
        )
        coEvery { circleRepository.rename("c1", "Close Friends") } returns
            Result.success(Circle(id = "c1", name = "Close Friends"))

        val vm = CirclesViewModel(circleRepository)
        advanceUntilIdle()

        vm.rename("c1", "Close Friends")
        advanceUntilIdle()

        assertEquals(listOf("Close Friends"), vm.uiState.value.circles.map { it.name })
    }

    private fun duplicateError() = ApiError.Client(
        409,
        "A circle with this name already exists",
        reason = ErrorReason(ErrorReasonCode.CIRCLE_ALREADY_EXISTS, emptyMap())
    )

    @Test
    fun `create with a taken name sets the inline name error and not the snackbar error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.list(any(), any()) } returns Result.success(
                listOf(Circle(id = "c1", name = "work"))
            )
            coEvery { circleRepository.create("Work") } returns Result.failure(duplicateError())

            val vm = CirclesViewModel(circleRepository)
            advanceUntilIdle()

            var done = false
            vm.create("Work") { done = true }
            advanceUntilIdle()

            assertFalse(done)
            assertTrue(vm.uiState.value.nameTaken)
            assertNull(vm.uiState.value.error)
            assertFalse(vm.uiState.value.isSaving)
            assertEquals(listOf("work"), vm.uiState.value.circles.map { it.name })
        }

    @Test
    fun `rename to a taken name sets the inline name error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.list(any(), any()) } returns Result.success(
                listOf(Circle(id = "c1", name = "Work"), Circle(id = "c2", name = "Home"))
            )
            val taken = Result.failure<Circle>(duplicateError())
            coEvery { circleRepository.rename("c2", "work") } returns taken

            val vm = CirclesViewModel(circleRepository)
            advanceUntilIdle()

            var done = false
            vm.rename("c2", "work") { done = true }
            advanceUntilIdle()

            assertFalse(done)
            assertTrue(vm.uiState.value.nameTaken)
            assertNull(vm.uiState.value.error)
        }

    @Test
    fun `a non-duplicate create failure still surfaces as the general error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.list(any(), any()) } returns Result.success(emptyList())
            val boom = Result.failure<Circle>(ApiError.Client(500, "boom"))
            coEvery { circleRepository.create("X") } returns boom

            val vm = CirclesViewModel(circleRepository)
            advanceUntilIdle()
            vm.create("X")
            advanceUntilIdle()

            assertFalse(vm.uiState.value.nameTaken)
            assertEquals("boom", vm.uiState.value.error?.message)
        }

    @Test
    fun `clearNameError and a fresh attempt reset the inline error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.list(any(), any()) } returns Result.success(emptyList())
            coEvery { circleRepository.create("Work") } returns Result.failure(duplicateError())
            coEvery { circleRepository.create("Play") } returns
                Result.success(Circle(id = "c2", name = "Play"))

            val vm = CirclesViewModel(circleRepository)
            advanceUntilIdle()
            vm.create("Work")
            advanceUntilIdle()
            assertTrue(vm.uiState.value.nameTaken)

            vm.clearNameError()
            assertFalse(vm.uiState.value.nameTaken)

            vm.create("Work")
            advanceUntilIdle()
            assertTrue(vm.uiState.value.nameTaken)
            vm.create("Play")
            advanceUntilIdle()
            assertFalse(vm.uiState.value.nameTaken)
        }

    @Test
    fun `a new circle b lands between a and c without a reload`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.list(any(), any()) } returns Result.success(
                listOf(Circle(id = "c1", name = "a"), Circle(id = "c3", name = "c"))
            )
            coEvery { circleRepository.create("b") } returns
                Result.success(Circle(id = "c2", name = "b"))

            val vm = CirclesViewModel(circleRepository)
            advanceUntilIdle()
            vm.create("b")
            advanceUntilIdle()

            assertEquals(listOf("a", "b", "c"), vm.uiState.value.circles.map { it.name })
        }

    @Test
    fun `sorting is case-insensitive and applies to the loaded list`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.list(any(), any()) } returns Result.success(
                listOf(
                    Circle(id = "c1", name = "banana"),
                    Circle(id = "c2", name = "Cherry"),
                    Circle(id = "c3", name = "Apple")
                )
            )

            val vm = CirclesViewModel(circleRepository)
            advanceUntilIdle()

            assertEquals(
                listOf("Apple", "banana", "Cherry"),
                vm.uiState.value.circles.map { it.name }
            )
        }

    @Test
    fun `a rename re-sorts the list without a reload`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.list(any(), any()) } returns Result.success(
                listOf(Circle(id = "c1", name = "a"), Circle(id = "c2", name = "b"))
            )
            coEvery { circleRepository.rename("c1", "c") } returns
                Result.success(Circle(id = "c1", name = "c"))

            val vm = CirclesViewModel(circleRepository)
            advanceUntilIdle()
            vm.rename("c1", "c")
            advanceUntilIdle()

            assertEquals(listOf("b", "c"), vm.uiState.value.circles.map { it.name })
        }

    @Test
    fun `renaming a circle to its own name in a different case saves`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { circleRepository.list(any(), any()) } returns Result.success(
                listOf(Circle(id = "c1", name = "work"))
            )
            coEvery { circleRepository.rename("c1", "Work") } returns
                Result.success(Circle(id = "c1", name = "Work"))

            val vm = CirclesViewModel(circleRepository)
            advanceUntilIdle()
            var done = false
            vm.rename("c1", "Work") { done = true }
            advanceUntilIdle()

            assertTrue(done)
            assertFalse(vm.uiState.value.nameTaken)
            assertEquals(listOf("Work"), vm.uiState.value.circles.map { it.name })
        }

    @Test
    fun `delete removes the circle`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { circleRepository.list(any(), any()) } returns Result.success(
            listOf(Circle(id = "c1", name = "Friends"), Circle(id = "c2", name = "Family"))
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
    private val contactRepository = mockk<ContactRepository>()

    @Before
    fun setUp() {
        // Name/phone resolution is best-effort; tests that don't care get an empty map.
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())
    }

    private fun viewModel(id: String = "c1"): CircleDetailViewModel = CircleDetailViewModel(
        circleRepository,
        contactRepository,
        SavedStateHandle(mapOf("circleId" to id))
    )

    @Test
    fun `loads circle and members on init`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { circleRepository.getWithMembers("c1") } returns Result.success(
            CircleDetail(
                circle = Circle(id = "c1", name = "Friends"),
                members = listOf(CircleMember(id = 1, circleId = "c1", memberVCardUid = "uid-1"))
            )
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
    fun `load resolves member names and phones via contact resolution`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { circleRepository.getWithMembers("c1") } returns Result.success(
            CircleDetail(
                circle = Circle(id = "c1", name = "Friends"),
                members = listOf(CircleMember(id = 1, circleId = "c1", memberVCardUid = "uid-1"))
            )
        )
        coEvery { contactRepository.resolveByUid(listOf("uid-1")) } returns Result.success(
            mapOf(
                "uid-1" to ContactSummary(
                    id = 7,
                    uid = "uid-1",
                    firstname = "Alice",
                    primaryPhone = "+1-555-0100"
                )
            )
        )

        val vm = viewModel()
        advanceUntilIdle()

        assertEquals("Alice", vm.uiState.value.contactsByUid["uid-1"]?.firstname)
        assertEquals("+1-555-0100", vm.uiState.value.contactsByUid["uid-1"]?.primaryPhone)
    }

    @Test
    fun `addMember appends to the members list and resolves its name`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { circleRepository.getWithMembers("c1") } returns Result.success(
            CircleDetail(
                circle = Circle(id = "c1", name = "Friends"),
                members = emptyList()
            )
        )
        coEvery { circleRepository.addMember("c1", "uid-2") } returns Result.success(
            CircleMember(id = 2, circleId = "c1", memberVCardUid = "uid-2")
        )
        coEvery { contactRepository.resolveByUid(listOf("uid-2")) } returns Result.success(
            mapOf(
                "uid-2" to ContactSummary(
                    id = 8,
                    uid = "uid-2",
                    firstname = "Bob",
                    primaryPhone = "+1-555-0101"
                )
            )
        )

        val vm = viewModel()
        advanceUntilIdle()

        vm.addMember("uid-2")
        advanceUntilIdle()

        assertEquals(listOf("uid-2"), vm.uiState.value.members.map { it.memberVCardUid })
        assertEquals("Bob", vm.uiState.value.contactsByUid["uid-2"]?.firstname)
    }

    @Test
    fun `removeMember drops the member`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { circleRepository.getWithMembers("c1") } returns Result.success(
            CircleDetail(
                circle = Circle(id = "c1", name = "Friends"),
                members = listOf(
                    CircleMember(id = 1, circleId = "c1", memberVCardUid = "uid-1"),
                    CircleMember(id = 2, circleId = "c1", memberVCardUid = "uid-2")
                )
            )
        )
        coEvery { circleRepository.removeMember("c1", "uid-1") } returns Result.success(Unit)

        val vm = viewModel()
        advanceUntilIdle()

        vm.removeMember("uid-1")
        advanceUntilIdle()

        assertEquals(listOf("uid-2"), vm.uiState.value.members.map { it.memberVCardUid })
    }
}
