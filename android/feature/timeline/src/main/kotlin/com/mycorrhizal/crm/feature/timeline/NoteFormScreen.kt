package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
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
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.R

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NoteFormScreen(
    onSaved: () -> Unit,
    onBack: () -> Unit,
    viewModel: NoteFormViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val events by viewModel.events.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    LaunchedEffect(events) {
        if (events is NoteFormEvent.Saved) {
            viewModel.onSaveShown()
            onSaved()
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(
                        text = if (state.isEdit) {
                            stringResource(R.string.note_edit)
                        } else {
                            stringResource(R.string.note_new)
                        },
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
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        when {
            state.isLoading -> LoadingSkeleton(modifier = Modifier.padding(padding))
            else -> NoteFormContent(
                state = state,
                onContentChange = viewModel::onContentChange,
                onDateChange = viewModel::onDateChange,
                onContactSearchChange = viewModel::searchContacts,
                onPickContact = viewModel::selectContact,
                onClearContact = viewModel::clearContact,
                onSave = viewModel::save,
                modifier = Modifier.padding(padding),
            )
        }
    }

    val errorMessage = state.errorRes?.let { stringResource(it) } ?: state.error
    errorMessage?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
        }
    }
}

@Composable
fun NoteFormContent(
    state: NoteFormState,
    onContentChange: (String) -> Unit,
    onDateChange: (String) -> Unit,
    onContactSearchChange: (String) -> Unit,
    onPickContact: (com.mycorrhizal.crm.model.network.ContactSummary) -> Unit,
    onClearContact: () -> Unit,
    onSave: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        OutlinedTextField(
            value = state.content,
            onValueChange = onContentChange,
            label = { Text(stringResource(R.string.note_content)) },
            minLines = 6,
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = state.date,
            onValueChange = onDateChange,
            label = { Text(stringResource(R.string.activity_date)) },
            placeholder = { Text(stringResource(R.string.activity_date_hint)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )

        // M19: contact reassignment (web's EditTimelineItemDialog) — the
        // currently assigned contact as a removable chip, plus a debounced
        // search to move the note to any contact (or clear it back to unfiled).
        HorizontalDivider()
        Text(
            text = stringResource(R.string.note_assign_contact),
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        if (state.targetContactName != null) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    text = state.targetContactName,
                    style = MaterialTheme.typography.bodyLarge,
                    modifier = Modifier.weight(1f),
                )
                TextButton(onClick = onClearContact) {
                    Text(stringResource(R.string.note_clear_contact))
                }
            }
        }
        ContactSearchField(
            query = state.contactSearchQuery,
            results = state.contactSearchResults,
            loading = state.contactSearchLoading,
            onQueryChange = onContactSearchChange,
            onPick = onPickContact,
            labelRes = R.string.note_search_contact,
        )

        val savingLabel = stringResource(R.string.a11y_state_saving)
        Button(
            onClick = onSave,
            enabled = !state.isSaving,
            modifier = Modifier
                .fillMaxWidth()
                .semantics { if (state.isSaving) stateDescription = savingLabel },
        ) {
            if (state.isSaving) {
                CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp))
            }
            Text(if (state.isEdit) stringResource(R.string.note_save) else stringResource(R.string.note_create))
        }
    }
}
