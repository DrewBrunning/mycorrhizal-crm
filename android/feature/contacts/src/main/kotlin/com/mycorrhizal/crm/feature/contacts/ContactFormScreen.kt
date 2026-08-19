@file:OptIn(ExperimentalComposeUiApi::class)

package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.KeyboardArrowDown
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.InputChip
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
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.autofill.AutofillType
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.model.network.Tag
import com.mycorrhizal.crm.ui.components.AddressEditor
import com.mycorrhizal.crm.ui.components.AutofillOutlinedTextField
import com.mycorrhizal.crm.ui.components.EmailSpec
import com.mycorrhizal.crm.ui.components.LinkSpec
import com.mycorrhizal.crm.ui.components.MultiValueEditor
import com.mycorrhizal.crm.ui.components.OnlineServiceSpec
import com.mycorrhizal.crm.ui.components.PersonalInfoSpec
import com.mycorrhizal.crm.ui.components.PhoneSpec
import com.mycorrhizal.crm.ui.components.TitleSpec
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
            else ->             ContactFormContent(
                modifier = Modifier.padding(padding),
                state = state,
                onGivenNameChange = viewModel::onGivenNameChange,
                onSurnameChange = viewModel::onSurnameChange,
                onPrefixChange = viewModel::onPrefixChange,
                onMiddleNameChange = viewModel::onMiddleNameChange,
                onSuffixChange = viewModel::onSuffixChange,
                onNicknameChange = viewModel::onNicknameChange,
                onKindChange = viewModel::onKindChange,
                onLanguageChange = viewModel::onLanguageChange,
                onEmailsChange = viewModel::onEmailsChange,
                onPhonesChange = viewModel::onPhonesChange,
                onAddressesChange = viewModel::onAddressesChange,
                onTitlesChange = viewModel::onTitlesChange,
                onImppChange = viewModel::onImppChange,
                onSocialChange = viewModel::onSocialChange,
                onOtherServicesChange = viewModel::onOtherServicesChange,
                onLinksChange = viewModel::onLinksChange,
                onPersonalInfoChange = viewModel::onPersonalInfoChange,
                onOrganizationNameChange = viewModel::onOrganizationNameChange,
                onDepartmentChange = viewModel::onDepartmentChange,
                onHowWeMetChange = viewModel::onHowWeMetChange,
                onWorkInformationChange = viewModel::onWorkInformationChange,
                onContactInformationChange = viewModel::onContactInformationChange,
                onBirthdayChange = viewModel::onBirthdayChange,
                onNotesChange = viewModel::onNotesChange,
                onCircleToggle = viewModel::onCircleToggle,
                onTagToggle = viewModel::onTagToggle,
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
@OptIn(androidx.compose.foundation.layout.ExperimentalLayoutApi::class)
fun ContactFormContent(
    state: ContactFormState,
    onGivenNameChange: (String) -> Unit,
    onSurnameChange: (String) -> Unit,
    onPrefixChange: (String) -> Unit = {},
    onMiddleNameChange: (String) -> Unit = {},
    onSuffixChange: (String) -> Unit = {},
    onNicknameChange: (String) -> Unit,
    onKindChange: (String) -> Unit = {},
    onLanguageChange: (String) -> Unit = {},
    onEmailsChange: (List<Email>) -> Unit,
    onPhonesChange: (List<Phone>) -> Unit,
    onAddressesChange: (List<com.mycorrhizal.crm.model.network.Address>) -> Unit,
    onTitlesChange: (List<com.mycorrhizal.crm.model.network.Title>) -> Unit,
    onImppChange: (List<com.mycorrhizal.crm.model.network.OnlineService>) -> Unit,
    onSocialChange: (List<com.mycorrhizal.crm.model.network.OnlineService>) -> Unit,
    onOtherServicesChange: (List<com.mycorrhizal.crm.model.network.OnlineService>) -> Unit,
    onLinksChange: (List<com.mycorrhizal.crm.model.network.Resource>) -> Unit,
    onPersonalInfoChange: (List<com.mycorrhizal.crm.model.network.PersonalInfo>) -> Unit,
    onOrganizationNameChange: (String) -> Unit = {},
    onDepartmentChange: (String) -> Unit = {},
    onHowWeMetChange: (String) -> Unit = {},
    onWorkInformationChange: (String) -> Unit = {},
    onContactInformationChange: (String) -> Unit = {},
    onBirthdayChange: (String) -> Unit,
    onNotesChange: (String) -> Unit,
    onCircleToggle: (String) -> Unit = {},
    onTagToggle: (String) -> Unit = {},
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
        // T115: the name fields advertise their AutofillType so the Android
        // Autofill service can offer a fill (Google/device address book).
        AutofillOutlinedTextField(
            value = state.prefix,
            onValueChange = onPrefixChange,
            label = stringResource(R.string.contact_prefix),
            autofillType = AutofillType.PersonNamePrefix,
        )
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            AutofillOutlinedTextField(
                value = state.givenName,
                onValueChange = onGivenNameChange,
                label = stringResource(R.string.contact_given_name),
                autofillType = AutofillType.PersonFirstName,
                modifier = Modifier.weight(1f),
            )
            AutofillOutlinedTextField(
                value = state.surname,
                onValueChange = onSurnameChange,
                label = stringResource(R.string.contact_surname),
                autofillType = AutofillType.PersonLastName,
                modifier = Modifier.weight(1f),
            )
        }
        AutofillOutlinedTextField(
            value = state.middleName,
            onValueChange = onMiddleNameChange,
            label = stringResource(R.string.contact_middle_name),
            autofillType = AutofillType.PersonMiddleName,
        )
        AutofillOutlinedTextField(
            value = state.suffix,
            onValueChange = onSuffixChange,
            label = stringResource(R.string.contact_suffix),
            autofillType = AutofillType.PersonNameSuffix,
        )
        OutlinedTextField(
            value = state.nickname,
            onValueChange = onNicknameChange,
            label = { Text(stringResource(R.string.contact_nickname)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = state.nickname,
            onValueChange = onNicknameChange,
            label = { Text(stringResource(R.string.contact_nickname)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )

        // M24: kind (human/animal) — the backend defaults to human; this makes it explicit.
        var kindMenuExpanded by remember { mutableStateOf(false) }
        Box(modifier = Modifier.fillMaxWidth()) {
            OutlinedTextField(
                value = if (state.kind == ContactFormState.KIND_ANIMAL) {
                    stringResource(R.string.contact_kind_animal)
                } else {
                    stringResource(R.string.contact_kind_human)
                },
                onValueChange = {},
                readOnly = true,
                label = { Text(stringResource(R.string.contact_kind)) },
                modifier = Modifier.fillMaxWidth(),
                trailingIcon = {
                    IconButton(onClick = { kindMenuExpanded = true }) {
                        Icon(Icons.Outlined.KeyboardArrowDown, contentDescription = null)
                    }
                },
            )
            DropdownMenu(
                expanded = kindMenuExpanded,
                onDismissRequest = { kindMenuExpanded = false },
            ) {
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.contact_kind_human)) },
                    onClick = {
                        kindMenuExpanded = false
                        onKindChange(ContactFormState.KIND_HUMAN)
                    },
                )
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.contact_kind_animal)) },
                    onClick = {
                        kindMenuExpanded = false
                        onKindChange(ContactFormState.KIND_ANIMAL)
                    },
                )
            }
        }

        // M24: default language tag (web's LanguageField is a full picker; a text field is the
        // pragmatic mobile equivalent — the backend stores any RFC 9554 tag unvalidated).
        OutlinedTextField(
            value = state.language,
            onValueChange = onLanguageChange,
            label = { Text(stringResource(R.string.contact_language)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )

        MultiValueEditor(
            items = state.emails,
            spec = EmailSpec,
            onChange = onEmailsChange,
            label = stringResource(R.string.contact_email),
        )

        MultiValueEditor(
            items = state.phones,
            spec = PhoneSpec,
            onChange = onPhonesChange,
            label = stringResource(R.string.contact_phone),
        )

        // M7 Tier 1: addresses get their own editor (components[], not a scalar).
        SectionLabel(stringResource(R.string.contact_address))
        AddressEditor(
            addresses = state.addresses,
            onChange = onAddressesChange,
        )

        // M7 Tier 1: organization + department are plain fields (web parity — only the
        // first organization is surfaced), edited onto organizations[0] on save.
        SectionLabel(stringResource(R.string.contact_organization))
        OutlinedTextField(
            value = state.organizationName,
            onValueChange = onOrganizationNameChange,
            label = { Text(stringResource(R.string.contact_organization)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = state.department,
            onValueChange = onDepartmentChange,
            label = { Text(stringResource(R.string.contact_department)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )

        MultiValueEditor(
            items = state.titles,
            spec = TitleSpec,
            onChange = onTitlesChange,
            label = stringResource(R.string.contact_job_titles),
        )

        // M7 Tier 1: online services. The detail screen resolves handles via `service`
        // (MobileLinkRegistry) — the editor's spec edits the uri and label only, so a
        // loaded row's `service` rides along untouched and the resolved chips keep working.
        SectionLabel(stringResource(R.string.contact_online_services))
        MultiValueEditor(
            items = state.imppAddresses,
            spec = OnlineServiceSpec,
            onChange = onImppChange,
            label = stringResource(R.string.contact_impps),
        )
        MultiValueEditor(
            items = state.socialProfiles,
            spec = OnlineServiceSpec,
            onChange = onSocialChange,
            label = stringResource(R.string.contact_social_profiles),
        )
        MultiValueEditor(
            items = state.otherOnlineServices,
            spec = OnlineServiceSpec,
            onChange = onOtherServicesChange,
            label = stringResource(R.string.contact_other_online_services),
        )

        MultiValueEditor(
            items = state.links,
            spec = LinkSpec,
            onChange = onLinksChange,
            label = stringResource(R.string.contact_links),
        )

        MultiValueEditor(
            items = state.personalInfo,
            spec = PersonalInfoSpec,
            onChange = onPersonalInfoChange,
            label = stringResource(R.string.contact_personal_info),
        )

        OutlinedTextField(
            value = state.birthday,
            onValueChange = onBirthdayChange,
            label = { Text(stringResource(R.string.contact_birthday)) },
            placeholder = { Text(stringResource(R.string.contact_birthday_hint)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )

        // M7 Tier 3: CRM-envelope strings that appeared in neither the old form nor detail.
        OutlinedTextField(
            value = state.howWeMet,
            onValueChange = onHowWeMetChange,
            label = { Text(stringResource(R.string.contact_how_we_met)) },
            minLines = 2,
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = state.workInformation,
            onValueChange = onWorkInformationChange,
            label = { Text(stringResource(R.string.contact_work_information)) },
            minLines = 2,
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = state.contactInformation,
            onValueChange = onContactInformationChange,
            label = { Text(stringResource(R.string.contact_contact_information)) },
            minLines = 2,
            modifier = Modifier.fillMaxWidth(),
        )

        OutlinedTextField(
            value = state.notes,
            onValueChange = onNotesChange,
            label = { Text(stringResource(R.string.nav_notes)) },
            minLines = 3,
            modifier = Modifier.fillMaxWidth(),
        )

        // M24: circles — an autocomplete of existing circles, not the old free-text
        // comma-separated field. Selected circles become real CircleMember rows on save.
        SelectorChipEditor(
            label = stringResource(R.string.contact_circles),
            selected = state.circles,
            available = state.allCircles.map { it.name },
            selectLabel = stringResource(R.string.contact_circles_select),
            emptyText = stringResource(R.string.contact_circles_empty),
            onToggle = onCircleToggle,
        )

        // M24: tags — entirely absent from the form before.
        SelectorChipEditor(
            label = stringResource(R.string.contact_tags),
            selected = state.tags,
            available = state.allTags.map { it.name },
            selectLabel = stringResource(R.string.contact_tags_select),
            emptyText = stringResource(R.string.contact_tags_empty),
            onToggle = onTagToggle,
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

/**
 * A chip selector: selected entries render as removable chips, an add dropdown lists the
 * unselected ones. M24's replacement for the free-text comma-separated field — selection is
 * always from the existing set (no free text), mirroring web's AddContactDialog selectors.
 */@Composable
@OptIn(androidx.compose.foundation.layout.ExperimentalLayoutApi::class)
private fun SelectorChipEditor(
    label: String,
    selected: List<String>,
    available: List<String>,
    selectLabel: String,
    emptyText: String,
    onToggle: (String) -> Unit,
) {
    SectionLabel(label)
    if (selected.isEmpty()) {
        Text(
            text = emptyText,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(vertical = 4.dp),
        )
    }
    FlowRow(
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        modifier = Modifier.fillMaxWidth(),
    ) {
        selected.forEach { name ->
            InputChip(
                selected = true,
                onClick = { onToggle(name) },
                label = { Text(name) },
                trailingIcon = {
                    Icon(
                        Icons.Outlined.Close,
                        contentDescription = stringResource(R.string.contact_remove),
                        modifier = Modifier.size(16.dp),
                    )
                },
            )
        }
    }
    val addable = available.filter { it !in selected }
    if (addable.isNotEmpty()) {
        var menuExpanded by remember { mutableStateOf(false) }
        Box(modifier = Modifier.padding(top = 4.dp)) {
            androidx.compose.material3.AssistChip(
                onClick = { menuExpanded = true },
                label = { Text(selectLabel) },
                leadingIcon = {
                    Icon(
                        Icons.Outlined.Add,
                        contentDescription = null,
                        modifier = Modifier.size(18.dp),
                    )
                },
                modifier = Modifier.height(32.dp),
            )
            DropdownMenu(
                expanded = menuExpanded,
                onDismissRequest = { menuExpanded = false },
            ) {
                addable.forEach { name ->
                    DropdownMenuItem(
                        text = { Text(name) },
                        onClick = {
                            menuExpanded = false
                            onToggle(name)
                        },
                    )
                }
            }
        }
    }
}
