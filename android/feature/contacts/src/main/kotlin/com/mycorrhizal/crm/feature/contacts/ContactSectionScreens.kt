package com.mycorrhizal.crm.feature.contacts

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
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
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.Anniversary
import com.mycorrhizal.crm.model.network.ContactFieldKey
import com.mycorrhizal.crm.model.network.Organization
import com.mycorrhizal.crm.model.network.PersonalInfo
import com.mycorrhizal.crm.model.network.PersonalInfoMeta
import com.mycorrhizal.crm.model.network.Title
import com.mycorrhizal.crm.model.util.DateFieldMode
import com.mycorrhizal.crm.model.util.DateFieldValue
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.DateField
import com.mycorrhizal.crm.ui.components.DropdownOption
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.FormScaffold
import com.mycorrhizal.crm.ui.components.LabelledDropdown
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.WriteConflictDialog
import com.mycorrhizal.crm.ui.components.typeOptionLabel
import com.mycorrhizal.crm.ui.R

private val SkillsKinds = listOf("expertise", "hobby", "interest")
private val Sensitivities = listOf("normal", "private", "secret")
private val AnniversaryKinds = listOf("wedding", "death")

// ---------------------------------------------------------------------------
// Shared scaffold
// ---------------------------------------------------------------------------

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SectionListScaffold(
    title: String,
    addLabel: String,
    onAdd: (() -> Unit)?,
    onBack: () -> Unit,
    state: ContactSectionsUiState,
    onErrorShown: () -> Unit,
    content: @Composable () -> Unit,
) {
    val snackbarHostState = remember { SnackbarHostState() }
    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = { Text(title, style = MaterialTheme.typography.titleLarge) },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
        floatingActionButton = {
            if (onAdd != null) {
                BrandFab(onClick = onAdd) {
                    Icon(Icons.Outlined.Add, contentDescription = addLabel)
                }
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading && state.record == null -> LoadingSkeleton()
                state.record == null -> EmptyState(state.error ?: stringResource(R.string.contact_not_found))
                else -> content()
            }
        }
    }

    val message = state.errorRes?.let { stringResource(it) } ?: state.error
    if (state.record != null) {
        message?.let {
            LaunchedEffect(it) {
                snackbarHostState.showSnackbar(it)
                onErrorShown()
            }
        }
    }
}

@Composable
private fun SectionHeading(text: String, action: (@Composable () -> Unit)? = null) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Text(
            text = text,
            style = MaterialTheme.typography.titleSmall,
            color = MaterialTheme.colorScheme.primary,
        )
        action?.invoke()
    }
}

@Composable
private fun AddTextButton(label: String, tag: String, onClick: () -> Unit) {
    TextButton(onClick = onClick, modifier = Modifier.testTag(tag)) {
        Icon(Icons.Outlined.Add, contentDescription = null, modifier = Modifier.size(18.dp))
        Text(label, modifier = Modifier.padding(start = 4.dp))
    }
}

@Composable
private fun EntryRow(
    title: String,
    subtitle: String? = null,
    badges: @Composable () -> Unit = {},
    editTag: String,
    deleteTag: String,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    Surface(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
        tonalElevation = 1.dp,
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(modifier = Modifier.weight(1f)) {
                Text(text = title, style = MaterialTheme.typography.bodyLarge, maxLines = 2, overflow = TextOverflow.Ellipsis)
                if (!subtitle.isNullOrBlank()) {
                    Text(
                        text = subtitle,
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                badges()
            }
            IconButton(onClick = onEdit, modifier = Modifier.testTag(editTag)) {
                Icon(Icons.Outlined.Edit, contentDescription = stringResource(R.string.action_edit))
            }
            IconButton(onClick = onDelete, modifier = Modifier.testTag(deleteTag)) {
                Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.action_delete))
            }
        }
    }
}

/** An [EntryRow] whose delete action confirms first (the app-wide delete rule). */
@Composable
private fun ConfirmableEntryRow(
    title: String,
    subtitle: String? = null,
    editTag: String,
    deleteTag: String,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    var confirmDelete by remember { mutableStateOf(false) }
    EntryRow(
        title = title,
        subtitle = subtitle,
        editTag = editTag,
        deleteTag = deleteTag,
        onEdit = onEdit,
        onDelete = { confirmDelete = true },
    )
    if (confirmDelete) {
        DeleteConfirmDialog(
            label = title,
            onConfirm = { confirmDelete = false; onDelete() },
            onDismiss = { confirmDelete = false },
        )
    }
}

@Composable
private fun DeleteConfirmDialog(label: String, onConfirm: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.contact_section_delete_title)) },
        text = { Text(stringResource(R.string.contact_section_delete_confirm, label)) },
        confirmButton = {
            TextButton(onClick = onConfirm) {
                Text(stringResource(R.string.action_delete), color = MaterialTheme.colorScheme.error)
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

@Composable
private fun ConflictDialogIfNeeded(state: ContactSectionsUiState, viewModel: ContactSectionsViewModel) {
    if (state.conflict) {
        WriteConflictDialog(
            onLoadLatest = viewModel::onConflictLoadLatest,
            onKeep = viewModel::onConflictDismissed,
        )
    }
}

// ---------------------------------------------------------------------------
// Skills & interests (Card.personalInfo + crm.personalInfoMeta)
// ---------------------------------------------------------------------------

@Composable
fun ContactSkillsScreen(
    onBack: () -> Unit,
    viewModel: ContactSectionsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    var editingId by rememberSaveable { mutableStateOf<String?>(null) }
    var adding by rememberSaveable { mutableStateOf(false) }
    val record = state.record
    val entries = record?.card?.personalInfo.orEmpty()
    val editing = editingId?.let { id -> entries.firstOrNull { it.id == id } }

    if (adding || editing != null) {
        PersonalInfoFormScreen(
            initial = editing,
            meta = editing?.id?.let { record?.crm?.personalInfoMeta?.get(it) },
            isSaving = state.isSaving,
            onSave = { form ->
                viewModel.savePersonalInfo(editing?.id, form)
                adding = false
                editingId = null
            },
            onBack = { adding = false; editingId = null },
        )
        ConflictDialogIfNeeded(state, viewModel)
        return
    }

    SectionListScaffold(
        title = stringResource(R.string.contact_skills_interests),
        addLabel = stringResource(R.string.contact_skills_add),
        onAdd = { adding = true },
        onBack = onBack,
        state = state,
        onErrorShown = viewModel::onErrorShown,
    ) {
        LazyColumn(modifier = Modifier.fillMaxSize().testTag("contact-skills-list")) {
            if (entries.isEmpty()) {
                item {
                    EmptyState(
                        title = stringResource(R.string.contact_skills_empty),
                        body = stringResource(R.string.contact_skills_empty_body),
                        actionLabel = stringResource(R.string.contact_skills_add),
                        onAction = { adding = true },
                    )
                }
            } else {
                SkillsKinds.forEach { kind ->
                    val kindEntries = entries.filter { it.kind == kind }
                    if (kindEntries.isNotEmpty()) {
                        item(key = "header-$kind") { SectionHeading(typeOptionLabel(kind)) }
                        items(kindEntries, key = { it.id ?: "$kind-${it.value}" }) { entry ->
                            val meta = entry.id?.let { record?.crm?.personalInfoMeta?.get(it) }
                            PersonalInfoRow(
                                entry = entry,
                                meta = meta,
                                onEdit = { editingId = entry.id },
                                onDelete = { entry.id?.let(viewModel::deletePersonalInfo) },
                            )
                        }
                    }
                }
                val other = entries.filter { it.kind !in SkillsKinds }
                if (other.isNotEmpty()) {
                    item(key = "header-other") { SectionHeading(typeOptionLabel("other")) }
                    items(other, key = { it.id ?: "other-${it.value}" }) { entry ->
                        val meta = entry.id?.let { record?.crm?.personalInfoMeta?.get(it) }
                        PersonalInfoRow(
                            entry = entry,
                            meta = meta,
                            onEdit = { editingId = entry.id },
                            onDelete = { entry.id?.let(viewModel::deletePersonalInfo) },
                        )
                    }
                }
            }
            item { Box(modifier = Modifier.size(80.dp)) }
        }
    }
    ConflictDialogIfNeeded(state, viewModel)
}

@Composable
internal fun PersonalInfoRow(
    entry: PersonalInfo,
    meta: PersonalInfoMeta?,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    val id = entry.id.orEmpty()
    val sensitive = meta?.sensitivity != null && meta.sensitivity != "normal"
    var confirmDelete by remember { mutableStateOf(false) }
    EntryRow(
        title = entry.value.orEmpty(),
        subtitle = meta?.notes,
        badges = {
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
                entry.level?.takeIf { it.isNotBlank() }?.let {
                    Surface(
                        color = MaterialTheme.colorScheme.surfaceVariant,
                        shape = androidx.compose.foundation.shape.RoundedCornerShape(4.dp),
                    ) {
                        Text(
                            text = stringResource(personalInfoLevelLabelRes(it)),
                            style = MaterialTheme.typography.labelSmall,
                            modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp),
                        )
                    }
                }
                entry.label?.takeIf { it.isNotBlank() }?.let {
                    Text(it, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                if (meta?.favorite == true) {
                    Icon(
                        Icons.Filled.Star,
                        contentDescription = stringResource(R.string.contact_skills_favorite),
                        tint = MaterialTheme.colorScheme.tertiary,
                        modifier = Modifier.size(18.dp).testTag("personal-info-favorite-$id"),
                    )
                }
                if (sensitive) {
                    Icon(
                        Icons.Outlined.Lock,
                        contentDescription = stringResource(R.string.contact_skills_locked),
                        tint = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.size(18.dp).testTag("personal-info-lock-$id"),
                    )
                }
            }
        },
        editTag = "personal-info-edit-$id",
        deleteTag = "personal-info-delete-$id",
        onEdit = onEdit,
        onDelete = { confirmDelete = true },
    )
    if (confirmDelete) {
        DeleteConfirmDialog(
            label = entry.value.orEmpty(),
            onConfirm = { confirmDelete = false; onDelete() },
            onDismiss = { confirmDelete = false },
        )
    }
}

@Composable
private fun PersonalInfoFormScreen(
    initial: PersonalInfo?,
    meta: PersonalInfoMeta?,
    isSaving: Boolean,
    onSave: (PersonalInfoFormData) -> Unit,
    onBack: () -> Unit,
) {
    var kind by rememberSaveable(initial?.id) { mutableStateOf(initial?.kind?.takeIf { it in SkillsKinds } ?: "expertise") }
    var value by rememberSaveable(initial?.id) { mutableStateOf(initial?.value.orEmpty()) }
    var level by rememberSaveable(initial?.id) { mutableStateOf(initial?.level) }
    var label by rememberSaveable(initial?.id) { mutableStateOf(initial?.label.orEmpty()) }
    var notes by rememberSaveable(initial?.id) { mutableStateOf(meta?.notes.orEmpty()) }
    var sensitivity by rememberSaveable(initial?.id) { mutableStateOf(meta?.sensitivity ?: "normal") }
    var favorite by rememberSaveable(initial?.id) { mutableStateOf(meta?.favorite == true) }
    val snackbar = remember { SnackbarHostState() }

    FormScaffold(
        title = stringResource(if (initial != null) R.string.contact_skills_edit else R.string.contact_skills_add),
        onBack = onBack,
        saveLabel = stringResource(if (initial != null) R.string.action_save else R.string.action_create),
        isSaving = isSaving,
        onSave = {
            onSave(
                PersonalInfoFormData(
                    kind = kind,
                    value = value,
                    level = level?.takeIf { it in personalInfoLevelOptions(kind) },
                    label = label,
                    notes = notes,
                    sensitivity = sensitivity,
                    favorite = favorite,
                ),
            )
        },
        snackbarHostState = snackbar,
        saveEnabled = value.isNotBlank(),
        saveDisabledHint = stringResource(R.string.contact_section_value_required),
    ) { padding ->
        Column(
            modifier = Modifier
                .padding(padding)
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            LabelledDropdown(
                label = stringResource(R.string.contact_skills_kind),
                valueText = typeOptionLabel(kind),
                options = SkillsKinds.map { DropdownOption(it, typeOptionLabel(it)) },
                onSelect = {
                    kind = it
                    if (level !in personalInfoLevelOptions(it)) level = null
                },
                modifier = Modifier.testTag("personal-info-kind"),
            )
            OutlinedTextField(
                value = value,
                onValueChange = { value = it },
                label = { Text(stringResource(R.string.contact_skills_value)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth().testTag("personal-info-value"),
            )
            val levelOptions = personalInfoLevelOptions(kind)
            LabelledDropdown(
                label = stringResource(R.string.contact_skills_level),
                valueText = if (level == null) {
                    stringResource(R.string.contact_personal_info_level_none)
                } else {
                    stringResource(personalInfoLevelLabelRes(level))
                },
                options = listOf(DropdownOption<String?>(null, stringResource(R.string.contact_personal_info_level_none))) +
                    levelOptions.map { DropdownOption<String?>(it, stringResource(personalInfoLevelLabelRes(it))) },
                onSelect = { level = it },
                modifier = Modifier.testTag("personal-info-level"),
            )
            OutlinedTextField(
                value = label,
                onValueChange = { label = it },
                label = { Text(stringResource(R.string.contact_personal_info_label)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth().testTag("personal-info-label"),
            )
            OutlinedTextField(
                value = notes,
                onValueChange = { notes = it },
                label = { Text(stringResource(R.string.contact_skills_notes)) },
                supportingText = { Text(stringResource(R.string.contact_skills_notes_hint)) },
                modifier = Modifier.fillMaxWidth().testTag("personal-info-notes"),
            )
            LabelledDropdown(
                label = stringResource(R.string.relationships_sensitivity),
                valueText = stringResource(sensitivityLabelRes(sensitivity)),
                options = Sensitivities.map { DropdownOption(it, stringResource(sensitivityLabelRes(it))) },
                onSelect = { sensitivity = it },
                modifier = Modifier.testTag("personal-info-sensitivity"),
            )
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .testTag("personal-info-favorite")
                    .toggleable(value = favorite, onValueChange = { favorite = it }, role = Role.Switch),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    text = stringResource(R.string.contact_skills_favorite),
                    style = MaterialTheme.typography.bodyLarge,
                    modifier = Modifier.weight(1f),
                )
                Switch(checked = favorite, onCheckedChange = null)
            }
        }
    }
}

@StringRes
internal fun sensitivityLabelRes(sensitivity: String): Int = when (sensitivity) {
    "private" -> R.string.relationships_sensitivity_private
    "secret" -> R.string.relationships_sensitivity_secret
    else -> R.string.relationships_sensitivity_normal
}

// ---------------------------------------------------------------------------
// Work (Card.organizations / Card.titles / crm.work_information)
// ---------------------------------------------------------------------------

private sealed interface WorkEditor {
    data object Organization : WorkEditor
    data object Title : WorkEditor
    data object WorkInformation : WorkEditor
}

@Composable
fun ContactWorkScreen(
    onBack: () -> Unit,
    viewModel: ContactSectionsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    var editor by rememberSaveable { mutableStateOf<WorkEditor?>(null) }
    var editingOrgId by rememberSaveable { mutableStateOf<String?>(null) }
    var editingTitleId by rememberSaveable { mutableStateOf<String?>(null) }
    val record = state.record
    val organizations = record?.card?.organizations.orEmpty()
    val titles = record?.card?.titles.orEmpty()
    val workInformation = record?.crm?.workInformation
    // Issue #1728: visibility still follows the enabled-contact-fields
    // settings, per subsection (web's showOrganizations/showTitles/
    // showWorkInformation).
    val showOrganizations = ContactFieldKey.ORGANIZATIONS in state.enabledFields
    val showTitles = ContactFieldKey.TITLES in state.enabledFields
    val showWorkInformation = ContactFieldKey.WORK_INFORMATION in state.enabledFields

    when (editor) {
        WorkEditor.Organization -> {
            val initial = editingOrgId?.let { id -> organizations.firstOrNull { it.id == id } }
            OrganizationFormScreen(
                initial = initial,
                period = initial?.id?.let { id ->
                    record?.crm?.periods?.firstOrNull { it.kind == "organization" && it.entryId == id }
                },
                isSaving = state.isSaving,
                onSave = {
                    viewModel.saveOrganization(initial?.id, it)
                    editor = null
                    editingOrgId = null
                },
                onBack = { editor = null; editingOrgId = null },
            )
            ConflictDialogIfNeeded(state, viewModel)
            return
        }
        WorkEditor.Title -> {
            val initial = editingTitleId?.let { id -> titles.firstOrNull { it.id == id } }
            TitleFormScreen(
                initial = initial,
                period = initial?.id?.let { id ->
                    record?.crm?.periods?.firstOrNull { it.kind == "title" && it.entryId == id }
                },
                isSaving = state.isSaving,
                onSave = {
                    viewModel.saveTitle(initial?.id, it)
                    editor = null
                    editingTitleId = null
                },
                onBack = { editor = null; editingTitleId = null },
            )
            ConflictDialogIfNeeded(state, viewModel)
            return
        }
        WorkEditor.WorkInformation -> {
            WorkInformationFormScreen(
                initial = workInformation.orEmpty(),
                isSaving = state.isSaving,
                onSave = {
                    viewModel.saveWorkInformation(it)
                    editor = null
                },
                onBack = { editor = null },
            )
            ConflictDialogIfNeeded(state, viewModel)
            return
        }
        null -> Unit
    }

    SectionListScaffold(
        title = stringResource(R.string.contact_work),
        addLabel = stringResource(R.string.contact_work_add),
        onAdd = { editor = WorkEditor.Organization },
        onBack = onBack,
        state = state,
        onErrorShown = viewModel::onErrorShown,
    ) {
        LazyColumn(modifier = Modifier.fillMaxSize().testTag("contact-work-list")) {
            if (organizations.isEmpty() && titles.isEmpty() && workInformation.isNullOrBlank()) {
                item {
                    EmptyState(
                        title = stringResource(R.string.contact_work_empty),
                        body = stringResource(R.string.contact_work_empty_body),
                        actionLabel = stringResource(R.string.contact_work_add_organization),
                        onAction = { editor = WorkEditor.Organization },
                    )
                }
            }
            if (showOrganizations) {
                item(key = "org-header") {
                    SectionHeading(stringResource(R.string.contact_work_section_organizations)) {
                        AddTextButton(
                            label = stringResource(R.string.contact_work_add_organization),
                            tag = "contact-work-add-organization",
                        ) { editor = WorkEditor.Organization }
                    }
                }
                items(organizations, key = { it.id ?: it.name.orEmpty() }) { org ->
                    val period = org.id?.let { id ->
                        record?.crm?.periods?.firstOrNull { it.kind == "organization" && it.entryId == id }
                    }
                    ConfirmableEntryRow(
                        title = org.name.orEmpty(),
                        subtitle = listOfNotNull(
                            org.units?.firstOrNull()?.name?.takeIf { it.isNotBlank() },
                            periodLabel(period?.range),
                        ).joinToString(" · "),
                        editTag = "organization-edit-${org.id}",
                        deleteTag = "organization-delete-${org.id}",
                        onEdit = { editingOrgId = org.id; editor = WorkEditor.Organization },
                        onDelete = { org.id?.let(viewModel::deleteOrganization) },
                    )
                }
            }
            if (showTitles) {
                item(key = "title-header") {
                    SectionHeading(stringResource(R.string.contact_work_section_titles)) {
                        AddTextButton(
                            label = stringResource(R.string.contact_work_add_title),
                            tag = "contact-work-add-title",
                        ) { editor = WorkEditor.Title }
                    }
                }
                items(titles, key = { it.id ?: "${it.kind}-${it.name}" }) { title ->
                    val period = title.id?.let { id ->
                        record?.crm?.periods?.firstOrNull { it.kind == "title" && it.entryId == id }
                    }
                    ConfirmableEntryRow(
                        title = title.name.orEmpty(),
                        subtitle = listOfNotNull(
                            stringResource(
                                if (title.kind == "role") R.string.contact_work_kind_role else R.string.contact_work_kind_title,
                            ),
                            periodLabel(period?.range),
                        ).joinToString(" · "),
                        editTag = "title-edit-${title.id}",
                        deleteTag = "title-delete-${title.id}",
                        onEdit = { editingTitleId = title.id; editor = WorkEditor.Title },
                        onDelete = { title.id?.let(viewModel::deleteTitle) },
                    )
                }
            }
            if (showWorkInformation) {
                item(key = "information-header") {
                    SectionHeading(stringResource(R.string.contact_work_section_information)) {
                        AddTextButton(
                            label = stringResource(R.string.contact_work_edit_information),
                            tag = "contact-work-edit-information",
                        ) { editor = WorkEditor.WorkInformation }
                    }
                }
                if (!workInformation.isNullOrBlank()) {
                    item(key = "information-value") {
                        Text(
                            text = workInformation,
                            style = MaterialTheme.typography.bodyLarge,
                            modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
                        )
                    }
                }
            }
            item { Box(modifier = Modifier.size(80.dp)) }
        }
    }
    ConflictDialogIfNeeded(state, viewModel)
}

private fun periodLabel(range: com.mycorrhizal.crm.model.network.TemporalRange?): String? {
    val start = range?.start?.year
    val end = range?.end?.year
    return when {
        start != null && end != null -> "$start – $end"
        start != null -> "$start –"
        end != null -> "– $end"
        else -> null
    }
}

@Composable
private fun PeriodFields(
    start: String,
    end: String,
    onStartChange: (String) -> Unit,
    onEndChange: (String) -> Unit,
) {
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
        OutlinedTextField(
            value = start,
            onValueChange = onStartChange,
            label = { Text(stringResource(R.string.contact_period_from)) },
            singleLine = true,
            modifier = Modifier.weight(1f).testTag("work-period-start"),
        )
        OutlinedTextField(
            value = end,
            onValueChange = onEndChange,
            label = { Text(stringResource(R.string.contact_period_to)) },
            singleLine = true,
            modifier = Modifier.weight(1f).testTag("work-period-end"),
        )
    }
}

@Composable
private fun OrganizationFormScreen(
    initial: Organization?,
    period: com.mycorrhizal.crm.model.network.EntryPeriod?,
    isSaving: Boolean,
    onSave: (OrganizationFormData) -> Unit,
    onBack: () -> Unit,
) {
    var name by rememberSaveable(initial?.id) { mutableStateOf(initial?.name.orEmpty()) }
    var department by rememberSaveable(initial?.id) { mutableStateOf(initial?.units?.firstOrNull()?.name.orEmpty()) }
    var start by rememberSaveable(initial?.id) { mutableStateOf(period?.range?.start?.year?.toString().orEmpty()) }
    var end by rememberSaveable(initial?.id) { mutableStateOf(period?.range?.end?.year?.toString().orEmpty()) }
    val snackbar = remember { SnackbarHostState() }
    FormScaffold(
        title = stringResource(if (initial != null) R.string.contact_work_edit_organization else R.string.contact_work_add_organization),
        onBack = onBack,
        saveLabel = stringResource(if (initial != null) R.string.action_save else R.string.action_create),
        isSaving = isSaving,
        onSave = { onSave(OrganizationFormData(name = name, department = department, startYear = start, endYear = end)) },
        snackbarHostState = snackbar,
        saveEnabled = name.isNotBlank(),
        saveDisabledHint = stringResource(R.string.contact_section_value_required),
    ) { padding ->
        Column(
            modifier = Modifier.padding(padding).fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            OutlinedTextField(
                value = name,
                onValueChange = { name = it },
                label = { Text(stringResource(R.string.contact_work_organization_name)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth().testTag("organization-name"),
            )
            OutlinedTextField(
                value = department,
                onValueChange = { department = it },
                label = { Text(stringResource(R.string.contact_work_department_optional)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth().testTag("organization-department"),
            )
            PeriodFields(start, end, { start = it }, { end = it })
        }
    }
}

@Composable
private fun TitleFormScreen(
    initial: Title?,
    period: com.mycorrhizal.crm.model.network.EntryPeriod?,
    isSaving: Boolean,
    onSave: (TitleFormData) -> Unit,
    onBack: () -> Unit,
) {
    var name by rememberSaveable(initial?.id) { mutableStateOf(initial?.name.orEmpty()) }
    var kind by rememberSaveable(initial?.id) { mutableStateOf(initial?.kind?.takeIf { it == "role" } ?: "title") }
    var start by rememberSaveable(initial?.id) { mutableStateOf(period?.range?.start?.year?.toString().orEmpty()) }
    var end by rememberSaveable(initial?.id) { mutableStateOf(period?.range?.end?.year?.toString().orEmpty()) }
    val snackbar = remember { SnackbarHostState() }
    FormScaffold(
        title = stringResource(if (initial != null) R.string.contact_work_edit_title else R.string.contact_work_add_title),
        onBack = onBack,
        saveLabel = stringResource(if (initial != null) R.string.action_save else R.string.action_create),
        isSaving = isSaving,
        onSave = { onSave(TitleFormData(name = name, kind = kind, startYear = start, endYear = end)) },
        snackbarHostState = snackbar,
        saveEnabled = name.isNotBlank(),
        saveDisabledHint = stringResource(R.string.contact_section_value_required),
    ) { padding ->
        Column(
            modifier = Modifier.padding(padding).fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            LabelledDropdown(
                label = stringResource(R.string.contact_work_title_kind),
                valueText = stringResource(
                    if (kind == "role") R.string.contact_work_kind_role else R.string.contact_work_kind_title,
                ),
                options = listOf(
                    DropdownOption("title", stringResource(R.string.contact_work_kind_title)),
                    DropdownOption("role", stringResource(R.string.contact_work_kind_role)),
                ),
                onSelect = { kind = it },
                modifier = Modifier.testTag("title-kind"),
            )
            OutlinedTextField(
                value = name,
                onValueChange = { name = it },
                label = { Text(stringResource(R.string.contact_work_title_name)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth().testTag("title-name"),
            )
            PeriodFields(start, end, { start = it }, { end = it })
        }
    }
}

@Composable
private fun WorkInformationFormScreen(
    initial: String,
    isSaving: Boolean,
    onSave: (String) -> Unit,
    onBack: () -> Unit,
) {
    var value by rememberSaveable { mutableStateOf(initial) }
    val snackbar = remember { SnackbarHostState() }
    FormScaffold(
        title = stringResource(R.string.contact_work_edit_information),
        onBack = onBack,
        saveLabel = stringResource(R.string.action_save),
        isSaving = isSaving,
        onSave = { onSave(value) },
        snackbarHostState = snackbar,
    ) { padding ->
        Column(
            modifier = Modifier.padding(padding).fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            OutlinedTextField(
                value = value,
                onValueChange = { value = it },
                label = { Text(stringResource(R.string.contact_work_section_information)) },
                supportingText = { Text(stringResource(R.string.contact_work_information_hint)) },
                minLines = 3,
                modifier = Modifier.fillMaxWidth().testTag("work-information"),
            )
        }
    }
}

// ---------------------------------------------------------------------------
// Anniversaries (non-birth Card.anniversaries)
// ---------------------------------------------------------------------------

@Composable
fun ContactAnniversariesScreen(
    onBack: () -> Unit,
    viewModel: ContactSectionsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    var editingId by rememberSaveable { mutableStateOf<String?>(null) }
    var adding by rememberSaveable { mutableStateOf(false) }
    val entries = state.record?.nonBirthAnniversaries().orEmpty()
    val editing = editingId?.let { id -> entries.firstOrNull { it.id == id } }

    if (adding || editing != null) {
        AnniversaryFormScreen(
            initial = editing,
            isSaving = state.isSaving,
            onSave = {
                viewModel.saveAnniversary(editing?.id, it)
                adding = false
                editingId = null
            },
            onBack = { adding = false; editingId = null },
        )
        ConflictDialogIfNeeded(state, viewModel)
        return
    }

    SectionListScaffold(
        title = stringResource(R.string.contact_anniversaries),
        addLabel = stringResource(R.string.contact_anniversary_add),
        onAdd = { adding = true },
        onBack = onBack,
        state = state,
        onErrorShown = viewModel::onErrorShown,
    ) {
        LazyColumn(modifier = Modifier.fillMaxSize().testTag("contact-anniversaries-list")) {
            if (entries.isEmpty()) {
                item {
                    EmptyState(
                        title = stringResource(R.string.contact_anniversaries_empty),
                        body = stringResource(R.string.contact_anniversaries_empty_body),
                        actionLabel = stringResource(R.string.contact_anniversary_add),
                        onAction = { adding = true },
                    )
                }
            } else {
                items(entries, key = { it.id ?: "${it.kind}-${it.date?.partial}" }) { anniversary ->
                    val id = anniversary.id.orEmpty()
                    ConfirmableEntryRow(
                        title = stringResource(anniversaryKindLabelRes(anniversary)),
                        subtitle = anniversary.date?.partial?.let { DateFieldValue.partialFieldValue(it) }.orEmpty(),
                        editTag = "anniversary-edit-$id",
                        deleteTag = "anniversary-delete-$id",
                        onEdit = { editingId = anniversary.id },
                        onDelete = { anniversary.id?.let(viewModel::deleteAnniversary) },
                    )
                }
            }
            item { Box(modifier = Modifier.size(80.dp)) }
        }
    }
    ConflictDialogIfNeeded(state, viewModel)
}

@StringRes
private fun anniversaryKindLabelRes(anniversary: Anniversary): Int =
    if (anniversary.kind == "death") R.string.contact_anniversary_kind_death else R.string.contact_anniversary_kind_wedding

@Composable
private fun AnniversaryFormScreen(
    initial: Anniversary?,
    isSaving: Boolean,
    onSave: (AnniversaryFormData) -> Unit,
    onBack: () -> Unit,
) {
    var kind by rememberSaveable(initial?.id) {
        mutableStateOf(initial?.kind?.takeIf { it in AnniversaryKinds } ?: "wedding")
    }
    var date by rememberSaveable(initial?.id) {
        mutableStateOf(DateFieldValue.partialFieldValue(initial?.date?.partial))
    }
    val snackbar = remember { SnackbarHostState() }
    FormScaffold(
        title = stringResource(if (initial != null) R.string.contact_anniversary_edit else R.string.contact_anniversary_add),
        onBack = onBack,
        saveLabel = stringResource(if (initial != null) R.string.action_save else R.string.action_create),
        isSaving = isSaving,
        onSave = { onSave(AnniversaryFormData(kind = kind, date = date)) },
        snackbarHostState = snackbar,
        saveEnabled = date.isNotBlank(),
        saveDisabledHint = stringResource(R.string.contact_section_date_required),
    ) { padding ->
        Column(
            modifier = Modifier.padding(padding).fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            LabelledDropdown(
                label = stringResource(R.string.contact_anniversary_kind),
                valueText = stringResource(
                    if (kind == "death") R.string.contact_anniversary_kind_death else R.string.contact_anniversary_kind_wedding,
                ),
                options = AnniversaryKinds.map {
                    DropdownOption(
                        it,
                        stringResource(
                            if (it == "death") R.string.contact_anniversary_kind_death else R.string.contact_anniversary_kind_wedding,
                        ),
                    )
                },
                onSelect = { kind = it },
                modifier = Modifier.testTag("anniversary-kind"),
            )
            DateField(
                value = date,
                onValueChange = { date = it },
                label = stringResource(R.string.contact_anniversary_date),
                mode = DateFieldMode.PARTIAL,
                allowClear = true,
                testTag = "anniversary-date",
                modifier = Modifier.fillMaxWidth(),
            )
        }
    }
}
