package com.mycorrhizal.crm.feature.tags

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Remove
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.feature.contacts.ContactAvatar
import com.mycorrhizal.crm.feature.contacts.memberDisplayName
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.RefreshableContent
import com.mycorrhizal.crm.ui.R

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TagDetailScreen(
    onBack: () -> Unit,
    onNavigateToContact: (Int) -> Unit = {},
    viewModel: TagDetailViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }
    var showAddDialog by remember { mutableStateOf(false) }
    val errorMessage = state.errorRes?.let { stringResource(it) } ?: state.error

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    AccessibleIconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(
                        text = state.tag?.name ?: stringResource(R.string.tags_title),
                        style = MaterialTheme.typography.titleLarge,
                    )
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
        floatingActionButton = {
            BrandFab(onClick = { showAddDialog = true }) {
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.tags_add_contact))
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            RefreshableContent(
                isRefreshing = state.isLoading && state.contacts.isNotEmpty(),
                onRefresh = viewModel::load,
                modifier = Modifier.fillMaxSize(),
            ) {
                when {
                    state.isLoading && state.contacts.isEmpty() -> LoadingSkeleton()
                    state.contacts.isEmpty() && errorMessage == null ->
                        EmptyState(
                            title = stringResource(R.string.tags_contacts_empty),
                            actionLabel = stringResource(R.string.tags_add_contact),
                            onAction = { showAddDialog = true },
                        )
                    state.contacts.isEmpty() && errorMessage != null -> {
                        Text(
                            text = errorMessage,
                            color = MaterialTheme.colorScheme.error,
                            modifier = Modifier.align(Alignment.Center),
                        )
                    }
                    else -> {
                        LazyColumn(modifier = Modifier.fillMaxSize()) {
                            items(state.contacts, key = { it.id }) { tagging ->
                                val resolved = state.contactsByUid[tagging.contactVCardUid]
                                TaggedContactRow(
                                    displayName = memberDisplayName(resolved)
                                        ?: stringResource(R.string.contact_unknown),
                                    photoUri = resolved?.photoThumbnail ?: resolved?.photo,
                                    resolvedContactId = resolved?.takeIf { it.id != 0 }?.id,
                                    onNavigateToContact = onNavigateToContact,
                                    removing = state.removingUid == tagging.contactVCardUid,
                                    onRemove = { viewModel.removeContact(tagging.contactVCardUid) },
                                )
                            }
                        }
                    }
                }
            }
        }
    }

    if (showAddDialog) {
        AddContactDialog(
            onConfirm = { uid ->
                viewModel.addContact(uid)
                showAddDialog = false
            },
            onDismiss = { showAddDialog = false },
        )
    }

    errorMessage?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
        }
    }
}

@Composable
private fun TaggedContactRow(
    displayName: String,
    photoUri: String?,
    resolvedContactId: Int?,
    onNavigateToContact: (Int) -> Unit,
    removing: Boolean,
    onRemove: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .then(if (resolvedContactId != null) Modifier.clickable { onNavigateToContact(resolvedContactId) } else Modifier)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        ContactAvatar(photoUri = photoUri, contentDescription = null, size = 40.dp)
        Text(
            text = displayName,
            style = MaterialTheme.typography.bodyLarge,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f).padding(horizontal = 8.dp),
        )
        AccessibleIconButton(onClick = onRemove, enabled = !removing) {
            Icon(
                imageVector = Icons.Outlined.Remove,
                // #205/#1768: the remove action carries the contact's name (never the
                // raw UID) so TalkBack doesn't read a bare "Remove tag from contact" on every row.
                contentDescription = stringResource(R.string.tags_remove_contact_named, displayName),
            )
        }
    }
}

@Composable
fun AddContactDialog(
    onConfirm: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    var uid by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.tags_add_contact)) },
        text = {
            OutlinedTextField(
                value = uid,
                onValueChange = { uid = it },
                label = { Text(stringResource(R.string.tags_contact_vcard_uid)) },
                singleLine = true,
            )
        },
        confirmButton = {
            TextButton(
                onClick = { onConfirm(uid) },
                enabled = uid.isNotBlank(),
            ) {
                Text(stringResource(R.string.action_add))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text(stringResource(R.string.action_cancel))
            }
        },
    )
}
