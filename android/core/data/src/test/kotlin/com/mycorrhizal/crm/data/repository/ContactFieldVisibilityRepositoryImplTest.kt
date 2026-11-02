package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.model.network.ContactFieldVisibility
import com.mycorrhizal.crm.model.network.ContactFieldVisibilityInput
import com.mycorrhizal.crm.network.ApiClient
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertTrue
import org.junit.Test

class ContactFieldVisibilityRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = ContactFieldVisibilityRepositoryImpl(apiClient)

    @Test
    fun `get forwards to the API`() = runTest {
        coEvery {
            apiClient.getContactFieldVisibility(5)
        } returns Result.success(ContactFieldVisibility())

        assertTrue(repository.get(5).isSuccess)
        coVerify { apiClient.getContactFieldVisibility(5) }
    }

    @Test
    fun `put forwards the complete input`() = runTest {
        val input = ContactFieldVisibilityInput(mapOf("emails" to "hide"), "hide")
        coEvery {
            apiClient.putContactFieldVisibility(5, input)
        } returns Result.success(ContactFieldVisibility())

        assertTrue(repository.put(5, input).isSuccess)
        coVerify { apiClient.putContactFieldVisibility(5, input) }
    }

    @Test
    fun `reset forwards the contact id`() = runTest {
        coEvery {
            apiClient.resetContactFieldVisibility(5)
        } returns Result.success(ContactFieldVisibility())

        assertTrue(repository.reset(5).isSuccess)
        coVerify { apiClient.resetContactFieldVisibility(5) }
    }
}
