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
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.R

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ActivitiesScreen(
    onBack: () -> Unit,
    onCreateActivity: () -> Unit,
    onEditActivity: (Int) -> Unit,
    viewModel: ActivitiesViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(stringResource(R.string.nav_activities), style = MaterialTheme.typography.titleLarge)
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.surface,
                ),
            )
        },
        floatingActionButton = {
            FloatingActionButton(onClick = onCreateActivity) {
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.cd_new_activity))
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading -> LoadingSkeleton()
                state.activities.isEmpty() && state.error == null ->
                    EmptyState(message = "No activities yet")
                state.activities.isEmpty() && (state.errorRes != null || state.error != null) ->
                    EmptyState(state.errorRes?.let { stringResource(it) } ?: state.error.orEmpty())
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

    // When the list is empty the error text is the persistent body content
    // (EmptyState above), so don't toast-and-clear it into a misleading
    // "No activities yet". Only surface a snackbar for errors over a populated list.
    val listError = state.error
    if (listError != null && state.activities.isNotEmpty()) {
        LaunchedEffect(listError) {
            snackbarHostState.showSnackbar(listError)
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
