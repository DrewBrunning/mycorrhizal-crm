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
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.feature.contacts.CustomFieldValueRow
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityContactLink
import com.mycorrhizal.crm.model.network.EntityCore
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.localizedMessage

/** Issue #2302: one entity - core details, custom fields, People, and edit/merge/delete/restore. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun EntityDetailScreen(
    onBack: () -> Unit,
    onOpenContact: (Int) -> Unit,
    onOpenEntity: (String) -> Unit,
    modifier: Modifier = Modifier,
    viewModel: EntityDetailViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { viewModel.onResumed() }
    EntityDetailContent(
        state = state,
        onBack = onBack,
        onOpenContact = onOpenContact,
        onOpenEntity = onOpenEntity,
        onRetry = viewModel::load,
        onSave = viewModel::save,
        onFormDismissed = viewModel::onFormDismissed,
        onSaveField = viewModel::saveFieldValue,
        onDelete = viewModel::delete,
        onRestore = viewModel::restore,
        onRestoreMerged = viewModel::restoreMergedLoser,
        onSearchMerge = viewModel::searchMergeCandidates,
        onMerge = viewModel::merge,
        onMergeDismissed = viewModel::onMergeDismissed,
        modifier = modifier
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun EntityDetailContent(
    state: EntityDetailUiState,
    onBack: () -> Unit,
    onOpenContact: (Int) -> Unit,
    onOpenEntity: (String) -> Unit,
    onRetry: () -> Unit,
    onSave: (com.mycorrhizal.crm.model.network.EntityInput, () -> Unit) -> Unit,
    onFormDismissed: () -> Unit,
    onSaveField: (String, Any?) -> Unit,
    onDelete: (Boolean) -> Unit,
    onRestore: () -> Unit,
    onRestoreMerged: () -> Unit,
    onSearchMerge: (String) -> Unit,
    onMerge: (Entity, Boolean, Set<String>) -> Unit,
    onMergeDismissed: () -> Unit,
    modifier: Modifier = Modifier
) {
    val entity = state.entity
    var showEdit by rememberSaveable { mutableStateOf(false) }
    var showDelete by rememberSaveable { mutableStateOf(false) }
    var showMerge by rememberSaveable { mutableStateOf(false) }

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
                        entity?.name ?: stringResource(R.string.shared_entities_title),
                        style = MaterialTheme.typography.titleLarge
                    )
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary
                )
            )
        }
    ) { padding ->
        val outcome = state.mergeOutcome
        Column(Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading && entity == null -> LoadingSkeleton()

                state.notFound -> Text(
                    stringResource(R.string.shared_entities_detail_not_found),
                    modifier = Modifier.padding(16.dp).testTag("entity-not-found")
                )

                entity == null -> Column(Modifier.padding(16.dp)) {
                    state.error?.let {
                        Text(it.localizedMessage(), color = MaterialTheme.colorScheme.error)
                    }
                    TextButton(onClick = onRetry) { Text(stringResource(R.string.action_retry)) }
                }

                state.deleted -> BannerCard(
                    title = stringResource(R.string.shared_entities_deleted_title, entity.name),
                    body = stringResource(R.string.shared_entities_deleted_body),
                    actionLabel = stringResource(R.string.shared_entities_deleted_restore),
                    actionTag = "entity-restore",
                    enabled = !state.isRestoring,
                    error = state.restoreError,
                    onAction = onRestore
                )

                outcome != null && outcome.thisWasMergedAway -> Column {
                    BannerCard(
                        title = stringResource(
                            R.string.shared_entities_merge_undo_title,
                            outcome.loser.name
                        ),
                        body = stringResource(
                            R.string.shared_entities_merge_summary_line,
                            outcome.loser.name,
                            outcome.survivor.name
                        ) + " " + stringResource(R.string.shared_entities_merge_undo_body),
                        actionLabel = stringResource(
                            R.string.shared_entities_merge_undo,
                            outcome.loser.name
                        ),
                        actionTag = "entity-merge-undo",
                        enabled = !state.isRestoring,
                        error = state.restoreError,
                        onAction = onRestoreMerged
                    )
                    TextButton(
                        onClick = { onOpenEntity(outcome.survivor.id) },
                        modifier = Modifier.padding(
                            horizontal = 8.dp
                        ).testTag("entity-open-survivor")
                    ) {
                        Text(
                            stringResource(
                                R.string.shared_entities_merge_open_survivor,
                                outcome.survivor.name
                            )
                        )
                    }
                }

                else -> EntityBody(
                    state = state,
                    entity = entity,
                    onOpenContact = onOpenContact,
                    onSaveField = onSaveField,
                    onEdit = { showEdit = true },
                    onMerge = { showMerge = true },
                    onDelete = { showDelete = true },
                    onRestoreMerged = onRestoreMerged
                )
            }
        }
    }

    if (entity != null && showEdit) {
        EntityFormDialog(
            kind = entity.kind,
            entity = entity,
            saving = state.isSaving,
            error = state.formError,
            onSave = { input -> onSave(input) { showEdit = false } },
            onDismiss = {
                showEdit = false
                onFormDismissed()
            }
        )
    }
    if (entity != null && showDelete) {
        EntityDeleteDialog(
            entity = entity,
            deleting = state.isDeleting,
            adoptFailedCount = state.adoptFailedCount,
            error = state.error,
            onConfirm = onDelete,
            onDismiss = { showDelete = false }
        )
    }
    if (entity != null && showMerge) {
        EntityMergeDialog(
            entity = entity,
            candidates = state.mergeCandidates,
            loading = state.mergeCandidatesLoading,
            merging = state.isMerging,
            error = state.mergeError,
            onSearch = onSearchMerge,
            onConfirm = { other, keepThis, fill -> onMerge(other, keepThis, fill) },
            onDismiss = {
                showMerge = false
                onMergeDismissed()
            }
        )
    }
    // Both dialogs stay open until the call lands so a failure is visible next to the controls.
    LaunchedEffect(state.mergeOutcome) { if (state.mergeOutcome != null) showMerge = false }
    LaunchedEffect(state.deleted) { if (state.deleted) showDelete = false }
}

@Composable
private fun BannerCard(
    title: String,
    body: String,
    actionLabel: String,
    actionTag: String,
    enabled: Boolean,
    error: ApiError?,
    onAction: () -> Unit
) {
    Column(
        modifier = Modifier.fillMaxWidth().padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Text(title, style = MaterialTheme.typography.titleMedium)
        Text(body, style = MaterialTheme.typography.bodyMedium)
        error?.let {
            Text(
                stringResource(
                    R.string.shared_entities_restore_failed
                ) + " " + it.localizedMessage(),
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodySmall
            )
        }
        OutlinedButton(
            onClick = onAction,
            enabled = enabled,
            modifier = Modifier.testTag(actionTag)
        ) {
            Text(actionLabel)
        }
    }
}

@Composable
private fun EntityBody(
    state: EntityDetailUiState,
    entity: Entity,
    onOpenContact: (Int) -> Unit,
    onSaveField: (String, Any?) -> Unit,
    onEdit: () -> Unit,
    onMerge: () -> Unit,
    onDelete: () -> Unit,
    onRestoreMerged: () -> Unit
) {
    LazyColumn(Modifier.fillMaxSize().testTag("entity-detail-list")) {
        item {
            Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(
                    stringResource(entityKindOneRes(entity.kind)),
                    style = MaterialTheme.typography.labelLarge,
                    color = MaterialTheme.colorScheme.primary
                )
                entity.linkCount?.let { count ->
                    Text(
                        pluralStringResource(
                            R.plurals.shared_entities_detail_linked_count,
                            count,
                            count
                        ),
                        style = MaterialTheme.typography.bodyMedium
                    )
                }
                Text(
                    stringResource(entitySensitivityRes(entity.sensitivity)),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
        }
        state.mergeOutcome?.takeUnless { it.thisWasMergedAway }?.let { outcome ->
            item(key = "merge-note") {
                BannerCard(
                    title = stringResource(R.string.shared_entities_merge_done, entity.name),
                    body = stringResource(R.string.shared_entities_merge_undo_body),
                    actionLabel = stringResource(
                        R.string.shared_entities_merge_undo,
                        outcome.loser.name
                    ),
                    actionTag = "entity-merge-undo",
                    enabled = !state.isRestoring,
                    error = state.restoreError,
                    onAction = onRestoreMerged
                )
            }
        }
        if (state.sameNameLive) {
            item(key = "same-name") {
                Text(
                    stringResource(R.string.shared_entities_deleted_same_name_live),
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error
                )
            }
        }
        item(key = "actions") {
            Row(
                Modifier.padding(horizontal = 8.dp),
                horizontalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                TextButton(onClick = onEdit, modifier = Modifier.testTag("entity-edit")) {
                    Text(stringResource(R.string.shared_entities_detail_edit))
                }
                TextButton(onClick = onMerge, modifier = Modifier.testTag("entity-merge")) {
                    Text(stringResource(R.string.shared_entities_detail_merge))
                }
                TextButton(onClick = onDelete, modifier = Modifier.testTag("entity-delete")) {
                    Text(
                        stringResource(R.string.shared_entities_detail_delete),
                        color = MaterialTheme.colorScheme.error
                    )
                }
            }
        }
        item(key = "details") { DetailsSection(entity) }
        item(
            key = "fields-header"
        ) { SectionTitle(stringResource(R.string.shared_entities_detail_custom_fields)) }
        if (state.fieldDefinitions.isEmpty()) {
            item(key = "fields-empty") {
                Text(
                    stringResource(R.string.shared_entities_detail_no_custom_fields),
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
                    style = MaterialTheme.typography.bodyMedium
                )
            }
        } else {
            items(state.fieldDefinitions, key = { "field-${it.id}" }) { definition ->
                CustomFieldValueRow(
                    definition = definition,
                    value = state.fieldValues[definition.id],
                    saving = state.savingFieldDefinitionId == definition.id,
                    onSave = { onSaveField(definition.id, it) }
                )
            }
        }
        item(
            key = "people-header"
        ) { SectionTitle(stringResource(R.string.shared_entities_people_title)) }
        when {
            state.peopleFailed -> item(key = "people-failed") {
                Text(
                    stringResource(R.string.shared_entities_people_load_failed),
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp)
                )
            }

            state.people.isEmpty() -> item(key = "people-empty") {
                Text(
                    stringResource(R.string.shared_entities_people_empty),
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
                    style = MaterialTheme.typography.bodyMedium
                )
            }

            else -> items(state.people, key = { "person-${it.linkId}" }) { person ->
                PersonRow(entity.kind, person) { onOpenContact(person.contactId) }
                HorizontalDivider()
            }
        }
    }
}

@Composable
private fun SectionTitle(text: String) {
    Text(
        text,
        style = MaterialTheme.typography.titleSmall,
        color = MaterialTheme.colorScheme.primary,
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp)
    )
}

@Composable
private fun DetailsSection(entity: Entity) {
    val form = EntityCore.toForm(entity.kind, entity.core)
    val rows = when (entity.kind) {
        EntityKind.ORGANISATION -> listOf(
            R.string.shared_entities_field_url to form.url,
            R.string.shared_entities_field_sort_as to form.sortAs,
            R.string.shared_entities_field_units to form.unitsText.replace("\n", ", "),
            R.string.shared_entities_field_notes to form.notes
        )

        EntityKind.PLACE -> listOf(
            R.string.shared_entities_field_street to form.street,
            R.string.shared_entities_field_city to form.city,
            R.string.shared_entities_field_region to form.region,
            R.string.shared_entities_field_postal to form.postal,
            R.string.shared_entities_field_country to form.country,
            R.string.shared_entities_field_notes to form.notes
        )

        else -> listOf(
            R.string.shared_entities_field_species to form.species,
            R.string.shared_entities_field_breed to form.breed,
            R.string.shared_entities_field_birth to form.birth,
            R.string.shared_entities_field_death to form.death,
            R.string.shared_entities_field_notes to form.notes
        )
    }.filter { it.second.isNotBlank() }
    Column {
        SectionTitle(stringResource(R.string.shared_entities_detail_details))
        if (rows.isEmpty()) {
            Text(
                stringResource(R.string.shared_entities_detail_no_details),
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
                style = MaterialTheme.typography.bodyMedium
            )
        }
        rows.forEach { (label, value) ->
            Column(Modifier.padding(horizontal = 16.dp, vertical = 4.dp)) {
                Text(
                    stringResource(label),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
                Text(value, style = MaterialTheme.typography.bodyMedium)
            }
        }
    }
}

@Composable
private fun PersonRow(kind: String, person: EntityContactLink, onClick: () -> Unit) {
    val name = person.displayName
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(
                onClickLabel = stringResource(R.string.shared_entities_open, name),
                role = Role.Button,
                onClick = onClick
            )
            .padding(horizontal = 16.dp, vertical = 10.dp)
            .testTag("entity-person-${person.linkId}"),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween
    ) {
        Column(Modifier.weight(1f)) {
            Text(name, style = MaterialTheme.typography.bodyLarge)
            val detail = listOfNotNull(
                entityRoleText(kind, person.role, person.roleLabel),
                entityPeriodText(person.period)
            ).joinToString(" · ")
            Text(
                detail,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
        if (person.isPrimary) {
            Text(
                stringResource(R.string.shared_entities_people_primary),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.primary
            )
        }
    }
}

/**
 * The delete confirmation (ADR 0043 decision 6): says how many contacts lose the projected value
 * and offers adopt-all (unticked) so the user can keep the value on those contacts first.
 */
@Composable
internal fun EntityDeleteDialog(
    entity: Entity,
    deleting: Boolean,
    adoptFailedCount: Int,
    error: ApiError?,
    onConfirm: (adoptAll: Boolean) -> Unit,
    onDismiss: () -> Unit
) {
    var adoptAll by remember { mutableStateOf(false) }
    val projections = entity.projectionCount ?: 0
    val links = entity.linkCount ?: 0
    val projects = EntityKind.projects(entity.kind)
    AlertDialog(
        modifier = Modifier.testTag("entity-delete-dialog"),
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.shared_entities_delete_title, entity.name)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (projects && projections > 0) {
                    Text(
                        pluralStringResource(
                            if (entity.kind == EntityKind.PLACE) {
                                R.plurals.shared_entities_delete_projects_place
                            } else {
                                R.plurals.shared_entities_delete_projects_organisation
                            },
                            projections,
                            projections,
                            entity.name
                        )
                    )
                }
                if (links > 0) {
                    Text(pluralStringResource(R.plurals.shared_entities_delete_links, links, links))
                }
                if (!(projects && projections > 0) && links == 0) {
                    Text(stringResource(R.string.shared_entities_delete_plain, entity.name))
                }
                if (projects && projections > 0) {
                    Row(
                        modifier = Modifier.fillMaxWidth()
                            .toggleable(
                                value = adoptAll,
                                role = Role.Checkbox,
                                onValueChange = { adoptAll = it }
                            )
                            .testTag("entity-delete-adopt"),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Checkbox(checked = adoptAll, onCheckedChange = null)
                        Text(
                            stringResource(
                                if (entity.kind == EntityKind.PLACE) {
                                    R.string.shared_entities_delete_adopt_place
                                } else {
                                    R.string.shared_entities_delete_adopt_organisation
                                }
                            ),
                            modifier = Modifier.padding(start = 8.dp)
                        )
                    }
                }
                if (adoptFailedCount > 0) {
                    Text(
                        pluralStringResource(
                            R.plurals.shared_entities_delete_adopt_failed,
                            adoptFailedCount,
                            adoptFailedCount
                        ),
                        color = MaterialTheme.colorScheme.error
                    )
                }
                error?.let { Text(it.localizedMessage(), color = MaterialTheme.colorScheme.error) }
            }
        },
        confirmButton = {
            TextButton(
                enabled = !deleting,
                modifier = Modifier.testTag("entity-delete-confirm"),
                onClick = { onConfirm(adoptAll) }
            ) {
                Text(
                    stringResource(R.string.shared_entities_delete_confirm),
                    color = MaterialTheme.colorScheme.error
                )
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        }
    )
}
