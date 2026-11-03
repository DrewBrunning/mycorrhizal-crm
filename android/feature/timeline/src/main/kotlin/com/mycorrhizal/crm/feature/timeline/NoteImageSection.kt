package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.rememberScrollState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.NoteImage
import com.mycorrhizal.crm.ui.R

/** ADR 0040 slice b: note image thumbnails plus edit controls ([OwnerImageSection]). */
@Composable
fun NoteImageSection(
    images: List<NoteImage>,
    pendingCount: Int,
    loading: Boolean,
    busy: Boolean,
    onAdd: () -> Unit,
    onDelete: (Int) -> Unit,
    modifier: Modifier = Modifier
) {
    OwnerImageSection(
        images = images,
        imageId = { it.id },
        thumbnailUrl = { it.thumbnailUrl },
        fileUrl = { it.fileUrl },
        labels = OwnerImageLabels(
            add = stringResource(R.string.note_images_add),
            empty = stringResource(R.string.note_images_empty),
            alt = stringResource(R.string.note_images_alt),
            delete = stringResource(R.string.note_images_delete),
            pending = stringResource(R.string.note_images_pending, pendingCount)
        ),
        pendingCount = pendingCount,
        loading = loading,
        busy = busy,
        onAdd = onAdd,
        onDelete = onDelete,
        modifier = modifier
    )
}

/**
 * Read-only thumbnails for a note row, with the same lightbox as the editor.
 * A plain scrolling Row, not a LazyRow: ListItem measures its supporting
 * content with intrinsics, which a SubcomposeLayout-backed list rejects.
 */
@Composable
fun NoteImageRow(images: List<NoteImage>, modifier: Modifier = Modifier) {
    if (images.isEmpty()) return
    var viewing by remember { mutableStateOf<NoteImage?>(null) }
    val alt = stringResource(R.string.note_images_alt)
    Row(
        horizontalArrangement = Arrangement.spacedBy(6.dp),
        modifier = modifier.horizontalScroll(rememberScrollState())
    ) {
        images.forEach { image ->
            ImageThumbnail(
                model = image.thumbnailUrl,
                contentDescription = alt,
                size = 64.dp,
                onClick = { viewing = image }
            )
        }
    }
    viewing?.let { image ->
        ImageLightbox(url = image.fileUrl, contentDescription = alt, onDismiss = { viewing = null })
    }
}
