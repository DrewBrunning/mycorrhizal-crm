package com.mycorrhizal.crm.feature.contacts

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.PhotoLibrary
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
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
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import coil3.compose.AsyncImage
import com.mycorrhizal.crm.model.network.ContactImage
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.FabListBottomPadding
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.RefreshableContent
import com.mycorrhizal.crm.ui.components.localizedMessage

/**
 * ADR 0040 slice c: the contact photo gallery (web Photos section parity).
 * Thumbnails come from the owner-nested endpoint and open a lightbox; the
 * upload FAB uses the system photo picker and each upload is re-encoded by the
 * server before it appears in the grid.
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
        onRefresh = viewModel::load,
        onErrorShown = viewModel::onErrorShown
    )
}

/**
 * Stateless gallery content (house pattern): loading / empty / populated, the
 * delete confirmation and the lightbox. The system picker stays in
 * [ContactGalleryScreen] so this surface is directly testable.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContactGalleryScreenContent(
    uiState: ContactGalleryUiState,
    modifier: Modifier = Modifier,
    onBack: () -> Unit = {},
    onAddClick: () -> Unit = {},
    onDelete: (ContactImage) -> Unit = {},
    onRefresh: () -> Unit = {},
    onErrorShown: () -> Unit = {}
) {
    val snackbarHostState = remember { SnackbarHostState() }
    var pendingDelete by remember { mutableStateOf<ContactImage?>(null) }
    var viewing by remember { mutableStateOf<ContactImage?>(null) }
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
                isRefreshing = uiState.isLoading && uiState.images.isNotEmpty(),
                onRefresh = onRefresh,
                modifier = Modifier.fillMaxSize()
            ) {
                when {
                    uiState.isLoading && uiState.images.isEmpty() -> LoadingSkeleton()

                    uiState.images.isEmpty() -> EmptyState(
                        title = stringResource(R.string.contact_images_empty),
                        body = stringResource(R.string.contact_images_empty_body),
                        icon = Icons.Outlined.PhotoLibrary,
                        actionLabel = stringResource(R.string.contact_images_add),
                        onAction = onAddClick,
                        modifier = Modifier.testTag("contact-gallery-empty")
                    )

                    else -> LazyVerticalGrid(
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
                        items(uiState.images, key = { it.id }) { image ->
                            Box(
                                modifier = Modifier
                                    .aspectRatio(1f)
                                    .clip(RoundedCornerShape(8.dp))
                                    .background(MaterialTheme.colorScheme.surfaceVariant)
                                    .clickable { viewing = image }
                                    .testTag("contact-image-${image.id}")
                            ) {
                                AsyncImage(
                                    model = image.thumbnailUrl,
                                    contentDescription = stringResource(
                                        R.string.contact_images_alt
                                    ),
                                    contentScale = ContentScale.Crop,
                                    modifier = Modifier.fillMaxSize()
                                )
                                IconButton(
                                    onClick = { pendingDelete = image },
                                    enabled = uiState.deletingId != image.id,
                                    modifier = Modifier
                                        .align(Alignment.TopEnd)
                                        .testTag("delete-contact-image-${image.id}")
                                ) {
                                    Icon(
                                        Icons.Outlined.Delete,
                                        contentDescription = stringResource(
                                            R.string.contact_images_delete
                                        )
                                    )
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    pendingDelete?.let { image ->
        AlertDialog(
            onDismissRequest = { pendingDelete = null },
            title = { Text(stringResource(R.string.contact_images_delete_title)) },
            text = { Text(stringResource(R.string.contact_images_delete_confirm)) },
            confirmButton = {
                TextButton(
                    onClick = {
                        onDelete(image)
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

    viewing?.let { image ->
        Dialog(onDismissRequest = { viewing = null }) {
            AsyncImage(
                model = image.fileUrl,
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
