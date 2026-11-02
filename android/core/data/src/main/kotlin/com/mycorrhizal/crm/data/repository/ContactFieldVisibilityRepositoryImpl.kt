package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.ContactFieldVisibilityRepository
import com.mycorrhizal.crm.model.network.ContactFieldVisibility
import com.mycorrhizal.crm.model.network.ContactFieldVisibilityInput
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

class ContactFieldVisibilityRepositoryImpl @Inject constructor(private val apiClient: ApiClient) :
    ContactFieldVisibilityRepository {
    override suspend fun get(contactId: Int): Result<ContactFieldVisibility> =
        apiClient.getContactFieldVisibility(contactId)

    override suspend fun put(
        contactId: Int,
        input: ContactFieldVisibilityInput
    ): Result<ContactFieldVisibility> = apiClient.putContactFieldVisibility(contactId, input)

    override suspend fun reset(contactId: Int): Result<ContactFieldVisibility> =
        apiClient.resetContactFieldVisibility(contactId)
}
