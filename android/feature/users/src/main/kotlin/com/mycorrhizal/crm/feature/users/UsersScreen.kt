package com.mycorrhizal.crm.feature.users

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.Person
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
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
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.AdminUser
import com.mycorrhizal.crm.model.network.AdminUserCreateInput
import com.mycorrhizal.crm.model.network.AdminUserUpdateInput
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * Admin user management (issue #348): list all users, create/edit/delete them,
 * mirroring web's UsersPage.tsx over the same five admin-group routes. The
 * screen is only reachable for admins (Settings gates the entry point on
 * SessionState.isAdmin), and the backend enforces that on every call anyway.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun UsersScreen(
    onBack: () -> Unit,
    viewModel: UsersViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }
    var editorOpen by remember { mutableStateOf(false) }
    var editingUser by remember { mutableStateOf<AdminUser?>(null) }
    var deletingUser by remember { mutableStateOf<AdminUser?>(null) }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    AccessibleIconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(stringResource(R.string.users_title), style = MaterialTheme.typography.titleLarge)
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
            BrandFab(onClick = {
                editingUser = null
                editorOpen = true
            }) {
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.users_add))
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading && state.users.isEmpty() -> {
                    CircularProgressIndicator(modifier = Modifier.align(Alignment.Center))
                }
                state.users.isEmpty() ->
                    EmptyState(message = stringResource(R.string.users_empty))
                else -> {
                    LazyColumn(modifier = Modifier.fillMaxSize()) {
                        items(state.users, key = { it.id }) { user ->
                            UserRow(
                                user = user,
                                onEdit = {
                                    editingUser = user
                                    editorOpen = true
                                },
                                onDelete = { deletingUser = user },
                            )
                        }
                    }
                }
            }
        }
    }

    // Errors that surface while no dialog is open (e.g. a failed load, or a
    // failed delete whose dialog already closed) land in the snackbar. Errors
    // inside an open dialog are rendered inline in the dialog itself.
    state.error?.let { message ->
        if (!editorOpen && deletingUser == null) {
            LaunchedEffect(message) {
                snackbarHostState.showSnackbar(message)
                viewModel.onErrorShown()
            }
        }
    }

    if (editorOpen) {
        UserEditorDialog(
            initial = editingUser,
            isSaving = state.isSaving,
            error = state.error,
            onConfirm = { username, email, password, isAdmin ->
                val current = editingUser
                if (current == null) {
                    viewModel.create(
                        AdminUserCreateInput(
                            username = username,
                            email = email,
                            password = password,
                            isAdmin = isAdmin,
                        ),
                        onDone = { editorOpen = false },
                    )
                } else {
                    // Password is optional on edit; blank means "keep current".
                    viewModel.update(
                        current.id,
                        AdminUserUpdateInput(
                            username = username,
                            email = email,
                            password = password.takeIf { it.isNotBlank() },
                            isAdmin = isAdmin,
                        ),
                        onDone = { editorOpen = false },
                    )
                }
            },
            onDismiss = {
                editorOpen = false
                if (state.error != null) viewModel.onErrorShown()
            },
        )
    }

    deletingUser?.let { user ->
        AlertDialog(
            onDismissRequest = {
                deletingUser = null
                if (state.error != null) viewModel.onErrorShown()
            },
            title = { Text(stringResource(R.string.users_delete_title)) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(stringResource(R.string.users_delete_body, user.username.orEmpty()))
                    state.error?.let { message ->
                        Text(
                            text = message,
                            color = MaterialTheme.colorScheme.error,
                            style = MaterialTheme.typography.bodySmall,
                            modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive },
                        )
                    }
                }
            },
            confirmButton = {
                val deletingLabel = stringResource(R.string.a11y_state_saving)
                TextButton(
                    onClick = {
                        viewModel.delete(user.id)
                        deletingUser = null
                    },
                    enabled = state.deletingId != user.id,
                    modifier = Modifier.semantics { if (state.deletingId == user.id) stateDescription = deletingLabel },
                ) {
                    if (state.deletingId == user.id) {
                        CircularProgressIndicator(modifier = Modifier.padding(end = 4.dp), strokeWidth = 2.dp)
                    }
                    Text(stringResource(R.string.action_delete))
                }
            },
            dismissButton = {
                TextButton(onClick = {
                    deletingUser = null
                    if (state.error != null) viewModel.onErrorShown()
                }) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }
}

@Composable
private fun UserRow(
    user: AdminUser,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Icon(
            imageVector = Icons.Outlined.Person,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = user.username.orEmpty(),
                style = MaterialTheme.typography.bodyLarge,
            )
            Text(
                text = user.email.orEmpty(),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    text = stringResource(if (user.isAdmin) R.string.users_role_admin else R.string.users_role_user),
                    style = MaterialTheme.typography.labelSmall,
                    color = if (user.isAdmin) MaterialTheme.colorScheme.tertiary else MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text(
                    text = formatCreatedDate(user.createdAt),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        AccessibleIconButton(onClick = onEdit) {
            Icon(
                Icons.Outlined.Edit,
                contentDescription = stringResource(R.string.users_edit_named, user.username.orEmpty()),
                tint = MaterialTheme.colorScheme.primary,
            )
        }
        AccessibleIconButton(onClick = onDelete) {
            Icon(
                Icons.Outlined.Delete,
                contentDescription = stringResource(R.string.users_delete_named, user.username.orEmpty()),
            )
        }
    }
}

@Composable
internal fun UserEditorDialog(
    initial: AdminUser?,
    isSaving: Boolean,
    error: String?,
    onConfirm: (username: String, email: String, password: String, isAdmin: Boolean) -> Unit,
    onDismiss: () -> Unit,
) {
    var username by remember { mutableStateOf(initial?.username.orEmpty()) }
    var email by remember { mutableStateOf(initial?.email.orEmpty()) }
    var password by remember { mutableStateOf("") }
    var isAdmin by remember { mutableStateOf(initial?.isAdmin ?: false) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = {
            Text(stringResource(if (initial == null) R.string.users_add else R.string.users_edit))
        },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                error?.let { message ->
                    Text(
                        text = message,
                        color = MaterialTheme.colorScheme.error,
                        style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive },
                    )
                }
                OutlinedTextField(
                    value = username,
                    onValueChange = { username = it },
                    label = { Text(stringResource(R.string.users_username)) },
                    singleLine = true,
                    enabled = !isSaving,
                )
                OutlinedTextField(
                    value = email,
                    onValueChange = { email = it },
                    label = { Text(stringResource(R.string.users_email)) },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email),
                    enabled = !isSaving,
                )
                OutlinedTextField(
                    value = password,
                    onValueChange = { password = it },
                    label = { Text(stringResource(R.string.users_password)) },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                    enabled = !isSaving,
                )
                if (initial != null) {
                    Text(
                        text = stringResource(R.string.users_password_hint),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                // #199: the labeled-switch pattern — the label Text and the
                // Switch are one toggleable node so TalkBack names the switch.
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .toggleable(value = isAdmin, onValueChange = { isAdmin = it }, role = Role.Switch),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(stringResource(R.string.users_is_admin), style = MaterialTheme.typography.bodyLarge)
                    Switch(checked = isAdmin, onCheckedChange = null)
                }
            }
        },
        confirmButton = {
            val savingLabel = stringResource(R.string.a11y_state_saving)
            TextButton(
                onClick = { onConfirm(username.trim(), email.trim(), password, isAdmin) },
                enabled = !isSaving && username.isNotBlank() && email.isNotBlank() &&
                    (initial != null || password.isNotBlank()),
                modifier = Modifier.semantics { if (isSaving) stateDescription = savingLabel },
            ) {
                if (isSaving) {
                    CircularProgressIndicator(modifier = Modifier.padding(end = 4.dp), strokeWidth = 2.dp)
                }
                Text(stringResource(R.string.action_save))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, enabled = !isSaving) {
                Text(stringResource(R.string.action_cancel))
            }
        },
    )
}

/**
 * The backend's RFC3339 created_at rendered with the device locale's medium
 * date format (mirroring web's formatDate → toLocaleDateString).
 */
private fun formatCreatedDate(iso: String?): String {
    if (iso.isNullOrBlank()) return ""
    return runCatching {
        DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM)
            .withZone(ZoneId.systemDefault())
            .format(Instant.parse(iso))
    }.getOrDefault(iso)
}
