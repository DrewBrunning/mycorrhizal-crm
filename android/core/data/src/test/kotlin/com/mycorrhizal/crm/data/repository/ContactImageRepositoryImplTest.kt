package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.ContactMainImage
import com.mycorrhizal.crm.model.network.ContactImage
import com.mycorrhizal.crm.model.network.ContactPhoto
import com.mycorrhizal.crm.model.network.ContactPhotosResponse
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ImageCrop
import com.mycorrhizal.crm.model.network.MainImageResponse
import com.mycorrhizal.crm.network.ApiClient
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class ContactImageRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = ContactImageRepositoryImpl(apiClient)

    @Test
    fun `photos unwraps the aggregated response`() = runTest {
        val photo = ContactPhoto(id = 3)
        coEvery { apiClient.listContactPhotos(5) } returns
            Result.success(ContactPhotosResponse(listOf(photo), 1))

        assertEquals(listOf(photo), repository.photos(5).getOrThrow())
    }

    @Test
    fun `mainImage reads the pointer and revision from the contact`() = runTest {
        coEvery { apiClient.getContact(5) } returns
            Result.success(ContactRecordResponse(id = 5, revision = 7, mainImageId = 9))

        assertEquals(ContactMainImage(9, 7), repository.mainImage(5).getOrThrow())
    }

    @Test
    fun `setMainImage forwards the revision and returns the new pointer`() = runTest {
        coEvery { apiClient.setContactMainImage(5, 9, 3) } returns
            Result.success(MainImageResponse(mainImageId = 9, revision = 4))

        assertEquals(ContactMainImage(9, 4), repository.setMainImage(5, 9, 3).getOrThrow())
    }

    @Test
    fun `clearMainImage returns a null pointer with the new revision`() = runTest {
        coEvery { apiClient.clearContactMainImage(5, 4) } returns
            Result.success(MainImageResponse(revision = 5))

        val cleared = repository.clearMainImage(5, 4).getOrThrow()

        assertNull(cleared.imageId)
        assertEquals(5, cleared.revision)
    }

    @Test
    fun `setSensitivity forwards to the client`() = runTest {
        coEvery { apiClient.patchContactImageSensitivity(5, 9, "secret") } returns
            Result.success(ContactImage(id = 9, sensitivity = "secret"))

        assertEquals("secret", repository.setSensitivity(5, 9, "secret").getOrThrow().sensitivity)
    }

    @Test
    fun `deleteSourceImage routes a note image to the note and an activity image to the activity`() =
        runTest {
            coEvery { apiClient.deleteNoteImage(8, 3) } returns Result.success(Unit)
            coEvery { apiClient.deleteActivityImage(9, 4) } returns Result.success(Unit)

            repository.deleteSourceImage("note", 8, 3).getOrThrow()
            repository.deleteSourceImage("activity", 9, 4).getOrThrow()

            coVerify(exactly = 1) { apiClient.deleteNoteImage(8, 3) }
            coVerify(exactly = 1) { apiClient.deleteActivityImage(9, 4) }
        }

    @Test
    fun `deleteSourceImage rejects the contact kind`() = runTest {
        assertEquals(
            true,
            repository.deleteSourceImage("contact", 5, 9).exceptionOrNull() is
                IllegalArgumentException
        )
    }

    @Test
    fun `setCrop and adopt forward to the client`() = runTest {
        val crop = ImageCrop(0.0, 0.0, 0.5, 0.5)
        coEvery { apiClient.patchContactImageCrop(5, 9, crop) } returns
            Result.success(ContactImage(id = 9))
        coEvery { apiClient.adoptContactImage(5, "note", 8, 3) } returns
            Result.success(ContactImage(id = 12))
        coEvery { apiClient.deleteContactImage(5, 9) } returns Result.success(Unit)
        coEvery { apiClient.uploadContactImage(5, "a.jpg", "image/jpeg", any()) } returns
            Result.success(ContactImage(id = 13))

        assertEquals(9, repository.setCrop(5, 9, crop).getOrThrow().id)
        assertEquals(12, repository.adopt(5, "note", 8, 3).getOrThrow().id)
        assertEquals(
            13,
            repository.upload(5, "a.jpg", "image/jpeg", byteArrayOf(1)).getOrThrow().id
        )
        repository.delete(5, 9).getOrThrow()
        coVerify(exactly = 1) { apiClient.deleteContactImage(5, 9) }
    }
}
