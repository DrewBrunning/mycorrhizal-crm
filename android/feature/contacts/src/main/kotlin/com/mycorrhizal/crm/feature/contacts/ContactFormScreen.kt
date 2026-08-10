package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
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
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mycorrhizal.crm.ui.components.LoadingSkeleton

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContactFormScreen(
    onSaved: () -> Unit,
    onBack: () -> Unit,
    viewModel: ContactFormViewModel = viewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val events by viewModel.events.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    LaunchedEffect(events) {
        if (events is ContactFormEvent.Saved) {
            viewModel.onSaveShown()
            onSaved()
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = "Back")
                    }
                },
                title = {
                    Text(
                        text = if (state.isEdit) "Edit contact" else "New contact",
                        style = MaterialTheme.typography.titleLarge,
                    )
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.surface,
                ),
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        when {
            state.isLoading -> LoadingSkeleton()
            else -> ContactFormContent(
                state = state,
                onGivenNameChange = viewModel::onGivenNameChange,
                onSurnameChange = viewModel::onSurnameChange,
                onNicknameChange = viewModel::onNicknameChange,
                onEmailsChange = viewModel::onEmailsChange,
                onPhonesChange = viewModel::onPhonesChange,
                onBirthdayChange = viewModel::onBirthdayChange,
                onNotesChange = viewModel::onNotesChange,
                onCirclesTextChange = viewModel::onCirclesTextChange,
                onSave = viewModel::save,
            )
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
fun ContactFormContent(
    state: ContactFormState,
    onGivenNameChange: (String) -> Unit,
    onSurnameChange: (String) -> Unit,
    onNicknameChange: (String) -> Unit,
    onEmailsChange: (List<String>) -> Unit,
    onPhonesChange: (List<String>) -> Unit,
    onBirthdayChange: (String) -> Unit,
    onNotesChange: (String) -> Unit,
    onCirclesTextChange: (String) -> Unit,
    onSave: () -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        SectionLabel("Name")
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedTextField(
                value = state.givenName,
                onValueChange = onGivenNameChange,
                label = { Text("Given name") },
                singleLine = true,
                modifier = Modifier.weight(1f),
            )
            OutlinedTextField(
                value = state.surname,
                onValueChange = onSurnameChange,
                label = { Text("Surname") },
                singleLine = true,
                modifier = Modifier.weight(1f),
            )
        }
        OutlinedTextField(
            value = state.nickname,
            onValueChange = onNicknameChange,
            label = { Text("Nickname") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )

        SectionLabel("Email")
        StringListEditor(
            values = state.emails,
            onValuesChange = onEmailsChange,
            placeholder = "email@example.com",
            keyboardType = KeyboardType.Email,
        )

        SectionLabel("Phone")
        StringListEditor(
            values = state.phones,
            onValuesChange = onPhonesChange,
            placeholder = "+1 555 0100",
            keyboardType = KeyboardType.Phone,
        )

        OutlinedTextField(
            value = state.birthday,
            onValueChange = onBirthdayChange,
            label = { Text("Birthday") },
            placeholder = { Text("1990-06-15 or --12-25") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )

        OutlinedTextField(
            value = state.notes,
            onValueChange = onNotesChange,
            label = { Text("Notes") },
            minLines = 3,
            modifier = Modifier.fillMaxWidth(),
        )

        OutlinedTextField(
            value = state.circlesText,
            onValueChange = onCirclesTextChange,
            label = { Text("Circles (comma-separated)") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )

        Button(
            onClick = onSave,
            enabled = !state.isSaving,
            modifier = Modifier.fillMaxWidth(),
        ) {
            if (state.isSaving) {
                CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp))
            }
            Text(if (state.isEdit) "Save changes" else "Create contact")
        }
    }
}

@Composable
private fun SectionLabel(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.labelLarge,
        color = MaterialTheme.colorScheme.primary,
        modifier = Modifier.padding(top = 8.dp),
    )
}

/** Editable list of strings with add/remove buttons (MultiValueField equivalent). */
@Composable
private fun StringListEditor(
    values: List<String>,
    onValuesChange: (List<String>) -> Unit,
    placeholder: String,
    keyboardType: KeyboardType,
) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        values.forEachIndexed { index, value ->
            Row(
                horizontalArrangement = Arrangement.spacedBy(4.dp),
                verticalAlignment = androidx.compose.ui.Alignment.CenterVertically,
            ) {
                OutlinedTextField(
                    value = value,
                    onValueChange = { updated ->
                        onValuesChange(values.toMutableList().apply { this[index] = updated })
                    },
                    label = { Text("Value ${index + 1}") },
                    placeholder = { Text(placeholder) },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = keyboardType),
                    modifier = Modifier.weight(1f),
                )
                IconButton(onClick = {
                    if (values.size > 1) {
                        onValuesChange(values.toMutableList().apply { removeAt(index) })
                    }
                }) {
                    Icon(Icons.Outlined.Delete, contentDescription = "Remove")
                }
            }
        }
        IconButton(onClick = { onValuesChange(values + "") }) {
            Icon(Icons.Outlined.Add, contentDescription = "Add")
        }
    }
}
