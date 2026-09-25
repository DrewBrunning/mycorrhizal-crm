package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.FieldDefinitionRepository
import com.mycorrhizal.crm.model.network.ContactFieldValuesInput
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.FieldDefinitionInput
import com.mycorrhizal.crm.model.network.FieldValue
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.toApiError
import javax.inject.Inject

/** Issue #830: field-definition CRUD + per-contact value read/write, thin over [ApiClient]. */
class FieldDefinitionRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
) : FieldDefinitionRepository {

    override suspend fun list(limit: Int?): Result<List<FieldDefinition>> =
        apiClient.listFieldDefinitions(limit).mapError().map { it.definitions }

    override suspend fun get(id: String): Result<FieldDefinition> =
        apiClient.getFieldDefinition(id).mapError()

    override suspend fun create(input: FieldDefinitionInput): Result<FieldDefinition> =
        apiClient.createFieldDefinition(input).mapError()

    override suspend fun update(id: String, input: FieldDefinitionInput): Result<FieldDefinition> =
        apiClient.updateFieldDefinition(id, input).mapError()

    override suspend fun delete(id: String): Result<Unit> =
        apiClient.deleteFieldDefinition(id).mapError()

    override suspend fun reorder(order: List<String>): Result<List<FieldDefinition>> =
        apiClient.reorderFieldDefinitions(order).mapError().map { it.definitions }

    override suspend fun contactValues(contactId: Int): Result<List<FieldValue>> =
        apiClient.listContactFieldValues(contactId).mapError().map { it.values }

    override suspend fun replaceContactValues(
        contactId: Int,
        input: ContactFieldValuesInput,
    ): Result<List<FieldValue>> =
        apiClient.replaceContactFieldValues(contactId, input).mapError().map { it.values }

    private fun <T> Result<T>.mapError(): Result<T> =
        fold(onSuccess = { Result.success(it) }, onFailure = { Result.failure(it.toApiError()) })
}
