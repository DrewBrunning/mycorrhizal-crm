package com.mycorrhizal.crm.feature.households

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.AutoAwesome
import androidx.compose.material.icons.outlined.Message
import androidx.compose.material.icons.outlined.Person
import androidx.compose.material.icons.outlined.Remove
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.FloatingActionButton
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
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.feature.contacts.FieldActions
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.HouseholdMember
import com.mycorrhizal.crm.model.network.HouseholdRoles
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.R

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HouseholdDetailScreen(
    onBack: () -> Unit,
    onNavigateToContact: (Int) -> Unit,
    viewModel: HouseholdDetailViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }
    var showAddDialog by remember { mutableStateOf(false) }
    val errorMessage = state.errorRes?.let { stringResource(it) } ?: state.error
    val context = LocalContext.current

    // Issue #218: numbers for the "text everyone" group SMS, resolved from
    // the same contactsByUid map the member rows use for display names.
    // Members with no phone number are silently skipped. Deduplicated in
    // case two members share a number (e.g. a shared landline).
    val textablePhones = remember(state.members, state.contactsByUid) {
        state.members
            .mapNotNull { member -> state.contactsByUid[member.memberVCardUid]?.primaryPhone }
            .map { it.trim() }
            .filter { it.isNotEmpty() }
            .distinct()
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
                        text = state.household?.name ?: stringResource(R.string.households_title),
                        style = MaterialTheme.typography.titleLarge,
                    )
                },
                actions = {
                    IconButton(
                        onClick = { context.startActivity(FieldActions.groupSmsIntent(textablePhones, context)) },
                        enabled = textablePhones.size >= 2,
                    ) {
                        Icon(
                            Icons.Outlined.Message,
                            contentDescription = stringResource(R.string.households_text_everyone),
                        )
                    }
                    IconButton(
                        onClick = { viewModel.suggestRelationships() },
                        enabled = state.members.size >= 2 && !state.isSuggestingRelationships,
                    ) {
                        if (state.isSuggestingRelationships) {
                            CircularProgressIndicator(
                                modifier = Modifier.padding(6.dp),
                                strokeWidth = 2.dp,
                                color = MaterialTheme.colorScheme.onPrimary,
                            )
                        } else {
                            Icon(
                                Icons.Outlined.AutoAwesome,
                                contentDescription = stringResource(R.string.households_suggest_relationships),
                            )
                        }
                    }
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
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.households_add_member))
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading -> LoadingSkeleton()
                state.members.isEmpty() && errorMessage == null ->
                    EmptyState(message = stringResource(R.string.households_members_empty))
                state.members.isEmpty() && errorMessage != null -> {
                    Text(
                        text = errorMessage,
                        color = MaterialTheme.colorScheme.error,
                        modifier = Modifier.align(Alignment.Center),
                    )
                }
                else -> {
                    LazyColumn(modifier = Modifier.fillMaxSize()) {
                        items(state.members, key = { it.id }) { member ->
                            val resolved = state.contactsByUid[member.memberVCardUid]
                            HouseholdMemberRow(
                                member = member,
                                displayName = displayNameFor(resolved)
                                    ?: stringResource(R.string.households_member_unknown),
                                resolvedContactId = resolved?.takeIf { it.id != 0 }?.id,
                                removing = state.removingUid == member.memberVCardUid,
                                updatingRole = state.updatingRoleUid == member.memberVCardUid,
                                onRoleChange = { role -> viewModel.updateMemberRole(member.memberVCardUid, role) },
                                onNavigateToContact = onNavigateToContact,
                                onRemove = { viewModel.removeMember(member.memberVCardUid) },
                            )
                        }
                    }
                }
            }
        }
    }

    if (showAddDialog) {
        AddHouseholdMemberDialog(
            existingMemberUids = state.members.map { it.memberVCardUid }.toSet(),
            query = state.memberSearchQuery,
            results = state.memberSearchResults,
            searchLoading = state.memberSearchLoading,
            onQueryChange = { viewModel.searchContacts(it) },
            onClearSearch = { viewModel.clearContactSearch() },
            onConfirm = { uid, role ->
                viewModel.addMember(uid, role)
                showAddDialog = false
                viewModel.clearContactSearch()
            },
            onDismiss = {
                showAddDialog = false
                viewModel.clearContactSearch()
            },
        )
    }

    errorMessage?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
        }
    }

    val infoMessage = state.infoRes?.let { res ->
        val count = state.infoCount
        if (count != null) stringResource(res, count) else stringResource(res)
    }

    infoMessage?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onInfoShown()
        }
    }
}

fun displayNameFor(contact: ContactSummary?): String? {
    if (contact == null) return null
    // M5 §3.2: use the shared components-first derivation (firstname "nickname"
    // lastname) instead of a local fn-first copy that regressed to
    // given-name-only — same fix as RelationshipsScreen.
    return contact.displayName.takeIf { !it.startsWith("#") } ?: contact.nickname
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun HouseholdMemberRow(
    member: HouseholdMember,
    displayName: String,
    resolvedContactId: Int?,
    removing: Boolean,
    updatingRole: Boolean,
    onRoleChange: (String?) -> Unit,
    onNavigateToContact: (Int) -> Unit,
    onRemove: () -> Unit,
) {
    var roleExpanded by remember { mutableStateOf(false) }

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .then(if (resolvedContactId != null) Modifier.clickable { onNavigateToContact(resolvedContactId) } else Modifier)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            imageVector = Icons.Outlined.Person,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Column(modifier = Modifier.weight(1f).padding(horizontal = 8.dp)) {
            Text(
                text = displayName,
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            HouseholdRolePicker(
                role = member.role,
                expanded = roleExpanded,
                onExpandedChange = { roleExpanded = it },
                updating = updatingRole,
                onSelect = { role ->
                    roleExpanded = false
                    onRoleChange(role)
                },
            )
        }
        IconButton(onClick = onRemove, enabled = !removing) {
            Icon(
                imageVector = Icons.Outlined.Remove,
                // #205: the remove action carries the member's identifier so
                // TalkBack doesn't read a bare "Remove member" on every row.
                contentDescription = stringResource(R.string.households_remove_member_named, member.memberVCardUid),
            )
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun HouseholdRolePicker(
    role: String?,
    expanded: Boolean,
    onExpandedChange: (Boolean) -> Unit,
    updating: Boolean,
    onSelect: (String?) -> Unit,
) {
    ExposedDropdownMenuBox(
        expanded = expanded,
        onExpandedChange = onExpandedChange,
    ) {
        OutlinedTextField(
            value = HouseholdRoleLabels.label(role),
            onValueChange = {},
            readOnly = true,
            singleLine = true,
            label = { Text(stringResource(R.string.households_role)) },
            trailingIcon = {
                if (updating) {
                    CircularProgressIndicator(modifier = Modifier.padding(4.dp), strokeWidth = 2.dp)
                } else {
                    ExposedDropdownMenuDefaults.TrailingIcon(expanded = expanded)
                }
            },
            textStyle = MaterialTheme.typography.labelMedium,
            modifier = Modifier.menuAnchor(),
        )
        ExposedDropdownMenu(
            expanded = expanded,
            onDismissRequest = { onExpandedChange(false) },
        ) {
            DropdownMenuItem(
                text = { Text(stringResource(R.string.households_no_role)) },
                onClick = { onSelect(null) },
            )
            HouseholdRoles.ALL.forEach { token ->
                DropdownMenuItem(
                    text = { Text(HouseholdRoleLabels.label(token)) },
                    onClick = { onSelect(token) },
                )
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AddHouseholdMemberDialog(
    existingMemberUids: Set<String>,
    query: String,
    results: List<ContactSummary>,
    searchLoading: Boolean,
    onQueryChange: (String) -> Unit,
    onClearSearch: () -> Unit,
    onConfirm: (String, String?) -> Unit,
    onDismiss: () -> Unit,
) {
    var selectedContact by remember { mutableStateOf<ContactSummary?>(null) }
    var role by remember { mutableStateOf(HouseholdRoles.ADULT) }
    var roleExpanded by remember { mutableStateOf(false) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.households_add_member)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (selectedContact != null) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(
                            text = displayNameFor(selectedContact).orEmpty(),
                            style = MaterialTheme.typography.bodyLarge,
                            modifier = Modifier.weight(1f),
                        )
                        TextButton(onClick = {
                            selectedContact = null
                            onClearSearch()
                        }) {
                            Text(stringResource(R.string.households_change_selection))
                        }
                    }
                } else {
                    OutlinedTextField(
                        value = query,
                        onValueChange = onQueryChange,
                        label = { Text(stringResource(R.string.households_member_search)) },
                        singleLine = true,
                        trailingIcon = {
                            if (searchLoading) {
                                CircularProgressIndicator(modifier = Modifier.padding(4.dp), strokeWidth = 2.dp)
                            }
                        },
                        modifier = Modifier.fillMaxWidth(),
                    )
                    when {
                        query.isBlank() -> Text(
                            text = stringResource(R.string.households_type_to_search),
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                        results.isEmpty() && !searchLoading -> Text(
                            text = stringResource(R.string.households_no_contacts_found),
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                        else -> LazyColumn(modifier = Modifier.heightIn(max = 200.dp)) {
                            items(results, key = { it.id }) { contact ->
                                Text(
                                    text = displayNameFor(contact).orEmpty(),
                                    style = MaterialTheme.typography.bodyMedium,
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .clickable { selectedContact = contact }
                                        .padding(vertical = 8.dp),
                                )
                            }
                        }
                    }
                }
                ExposedDropdownMenuBox(
                    expanded = roleExpanded,
                    onExpandedChange = { roleExpanded = it },
                ) {
                    OutlinedTextField(
                        value = HouseholdRoleLabels.label(role),
                        onValueChange = {},
                        readOnly = true,
                        label = { Text(stringResource(R.string.households_role)) },
                        trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = roleExpanded) },
                        modifier = Modifier
                            .fillMaxWidth()
                            .menuAnchor(),
                    )
                    ExposedDropdownMenu(
                        expanded = roleExpanded,
                        onDismissRequest = { roleExpanded = false },
                    ) {
                        HouseholdRoles.ALL.forEach { token ->
                            DropdownMenuItem(
                                text = { Text(HouseholdRoleLabels.label(token)) },
                                onClick = { role = token; roleExpanded = false },
                            )
                        }
                    }
                }
            }
        },
        confirmButton = {
            val selectedUid = selectedContact?.uid
            TextButton(
                onClick = {
                    if (selectedUid != null) onConfirm(selectedUid, role)
                },
                enabled = selectedUid != null && selectedUid !in existingMemberUids,
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

object HouseholdRoleLabels {
    @Composable
    fun label(role: String?): String = when (role) {
        HouseholdRoles.ADULT -> stringResource(R.string.households_role_adult)
        HouseholdRoles.CHILD -> stringResource(R.string.households_role_child)
        HouseholdRoles.PET -> stringResource(R.string.households_role_pet)
        HouseholdRoles.ROOMMATE -> stringResource(R.string.households_role_roommate)
        else -> stringResource(R.string.households_no_role)
    }
}
