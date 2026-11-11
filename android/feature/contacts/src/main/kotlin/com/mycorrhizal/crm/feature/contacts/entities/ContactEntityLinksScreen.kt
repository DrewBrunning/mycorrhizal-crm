package com.mycorrhizal.crm.feature.contacts.entities

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
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
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
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
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.ContactEntityLink
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.WriteConflictDialog
import com.mycorrhizal.crm.ui.components.localizedMessage

/**
 * Issue #2302: the contact page's "Organisations, places & pets" section - links with role, period
 * and primary switch, adopt, and linking one of the contact's own values. A projected value is
 * never edited here (or anywhere) as the contact's own.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContactEntityLinksScreen(
    onBack: () -> Unit,
    onOpenEntity: (String) -> Unit,
    modifier: Modifier = Modifier,
    viewModel: ContactEntityLinksViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { viewModel.onResumed() }
    ContactEntityLinksContent(
        state = state,
        onBack = onBack,
        onOpenEntity = onOpenEntity,
        onLink = viewModel::link,
        onSetPrimary = viewModel::setPrimary,
        onRemove = viewModel::remove,
        onAdopt = viewModel::adopt,
        onLinkOwn = viewModel::linkOwnValue,
        onFindMatch = viewModel::findMatch,
        onSearch = viewModel::searchEntities,
        onClearSearch = viewModel::clearSearch,
        onFormDismissed = viewModel::onFormDismissed,
        onErrorShown = viewModel::onErrorShown,
        onConflictLoadLatest = viewModel::onConflictLoadLatest,
        onConflictDismissed = viewModel::onConflictDismissed,
        onRetry = viewModel::load,
        modifier = modifier
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun ContactEntityLinksContent(
    state: ContactEntityLinksUiState,
    onBack: () -> Unit,
    onOpenEntity: (String) -> Unit,
    onLink: (LinkForm, () -> Unit) -> Unit,
    onSetPrimary: (ContactEntityLink, Boolean) -> Unit,
    onRemove: (ContactEntityLink) -> Unit,
    onAdopt: (ContactEntityLink) -> Unit,
    onLinkOwn: (OwnValue, String?, String, () -> Unit) -> Unit,
    onFindMatch: (OwnValue) -> Unit,
    onSearch: (String, String) -> Unit,
    onClearSearch: () -> Unit,
    onFormDismissed: () -> Unit,
    onErrorShown: () -> Unit,
    onConflictLoadLatest: () -> Unit,
    onConflictDismissed: () -> Unit,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier
) {
    val snackbar = remember { SnackbarHostState() }
    var linkDialog by rememberSaveable { mutableStateOf(false) }
    var editing by remember { mutableStateOf<ContactEntityLink?>(null) }
    var showOwn by rememberSaveable { mutableStateOf(false) }
    var removing by remember { mutableStateOf<ContactEntityLink?>(null) }

    val adopted = state.adoptedName?.let {
        stringResource(
            R.string.contact_entity_links_adopted,
            it
        )
    }
    val linkedOwn = state.linkedOwnName?.let {
        stringResource(R.string.contact_entity_links_from_own_linked, it)
    }
    val errorText = state.error?.localizedMessage()
    val message = adopted ?: linkedOwn ?: errorText
    val currentOnErrorShown by rememberUpdatedState(onErrorShown)
    LaunchedEffect(message) {
        if (message != null) {
            snackbar.showSnackbar(message)
            currentOnErrorShown()
        }
    }

    Scaffold(
        modifier = modifier,
        topBar = {
            TopAppBar(
                navigationIcon = {
                    AccessibleIconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.cd_back)
                        )
                    }
                },
                title = {
                    Text(
                        stringResource(R.string.contact_entities),
                        style = MaterialTheme.typography.titleLarge
                    )
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary
                )
            )
        },
        snackbarHost = { SnackbarHost(snackbar) }
    ) { padding ->
        LazyColumn(
            modifier = Modifier.fillMaxSize().padding(padding).testTag("contact-entity-links-list")
        ) {
            item(key = "actions") {
                Row(
                    Modifier.padding(horizontal = 8.dp),
                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                ) {
                    TextButton(
                        onClick = {
                            editing = null
                            linkDialog = true
                        },
                        modifier = Modifier.testTag("contact-entity-link-add")
                    ) { Text(stringResource(R.string.contact_entity_links_add)) }
                    TextButton(
                        onClick = { showOwn = true },
                        modifier = Modifier.testTag("contact-entity-link-from-own")
                    ) { Text(stringResource(R.string.contact_entity_links_link_from_own)) }
                }
            }
            when {
                state.isLoading && state.links.isEmpty() -> item { LoadingSkeleton(rows = 3) }

                state.error != null && state.links.isEmpty() -> item {
                    Column(Modifier.padding(16.dp)) {
                        Text(
                            stringResource(R.string.contact_entity_links_load_failed),
                            color = MaterialTheme.colorScheme.error
                        )
                        TextButton(
                            onClick = onRetry
                        ) { Text(stringResource(R.string.action_retry)) }
                    }
                }

                state.links.isEmpty() -> item {
                    Text(
                        stringResource(R.string.contact_entity_links_empty),
                        modifier = Modifier.padding(16.dp).testTag("contact-entity-links-empty")
                    )
                }

                else -> items(state.links, key = { it.id }) { link ->
                    LinkCard(
                        link = link,
                        busy = state.isSaving,
                        onOpen = { onOpenEntity(link.entityId) },
                        onEdit = {
                            editing = link
                            linkDialog = true
                        },
                        onRemove = { removing = link },
                        onSetPrimary = { onSetPrimary(link, it) },
                        onAdopt = { onAdopt(link) }
                    )
                }
            }
        }
    }

    if (linkDialog) {
        ContactEntityLinkDialog(
            editing = editing,
            state = state,
            onSearch = onSearch,
            onSave = { form ->
                onLink(form) {
                    linkDialog = false
                    editing = null
                    onClearSearch()
                }
            },
            onDismiss = {
                linkDialog = false
                editing = null
                onClearSearch()
                onFormDismissed()
            }
        )
    }
    if (showOwn) {
        LinkOwnValueDialog(
            state = state,
            onFindMatch = onFindMatch,
            onLink = { value, existing, role ->
                onLinkOwn(value, existing, role) {
                    showOwn = false
                    onClearSearch()
                }
            },
            onDismiss = {
                showOwn = false
                onClearSearch()
            }
        )
    }
    removing?.let { link ->
        AlertDialog(
            onDismissRequest = { removing = null },
            title = {
                Text(
                    stringResource(R.string.contact_entity_links_remove_for, link.entityName)
                )
            },
            text = {
                Text(
                    stringResource(R.string.contact_entity_links_remove_confirm, link.entityName)
                )
            },
            confirmButton = {
                TextButton(
                    modifier = Modifier.testTag("contact-entity-link-remove-confirm"),
                    onClick = {
                        onRemove(link)
                        removing = null
                    }
                ) {
                    Text(
                        stringResource(R.string.action_delete),
                        color = MaterialTheme.colorScheme.error
                    )
                }
            },
            dismissButton = {
                TextButton(
                    onClick = { removing = null }
                ) { Text(stringResource(R.string.action_cancel)) }
            }
        )
    }
    if (state.conflict) {
        WriteConflictDialog(onLoadLatest = onConflictLoadLatest, onKeep = onConflictDismissed)
    }
}

@Composable
private fun LinkCard(
    link: ContactEntityLink,
    busy: Boolean,
    onOpen: () -> Unit,
    onEdit: () -> Unit,
    onRemove: () -> Unit,
    onSetPrimary: (Boolean) -> Unit,
    onAdopt: () -> Unit
) {
    val projects = EntityKind.projects(link.kind)
    OutlinedCard(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp)
            .testTag("contact-entity-link-${link.id}")
    ) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(
                link.entityName,
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.clickable(role = Role.Button, onClick = onOpen)
            )
            val detail = listOfNotNull(
                stringResource(entityKindOneRes(link.kind)),
                entityRoleText(link.kind, link.role, link.roleLabel),
                entityPeriodText(link.period)
            ).joinToString(" · ")
            Text(
                detail,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            when (link.projectsAs) {
                "ORG" -> ChipText(stringResource(R.string.contact_entity_links_shown_as_org))
                "ADR" -> ChipText(stringResource(R.string.contact_entity_links_shown_as_adr))
            }
            if (!link.current) ChipText(stringResource(R.string.contact_entity_links_not_current))
            if (projects) {
                Row(
                    modifier = Modifier.fillMaxWidth()
                        .toggleable(
                            value = link.isPrimary,
                            enabled = !busy,
                            role = Role.Switch,
                            onValueChange = onSetPrimary
                        )
                        .testTag("contact-entity-link-primary-${link.id}"),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(
                        stringResource(R.string.contact_entity_links_primary_for, link.entityName),
                        modifier = Modifier.weight(1f),
                        style = MaterialTheme.typography.bodyMedium
                    )
                    Switch(checked = link.isPrimary, onCheckedChange = null, enabled = !busy)
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                TextButton(onClick = onEdit, enabled = !busy) {
                    Text(stringResource(R.string.action_edit))
                }
                TextButton(onClick = onRemove, enabled = !busy) {
                    Text(
                        stringResource(R.string.action_delete),
                        color = MaterialTheme.colorScheme.error
                    )
                }
                if (projects && link.projectsAs.isNotEmpty()) {
                    TextButton(
                        onClick = onAdopt,
                        enabled = !busy,
                        modifier = Modifier.testTag("contact-entity-link-adopt-${link.id}")
                    ) {
                        Text(
                            stringResource(
                                if (link.kind == EntityKind.PLACE) {
                                    R.string.contact_entity_links_adopt_place
                                } else {
                                    R.string.contact_entity_links_adopt_organisation
                                },
                                link.entityName
                            )
                        )
                    }
                }
            }
            if (projects && link.projectsAs.isNotEmpty()) {
                Text(
                    stringResource(R.string.contact_entity_links_adopt_hint),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
        }
    }
}

@Composable
private fun ChipText(text: String) {
    Text(
        text,
        style = MaterialTheme.typography.labelMedium,
        color = MaterialTheme.colorScheme.primary
    )
}
