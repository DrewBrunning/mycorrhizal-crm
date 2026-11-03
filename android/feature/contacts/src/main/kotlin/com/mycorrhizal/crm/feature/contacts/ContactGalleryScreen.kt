package com.mycorrhizal.crm.feature.contacts

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.MoreVert
import androidx.compose.material.icons.outlined.PhotoLibrary
import androidx.compose.material.icons.outlined.Star
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import coil3.compose.AsyncImage
import com.mycorrhizal.crm.model.network.ContactPhoto
import com.mycorrhizal.crm.model.network.ImageCrop
import com.mycorrhizal.crm.model.network.PhotoSource
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.FabListBottomPadding
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.RefreshableContent
import com.mycorrhizal.crm.ui.components.localizedMessage

/**
 * ADR 0040 slice c: the contact Photos screen (web Photos section parity). It shows
 * the aggregated gallery (own, note and activity images) with source labels; thumbnails
 * open a lightbox, the upload FAB uses the system photo picker, and an own image can be
 * made the main image (when normal sensitivity), cropped for the avatar, given another
 * sensitivity, or deleted. A note/activity image can be copied into the contact's own
 * gallery or deleted (which removes it from its note or activity too).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContactGalleryScreen(
    contactId: Int,
    onBack: () -> Unit,
    viewModel: ContactGalleryViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val context = LocalContext.current

    LaunchedEffect(contactId) {
        viewModel.setContact(contactId)
    }

    val picker = rememberLauncherForActivityResult(
        ActivityResultContracts.PickMultipleVisualMedia(20)
    ) { uris ->
        val picked = uris.mapNotNull { uri ->
            val bytes = context.contentResolver.openInputStream(uri)?.use { it.readBytes() }
            if (bytes == null) {
                null
            } else {
                PickedContactImage(
                    fileName = "photo.jpg",
                    mimeType = context.contentResolver.getType(uri) ?: "image/jpeg",
                    bytes = bytes
                )
            }
        }
        viewModel.uploadAll(picked)
    }

    ContactGalleryScreenContent(
        uiState = state,
        onBack = onBack,
        onAddClick = {
            picker.launch(
                PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly)
            )
        },
        onDelete = viewModel::delete,
        onSetMain = viewModel::setMain,
        onClearMain = viewModel::clearMain,
        onCrop = viewModel::saveCrop,
        onAdopt = viewModel::adopt,
        onSetSensitivity = viewModel::setSensitivity,
        onRefresh = viewModel::load,
        onErrorShown = viewModel::onErrorShown
    )
}

/**
 * Stateless gallery content (house pattern): loading / empty / populated, the per-photo
 * menu, the delete confirmation, the crop editor and the lightbox. The system picker
 * stays in [ContactGalleryScreen] so this surface is directly testable.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContactGalleryScreenContent(
    uiState: ContactGalleryUiState,
    modifier: Modifier = Modifier,
    onBack: () -> Unit = {},
    onAddClick: () -> Unit = {},
    onDelete: (ContactPhoto) -> Unit = {},
    onSetMain: (ContactPhoto) -> Unit = {},
    onClearMain: () -> Unit = {},
    onCrop: (ContactPhoto, ImageCrop?) -> Unit = { _, _ -> },
    onAdopt: (ContactPhoto) -> Unit = {},
    onSetSensitivity: (ContactPhoto, String) -> Unit = { _, _ -> },
    onRefresh: () -> Unit = {},
    onErrorShown: () -> Unit = {}
) {
    val snackbarHostState = remember { SnackbarHostState() }
    var pendingDelete by remember { mutableStateOf<ContactPhoto?>(null) }
    var cropping by remember { mutableStateOf<ContactPhoto?>(null) }
    var changingSensitivity by remember { mutableStateOf<ContactPhoto?>(null) }
    var viewing by remember { mutableStateOf<ContactPhoto?>(null) }
    val currentOnErrorShown = rememberUpdatedState(onErrorShown)

    Scaffold(
        modifier = modifier,
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.cd_back)
                        )
                    }
                },
                title = {
                    Text(
                        stringResource(R.string.contact_images_title),
                        style = MaterialTheme.typography.titleLarge
                    )
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary
                )
            )
        },
        floatingActionButton = {
            BrandFab(
                onClick = { if (!uiState.isUploading) onAddClick() },
                modifier = Modifier.testTag("add-contact-image")
            ) {
                if (uiState.isUploading) {
                    CircularProgressIndicator(
                        modifier = Modifier.size(24.dp),
                        strokeWidth = 2.dp,
                        color = MaterialTheme.colorScheme.onPrimary
                    )
                } else {
                    Icon(
                        Icons.Outlined.Add,
                        contentDescription = stringResource(R.string.contact_images_add)
                    )
                }
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) }
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            RefreshableContent(
                isRefreshing = uiState.isLoading && uiState.photos.isNotEmpty(),
                onRefresh = onRefresh,
                modifier = Modifier.fillMaxSize()
            ) {
                when {
                    uiState.isLoading && uiState.photos.isEmpty() -> LoadingSkeleton()

                    uiState.photos.isEmpty() -> EmptyState(
                        title = stringResource(R.string.contact_images_empty),
                        body = stringResource(R.string.contact_images_empty_body),
                        icon = Icons.Outlined.PhotoLibrary,
                        actionLabel = stringResource(R.string.contact_images_add),
                        onAction = onAddClick,
                        modifier = Modifier.testTag("contact-gallery-empty")
                    )

                    else -> PhotoGrid(
                        uiState = uiState,
                        onView = { viewing = it },
                        actions = PhotoActions(
                            onSetMain = onSetMain,
                            onClearMain = onClearMain,
                            onCrop = { cropping = it },
                            onAdopt = onAdopt,
                            onSensitivity = { changingSensitivity = it },
                            onDelete = { pendingDelete = it }
                        )
                    )
                }
            }
        }
    }

    pendingDelete?.let { photo ->
        AlertDialog(
            onDismissRequest = { pendingDelete = null },
            title = { Text(stringResource(R.string.contact_images_delete_title)) },
            text = {
                Text(
                    stringResource(
                        if (photo.isOwn) {
                            R.string.contact_images_delete_confirm
                        } else {
                            R.string.contact_images_delete_source_confirm
                        }
                    )
                )
            },
            confirmButton = {
                TextButton(
                    onClick = {
                        onDelete(photo)
                        pendingDelete = null
                    }
                ) {
                    Text(stringResource(R.string.action_delete))
                }
            },
            dismissButton = {
                TextButton(onClick = { pendingDelete = null }) {
                    Text(stringResource(R.string.action_cancel))
                }
            }
        )
    }

    cropping?.let { photo ->
        ImageCropDialog(
            photo = photo,
            onSave = {
                onCrop(photo, it)
                cropping = null
            },
            onClear = {
                onCrop(photo, null)
                cropping = null
            },
            onDismiss = { cropping = null }
        )
    }

    changingSensitivity?.let { photo ->
        SensitivityDialog(
            photo = photo,
            onSelect = {
                onSetSensitivity(photo, it)
                changingSensitivity = null
            },
            onDismiss = { changingSensitivity = null }
        )
    }

    viewing?.let { photo ->
        Dialog(onDismissRequest = { viewing = null }) {
            AsyncImage(
                model = photo.fileUrl,
                contentDescription = stringResource(R.string.contact_images_alt),
                contentScale = ContentScale.Fit,
                modifier = Modifier.fillMaxWidth().padding(8.dp)
            )
        }
    }

    val message = uiState.error?.localizedMessage()
    if (message != null) {
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            currentOnErrorShown.value()
        }
    }
}

/** The per-photo write callbacks, bundled so the grid and tile signatures stay short. */
private class PhotoActions(
    val onSetMain: (ContactPhoto) -> Unit,
    val onClearMain: () -> Unit,
    val onCrop: (ContactPhoto) -> Unit,
    val onAdopt: (ContactPhoto) -> Unit,
    val onSensitivity: (ContactPhoto) -> Unit,
    val onDelete: (ContactPhoto) -> Unit
)

@Composable
private fun PhotoGrid(
    uiState: ContactGalleryUiState,
    onView: (ContactPhoto) -> Unit,
    actions: PhotoActions
) {
    LazyVerticalGrid(
        columns = GridCells.Fixed(3),
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(
            start = 16.dp,
            end = 16.dp,
            top = 16.dp,
            bottom = FabListBottomPadding
        ),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        items(uiState.photos, key = { it.key }) { photo ->
            PhotoTile(
                photo = photo,
                isMain = uiState.isMain(photo),
                busy = uiState.busyKey != null,
                onView = { onView(photo) },
                actions = actions
            )
        }
    }
}

@Composable
private fun PhotoTile(
    photo: ContactPhoto,
    isMain: Boolean,
    busy: Boolean,
    onView: () -> Unit,
    actions: PhotoActions
) {
    Box(
        modifier = Modifier
            .aspectRatio(1f)
            .clip(RoundedCornerShape(8.dp))
            .background(MaterialTheme.colorScheme.surfaceVariant)
            .clickable(onClick = onView)
            .testTag("contact-photo-${photo.key}")
    ) {
        AsyncImage(
            model = photo.thumbnailUrl,
            contentDescription = stringResource(R.string.contact_images_alt),
            contentScale = ContentScale.Crop,
            modifier = Modifier.fillMaxSize()
        )
        if (isMain) {
            Box(
                modifier = Modifier
                    .align(Alignment.TopStart)
                    .padding(4.dp)
                    .size(MAIN_BADGE_SIZE)
                    .testTag("main-badge-${photo.key}")
            ) {
                Icon(
                    Icons.Outlined.Star,
                    contentDescription = stringResource(R.string.contact_images_main_badge),
                    tint = MaterialTheme.colorScheme.primary,
                    modifier = Modifier.fillMaxSize()
                )
            }
        }
        PhotoMenu(
            photo = photo,
            isMain = isMain,
            enabled = !busy,
            actions = actions,
            modifier = Modifier.align(Alignment.TopEnd)
        )
        PhotoCaption(photo, Modifier.align(Alignment.BottomStart))
    }
}

@Composable
private fun PhotoMenu(
    photo: ContactPhoto,
    isMain: Boolean,
    enabled: Boolean,
    actions: PhotoActions,
    modifier: Modifier = Modifier
) {
    var expanded by remember { mutableStateOf(false) }
    Box(modifier = modifier) {
        IconButton(
            onClick = { expanded = true },
            enabled = enabled,
            modifier = Modifier.testTag("photo-menu-${photo.key}")
        ) {
            Icon(
                Icons.Outlined.MoreVert,
                contentDescription = stringResource(R.string.contact_images_more)
            )
        }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            if (photo.isOwn) {
                if (isMain) {
                    PhotoMenuItem(R.string.contact_images_clear_main) { actions.onClearMain() }
                } else if (photo.isNormalSensitivity) {
                    // Only a normal-sensitivity image can be the main image (the server rejects the rest).
                    PhotoMenuItem(R.string.contact_images_set_main) { actions.onSetMain(photo) }
                }
                PhotoMenuItem(R.string.contact_images_crop) { actions.onCrop(photo) }
                PhotoMenuItem(R.string.contact_images_sensitivity_change) {
                    actions.onSensitivity(photo)
                }
            } else {
                PhotoMenuItem(R.string.contact_images_adopt) { actions.onAdopt(photo) }
            }
            PhotoMenuItem(R.string.contact_images_delete) { actions.onDelete(photo) }
        }
    }
}

/** Pick normal, private or secret for an own image (ADR 0040 decision 7). */
@Composable
private fun SensitivityDialog(
    photo: ContactPhoto,
    onSelect: (String) -> Unit,
    onDismiss: () -> Unit
) {
    val current = if (photo.isNormalSensitivity) SENSITIVITY_NORMAL else photo.sensitivity
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.contact_images_sensitivity_title)) },
        text = {
            Column {
                Text(
                    stringResource(R.string.contact_images_sensitivity_main_note),
                    style = MaterialTheme.typography.bodySmall
                )
                SENSITIVITY_LEVELS.forEach { (value, label) ->
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        modifier = Modifier
                            .fillMaxWidth()
                            .selectable(
                                selected = current == value,
                                role = Role.RadioButton,
                                onClick = { onSelect(value) }
                            )
                            .testTag("sensitivity-$value")
                    ) {
                        RadioButton(selected = current == value, onClick = null)
                        Text(
                            stringResource(label),
                            modifier = Modifier.padding(start = 8.dp, top = 12.dp, bottom = 12.dp)
                        )
                    }
                }
            }
        },
        confirmButton = {},
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        }
    )
}

@Composable
private fun PhotoMenuItem(@androidx.annotation.StringRes label: Int, onClick: () -> Unit) {
    DropdownMenuItem(text = { Text(stringResource(label)) }, onClick = onClick)
}

/** Source label (for note/activity images) and a badge for non-normal sensitivity. */
@Composable
private fun PhotoCaption(photo: ContactPhoto, modifier: Modifier = Modifier) {
    val source = sourceCaption(photo.source)
    val sensitivity = when (photo.sensitivity) {
        "private" -> stringResource(R.string.contact_images_sensitivity_private)
        "secret" -> stringResource(R.string.contact_images_sensitivity_secret)
        else -> null
    }
    if (source == null && sensitivity == null) return
    Column(
        modifier = modifier
            .fillMaxWidth()
            .background(Color.Black.copy(alpha = CAPTION_SCRIM_ALPHA))
            .padding(horizontal = 4.dp, vertical = 2.dp)
    ) {
        source?.let { CaptionText(it) }
        sensitivity?.let { CaptionText(it) }
    }
}

@Composable
private fun CaptionText(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.labelSmall,
        color = Color.White,
        maxLines = 1
    )
}

@Composable
private fun sourceCaption(source: PhotoSource): String? {
    val kind = when (source.kind) {
        PhotoSource.KIND_NOTE -> stringResource(R.string.contact_images_source_note)
        PhotoSource.KIND_ACTIVITY -> stringResource(R.string.contact_images_source_activity)
        else -> return null
    }
    val label = source.label?.takeIf { it.isNotBlank() } ?: return kind
    return stringResource(R.string.contact_images_source_labeled, kind, label)
}

private const val SENSITIVITY_NORMAL = "normal"

/** Mirror of the backend `sensitivity` oneof (normal|private|secret); keep in sync by hand. */
private val SENSITIVITY_LEVELS = listOf(
    SENSITIVITY_NORMAL to R.string.contact_images_sensitivity_normal,
    "private" to R.string.contact_images_sensitivity_private,
    "secret" to R.string.contact_images_sensitivity_secret
)

private const val CAPTION_SCRIM_ALPHA = 0.55f
private val MAIN_BADGE_SIZE = 20.dp
