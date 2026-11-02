package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.ContactImageRepository
import com.mycorrhizal.crm.model.network.ContactImage
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

class ContactImageRepositoryImpl @Inject constructor(private val apiClient: ApiClient) :
    ContactImageRepository {
    override suspend fun list(contactId: Int): Result<List<ContactImage>> =
        apiClient.listContactImages(contactId).map { it.images }

    override suspend fun upload(
        contactId: Int,
        fileName: String,
        mimeType: String,
        bytes: ByteArray
    ): Result<ContactImage> = apiClient.uploadContactImage(contactId, fileName, mimeType, bytes)

    override suspend fun delete(contactId: Int, imageId: Int): Result<Unit> =
        apiClient.deleteContactImage(contactId, imageId)
}
