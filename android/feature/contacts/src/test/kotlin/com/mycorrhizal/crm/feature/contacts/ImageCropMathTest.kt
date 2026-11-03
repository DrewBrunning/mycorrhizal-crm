package com.mycorrhizal.crm.feature.contacts

import com.mycorrhizal.crm.model.network.ImageCrop
import org.junit.Assert.assertEquals
import org.junit.Test

class ImageCropMathTest {

    private fun assertCrop(expected: ImageCrop, actual: ImageCrop) {
        assertEquals(expected.x, actual.x, EPS)
        assertEquals(expected.y, actual.y, EPS)
        assertEquals(expected.w, actual.w, EPS)
        assertEquals(expected.h, actual.h, EPS)
    }

    @Test
    fun `full zoom on a landscape image is a centred full-height square`() {
        assertCrop(ImageCrop(0.25, 0.0, 0.5, 1.0), squareCrop(1000, 500, 1f, 0.5f, 0.5f))
    }

    @Test
    fun `full zoom on a portrait image is a full-width square`() {
        assertCrop(ImageCrop(0.0, 0.25, 1.0, 0.5), squareCrop(500, 1000, 1f, 0.5f, 0.5f))
    }

    @Test
    fun `half zoom pinned to the bottom right corner stays inside the image`() {
        assertCrop(ImageCrop(0.75, 0.5, 0.25, 0.5), squareCrop(1000, 500, 0.5f, 1f, 1f))
    }

    @Test
    fun `out of range inputs are clamped`() {
        assertCrop(ImageCrop(0.0, 0.0, 0.5, 1.0), squareCrop(1000, 500, 3f, -1f, -1f))
        val tiny = squareCrop(1000, 500, 0f, 0f, 0f)
        assertEquals(MIN_CROP_ZOOM * 0.5, tiny.w, EPS)
    }

    @Test
    fun `an unknown size yields the whole image`() {
        assertCrop(ImageCrop(0.0, 0.0, 1.0, 1.0), squareCrop(0, 0, 0.5f, 0.5f, 0.5f))
    }

    private companion object {
        const val EPS = 1e-6
    }
}
