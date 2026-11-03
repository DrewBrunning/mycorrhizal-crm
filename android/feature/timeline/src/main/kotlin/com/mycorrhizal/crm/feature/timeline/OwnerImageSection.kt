package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.AddPhotoAlternate
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import coil3.compose.AsyncImage

/** A rounded, cropped thumbnail; clickable only when [onClick] is given. */
@Composable
fun ImageThumbnail(
    model: String?,
    contentDescription: String,
    size: Dp,
    modifier: Modifier = Modifier,
    onClick: (() -> Unit)? = null
) {
    AsyncImage(
        model = model,
        contentDescription = contentDescription,
        contentScale = ContentScale.Crop,
        modifier = modifier
            .size(size)
            .clip(RoundedCornerShape(8.dp))
            .background(MaterialTheme.colorScheme.surfaceVariant)
            .let { if (onClick != null) it.clickable(onClick = onClick) else it }
    )
}

/** The full-size image in a simple dialog, mirroring the web lightbox. */
@Composable
fun ImageLightbox(url: String?, contentDescription: String, onDismiss: () -> Unit) {
    Dialog(onDismissRequest = onDismiss) {
        AsyncImage(
            model = url,
            contentDescription = contentDescription,
            contentScale = ContentScale.Fit,
            modifier = Modifier.fillMaxWidth().padding(8.dp)
        )
    }
}

/**
 * ADR 0040: the thumbnails-plus-edit-controls strip shared by the activity and
 * note forms (#2312). Coil resolves the server-relative URLs through the shared
 * okhttp client. The owner wrappers only supply the model type, URL accessors
 * and strings.
 */
@Composable
fun <T> OwnerImageSection(
    images: List<T>,
    imageId: (T) -> Int,
    thumbnailUrl: (T) -> String?,
    fileUrl: (T) -> String?,
    labels: OwnerImageLabels,
    pendingCount: Int,
    loading: Boolean,
    busy: Boolean,
    onAdd: () -> Unit,
    onDelete: (Int) -> Unit,
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

    var viewing by remember { mutableStateOf<T?>(null) }
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
                        onClick = { viewing = image }
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

    viewing?.let { image ->
        ImageLightbox(
            url = fileUrl(image),
            contentDescription = labels.alt,
            onDismiss = { viewing = null }
        )
    }
}
