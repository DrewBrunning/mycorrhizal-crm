package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ActivitiesScreen(
    onBack: () -> Unit,
    onCreateActivity: () -> Unit,
    onEditActivity: (Int) -> Unit,
    viewModel: ActivitiesViewModel = viewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = "Back")
                    }
                },
                title = {
                    Text("Activities", style = MaterialTheme.typography.titleLarge)
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.surface,
                ),
            )
        },
        floatingActionButton = {
            FloatingActionButton(onClick = onCreateActivity) {
                Icon(Icons.Outlined.Add, contentDescription = "New activity")
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading -> LoadingSkeleton()
                state.activities.isEmpty() && state.error == null ->
                    EmptyState(message = "No activities yet")
                state.activities.isEmpty() && state.error != null ->
                    EmptyState(state.error.orEmpty())
                else -> {
                    LazyColumn(modifier = Modifier.fillMaxSize()) {
                        items(state.activities, key = { it.id }) { activity ->
                            ActivityListItem(
                                activity = activity,
                                onClick = { onEditActivity(activity.id) },
                            )
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
fun ActivityListItem(
    activity: Activity,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    androidx.compose.material3.ListItem(
        headlineContent = { Text(activity.title.orEmpty(), style = MaterialTheme.typography.bodyLarge) },
        supportingContent = {
            val parts = listOfNotNull(
                activity.type?.takeIf { it.isNotBlank() },
                activity.location?.takeIf { it.isNotBlank() },
            )
            Text(parts.joinToString(" · "), style = MaterialTheme.typography.bodyMedium)
        },
        modifier = modifier
            .fillMaxWidth()
            .clickable(onClick = onClick),
    )
}
