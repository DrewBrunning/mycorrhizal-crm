package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SnackbarHostState
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
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.util.DateFieldMode
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.DateField
import com.mycorrhizal.crm.ui.components.FormScaffold
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.WriteConflictDialog

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NoteFormScreen(
    onSaved: () -> Unit,
    onBack: () -> Unit,
    viewModel: NoteFormViewModel = hiltViewModel()
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

    FormScaffold(
        title = if (state.isEdit) {
            stringResource(
                R.string.note_edit
            )
        } else {
            stringResource(R.string.note_new)
        },
        onBack = onBack,
        saveLabel = if (state.isEdit) {
            stringResource(
                R.string.note_save
            )
        } else {
            stringResource(R.string.note_create)
        },
        isSaving = state.isSaving,
        onSave = viewModel::save,
        snackbarHostState = snackbarHostState,
        isDirty = state.isDirty,
        entityLabel = stringResource(R.string.entity_note),
        showSaveBar = !state.isLoading
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
                modifier = Modifier.padding(padding)
            )
        }
    }

    if (state.conflict) {
        WriteConflictDialog(
            onLoadLatest = viewModel::onConflictLoadLatest,
            onKeep = viewModel::onConflictKeepEditing
        )
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
    modifier: Modifier = Modifier
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        OutlinedTextField(
            value = state.content,
            onValueChange = onContentChange,
            label = { Text(stringResource(R.string.note_content)) },
            minLines = 6,
            modifier = Modifier.fillMaxWidth()
        )
        DateField(
            value = state.date,
            onValueChange = onDateChange,
            label = stringResource(R.string.note_date),
            mode = DateFieldMode.DATE_TIME,
            testTag = "note-date",
            modifier = Modifier.fillMaxWidth()
        )

        // M19: contact reassignment (web's EditTimelineItemDialog) — the
        // currently assigned contact as a removable chip, plus a debounced
        // search to move the note to any contact (or clear it back to unfiled).
        HorizontalDivider()
        Text(
            text = stringResource(R.string.note_assign_contact),
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
        // Issue #1773: with a contact already assigned, the search stays hidden until
        // "Change" is tapped -- an empty "Search contacts" box under the assigned
        // contact read as a second, unfilled field.
        var changing by remember(state.targetContactName != null) { mutableStateOf(false) }
        if (state.targetContactName != null) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    text = state.targetContactName,
                    style = MaterialTheme.typography.bodyLarge,
                    modifier = Modifier.weight(1f)
                )
                if (!changing) {
                    TextButton(onClick = { changing = true }) {
                        Text(stringResource(R.string.note_change_contact))
                    }
                }
                TextButton(onClick = onClearContact) {
                    Text(stringResource(R.string.note_clear_contact))
                }
            }
        }
        if (state.targetContactName == null || changing) {
            ContactSearchField(
                query = state.contactSearchQuery,
                results = state.contactSearchResults,
                loading = state.contactSearchLoading,
                onQueryChange = onContactSearchChange,
                onPick = {
                    changing = false
                    onPickContact(it)
                },
                labelRes = R.string.note_search_contact
            )
        }
    }
}
