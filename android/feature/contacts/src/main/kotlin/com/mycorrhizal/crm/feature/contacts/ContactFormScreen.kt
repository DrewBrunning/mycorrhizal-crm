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
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalFonts

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContactFormScreen(
    onSaved: () -> Unit,
    onBack: () -> Unit,
    viewModel: ContactFormViewModel = hiltViewModel(),
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
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(
                        text = if (state.isEdit) {
                            stringResource(R.string.contact_edit_title)
                        } else {
                            stringResource(R.string.contact_new)
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
            state.isLoading -> LoadingSkeleton()
            else -> ContactFormContent(
                modifier = Modifier.padding(padding),
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

    val errorMessage = state.errorRes?.let { stringResource(it) } ?: state.error
    errorMessage?.let { message ->
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
    modifier: Modifier = Modifier,
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        SectionLabel(stringResource(R.string.contact_name_section))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedTextField(
                value = state.givenName,
                onValueChange = onGivenNameChange,
                label = { Text(stringResource(R.string.contact_given_name)) },
                singleLine = true,
                modifier = Modifier.weight(1f),
            )
            OutlinedTextField(
                value = state.surname,
                onValueChange = onSurnameChange,
                label = { Text(stringResource(R.string.contact_surname)) },
                singleLine = true,
                modifier = Modifier.weight(1f),
            )
        }
        OutlinedTextField(
            value = state.nickname,
            onValueChange = onNicknameChange,
            label = { Text(stringResource(R.string.contact_nickname)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )

        SectionLabel(stringResource(R.string.contact_email))
        StringListEditor(
            values = state.emails,
            onValuesChange = onEmailsChange,
            placeholder = "email@example.com",
            keyboardType = KeyboardType.Email,
        )

        SectionLabel(stringResource(R.string.contact_phone))
        StringListEditor(
            values = state.phones,
            onValuesChange = onPhonesChange,
            placeholder = "+1 555 0100",
            keyboardType = KeyboardType.Phone,
        )

        OutlinedTextField(
            value = state.birthday,
            onValueChange = onBirthdayChange,
            label = { Text(stringResource(R.string.contact_birthday)) },
            placeholder = { Text(stringResource(R.string.contact_birthday_hint)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )

        OutlinedTextField(
            value = state.notes,
            onValueChange = onNotesChange,
            label = { Text(stringResource(R.string.nav_notes)) },
            minLines = 3,
            modifier = Modifier.fillMaxWidth(),
        )

        OutlinedTextField(
            value = state.circlesText,
            onValueChange = onCirclesTextChange,
            label = { Text(stringResource(R.string.contact_circles_label)) },
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
            Text(
                if (state.isEdit) {
                    stringResource(R.string.contact_save)
                } else {
                    stringResource(R.string.contact_create)
                },
            )
        }
    }
}

@Composable
private fun SectionLabel(text: String) {
    // T63 Android port: see ContactDetailScreen.kt's SectionCard comment —
    // same field-group-caption-gets-Mono treatment, scoped here rather than
    // through the shared labelLarge role.
    Text(
        text = text,
        style = MaterialTheme.typography.labelLarge.copy(fontFamily = MycorrhizalFonts.mono),
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
                    label = { Text(stringResource(R.string.contact_value_n, index + 1)) },
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
                    Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.contact_remove))
                }
            }
        }
        IconButton(onClick = { onValuesChange(values + "") }) {
            Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.contact_add))
        }
    }
}
