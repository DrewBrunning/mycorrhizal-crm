package com.mycorrhizal.crm.feature.timeline

import androidx.annotation.StringRes
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.AddPhotoAlternate
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import coil3.compose.AsyncImage
import com.mycorrhizal.crm.ui.LocalServerUrl
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.resolveServerUri

/** The stored sensitivity of an unrestricted image; a missing value reads as this. */
internal const val SENSITIVITY_NORMAL = "normal"

/** ADR 0040 decision 7: the levels, in the order the viewer offers them. */
private val SENSITIVITY_LEVELS = listOf(
    SENSITIVITY_NORMAL to R.string.contact_images_sensitivity_normal,
    "private" to R.string.contact_images_sensitivity_private,
    "secret" to R.string.contact_images_sensitivity_secret
)

private const val BADGE_SCRIM_ALPHA = 0.55f

@StringRes
private fun badgeLabel(sensitivity: String?): Int? = when (sensitivity) {
    "private" -> R.string.contact_images_sensitivity_private
    "secret" -> R.string.contact_images_sensitivity_secret
    else -> null
}

/**
 * A rounded, cropped thumbnail; clickable only when [onClick] is given. A
 * private or secret [sensitivity] is badged, as the web gallery does (#2369).
 */
@Composable
fun ImageThumbnail(
    model: String?,
    contentDescription: String,
    size: Dp,
    modifier: Modifier = Modifier,
    sensitivity: String? = null,
    onClick: (() -> Unit)? = null
) {
    Box(
        modifier = modifier
            .size(size)
            .clip(RoundedCornerShape(8.dp))
            .background(MaterialTheme.colorScheme.surfaceVariant)
            .let { if (onClick != null) it.clickable(onClick = onClick) else it }
    ) {
        AsyncImage(
            model = resolveServerUri(model, LocalServerUrl.current),
            contentDescription = contentDescription,
            contentScale = ContentScale.Crop,
            modifier = Modifier.fillMaxSize()
        )
        badgeLabel(sensitivity)?.let { label ->
            Text(
                text = stringResource(label),
                style = MaterialTheme.typography.labelSmall,
                color = Color.White,
                maxLines = 1,
                modifier = Modifier
                    .align(Alignment.BottomStart)
                    .fillMaxWidth()
                    .background(Color.Black.copy(alpha = BADGE_SCRIM_ALPHA))
                    .padding(horizontal = 4.dp, vertical = 2.dp)
            )
        }
    }
}

/**
 * The full-size image in a simple dialog, mirroring the web lightbox. With
 * [onSetSensitivity] (an owner's edit surface) it also offers the
 * normal/private/secret choice (#2369); [sensitivityFailed] shows that the last
 * change was not saved.
 */
@Composable
fun ImageLightbox(
    url: String?,
    contentDescription: String,
    onDismiss: () -> Unit,
    sensitivity: String? = null,
    onSetSensitivity: ((String) -> Unit)? = null,
    busy: Boolean = false,
    sensitivityFailed: Boolean = false
) {
    Dialog(onDismissRequest = onDismiss) {
        Column {
            AsyncImage(
                model = resolveServerUri(url, LocalServerUrl.current),
                contentDescription = contentDescription,
                contentScale = ContentScale.Fit,
                modifier = Modifier.fillMaxWidth().padding(8.dp)
            )
            if (onSetSensitivity != null) {
                SensitivityControl(
                    current = sensitivity.orEmpty().ifEmpty { SENSITIVITY_NORMAL },
                    onSelect = onSetSensitivity,
                    enabled = !busy,
                    failed = sensitivityFailed
                )
            }
        }
    }
}

@Composable
private fun SensitivityControl(
    current: String,
    onSelect: (String) -> Unit,
    enabled: Boolean,
    failed: Boolean
) {
    Surface(shape = RoundedCornerShape(8.dp), modifier = Modifier.fillMaxWidth().padding(8.dp)) {
        Column(modifier = Modifier.padding(12.dp)) {
            Text(
                stringResource(R.string.image_sensitivity_label),
                style = MaterialTheme.typography.labelLarge
            )
            SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth().padding(top = 8.dp)) {
                SENSITIVITY_LEVELS.forEachIndexed { index, (value, label) ->
                    SegmentedButton(
                        selected = current == value,
                        onClick = { if (value != current) onSelect(value) },
                        enabled = enabled,
                        shape = SegmentedButtonDefaults.itemShape(
                            index = index,
                            count = SENSITIVITY_LEVELS.size
                        ),
                        modifier = Modifier.testTag("image-sensitivity-$value")
                    ) { Text(stringResource(label)) }
                }
            }
            Text(
                stringResource(R.string.image_sensitivity_help),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 8.dp)
            )
            if (failed) {
                Text(
                    stringResource(R.string.image_sensitivity_failed),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.padding(top = 4.dp)
                )
            }
        }
    }
}

/**
 * ADR 0040: the thumbnails-plus-edit-controls strip shared by the activity and
 * note forms (#2312). [ImageThumbnail] and [ImageLightbox] resolve the
 * server-relative URLs against [LocalServerUrl] for the shared okhttp
 * client. The owner wrappers only supply the model type, accessors and
 * strings. The viewer follows the image by id, so a changed sensitivity shows
 * as soon as the list updates.
 */
@Composable
fun <T> OwnerImageSection(
    images: List<T>,
    imageId: (T) -> Int,
    thumbnailUrl: (T) -> String?,
    fileUrl: (T) -> String?,
    sensitivity: (T) -> String?,
    labels: OwnerImageLabels,
    pendingCount: Int,
    loading: Boolean,
    busy: Boolean,
    onAdd: () -> Unit,
    onDelete: (Int) -> Unit,
    onSetSensitivity: (Int, String) -> Unit,
    sensitivityFailedImageId: Int?,
    modifier: Modifier = Modifier
) {
    if (images.isEmpty() && pendingCount == 0 && !loading) {
        Row(modifier = modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            IconButton(onClick = onAdd, enabled = !busy) {
                Icon(Icons.Outlined.AddPhotoAlternate, contentDescription = labels.add)
            }
            Text(
                labels.empty,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
        return
    }

    var viewingId by remember { mutableStateOf<Int?>(null) }
    Row(
        modifier = modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        LazyRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            items(images, key = { imageId(it) }) { image ->
                Box {
                    ImageThumbnail(
                        model = thumbnailUrl(image),
                        contentDescription = labels.alt,
                        size = 72.dp,
                        sensitivity = sensitivity(image),
                        onClick = { viewingId = imageId(image) }
                    )
                    IconButton(
                        onClick = { onDelete(imageId(image)) },
                        enabled = !busy,
                        modifier = Modifier.align(Alignment.TopEnd).size(28.dp)
                    ) {
                        Icon(
                            Icons.Outlined.Close,
                            contentDescription = labels.delete,
                            modifier = Modifier.size(16.dp)
                        )
                    }
                }
            }
        }
        if (pendingCount > 0) {
            Text(
                labels.pending,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
        if (busy || loading) {
            CircularProgressIndicator(modifier = Modifier.size(20.dp))
        }
        IconButton(onClick = onAdd, enabled = !busy) {
            Icon(Icons.Outlined.AddPhotoAlternate, contentDescription = labels.add)
        }
    }

    images.firstOrNull { imageId(it) == viewingId }?.let { image ->
        val id = imageId(image)
        ImageLightbox(
            url = fileUrl(image),
            contentDescription = labels.alt,
            onDismiss = { viewingId = null },
            sensitivity = sensitivity(image),
            onSetSensitivity = { onSetSensitivity(id, it) },
            busy = busy,
            sensitivityFailed = sensitivityFailedImageId == id
        )
    }
}

/**
 * Read-only thumbnails for a list row: the private/secret badge and the
 * viewer, without the edit surface's sensitivity control. A plain scrolling
 * Row, not a LazyRow: ListItem measures its supporting content with
 * intrinsics, which a SubcomposeLayout-backed list rejects.
 */
@Composable
fun <T> OwnerImageRow(
    images: List<T>,
    thumbnailUrl: (T) -> String?,
    fileUrl: (T) -> String?,
    sensitivity: (T) -> String?,
    contentDescription: String,
    modifier: Modifier = Modifier
) {
    if (images.isEmpty()) return
    var viewing by remember { mutableStateOf<T?>(null) }
    Row(
        horizontalArrangement = Arrangement.spacedBy(6.dp),
        modifier = modifier.horizontalScroll(rememberScrollState())
    ) {
        images.forEach { image ->
            ImageThumbnail(
                model = thumbnailUrl(image),
                contentDescription = contentDescription,
                size = 64.dp,
                sensitivity = sensitivity(image),
                onClick = { viewing = image }
            )
        }
    }
    viewing?.let { image ->
        ImageLightbox(
            url = fileUrl(image),
            contentDescription = contentDescription,
            onDismiss = { viewing = null }
        )
    }
}
