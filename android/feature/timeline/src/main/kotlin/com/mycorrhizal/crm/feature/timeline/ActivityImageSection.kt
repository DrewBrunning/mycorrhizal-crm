package com.mycorrhizal.crm.feature.timeline

import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import com.mycorrhizal.crm.model.network.ActivityImage
import com.mycorrhizal.crm.ui.R

/** ADR 0040: activity image thumbnails plus edit controls ([OwnerImageSection]). */
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
    OwnerImageSection(
        images = images,
        imageId = { it.id },
        thumbnailUrl = { it.thumbnailUrl },
        fileUrl = { it.fileUrl },
        labels = OwnerImageLabels(
            add = stringResource(R.string.activity_images_add),
            empty = stringResource(R.string.activity_images_empty),
            alt = stringResource(R.string.activity_images_alt),
            delete = stringResource(R.string.activity_images_delete),
            pending = stringResource(R.string.activity_images_pending, pendingCount)
        ),
        pendingCount = pendingCount,
        loading = loading,
        busy = busy,
        onAdd = onAdd,
        onDelete = onDelete,
        modifier = modifier
    )
}
