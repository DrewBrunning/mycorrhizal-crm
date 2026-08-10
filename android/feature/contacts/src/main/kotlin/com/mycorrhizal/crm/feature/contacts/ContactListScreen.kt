package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Person
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import coil3.compose.AsyncImage
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.theme.MycorrhizalTypography

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContactListScreen(
    onContactClick: (Int) -> Unit,
    viewModel: ContactListViewModel = viewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = rememberSaveable { SnackbarHostState() }

    LaunchedEffect(Unit) {
        viewModel.events.collect { event ->
            when (event) {
                is ContactListEvent.NavigateToContact -> onContactClick(event.contactId)
                ContactListEvent.ForceLogout -> Unit // handled by the app-level nav
            }
        }
    }

    var search by rememberSaveable { mutableStateOf(state.searchQuery) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Text("Contacts", style = MycorrhizalTypography.appBarTitle)
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.surface,
                ),
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding),
        ) {
            OutlinedTextField(
                value = search,
                onValueChange = {
                    search = it
                    viewModel.onSearchQueryChange(it)
                },
                label = { Text("Search contacts") },
                singleLine = true,
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp, vertical = 8.dp),
            )

            when {
                state.isLoading -> LoadingSkeleton()
                state.contacts.isEmpty() && state.error == null ->
                    EmptyState(message = "No contacts yet")
                state.contacts.isEmpty() && state.error != null -> {
                    Box(modifier = Modifier.fillMaxWidth().padding(24.dp)) {
                        Text(
                            text = state.error.orEmpty(),
                            color = MaterialTheme.colorScheme.error,
                            modifier = Modifier.align(Alignment.Center),
                        )
                    }
                }
                else -> {
                    LazyColumn(modifier = Modifier.fillMaxSize()) {
                        items(state.contacts, key = { it.id }) { contact ->
                            ContactListItem(
                                contact = contact,
                                onClick = { viewModel.onContactClick(contact.id) },
                            )
                        }
                        if (state.pagination.isLoadingMore) {
                            item {
                                Box(modifier = Modifier.fillMaxWidth().padding(16.dp)) {
                                    CircularProgressIndicator(
                                        modifier = Modifier.size(32.dp).align(Alignment.Center),
                                    )
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    state.error?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
        }
    }
}

@Composable
fun ContactListItem(
    contact: ContactSummary,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Row(
        modifier = modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        val thumbnail = contact.photoThumbnail
        if (thumbnail != null && thumbnail.startsWith("data:")) {
            AsyncImage(
                model = thumbnail,
                contentDescription = "Photo of ${contact.displayName}",
                contentScale = ContentScale.Crop,
                modifier = Modifier.size(40.dp).clip(CircleShape),
            )
        } else {
            Icon(
                imageVector = Icons.Outlined.Person,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.size(40.dp),
            )
        }
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = contact.displayName,
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            val subtitle = listOfNotNull(contact.primaryEmail, contact.primaryPhone)
                .joinToString(" · ")
            if (subtitle.isNotBlank()) {
                Text(
                    text = subtitle,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
    }
}
