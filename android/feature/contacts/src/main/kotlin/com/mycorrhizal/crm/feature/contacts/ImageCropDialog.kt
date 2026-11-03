package com.mycorrhizal.crm.feature.contacts

import androidx.annotation.StringRes
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Slider
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import coil3.compose.AsyncImage
import com.mycorrhizal.crm.model.network.ContactPhoto
import com.mycorrhizal.crm.model.network.ImageCrop
import com.mycorrhizal.crm.ui.R

/**
 * Square avatar-crop editor for one of the contact's own images (ADR 0040 decision 10).
 * The stored image is never cropped destructively; the saved fractions only steer the
 * server-derived avatar. [onClear] drops the crop so the whole photo is used.
 */
@Composable
fun ImageCropDialog(
    photo: ContactPhoto,
    onSave: (ImageCrop) -> Unit,
    onClear: () -> Unit,
    onDismiss: () -> Unit
) {
    // Open at the stored crop (the image DTO returns it); whole photo when none is stored.
    val initial = remember(photo) { cropToControls(photo.width, photo.height, photo.crop) }
    var zoom by remember { mutableFloatStateOf(initial.zoom) }
    var posX by remember { mutableFloatStateOf(initial.posX) }
    var posY by remember { mutableFloatStateOf(initial.posY) }
    val crop = squareCrop(photo.width, photo.height, zoom, posX, posY)
    val aspect = if (photo.width > 0 && photo.height > 0) {
        photo.width.toFloat() / photo.height
    } else {
        1f
    }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.contact_images_crop_title)) },
        text = {
            Column(modifier = Modifier.verticalScroll(rememberScrollState())) {
                CropPreview(photo, crop, aspect)
                CropSlider(R.string.contact_images_crop_zoom, zoom, MIN_CROP_ZOOM) { zoom = it }
                CropSlider(R.string.contact_images_crop_horizontal, posX) { posX = it }
                CropSlider(R.string.contact_images_crop_vertical, posY) { posY = it }
                TextButton(onClick = onClear) {
                    Text(stringResource(R.string.contact_images_crop_reset))
                }
            }
        },
        confirmButton = {
            TextButton(onClick = { onSave(crop) }) {
                Text(stringResource(R.string.contact_images_crop_save))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        }
    )
}

@Composable
private fun CropPreview(photo: ContactPhoto, crop: ImageCrop, aspect: Float) {
    val outline = MaterialTheme.colorScheme.primary
    Box(modifier = Modifier.fillMaxWidth().aspectRatio(aspect).testTag("crop-preview")) {
        AsyncImage(
            model = photo.thumbnailUrl,
            contentDescription = stringResource(R.string.contact_images_alt),
            contentScale = ContentScale.FillBounds,
            modifier = Modifier.fillMaxSize()
        )
        Canvas(modifier = Modifier.fillMaxSize()) {
            val topLeft = Offset((crop.x * size.width).toFloat(), (crop.y * size.height).toFloat())
            val rect = Size((crop.w * size.width).toFloat(), (crop.h * size.height).toFloat())
            val dim = Color.Black.copy(alpha = DIM_ALPHA)
            // Dim everything outside the crop square.
            drawRect(dim, Offset.Zero, Size(size.width, topLeft.y))
            drawRect(dim, Offset(0f, topLeft.y + rect.height), Size(size.width, size.height))
            drawRect(dim, Offset(0f, topLeft.y), Size(topLeft.x, rect.height))
            drawRect(dim, Offset(topLeft.x + rect.width, topLeft.y), Size(size.width, rect.height))
            drawRect(outline, topLeft, rect, style = Stroke(width = OUTLINE_PX))
        }
    }
}

@Composable
private fun CropSlider(
    @StringRes labelRes: Int,
    value: Float,
    min: Float = 0f,
    onChange: (Float) -> Unit
) {
    val label = stringResource(labelRes)
    Column {
        Text(label, style = MaterialTheme.typography.labelMedium)
        Slider(
            value = value,
            onValueChange = onChange,
            valueRange = min..1f,
            modifier = Modifier.fillMaxWidth().testTag("crop-slider-$labelRes")
        )
    }
}

private const val DIM_ALPHA = 0.5f
private const val OUTLINE_PX = 4f
