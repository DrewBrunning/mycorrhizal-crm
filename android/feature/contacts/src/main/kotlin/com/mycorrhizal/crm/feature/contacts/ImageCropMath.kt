package com.mycorrhizal.crm.feature.contacts

import com.mycorrhizal.crm.model.network.ImageCrop

/** Smallest zoom the crop editor allows: the square side as a fraction of the shorter edge. */
const val MIN_CROP_ZOOM = 0.1f

/**
 * A square crop (the avatar is square-cropped server-side) as fractions of the full
 * variant (ADR 0040 decision 10). [zoom] is the square side relative to the shorter
 * edge (1 = the largest square); [posX]/[posY] in 0..1 slide it across the leftover
 * space, so the rectangle always stays inside the image. Out-of-range input is clamped.
 */
fun squareCrop(width: Int, height: Int, zoom: Float, posX: Float, posY: Float): ImageCrop {
    if (width <= 0 || height <= 0) return ImageCrop(0.0, 0.0, 1.0, 1.0)
    val side = zoom.coerceIn(MIN_CROP_ZOOM, 1f) * minOf(width, height)
    val w = side.toDouble() / width
    val h = side.toDouble() / height
    return ImageCrop(
        x = posX.coerceIn(0f, 1f) * (1.0 - w),
        y = posY.coerceIn(0f, 1f) * (1.0 - h),
        w = w,
        h = h
    )
}

/** The three crop-editor sliders: [zoom] as in [squareCrop], [posX]/[posY] in 0..1. */
data class CropControls(val zoom: Float, val posX: Float, val posY: Float)

/**
 * Inverse of [squareCrop], so the editor opens at the stored crop. No crop (or an unknown
 * size) opens on the whole photo, centred; a crop that leaves no room to slide on an axis
 * opens centred on it.
 */
fun cropToControls(width: Int, height: Int, crop: ImageCrop?): CropControls {
    if (crop == null || width <= 0 || height <= 0) return CropControls(1f, CENTRE, CENTRE)
    val zoom = (crop.w * width / minOf(width, height)).toFloat().coerceIn(MIN_CROP_ZOOM, 1f)
    val spareX = 1.0 - crop.w
    val spareY = 1.0 - crop.h
    return CropControls(
        zoom = zoom,
        posX = if (spareX > EPSILON) (crop.x / spareX).toFloat().coerceIn(0f, 1f) else CENTRE,
        posY = if (spareY > EPSILON) (crop.y / spareY).toFloat().coerceIn(0f, 1f) else CENTRE
    )
}

private const val CENTRE = 0.5f
private const val EPSILON = 1e-9
