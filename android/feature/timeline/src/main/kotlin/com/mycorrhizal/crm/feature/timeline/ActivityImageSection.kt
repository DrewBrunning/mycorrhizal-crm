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
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import coil3.compose.AsyncImage
import com.mycorrhizal.crm.model.network.ActivityImage
import com.mycorrhizal.crm.ui.R

/**
 * ADR 0040: activity image thumbnails plus edit controls. Coil resolves the
 * server-relative URLs through the shared okhttp client; the full variant opens
 * in a simple dialog, mirroring the web lightbox.
 */
@Composable
fun ActivityImageSection(
    images: List<ActivityImage>,
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
                Icon(
                    Icons.Outlined.AddPhotoAlternate,
                    contentDescription = stringResource(R.string.activity_images_add)
                )
            }
            Text(
                stringResource(R.string.activity_images_empty),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
        return
    }

    var viewing by remember { mutableStateOf<ActivityImage?>(null) }
    Row(
        modifier = modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        LazyRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            items(images, key = { it.id }) { image ->
                Box {
                    AsyncImage(
                        model = image.thumbnailUrl,
                        contentDescription = stringResource(R.string.activity_images_alt),
                        contentScale = ContentScale.Crop,
                        modifier = Modifier
                            .size(72.dp)
                            .clip(RoundedCornerShape(8.dp))
                            .background(MaterialTheme.colorScheme.surfaceVariant)
                            .clickable { viewing = image }
                    )
                    IconButton(
                        onClick = { onDelete(image.id) },
                        enabled = !busy,
                        modifier = Modifier.align(Alignment.TopEnd).size(28.dp)
                    ) {
                        Icon(
                            Icons.Outlined.Close,
                            contentDescription = stringResource(R.string.activity_images_delete),
                            modifier = Modifier.size(16.dp)
                        )
                    }
                }
            }
        }
        if (pendingCount > 0) {
            Text(
                stringResource(R.string.activity_images_pending, pendingCount),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
        if (busy || loading) {
            CircularProgressIndicator(modifier = Modifier.size(20.dp))
        }
        IconButton(onClick = onAdd, enabled = !busy) {
            Icon(
                Icons.Outlined.AddPhotoAlternate,
                contentDescription = stringResource(R.string.activity_images_add)
            )
        }
    }

    viewing?.let { image ->
        Dialog(onDismissRequest = { viewing = null }) {
            AsyncImage(
                model = image.fileUrl,
                contentDescription = stringResource(R.string.activity_images_alt),
                contentScale = ContentScale.Fit,
                modifier = Modifier.fillMaxWidth().padding(8.dp)
            )
        }
    }
}
