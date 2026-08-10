package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.automirrored.outlined.ArrowForward
import androidx.compose.material.icons.outlined.Call
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.Email
import androidx.compose.material.icons.outlined.Map
import androidx.compose.material.icons.outlined.Message
import androidx.compose.material.icons.outlined.OpenInNew
import androidx.compose.material.icons.outlined.Person
import androidx.compose.material.icons.outlined.Videocam
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import coil3.compose.AsyncImage
import com.mycorrhizal.crm.model.network.Address
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.model.util.DateFormat.display
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.theme.AppTypography
import com.mycorrhizal.crm.feature.timeline.TimelineSection
import com.mycorrhizal.crm.feature.timeline.toTimelineItems
import com.mycorrhizal.crm.ui.R

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContactDetailScreen(
    onBack: () -> Unit,
    onEdit: (Int) -> Unit = {},
    onViewActivities: (Int) -> Unit = {},
    onViewNotes: (Int) -> Unit = {},
    onViewReminders: (Int) -> Unit = {},
    onEditActivity: (Int) -> Unit = {},
    onEditNote: (Int) -> Unit = {},
    onEditReminder: (Int) -> Unit = {},
    viewModel: ContactDetailViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()

    // Reload when returning from the edit form so the detail shows saved changes.
    val lifecycleOwner = LocalLifecycleOwner.current
    DisposableEffect(lifecycleOwner) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) viewModel.load()
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
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
                        text = state.contact?.card?.name?.full ?: "Contact",
                        style = MaterialTheme.typography.titleLarge,
                    )
                },
                actions = {
                    state.contact?.let { contact ->
                        IconButton(onClick = { onEdit(contact.id) }) {
                            Icon(Icons.Outlined.Edit, contentDescription = stringResource(R.string.contact_edit))
                        }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.surface,
                ),
            )
        },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading -> LoadingSkeleton()
                state.contact == null && (state.errorRes != null || state.error != null) ->
                    EmptyState(state.errorRes?.let { stringResource(it) } ?: state.error.orEmpty())
                state.contact == null -> EmptyState("Contact not found")
                else -> ContactDetailContent(
                    contact = state.contact!!,
                    onViewActivities = onViewActivities,
                    onViewNotes = onViewNotes,
                    onViewReminders = onViewReminders,
                    onEditActivity = onEditActivity,
                    onEditNote = onEditNote,
                    onEditReminder = onEditReminder,
                    onCompleteReminder = viewModel::completeReminder,
                )
            }
        }
    }
}

@Composable
fun ContactDetailContent(
    contact: ContactRecordResponse,
    onViewActivities: (Int) -> Unit = {},
    onViewNotes: (Int) -> Unit = {},
    onViewReminders: (Int) -> Unit = {},
    onEditActivity: (Int) -> Unit = {},
    onEditNote: (Int) -> Unit = {},
    onEditReminder: (Int) -> Unit = {},
    onCompleteReminder: (Int) -> Unit = {},
) {
    val card = contact.card
    LazyColumn(modifier = Modifier.fillMaxSize().testTag("contact-detail-list")) {
        item {
            ContactHeader(contact = contact, card = card)
        }
        item {
            // Unified timeline: the contact's activities/notes/reminders merged
            // newest-first (Phase 2 item 10). Tapping a row routes to its edit form.
            SectionTitle("Timeline")
            TimelineSection(
                items = contact.toTimelineItems(),
                onEditActivity = onEditActivity,
                onEditNote = onEditNote,
                onEditReminder = onEditReminder,
                onCompleteReminder = onCompleteReminder,
            )
        }
        item {
            // Entry points to the per-type list screens (full management view).
            androidx.compose.material3.ListItem(
                headlineContent = { Text(stringResource(R.string.contact_activities), style = MaterialTheme.typography.bodyLarge) },
                trailingContent = {
                    Icon(
                        imageVector = Icons.AutoMirrored.Outlined.ArrowForward,
                        contentDescription = null,
                    )
                },
                modifier = Modifier.fillMaxWidth().clickable { onViewActivities(contact.id) },
            )
            androidx.compose.material3.ListItem(
                headlineContent = { Text(stringResource(R.string.contact_notes), style = MaterialTheme.typography.bodyLarge) },
                trailingContent = {
                    Icon(
                        imageVector = Icons.AutoMirrored.Outlined.ArrowForward,
                        contentDescription = null,
                    )
                },
                modifier = Modifier.fillMaxWidth().clickable { onViewNotes(contact.id) },
            )
            androidx.compose.material3.ListItem(
                headlineContent = { Text(stringResource(R.string.contact_reminders), style = MaterialTheme.typography.bodyLarge) },
                trailingContent = {
                    Icon(
                        imageVector = Icons.AutoMirrored.Outlined.ArrowForward,
                        contentDescription = null,
                    )
                },
                modifier = Modifier.fillMaxWidth().clickable { onViewReminders(contact.id) },
            )
        }
        if (!card?.emails.isNullOrEmpty()) {
            item {
                SectionTitle("Email")
                card?.emails?.forEach { EmailRow(it) }
            }
        }
        if (!card?.phones.isNullOrEmpty()) {
            item {
                SectionTitle("Phone")
                card?.phones?.forEach { PhoneRow(it) }
            }
        }
        if (!card?.addresses.isNullOrEmpty()) {
            item {
                SectionTitle("Address")
                card?.addresses?.forEach { AddressRow(it) }
            }
        }
        if (!card?.links.isNullOrEmpty()) {
            item {
                SectionTitle("Links")
                card?.links?.forEach { link ->
                    val uri = link.uri.orEmpty()
                    LinkRow(
                        uri = uri,
                        label = link.label ?: uri,
                        linkType = MobileLinkRegistry.forProtocol(link.label),
                    )
                }
            }
        }
        if (!card?.organizations.isNullOrEmpty()) {
            item {
                SectionTitle("Organization")
                card?.organizations?.forEach { org ->
                    org.name?.let { InfoRow(it) }
                }
            }
        }
        if (!card?.notes.isNullOrEmpty()) {
            item {
                SectionTitle("Notes")
                card?.notes?.forEach { note ->
                    note.note?.let { InfoRow(it) }
                }
            }
        }
        if (!card?.personalInfo.isNullOrEmpty()) {
            item {
                SectionTitle("Personal information")
                card?.personalInfo?.forEach { info ->
                    InfoRow("${info.kind.orEmpty()}: ${info.value.orEmpty()}")
                }
            }
        }
        if (!contact.crm?.circles.isNullOrEmpty()) {
            item {
                SectionTitle("Circles")
                InfoRow(contact.crm?.circles?.joinToString(", ").orEmpty())
            }
        }
        item { Box(modifier = Modifier.size(32.dp)) }
    }
}

@Composable
private fun ContactHeader(contact: ContactRecordResponse, card: Card?) {
    Column(
        modifier = Modifier.fillMaxWidth().padding(24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        val photo = contact.photoThumbnail
        if (photo != null && photo.startsWith("data:")) {
            AsyncImage(
                model = photo,
                contentDescription = "Photo of ${card?.name?.full.orEmpty()}",
                contentScale = ContentScale.Crop,
                modifier = Modifier.size(96.dp).clip(CircleShape),
            )
        } else {
            Icon(
                imageVector = Icons.Outlined.Person,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.size(96.dp),
            )
        }
        val displayName = card?.name?.full
            ?: listOfNotNull(card?.name?.components?.firstOrNull { it.kind == "given" }?.value)
                .joinToString(" ")
                .ifBlank { "Contact" }
        Text(
            text = displayName,
            style = MaterialTheme.typography.titleLarge,
            textAlign = TextAlign.Center,
        )
        card?.nicknames?.firstOrNull()?.name?.let { nickname ->
            Text(
                text = "\"$nickname\"",
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        card?.anniversaries?.firstOrNull()?.let { anniversary ->
            val partial = anniversary.date?.partial
            if (partial != null) {
                Text(
                    text = "Birthday: ${partial.display("eu")}",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun SectionTitle(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.labelLarge,
        color = MaterialTheme.colorScheme.primary,
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
    )
}

@Composable
private fun InfoRow(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.bodyLarge,
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 4.dp),
    )
}

@Composable
private fun EmailRow(email: Email) {
    val context = LocalContext.current
    val address = email.address.orEmpty()
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = address,
            style = AppTypography.mono,
            modifier = Modifier.weight(1f),
        )
        email.label?.let { Text(it, color = MaterialTheme.colorScheme.onSurfaceVariant) }
        if (address.isNotBlank()) {
            IconButton(onClick = { context.startActivity(FieldActions.emailIntent(address)) }) {
                Icon(Icons.Outlined.Email, contentDescription = stringResource(R.string.cd_compose_email))
            }
            IconButton(onClick = { FieldActions.copyText(context, "email", address) }) {
                Icon(Icons.Outlined.ContentCopy, contentDescription = stringResource(R.string.cd_copy_email))
            }
        }
    }
}

@Composable
private fun PhoneRow(phone: Phone) {
    val context = LocalContext.current
    val number = phone.number.orEmpty()
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = number,
            style = AppTypography.mono,
            modifier = Modifier.weight(1f),
        )
        val features = phone.features?.joinToString(", ").orEmpty()
        if (features.isNotBlank()) {
            Text(features, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        if (number.isNotBlank()) {
            IconButton(onClick = { context.startActivity(FieldActions.dialIntent(number)) }) {
                Icon(Icons.Outlined.Call, contentDescription = stringResource(R.string.cd_call))
            }
            // SMS only for mobile numbers (T34: phone feature detection).
            if (phone.features?.contains("cell") == true) {
                IconButton(onClick = { context.startActivity(FieldActions.smsIntent(number)) }) {
                    Icon(Icons.Outlined.Message, contentDescription = stringResource(R.string.cd_text))
                }
            }
            IconButton(onClick = { FieldActions.copyText(context, "phone", number) }) {
                Icon(Icons.Outlined.ContentCopy, contentDescription = stringResource(R.string.cd_copy_phone))
            }
        }
    }
}

@Composable
private fun AddressRow(address: Address) {
    val context = LocalContext.current
    val text = address.full
        ?: address.components?.joinToString(", ") { it.value.orEmpty() }
        ?: ""
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = text,
            style = MaterialTheme.typography.bodyLarge,
            modifier = Modifier.weight(1f),
        )
        if (text.isNotBlank()) {
            IconButton(onClick = { context.startActivity(FieldActions.mapIntent(text)) }) {
                Icon(Icons.Outlined.Map, contentDescription = stringResource(R.string.cd_open_maps))
            }
            IconButton(onClick = { FieldActions.copyText(context, "address", text) }) {
                Icon(Icons.Outlined.ContentCopy, contentDescription = stringResource(R.string.cd_copy_address))
            }
        }
    }
}

@Composable
private fun LinkRow(
    uri: String,
    label: String,
    linkType: MobileLinkType?,
) {
    val context = LocalContext.current
    // Enrichment (ticket §7.6): for a known protocol, query the device's
    // ContactsContract.Data for the app's MIMETYPEs and show only the actions
    // whose apps are installed. This requires READ_CONTACTS at runtime; without
    // it (or without a matching app) the row degrades to the web app's plain
    // link + copy.
    var availableActions by remember { mutableStateOf<List<MobileLinkAction>>(emptyList()) }
    LaunchedEffect(linkType, uri) {
        if (linkType == null) {
            availableActions = emptyList()
        } else {
            availableActions = try {
                val resolver = MobileLinkActionResolver(context.contentResolver)
                resolver.resolveAvailableActions(linkType, uri)
            } catch (_: SecurityException) {
                emptyList()
            }
        }
    }

    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp)) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Text(
                text = label,
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.primary,
                maxLines = 1,
                overflow = androidx.compose.ui.text.style.TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f),
            )
            if (uri.isNotBlank()) {
                IconButton(onClick = { context.startActivity(FieldActions.browserIntent(uri)) }) {
                    Icon(Icons.Outlined.OpenInNew, contentDescription = stringResource(R.string.cd_open_link))
                }
                IconButton(onClick = { FieldActions.copyText(context, "url", uri) }) {
                    Icon(Icons.Outlined.ContentCopy, contentDescription = stringResource(R.string.cd_copy_link))
                }
            }
        }
        if (availableActions.isNotEmpty()) {
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.padding(top = 4.dp),
            ) {
                availableActions.forEach { action ->
                    AssistChip(
                        onClick = {
                            try {
                                context.startActivity(action.intentBuilder(uri))
                            } catch (_: Exception) {
                                // No handler for the deep link — fall back to the browser.
                                context.startActivity(FieldActions.browserIntent(uri))
                            }
                        },
                        label = { Text(action.label) },
                        leadingIcon = {
                            Icon(
                                imageVector = when (action.kind) {
                                    MobileActionKind.MESSAGE -> Icons.Outlined.Message
                                    MobileActionKind.VOICE_CALL -> Icons.Outlined.Call
                                    MobileActionKind.VIDEO_CALL -> Icons.Outlined.Videocam
                                    MobileActionKind.APP_OPEN -> Icons.Outlined.OpenInNew
                                    MobileActionKind.APP_CALL -> Icons.Outlined.Call
                                },
                                contentDescription = action.label,
                                modifier = Modifier.size(18.dp),
                            )
                        },
                        modifier = Modifier.height(32.dp),
                    )
                }
            }
        }
    }
}
