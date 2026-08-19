package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
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
import coil3.compose.AsyncImage
import com.mycorrhizal.crm.model.network.ImmichAssetSummary
import com.mycorrhizal.crm.model.network.ImmichPerson
import com.mycorrhizal.crm.ui.LocalServerUrl
import com.mycorrhizal.crm.ui.R

/**
 * Issue #219/#220: the "choose from Immich" profile-photo picker, mirroring
 * web's ImmichPhotoPickerDialog. Two phases, never both at once:
 *  - unlinked → person search (client-side name filter over the fetched
 *    people, exactly like web — Immich has no stable name-search endpoint);
 *    selecting a person links it AND flips to the browse phase.
 *  - linked → the person's primary thumbnail plus a grid of recent photos;
 *    tapping one hands its id (null = the primary photo) to [onPick], which
 *    feeds the crop+upload flow.
 */
@Composable
fun ImmichPhotoPickerDialog(
    contactUid: String,
    isLinked: Boolean,
    people: List<ImmichPerson>,
    peopleLoading: Boolean,
    assets: List<ImmichAssetSummary>,
    assetsLoading: Boolean,
    onLoadPeople: () -> Unit,
    onLinkPerson: (ImmichPerson) -> Unit,
    onLoadAssets: () -> Unit,
    onPick: (String?) -> Unit,
    onDismiss: () -> Unit,
) {
    // Optimistically flip to the browse phase when a person is picked; the
    // `isLinked` prop (from the refreshed ExternalIdentity list) re-syncs it.
    var linked by remember { mutableStateOf(isLinked) }
    LaunchedEffect(isLinked) { linked = isLinked }
    LaunchedEffect(Unit) {
        if (linked) onLoadAssets() else onLoadPeople()
    }

    if (!linked) {
        ImmichPersonSearchStep(
            people = people,
            loading = peopleLoading,
            onLink = { person ->
                linked = true
                onLinkPerson(person)
            },
            onDismiss = onDismiss,
        )
    } else {
        ImmichAssetGridStep(
            contactUid = contactUid,
            assets = assets,
            loading = assetsLoading,
            onPick = onPick,
            onDismiss = onDismiss,
        )
    }
}

@Composable
private fun ImmichPersonSearchStep(
    people: List<ImmichPerson>,
    loading: Boolean,
    onLink: (ImmichPerson) -> Unit,
    onDismiss: () -> Unit,
) {
    var query by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.immich_person_search_title)) },
        text = {
            Column(modifier = Modifier.fillMaxWidth()) {
                OutlinedTextField(
                    value = query,
                    onValueChange = { query = it },
                    label = { Text(stringResource(R.string.immich_person_search_placeholder)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                Spacer(Modifier.height(8.dp))
                when {
                    loading -> Box(
                        modifier = Modifier.fillMaxWidth().padding(24.dp),
                        contentAlignment = Alignment.Center,
                    ) {
                        CircularProgressIndicator()
                    }
                    else -> {
                        val filtered = people.filter {
                            query.isBlank() || it.name.contains(query.trim(), ignoreCase = true)
                        }
                        if (filtered.isEmpty()) {
                            Text(
                                text = stringResource(R.string.immich_person_search_no_matches),
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                modifier = Modifier.padding(vertical = 8.dp),
                            )
                        } else {
                            LazyColumn(modifier = Modifier.heightIn(max = 320.dp)) {
                                items(filtered, key = { it.id }) { person ->
                                    Text(
                                        text = person.name.ifBlank { stringResource(R.string.immich_person_search_unnamed) },
                                        style = MaterialTheme.typography.bodyLarge,
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .clickable { onLink(person) }
                                            .padding(vertical = 10.dp),
                                    )
                                }
                            }
                        }
                    }
                }
            }
        },
        confirmButton = {},
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun ImmichAssetGridStep(
    contactUid: String,
    assets: List<ImmichAssetSummary>,
    loading: Boolean,
    onPick: (String?) -> Unit,
    onDismiss: () -> Unit,
) {
    val serverOrigin = LocalServerUrl.current
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.immich_photo_picker_title)) },
        text = {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(max = 400.dp)
                    .verticalScroll(rememberScrollState()),
            ) {
                when {
                    loading -> Box(
                        modifier = Modifier.fillMaxWidth().padding(24.dp),
                        contentAlignment = Alignment.Center,
                    ) {
                        CircularProgressIndicator()
                    }
                    assets.isEmpty() -> Text(
                        text = stringResource(R.string.immich_photo_picker_no_photos),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(vertical = 8.dp),
                    )
                    else -> {
                        // The linked person's own photo, always first.
                        ImmichPhotoCell(
                            url = resolvePhotoUri("/api/v1/immich/contacts/$contactUid/thumbnail", serverOrigin),
                            contentDescription = stringResource(R.string.immich_photo_picker_primary_desc),
                            onClick = { onPick(null) },
                        )
                        Spacer(Modifier.height(8.dp))
                        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            assets.forEach { asset ->
                                ImmichPhotoCell(
                                    url = resolvePhotoUri("/api/v1/immich/contacts/$contactUid/assets/${asset.id}/image", serverOrigin),
                                    contentDescription = stringResource(R.string.immich_photo_picker_photo_desc),
                                    onClick = { onPick(asset.id) },
                                )
                            }
                        }
                    }
                }
            }
        },
        confirmButton = {},
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

@Composable
private fun ImmichPhotoCell(
    url: String?,
    contentDescription: String,
    onClick: () -> Unit,
) {
    AsyncImage(
        model = url,
        contentDescription = contentDescription,
        contentScale = ContentScale.Crop,
        modifier = Modifier
            .size(96.dp)
            .clip(RoundedCornerShape(8.dp))
            .background(MaterialTheme.colorScheme.surfaceVariant)
            .clickable(onClick = onClick),
    )
}
