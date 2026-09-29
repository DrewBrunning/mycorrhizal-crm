package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.model.network.AdminUser
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class UserManagementRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = UserManagementRepositoryImpl(apiClient)

    @Test
    fun `resetTwoFactor delegates to the api client with the same id`() = runTest {
        val bob = AdminUser(id = 7, username = "bob", email = "bob@example.com")
        coEvery { apiClient.resetUserTwoFactor(7) } returns Result.success(bob)

        val result = repository.resetTwoFactor(7)

        assertEquals(bob, result.getOrThrow())
        coVerify(exactly = 1) { apiClient.resetUserTwoFactor(7) }
    }

    @Test
    fun `resetTwoFactor passes the api failure through unchanged`() = runTest {
        val failure = ApiError.Client(404, "User not found")
        coEvery { apiClient.resetUserTwoFactor(9) } returns Result.failure(failure)

        val result = repository.resetTwoFactor(9)

        assertTrue(result.isFailure)
        assertEquals(failure, result.exceptionOrNull())
    }
}
