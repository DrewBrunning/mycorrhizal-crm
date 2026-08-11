package com.mycorrhizal.crm.feature.contacts

import android.Manifest
import android.content.pm.PackageManager
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
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
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
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
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
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
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.lerp
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.res.stringResource
import androidx.core.content.ContextCompat
import androidx.core.view.WindowCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.model.network.Address
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.OnlineService
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.model.util.DateFormat
import com.mycorrhizal.crm.model.util.DateFormat.display
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.LocalDrawerOpen
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.theme.AppTypography
import com.mycorrhizal.crm.ui.theme.MycorrhizalFonts
import com.mycorrhizal.crm.feature.timeline.TimelineSection
import com.mycorrhizal.crm.feature.timeline.toTimelineItems
import com.mycorrhizal.crm.ui.R
import kotlinx.coroutines.launch

private fun androidx.compose.ui.graphics.Color.toArgbCompat(): Int =
    android.graphics.Color.argb(
        (alpha * 255).toInt(),
        (red * 255).toInt(),
        (green * 255).toInt(),
        (blue * 255).toInt(),
    )

/**
 * The scroll window over which the contact name cross-fades out of the content
 * and into the collapsing app bar.
 *
 * This spans the two previous behaviors. A 96dp snap threshold felt "too
 * aggressive" (the bar switched while the photo still dominated), and waiting
 * for the whole ~204dp header block (photo + name + nickname + birthday) to
 * leave felt "too delayed". Fading across 120–180dp puts the midpoint at
 * ~150dp — exactly where the name reaches the top edge (24dp top padding +
 * 120dp photo + 8dp spacing = 152dp) — so the transfer reads as seamless and
 * tracks the scroll gesture continuously like an iOS interactive transition.
 */
private val HeaderFadeStartDp = 120.dp
private val HeaderFadeEndDp = 180.dp

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContactDetailScreen(
    onBack: () -> Unit,
    onEdit: (Int) -> Unit = {},
    onViewActivities: (Int) -> Unit = {},
    onViewNotes: (Int) -> Unit = {},
    onViewReminders: (Int) -> Unit = {},
    onViewRelationships: (Int) -> Unit = {},
    onOpenInContacts: (String) -> Unit = {},
    onMerge: (Int) -> Unit = {},
    onViewLifeEvents: (Int) -> Unit = {},
    onViewGifts: (Int) -> Unit = {},
    onViewPreferences: (Int) -> Unit = {},
    onViewAgenda: (Int) -> Unit = {},
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

    // Drives the collapsing app bar: at the top the bar is transparent so the
    // large photo + name (the first list item) sit on the bone surface; as the
    // user scrolls, the name cross-fades out of the content and into the bar
    // while it turns green (Android Contacts / iOS-style interactive gesture —
    // the transition tracks the scroll position continuously, not a binary
    // show/hide).
    //
    // The fade spans the window between the two previous behaviors: it starts
    // at 120dp (where a 96dp threshold felt "too aggressive" — it switched
    // while the photo was still dominant) and completes at 180dp (where waiting
    // for the whole ~204dp header block felt "too delayed"). Midpoint is 150dp
    // — the point where the name reaches the top edge (24dp padding + 120dp
    // photo + 8dp spacing), so the transfer reads as seamless.
    val listState = rememberLazyListState()
    val collapseStartPx = with(LocalDensity.current) { HeaderFadeStartDp.toPx() }
    val collapseEndPx = with(LocalDensity.current) { HeaderFadeEndDp.toPx() }
    val collapseProgress by remember {
        derivedStateOf {
            if (listState.firstVisibleItemIndex > 0) {
                1f
            } else {
                ((listState.firstVisibleItemScrollOffset - collapseStartPx) /
                    (collapseEndPx - collapseStartPx)).coerceIn(0f, 1f)
            }
        }
    }
    // Icon color: dark over the bone surface at the top, white once the green
    // bar collapses in — lerped continuously so the tint follows the drag.
    val onSurface = MaterialTheme.colorScheme.onSurface
    val onPrimary = MaterialTheme.colorScheme.onPrimary
    val barIconColor = lerp(onSurface, onPrimary, collapseProgress)

    // The status bar follows the collapse: bone with dark icons at the top,
    // brand green with light icons once the bar collapses in (overrides the
    // theme's default green for this screen). isAppearanceLightStatusBars=true
    // means dark icons (for the light bone bar); false means white icons.
    // While the navigation drawer is open, the app-level scaffold owns the
    // status bar (parchment + dark icons), so this screen stays out of the way
    // and re-applies its own state when the drawer closes.
    val drawerOpen = LocalDrawerOpen.current
    val window = (LocalContext.current as android.app.Activity).window
    LaunchedEffect(drawerOpen) {
        if (drawerOpen) return@LaunchedEffect
        snapshotFlow { collapseProgress }
            .collect { progress ->
                val collapsedNow = progress > 0.5f
                window.statusBarColor = if (collapsedNow) {
                    com.mycorrhizal.crm.ui.theme.MycorrhizalColors.mycelium.toArgbCompat()
                } else {
                    com.mycorrhizal.crm.ui.theme.MycorrhizalColors.bone.toArgbCompat()
                }
                WindowCompat.getInsetsController(window, window.decorView)
                    .isAppearanceLightStatusBars = !collapsedNow
            }
    }
    // Restore the app-wide default (green + white icons) when leaving this
    // screen, so the always-green sub-screens (life events, gifts, ...) get
    // white status-bar icons rather than inheriting this screen's bone+dark.
    DisposableEffect(Unit) {
        onDispose {
            window.statusBarColor = com.mycorrhizal.crm.ui.theme.MycorrhizalColors.mycelium.toArgbCompat()
            WindowCompat.getInsetsController(window, window.decorView)
                .isAppearanceLightStatusBars = false
        }
    }

    Scaffold(
        topBar = {
            // Transparent at the top (the large header shows through); green
            // with a small avatar + name once collapsed.
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.cd_back),
                            tint = barIconColor,
                        )
                    }
                },
                title = {
                    // Always composed so it can cross-fade in with the scroll;
                    // the alpha tracks collapseProgress (0 = hidden over the
                    // bone surface, 1 = fully shown once the bar is green).
                    state.contact?.let { contact ->
                        val card = contact.card
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(12.dp),
                            modifier = Modifier.graphicsLayer { alpha = collapseProgress },
                        ) {
                            ContactAvatar(
                                photoUri = card?.photoUri ?: contact.photoThumbnail,
                                contentDescription = stringResource(
                                    R.string.contacts_photo_description,
                                    card?.displayName.orEmpty(),
                                ),
                                size = 36.dp,
                            )
                            Text(
                                text = card?.displayName ?: stringResource(R.string.contact_title_fallback),
                                style = MaterialTheme.typography.titleLarge,
                                color = barIconColor,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                            )
                        }
                    }
                },
                actions = {
                    state.contact?.let { contact ->
                        IconButton(onClick = { onEdit(contact.id) }) {
                            Icon(
                                Icons.Outlined.Edit,
                                contentDescription = stringResource(R.string.contact_edit),
                                tint = barIconColor,
                            )
                        }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    // Transparent at the top, brand green once collapsed —
                    // lerped continuously so the color follows the drag rather
                    // than snapping. The nav/title/action icons are tinted
                    // explicitly from the same continuous progress.
                    containerColor = lerp(Color.Transparent, MaterialTheme.colorScheme.primary, collapseProgress),
                    scrolledContainerColor = MaterialTheme.colorScheme.primary,
                    navigationIconContentColor = barIconColor,
                    titleContentColor = barIconColor,
                    actionIconContentColor = barIconColor,
                ),
            )
        },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading -> LoadingSkeleton()
                state.contact == null && (state.errorRes != null || state.error != null) ->
                    EmptyState(state.errorRes?.let { stringResource(it) } ?: state.error.orEmpty())
                state.contact == null -> EmptyState(stringResource(R.string.contact_not_found))
                else -> ContactDetailContent(
                    contact = state.contact!!,
                    listState = listState,
                    headerContentAlpha = 1f - collapseProgress,
                    deviceLookupKey = state.deviceLookupKey,
                    dateFormat = state.dateFormat,
                    onOpenInContacts = onOpenInContacts,
                    onViewActivities = onViewActivities,
                    onViewNotes = onViewNotes,
                    onViewReminders = onViewReminders,
                    onViewRelationships = onViewRelationships,
                    onMerge = onMerge,
                    onViewLifeEvents = onViewLifeEvents,
                    onViewGifts = onViewGifts,
                    onViewPreferences = onViewPreferences,
                    onViewAgenda = onViewAgenda,
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
@OptIn(ExperimentalMaterial3Api::class)
fun ContactDetailContent(
    contact: ContactRecordResponse,
    listState: LazyListState = rememberLazyListState(),
    /** 0→1 as the name fades out of the content into the collapsing app bar. */
    headerContentAlpha: Float = 1f,
    deviceLookupKey: String? = null,
    /** The signed-in user's `date_format` preference; falls back to "eu" when absent. */
    dateFormat: String? = null,
    onOpenInContacts: (String) -> Unit = {},
    onViewActivities: (Int) -> Unit = {},
    onViewNotes: (Int) -> Unit = {},
    onViewReminders: (Int) -> Unit = {},
    onViewRelationships: (Int) -> Unit = {},
    onMerge: (Int) -> Unit = {},
    onViewLifeEvents: (Int) -> Unit = {},
    onViewGifts: (Int) -> Unit = {},
    onViewPreferences: (Int) -> Unit = {},
    onViewAgenda: (Int) -> Unit = {},
    onEditActivity: (Int) -> Unit = {},
    onEditNote: (Int) -> Unit = {},
    onEditReminder: (Int) -> Unit = {},
    onCompleteReminder: (Int) -> Unit = {},
) {
    val card = contact.card
    LazyColumn(
        modifier = Modifier
            .fillMaxSize()
            .testTag("contact-detail-list"),
        state = listState,
    ) {
        item {
            // Large photo + name header on the bone surface. It sits under the
            // transparent app bar; scrolling collapses the app bar over it
            // (Android Contacts pattern).
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 24.dp, vertical = 24.dp)
                    // Cross-fade the large header out as the name transfers
                    // into the collapsing app bar (mirrors the bar-title alpha).
                    .graphicsLayer { alpha = headerContentAlpha },
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                ContactAvatar(
                    photoUri = card?.photoUri ?: contact.photoThumbnail,
                    contentDescription = stringResource(
                        R.string.contacts_photo_description,
                        card?.displayName.orEmpty(),
                    ),
                    size = 120.dp,
                )
                Text(
                    text = card?.displayName ?: stringResource(R.string.contact_title_fallback),
                    style = MaterialTheme.typography.titleLarge,
                    color = MaterialTheme.colorScheme.onSurface,
                    textAlign = TextAlign.Center,
                )
                val nickname = card?.nicknames?.firstOrNull()?.name
                val birthday = card?.anniversaries?.firstOrNull { it.kind == "birth" }?.date?.partial
                if (nickname != null) {
                    Text(
                        text = "\"$nickname\"",
                        style = MaterialTheme.typography.bodyLarge,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                if (birthday != null) {
                    Text(
                        text = stringResource(
                            R.string.contact_birthday_label,
                            birthday.display(dateFormat ?: DateFormat.EU),
                        ),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
        if (!card?.personalInfo.isNullOrEmpty()) {
            item {
                SectionCard(stringResource(R.string.contact_personal_info)) {
                    card?.personalInfo?.forEach { info ->
                        InfoRow("${info.kind.orEmpty()}: ${info.value.orEmpty()}")
                    }
                }
            }
        }
        if (!card?.phones.isNullOrEmpty()) {
            item {
                SectionCard(stringResource(R.string.contact_phone)) {
                    card?.phones?.forEach { PhoneRow(it) }
                }
            }
        }
        if (!card?.addresses.isNullOrEmpty()) {
            item {
                SectionCard(stringResource(R.string.contact_address)) {
                    card?.addresses?.forEach { AddressRow(it) }
                }
            }
        }
        if (!card?.emails.isNullOrEmpty()) {
            item {
                SectionCard(stringResource(R.string.contact_email)) {
                    card?.emails?.forEach { EmailRow(it) }
                }
            }
        }
        val onlineServices = (card?.imppAddresses.orEmpty() +
            card?.socialProfiles.orEmpty() +
            card?.otherOnlineServices.orEmpty())
        if (onlineServices.isNotEmpty()) {
            item {
                SectionCard(stringResource(R.string.contact_online_services)) {
                    onlineServices.forEach { OnlineServiceRow(it) }
                }
            }
        }
        if (!card?.links.isNullOrEmpty()) {
            item {
                SectionCard(stringResource(R.string.contact_links)) {
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
        }
        if (!card?.organizations.isNullOrEmpty()) {
            item {
                SectionCard(stringResource(R.string.contact_organization)) {
                    card?.organizations?.forEach { org ->
                        org.name?.let { InfoRow(it) }
                    }
                }
            }
        }
        if (!card?.notes.isNullOrEmpty()) {
            item {
                SectionCard(stringResource(R.string.contact_notes)) {
                    card?.notes?.forEach { note ->
                        note.note?.let { InfoRow(it) }
                    }
                }
            }
        }
        if (!contact.crm?.circles.isNullOrEmpty()) {
            item {
                SectionCard(stringResource(R.string.contact_circles)) {
                    InfoRow(contact.crm?.circles?.joinToString(", ").orEmpty())
                }
            }
        }
        item {
            // Unified timeline: the contact's activities/notes/reminders merged
            // newest-first (Phase 2 item 10). Tapping a row routes to its edit form.
            SectionTitle(stringResource(R.string.contact_timeline))
            TimelineSection(
                items = contact.toTimelineItems(),
                onEditActivity = onEditActivity,
                onEditNote = onEditNote,
                onEditReminder = onEditReminder,
                onCompleteReminder = onCompleteReminder,
            )
        }
        if (deviceLookupKey != null) {
            item {
                androidx.compose.material3.ListItem(
                    headlineContent = { Text(stringResource(R.string.contact_open_in_contacts), style = MaterialTheme.typography.bodyLarge) },
                    trailingContent = {
                        Icon(
                            imageVector = Icons.AutoMirrored.Outlined.ArrowForward,
                            contentDescription = null,
                        )
                    },
                    modifier = Modifier.fillMaxWidth().clickable { onOpenInContacts(deviceLookupKey) },
                )
            }
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
            androidx.compose.material3.ListItem(
                headlineContent = { Text(stringResource(R.string.contact_relationships), style = MaterialTheme.typography.bodyLarge) },
                trailingContent = {
                    Icon(
                        imageVector = Icons.AutoMirrored.Outlined.ArrowForward,
                        contentDescription = null,
                    )
                },
                modifier = Modifier.fillMaxWidth().clickable { onViewRelationships(contact.id) },
            )
            androidx.compose.material3.ListItem(
                headlineContent = { Text(stringResource(R.string.contact_merge), style = MaterialTheme.typography.bodyLarge) },
                trailingContent = {
                    Icon(
                        imageVector = Icons.AutoMirrored.Outlined.ArrowForward,
                        contentDescription = null,
                    )
                },
                modifier = Modifier.fillMaxWidth().clickable { onMerge(contact.id) },
            )
            androidx.compose.material3.ListItem(
                headlineContent = { Text(stringResource(R.string.contact_life_events), style = MaterialTheme.typography.bodyLarge) },
                trailingContent = {
                    Icon(
                        imageVector = Icons.AutoMirrored.Outlined.ArrowForward,
                        contentDescription = null,
                    )
                },
                modifier = Modifier.fillMaxWidth().clickable { onViewLifeEvents(contact.id) },
            )
            androidx.compose.material3.ListItem(
                headlineContent = { Text(stringResource(R.string.contact_gifts), style = MaterialTheme.typography.bodyLarge) },
                trailingContent = {
                    Icon(
                        imageVector = Icons.AutoMirrored.Outlined.ArrowForward,
                        contentDescription = null,
                    )
                },
                modifier = Modifier.fillMaxWidth().clickable { onViewGifts(contact.id) },
            )
            androidx.compose.material3.ListItem(
                headlineContent = { Text(stringResource(R.string.contact_preferences), style = MaterialTheme.typography.bodyLarge) },
                trailingContent = {
                    Icon(
                        imageVector = Icons.AutoMirrored.Outlined.ArrowForward,
                        contentDescription = null,
                    )
                },
                modifier = Modifier.fillMaxWidth().clickable { onViewPreferences(contact.id) },
            )
            androidx.compose.material3.ListItem(
                headlineContent = { Text(stringResource(R.string.contact_agenda), style = MaterialTheme.typography.bodyLarge) },
                trailingContent = {
                    Icon(
                        imageVector = Icons.AutoMirrored.Outlined.ArrowForward,
                        contentDescription = null,
                    )
                },
                modifier = Modifier.fillMaxWidth().clickable { onViewAgenda(contact.id) },
            )
        }
        item { Box(modifier = Modifier.size(32.dp)) }
    }
}


@Composable
private fun SectionCard(title: String, content: @Composable () -> Unit) {
    Card(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHighest),
    ) {
        Column(modifier = Modifier.padding(vertical = 8.dp)) {
            // T63 Android port: field-group captions ("Phone", "Address",
            // "Email", ...) get IBM Plex Mono, mirroring the web's
            // EditableField/EditableArrayField caption treatment (contrast
            // against the sans/mono field values below). labelLarge itself
            // stays global-sans (it's also the Button/NavigationDrawerItem
            // default) -- scoped here via .copy(fontFamily = ...) instead.
            Text(
                text = title,
                style = MaterialTheme.typography.labelLarge.copy(fontFamily = MycorrhizalFonts.mono),
                color = MaterialTheme.colorScheme.primary,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
            )
            content()
        }
    }
}

@Composable
private fun SectionTitle(text: String) {
    // T63 Android port: see SectionCard's matching comment above.
    Text(
        text = text,
        style = MaterialTheme.typography.labelLarge.copy(fontFamily = MycorrhizalFonts.mono),
        color = MaterialTheme.colorScheme.primary,
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
    )}

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
                Icon(
                    Icons.Outlined.Email,
                    contentDescription = stringResource(R.string.cd_compose_email),
                    tint = MaterialTheme.colorScheme.primary,
                )
            }
            IconButton(onClick = { FieldActions.copyText(context, "email", address) }) {
                Icon(
                    Icons.Outlined.ContentCopy,
                    contentDescription = stringResource(R.string.cd_copy_email),
                    tint = MaterialTheme.colorScheme.primary,
                )
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
                Icon(
                    Icons.Outlined.Call,
                    contentDescription = stringResource(R.string.cd_call),
                    tint = MaterialTheme.colorScheme.primary,
                )
            }
            // SMS for mobile numbers (T34 phone detection). Mirrors the web
            // app's `phoneHasToken` (ContactInformation.tsx): a phone is SMS-able
            // if `features`, `contexts`, or the type/label carries a cell/mobile
            // token. The backend populates `features` only on vCard import —
            // CRM-created contacts carry the type in `label`, so checking only
            // `features` (as this did) hid the SMS button for those.
            if (phone.isMobile()) {
                IconButton(onClick = { context.startActivity(FieldActions.smsIntent(number)) }) {
                    Icon(
                        Icons.Outlined.Message,
                        contentDescription = stringResource(R.string.cd_text),
                        tint = MaterialTheme.colorScheme.primary,
                    )
                }
            }
            IconButton(onClick = { FieldActions.copyText(context, "phone", number) }) {
                Icon(
                    Icons.Outlined.ContentCopy,
                    contentDescription = stringResource(R.string.cd_copy_phone),
                    tint = MaterialTheme.colorScheme.primary,
                )
            }
        }
    }
}

/** T34 phone-feature detection, mirroring the web app's `phoneHasToken`. */
private fun Phone.isMobile(): Boolean {
    val tokens = (features.orEmpty() + contexts.orEmpty() + listOfNotNull(label))
        .map { it.trim().lowercase() }
    return tokens.any { it == "cell" || it == "mobile" }
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
                Icon(
                    Icons.Outlined.Map,
                    contentDescription = stringResource(R.string.cd_open_maps),
                    tint = MaterialTheme.colorScheme.primary,
                )
            }
            IconButton(onClick = { FieldActions.copyText(context, "address", text) }) {
                Icon(
                    Icons.Outlined.ContentCopy,
                    contentDescription = stringResource(R.string.cd_copy_address),
                    tint = MaterialTheme.colorScheme.primary,
                )
            }
        }
    }
}

@Composable
private fun OnlineServiceRow(service: OnlineService) {
    val context = LocalContext.current
    val handle = service.uri ?: service.user.orEmpty()
    val serviceName = service.service ?: service.label.orEmpty()
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp)) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Column(modifier = Modifier.weight(1f)) {
                if (serviceName.isNotBlank()) {
                    Text(
                        text = serviceName,
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                if (handle.isNotBlank()) {
                    Text(
                        text = handle,
                        style = MaterialTheme.typography.bodyLarge,
                        maxLines = 1,
                        overflow = androidx.compose.ui.text.style.TextOverflow.Ellipsis,
                    )
                }
            }
            if (handle.isNotBlank()) {
                IconButton(onClick = { FieldActions.copyText(context, "url", handle) }) {
                    Icon(
                        Icons.Outlined.ContentCopy,
                        contentDescription = stringResource(R.string.cd_copy_link),
                        tint = MaterialTheme.colorScheme.primary,
                    )
                }
            }
        }
        // Item 12 (§7.6) enrichment: the service (e.g. "Signal") maps to a
        // registry protocol, whose actions show once the app is detected.
        MobileLinkRegistry.forProtocol(serviceName)?.let { linkType ->
            if (handle.isNotBlank()) {
                MobileLinkActions(linkType = linkType, handle = handle)
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
                    Icon(
                        Icons.Outlined.OpenInNew,
                        contentDescription = stringResource(R.string.cd_open_link),
                        tint = MaterialTheme.colorScheme.primary,
                    )
                }
                IconButton(onClick = { FieldActions.copyText(context, "url", uri) }) {
                    Icon(
                        Icons.Outlined.ContentCopy,
                        contentDescription = stringResource(R.string.cd_copy_link),
                        tint = MaterialTheme.colorScheme.primary,
                    )
                }
            }
        }
        if (linkType != null) {
            MobileLinkActions(linkType = linkType, handle = uri)
        }
    }
}

/**
 * Item 12 (§7.6) enrichment chips for a known-protocol link. Requests
 * READ_CONTACTS inline (not at startup, per §8.3) so the resolver can query
 * ContactsContract.Data for the installed-app MIMETYPEs; a denied permission
 * degrades to nothing (the caller already shows the plain link + copy row).
 */
@Composable
private fun MobileLinkActions(
    linkType: MobileLinkType,
    handle: String,
) {
    val context = LocalContext.current
    var availableActions by remember { mutableStateOf<List<MobileLinkAction>>(emptyList()) }
    var permissionAsked by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()

    val permissionLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestPermission(),
    ) { granted ->
        permissionAsked = true
        if (granted) {
            scope.launch {
                val resolver = MobileLinkActionResolver(context.contentResolver)
                availableActions = resolver.resolveAvailableActions(linkType, handle)
            }
        }
    }

    LaunchedEffect(linkType, handle) {
        val granted = ContextCompat.checkSelfPermission(
            context, Manifest.permission.READ_CONTACTS,
        ) == PackageManager.PERMISSION_GRANTED
        if (granted) {
            val resolver = MobileLinkActionResolver(context.contentResolver)
            availableActions = try {
                resolver.resolveAvailableActions(linkType, handle)
            } catch (_: SecurityException) {
                emptyList()
            }
        } else if (!permissionAsked) {
            permissionLauncher.launch(Manifest.permission.READ_CONTACTS)
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
                            context.startActivity(action.intentBuilder(handle))
                        } catch (_: Exception) {
                            // No handler for the deep link — fall back to the browser.
                            context.startActivity(FieldActions.browserIntent(handle))
                        }
                    },
                    label = { Text(stringResource(action.label)) },
                    leadingIcon = {
                        Icon(
                                imageVector = when (action.kind) {
                                    MobileActionKind.MESSAGE -> Icons.Outlined.Message
                                    MobileActionKind.VOICE_CALL -> Icons.Outlined.Call
                                    MobileActionKind.VIDEO_CALL -> Icons.Outlined.Videocam
                                    MobileActionKind.APP_OPEN -> Icons.Outlined.OpenInNew
                                    MobileActionKind.APP_CALL -> Icons.Outlined.Call
                                },
                                contentDescription = stringResource(action.label),
                                modifier = Modifier.size(18.dp),
                            )
                        },
                        modifier = Modifier.height(32.dp),
                    )
                }
            }
        }
    }
