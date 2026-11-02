package com.mycorrhizal.crm.model.network

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ActivityImageTest {

    @Test
    fun `parses the owner-nested image payload`() {
        val image = ActivityImage(
            id = 9,
            uuid = "u",
            activityId = 4,
            mimeType = "image/jpeg",
            width = 10,
            height = 10,
            sizeBytes = 100,
            sensitivity = "normal",
            takenAt = "2024-07-03T08:11:12Z",
            takenDate = "2024-07-03",
            createdAt = "2026-01-01T00:00:00Z",
            fileUrl = "/api/v1/activities/4/images/9/file",
            thumbnailUrl = "/api/v1/activities/4/images/9/thumbnail"
        )
        assertEquals(4, image.activityId)
        assertEquals("/api/v1/activities/4/images/9/file", image.fileUrl)

        assertTrue(ActivityImagesResponse(imagesRaw = null).images.isEmpty())
        assertEquals(listOf(image), ActivityImagesResponse(imagesRaw = listOf(image)).images)
        assertEquals(image, UploadActivityImageResponse(image).image)
    }
}
