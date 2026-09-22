package com.mycorrhizal.crm.feature.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Switch
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
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.FIELD_TYPES
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import com.mycorrhizal.crm.ui.components.LoadingSkeleton

/**
 * Issue #830: the field-definition create/edit form — a full screen (not an `AlertDialog`, unlike
 * `TagNameDialog`) since this has ~10 fields, matching `ReminderFormScreen`'s established shape
 * for a form this size. Fields/validation/constraint-building are a direct port of web's
 * `FieldDefinitionDialog` (`frontend/src/components/FieldDefinitionDialog.tsx`).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FieldDefinitionFormScreen(
    onSaved: () -> Unit,
    onBack: () -> Unit,
    viewModel: FieldDefinitionFormViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val events by viewModel.events.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    LaunchedEffect(events) {
        if (events is FieldDefinitionFormEvent.Saved) {
            viewModel.onEventShown()
            onSaved()
        }
    }

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
                        text = if (state.isEdit) {
                            stringResource(R.string.settings_custom_fields_edit_title)
                        } else {
                            stringResource(R.string.settings_custom_fields_new_title)
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
        FieldDefinitionFormContent(
            state = state,
            onLabelChange = viewModel::onLabelChange,
            onKeyChange = viewModel::onKeyChange,
            onTypeChange = viewModel::onTypeChange,
            onMultiChange = viewModel::onMultiChange,
            onMinChange = viewModel::onMinChange,
            onMaxChange = viewModel::onMaxChange,
            onMaxLengthChange = viewModel::onMaxLengthChange,
            onPatternChange = viewModel::onPatternChange,
            onAddEnumValue = viewModel::addEnumValue,
            onUpdateEnumValue = viewModel::updateEnumValue,
            onRemoveEnumValue = viewModel::removeEnumValue,
            onProjectionModeChange = viewModel::onProjectionModeChange,
            onVcardNameChange = viewModel::onVcardNameChange,
            onSensitivityChange = viewModel::onSensitivityChange,
            onSave = viewModel::save,
            modifier = Modifier.padding(padding),
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

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FieldDefinitionFormContent(
    state: FieldDefinitionFormState,
    onLabelChange: (String) -> Unit,
    onKeyChange: (String) -> Unit,
    onTypeChange: (String) -> Unit,
    onMultiChange: (Boolean) -> Unit,
    onMinChange: (String) -> Unit,
    onMaxChange: (String) -> Unit,
    onMaxLengthChange: (String) -> Unit,
    onPatternChange: (String) -> Unit,
    onAddEnumValue: () -> Unit,
    onUpdateEnumValue: (Int, String) -> Unit,
    onRemoveEnumValue: (Int) -> Unit,
    onProjectionModeChange: (String) -> Unit,
    onVcardNameChange: (String) -> Unit,
    onSensitivityChange: (String) -> Unit,
    onSave: () -> Unit,
    modifier: Modifier = Modifier,
) {
    if (state.isLoading) {
        LoadingSkeleton(modifier = modifier.fillMaxSize())
        return
    }

    var typeExpanded by remember { mutableStateOf(false) }
    var projectionExpanded by remember { mutableStateOf(false) }
    var sensitivityExpanded by remember { mutableStateOf(false) }

    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        OutlinedTextField(
            value = state.label,
            onValueChange = onLabelChange,
            label = { Text(stringResource(R.string.settings_custom_fields_label)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = state.key,
            onValueChange = onKeyChange,
            enabled = !state.isEdit,
            label = { Text(stringResource(R.string.settings_custom_fields_key)) },
            supportingText = { Text(stringResource(R.string.settings_custom_fields_key_hint)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )

        ExposedDropdownMenuBox(expanded = typeExpanded, onExpandedChange = { typeExpanded = it }) {
            OutlinedTextField(
                value = stringResource(fieldTypeLabelRes(state.type)),
                onValueChange = {},
                readOnly = true,
                label = { Text(stringResource(R.string.settings_custom_fields_type)) },
                trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = typeExpanded) },
                modifier = Modifier.fillMaxWidth().menuAnchor(),
            )
            ExposedDropdownMenu(
                expanded = typeExpanded,
                onDismissRequest = { typeExpanded = false },
            ) {
                FIELD_TYPES.forEach { option ->
                    DropdownMenuItem(
                        text = { Text(stringResource(fieldTypeLabelRes(option))) },
                        onClick = { onTypeChange(option); typeExpanded = false },
                    )
                }
            }
        }

        ListItem(
            modifier = Modifier.toggleable(
                value = state.multi,
                onValueChange = onMultiChange,
                role = Role.Switch,
            ),
            headlineContent = { Text(stringResource(R.string.settings_custom_fields_multi)) },
            trailingContent = { Switch(checked = state.multi, onCheckedChange = null) },
        )

        when (state.type) {
            "string", "text" -> Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                OutlinedTextField(
                    value = state.maxLength,
                    onValueChange = onMaxLengthChange,
                    label = { Text(stringResource(R.string.settings_custom_fields_max_length)) },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                )
                OutlinedTextField(
                    value = state.pattern,
                    onValueChange = onPatternChange,
                    label = { Text(stringResource(R.string.settings_custom_fields_pattern)) },
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                )
            }
            "number" -> Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                OutlinedTextField(
                    value = state.min,
                    onValueChange = onMinChange,
                    label = { Text(stringResource(R.string.settings_custom_fields_min)) },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                )
                OutlinedTextField(
                    value = state.max,
                    onValueChange = onMaxChange,
                    label = { Text(stringResource(R.string.settings_custom_fields_max)) },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                )
            }
            "enum" -> Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(stringResource(R.string.settings_custom_fields_enum_values), style = MaterialTheme.typography.titleSmall)
                state.enumValues.forEachIndexed { index, value ->
                    Row(verticalAlignment = androidx.compose.ui.Alignment.CenterVertically) {
                        OutlinedTextField(
                            value = value,
                            onValueChange = { onUpdateEnumValue(index, it) },
                            singleLine = true,
                            modifier = Modifier.weight(1f),
                        )
                        AccessibleIconButton(onClick = { onRemoveEnumValue(index) }) {
                            Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.settings_custom_fields_remove_value))
                        }
                    }
                }
                TextButton(onClick = onAddEnumValue) {
                    Icon(Icons.Outlined.Add, contentDescription = null, modifier = Modifier.padding(end = 4.dp))
                    Text(stringResource(R.string.settings_custom_fields_add_value))
                }
            }
        }

        ExposedDropdownMenuBox(expanded = projectionExpanded, onExpandedChange = { projectionExpanded = it }) {
            OutlinedTextField(
                value = if (state.projectionMode == "vcard") {
                    stringResource(R.string.settings_custom_fields_projection_vcard)
                } else {
                    stringResource(R.string.settings_custom_fields_projection_internal)
                },
                onValueChange = {},
                readOnly = true,
                label = { Text(stringResource(R.string.settings_custom_fields_projection)) },
                trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = projectionExpanded) },
                modifier = Modifier.fillMaxWidth().menuAnchor(),
            )
            ExposedDropdownMenu(
                expanded = projectionExpanded,
                onDismissRequest = { projectionExpanded = false },
            ) {
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.settings_custom_fields_projection_internal)) },
                    onClick = { onProjectionModeChange("internal"); projectionExpanded = false },
                )
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.settings_custom_fields_projection_vcard)) },
                    onClick = { onProjectionModeChange("vcard"); projectionExpanded = false },
                )
            }
        }
        if (state.projectionMode == "vcard") {
            OutlinedTextField(
                value = state.vcardName,
                onValueChange = onVcardNameChange,
                label = { Text(stringResource(R.string.settings_custom_fields_vcard_name)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
        }

        ExposedDropdownMenuBox(expanded = sensitivityExpanded, onExpandedChange = { sensitivityExpanded = it }) {
            OutlinedTextField(
                value = stringResource(sensitivityLabelRes(state.sensitivity)),
                onValueChange = {},
                readOnly = true,
                label = { Text(stringResource(R.string.settings_custom_fields_sensitivity)) },
                trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = sensitivityExpanded) },
                modifier = Modifier.fillMaxWidth().menuAnchor(),
            )
            ExposedDropdownMenu(
                expanded = sensitivityExpanded,
                onDismissRequest = { sensitivityExpanded = false },
            ) {
                listOf("normal", "private", "secret").forEach { option ->
                    DropdownMenuItem(
                        text = { Text(stringResource(sensitivityLabelRes(option))) },
                        onClick = { onSensitivityChange(option); sensitivityExpanded = false },
                    )
                }
            }
        }

        Button(
            onClick = onSave,
            enabled = !state.isSaving,
            modifier = Modifier.fillMaxWidth(),
        ) {
            if (state.isSaving) {
                CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp))
            }
            Text(
                if (state.isEdit) stringResource(R.string.action_save) else stringResource(R.string.action_create),
            )
        }
    }
}
