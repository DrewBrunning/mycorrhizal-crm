package com.mycorrhizal.crm.feature.timeline

import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
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
    onSetSensitivity: (Int, String) -> Unit,
    sensitivityFailedImageId: Int?,
    modifier: Modifier = Modifier
) {
    OwnerImageSection(
        images = images,
        imageId = { it.id },
        thumbnailUrl = { it.thumbnailUrl },
        fileUrl = { it.fileUrl },
        sensitivity = { it.sensitivity },
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
        onSetSensitivity = onSetSensitivity,
        sensitivityFailedImageId = sensitivityFailedImageId,
        modifier = modifier
    )
}

/** Read-only thumbnails for a note row ([OwnerImageRow]). */
@Composable
fun NoteImageRow(images: List<NoteImage>, modifier: Modifier = Modifier) {
    OwnerImageRow(
        images = images,
        thumbnailUrl = { it.thumbnailUrl },
        fileUrl = { it.fileUrl },
        sensitivity = { it.sensitivity },
        contentDescription = stringResource(R.string.note_images_alt),
        modifier = modifier
    )
}
