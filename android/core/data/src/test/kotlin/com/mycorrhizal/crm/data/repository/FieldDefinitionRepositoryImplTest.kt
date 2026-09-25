package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.model.network.ContactFieldValuesInput
import com.mycorrhizal.crm.model.network.ContactFieldValuesResponse
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.FieldDefinitionInput
import com.mycorrhizal.crm.model.network.FieldDefinitionsResponse
import com.mycorrhizal.crm.model.network.FieldValue
import com.mycorrhizal.crm.model.network.FieldValueInput
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** Issue #830. Thin-wrapper test, mirroring `WebhookRepositoryImplTest`'s shape. */
class FieldDefinitionRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = FieldDefinitionRepositoryImpl(apiClient)

    @Test
    fun `list unwraps the definitions list on success`() = runTest {
        coEvery { apiClient.listFieldDefinitions(null) } returns Result.success(
            FieldDefinitionsResponse(fieldDefinitions = listOf(FieldDefinition(id = "d1", label = "Coffee order"))),
        )

        val result = repository.list()

        assertTrue(result.isSuccess)
        assertEquals(listOf("d1"), result.getOrThrow().map { it.id })
    }

    @Test
    fun `list failure is normalized through mapError`() = runTest {
        coEvery { apiClient.listFieldDefinitions(null) } returns Result.failure(ApiError.Server(500, "boom"))

        val result = repository.list()

        assertTrue(result.isFailure)
        assertTrue(result.exceptionOrNull() is ApiError)
    }

    @Test
    fun `get returns the field definition on success`() = runTest {
        coEvery { apiClient.getFieldDefinition("d1") } returns Result.success(
            FieldDefinition(id = "d1", label = "Coffee order"),
        )

        val result = repository.get("d1")

        assertTrue(result.isSuccess)
        assertEquals("Coffee order", result.getOrThrow().label)
    }

    @Test
    fun `create forwards the input and returns the created definition`() = runTest {
        val input = FieldDefinitionInput(label = "Coffee order", key = "coffee_order", type = "string")
        coEvery { apiClient.createFieldDefinition(input) } returns Result.success(
            FieldDefinition(id = "d1", label = "Coffee order", key = "coffee_order", type = "string"),
        )

        val result = repository.create(input)

        assertTrue(result.isSuccess)
        assertEquals("d1", result.getOrThrow().id)
        coVerify { apiClient.createFieldDefinition(input) }
    }

    @Test
    fun `update forwards id and input and returns the updated definition`() = runTest {
        val input = FieldDefinitionInput(label = "Renamed", key = "coffee_order", type = "string")
        coEvery { apiClient.updateFieldDefinition("d1", input) } returns Result.success(
            FieldDefinition(id = "d1", label = "Renamed"),
        )

        val result = repository.update("d1", input)

        assertTrue(result.isSuccess)
        assertEquals("Renamed", result.getOrThrow().label)
    }

    @Test
    fun `delete forwards the id`() = runTest {
        coEvery { apiClient.deleteFieldDefinition("d1") } returns Result.success(Unit)

        val result = repository.delete("d1")

        assertTrue(result.isSuccess)
        coVerify { apiClient.deleteFieldDefinition("d1") }
    }

    @Test
    fun `reorder forwards the full order and returns the reordered definitions`() = runTest {
        coEvery { apiClient.reorderFieldDefinitions(listOf("b", "a")) } returns Result.success(
            FieldDefinitionsResponse(
                fieldDefinitions = listOf(
                    FieldDefinition(id = "b", label = "Telegram", position = 0),
                    FieldDefinition(id = "a", label = "Signal", position = 1),
                ),
            ),
        )

        val result = repository.reorder(listOf("b", "a"))

        assertTrue(result.isSuccess)
        assertEquals(listOf("b", "a"), result.getOrThrow().map { it.id })
        coVerify { apiClient.reorderFieldDefinitions(listOf("b", "a")) }
    }

    @Test
    fun `contactValues unwraps the values list on success`() = runTest {
        coEvery { apiClient.listContactFieldValues(5) } returns Result.success(
            ContactFieldValuesResponse(fieldValues = listOf(FieldValue(id = 1, fieldDefinitionId = "d1", value = "Latte"))),
        )

        val result = repository.contactValues(5)

        assertTrue(result.isSuccess)
        assertEquals("Latte", result.getOrThrow().first().value)
    }

    @Test
    fun `replaceContactValues forwards the input and returns the new values`() = runTest {
        val input = ContactFieldValuesInput(fieldValues = listOf(FieldValueInput(fieldDefinitionId = "d1", value = "Latte")))
        coEvery { apiClient.replaceContactFieldValues(5, input) } returns Result.success(
            ContactFieldValuesResponse(fieldValues = listOf(FieldValue(id = 1, fieldDefinitionId = "d1", value = "Latte"))),
        )

        val result = repository.replaceContactValues(5, input)

        assertTrue(result.isSuccess)
        assertEquals("Latte", result.getOrThrow().first().value)
    }
}
