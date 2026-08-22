package com.mycorrhizal.crm.feature.users

import com.mycorrhizal.crm.domain.repository.UserManagementRepository
import com.mycorrhizal.crm.model.network.AdminUser
import com.mycorrhizal.crm.model.network.AdminUserCreateInput
import com.mycorrhizal.crm.model.network.AdminUserUpdateInput
import com.mycorrhizal.crm.model.network.AdminUsersListResponse
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

class UsersViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<UserManagementRepository>()

    private val alice = AdminUser(id = 1, username = "alice", email = "alice@example.com", isAdmin = true)
    private val bob = AdminUser(id = 2, username = "bob", email = "bob@example.com")

    @Test
    fun `loads users on init`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list(any(), any()) } returns Result.success(
            AdminUsersListResponse(users = listOf(alice, bob), total = 2),
        )

        val vm = UsersViewModel(repository)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.isLoading)
        assertEquals(2, state.users.size)
        assertEquals(2, state.total)
        assertEquals("alice", state.users[0].username)
        assertNull(state.error)
    }

    @Test
    fun `load failure surfaces the server error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list(any(), any()) } returns Result.failure(
            ApiError.Client(403, "Forbidden"),
        )

        val vm = UsersViewModel(repository)
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isLoading)
        assertEquals("Forbidden", vm.uiState.value.error)
    }

    @Test
    fun `create saves then reloads the list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list(any(), any()) } returnsMany listOf(
            Result.success(AdminUsersListResponse(users = listOf(alice), total = 1)),
            Result.success(AdminUsersListResponse(users = listOf(alice, bob), total = 2)),
        )
        coEvery { repository.create(any()) } returns Result.success(bob)

        val vm = UsersViewModel(repository)
        advanceUntilIdle()

        var done = false
        vm.create(
            AdminUserCreateInput(username = "bob", email = "bob@example.com", password = "password123"),
            onDone = { done = true },
        )
        advanceUntilIdle()

        assertTrue(done)
        // The reload (not a local splice) produced the two-user list.
        assertEquals(listOf("alice", "bob"), vm.uiState.value.users.map { it.username })
        assertFalse(vm.uiState.value.isSaving)
    }

    @Test
    fun `create failure keeps the error for the dialog`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list(any(), any()) } returns Result.success(
            AdminUsersListResponse(users = listOf(alice), total = 1),
        )
        coEvery { repository.create(any()) } returns Result.failure(
            ApiError.Client(409, "User already exists"),
        )

        val vm = UsersViewModel(repository)
        advanceUntilIdle()

        var done = false
        vm.create(
            AdminUserCreateInput(username = "bob", email = "bob@example.com", password = "password123"),
            onDone = { done = true },
        )
        advanceUntilIdle()

        assertFalse(done)
        assertEquals("User already exists", vm.uiState.value.error)
        assertFalse(vm.uiState.value.isSaving)
    }

    @Test
    fun `update replaces the user in place`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list(any(), any()) } returns Result.success(
            AdminUsersListResponse(users = listOf(alice, bob), total = 2),
        )
        val promotedBob = bob.copy(isAdmin = true)
        coEvery { repository.update(2, any()) } returns Result.success(promotedBob)

        val vm = UsersViewModel(repository)
        advanceUntilIdle()

        var done = false
        vm.update(2, AdminUserUpdateInput(isAdmin = true), onDone = { done = true })
        advanceUntilIdle()

        assertTrue(done)
        assertTrue(vm.uiState.value.users.first { it.id == 2 }.isAdmin)
        assertFalse(vm.uiState.value.isSaving)
    }

    @Test
    fun `update failure keeps the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list(any(), any()) } returns Result.success(
            AdminUsersListResponse(users = listOf(alice, bob), total = 2),
        )
        coEvery { repository.update(1, any()) } returns Result.failure(
            ApiError.Client(403, "Cannot remove the last admin"),
        )

        val vm = UsersViewModel(repository)
        advanceUntilIdle()

        var done = false
        vm.update(1, AdminUserUpdateInput(isAdmin = false), onDone = { done = true })
        advanceUntilIdle()

        assertFalse(done)
        assertEquals("Cannot remove the last admin", vm.uiState.value.error)
    }

    @Test
    fun `delete removes the user and decrements the total`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list(any(), any()) } returns Result.success(
            AdminUsersListResponse(users = listOf(alice, bob), total = 2),
        )
        coEvery { repository.delete(2) } returns Result.success(Unit)

        val vm = UsersViewModel(repository)
        advanceUntilIdle()

        vm.delete(2)
        advanceUntilIdle()

        assertEquals(listOf(1), vm.uiState.value.users.map { it.id })
        assertEquals(1, vm.uiState.value.total)
        assertNull(vm.uiState.value.deletingId)
    }

    @Test
    fun `delete failure surfaces the server error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list(any(), any()) } returns Result.success(
            AdminUsersListResponse(users = listOf(alice, bob), total = 2),
        )
        coEvery { repository.delete(1) } returns Result.failure(
            ApiError.Client(403, "Cannot delete your own account"),
        )

        val vm = UsersViewModel(repository)
        advanceUntilIdle()

        vm.delete(1)
        advanceUntilIdle()

        assertEquals("Cannot delete your own account", vm.uiState.value.error)
        assertEquals(2, vm.uiState.value.users.size)
        assertNull(vm.uiState.value.deletingId)
    }

    @Test
    fun `onErrorShown clears the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list(any(), any()) } returns Result.failure(ApiError.Client(500, "boom"))

        val vm = UsersViewModel(repository)
        advanceUntilIdle()

        assertEquals("boom", vm.uiState.value.error)
        vm.onErrorShown()
        assertNull(vm.uiState.value.error)
    }
}
