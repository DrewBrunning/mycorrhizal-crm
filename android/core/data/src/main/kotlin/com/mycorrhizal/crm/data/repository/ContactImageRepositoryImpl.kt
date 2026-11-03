package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.ContactImageRepository
import com.mycorrhizal.crm.domain.repository.ContactMainImage
import com.mycorrhizal.crm.model.network.ContactImage
import com.mycorrhizal.crm.model.network.ContactPhoto
import com.mycorrhizal.crm.model.network.ImageCrop
import com.mycorrhizal.crm.model.network.PhotoSource
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

class ContactImageRepositoryImpl @Inject constructor(private val apiClient: ApiClient) :
    ContactImageRepository {
    override suspend fun photos(contactId: Int): Result<List<ContactPhoto>> =
        apiClient.listContactPhotos(contactId).map { it.photos }

    override suspend fun upload(
        contactId: Int,
        fileName: String,
        mimeType: String,
        bytes: ByteArray
    ): Result<ContactImage> = apiClient.uploadContactImage(contactId, fileName, mimeType, bytes)

    override suspend fun delete(contactId: Int, imageId: Int): Result<Unit> =
        apiClient.deleteContactImage(contactId, imageId)

    override suspend fun mainImage(contactId: Int): Result<ContactMainImage> =
        apiClient.getContact(contactId).map { ContactMainImage(it.mainImageId, it.revision) }

    override suspend fun setMainImage(
        contactId: Int,
        imageId: Int,
        revision: Int?
    ): Result<ContactMainImage> = apiClient.setContactMainImage(contactId, imageId, revision)
        .map { ContactMainImage(it.mainImageId ?: imageId, it.revision) }

    override suspend fun clearMainImage(contactId: Int, revision: Int?): Result<ContactMainImage> =
        apiClient.clearContactMainImage(contactId, revision)
            .map { ContactMainImage(null, it.revision) }

    override suspend fun setCrop(
        contactId: Int,
        imageId: Int,
        crop: ImageCrop?
    ): Result<ContactImage> = apiClient.patchContactImageCrop(contactId, imageId, crop)

    override suspend fun setSensitivity(
        contactId: Int,
        imageId: Int,
        sensitivity: String
    ): Result<ContactImage> = apiClient.patchContactImageSensitivity(
        contactId,
        imageId,
        sensitivity
    )

    override suspend fun deleteSourceImage(
        sourceKind: String,
        sourceId: Int,
        imageId: Int
    ): Result<Unit> = when (sourceKind) {
        PhotoSource.KIND_NOTE -> apiClient.deleteNoteImage(sourceId, imageId)

        PhotoSource.KIND_ACTIVITY -> apiClient.deleteActivityImage(sourceId, imageId)

        else -> Result.failure(
            IllegalArgumentException("Not a note or activity image: $sourceKind")
        )
    }

    override suspend fun adopt(
        contactId: Int,
        sourceKind: String,
        sourceId: Int,
        imageId: Int
    ): Result<ContactImage> = apiClient.adoptContactImage(contactId, sourceKind, sourceId, imageId)
}
