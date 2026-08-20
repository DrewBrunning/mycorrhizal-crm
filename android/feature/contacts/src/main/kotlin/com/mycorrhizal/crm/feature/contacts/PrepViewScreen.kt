package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.automirrored.outlined.ArrowForward
import androidx.compose.material.icons.outlined.Cake
import androidx.compose.material.icons.outlined.Celebration
import androidx.compose.material.icons.outlined.ChatBubbleOutline
import androidx.compose.material.icons.outlined.DateRange
import androidx.compose.material.icons.outlined.Event
import androidx.compose.material.icons.outlined.Favorite
import androidx.compose.material.icons.outlined.Notes
import androidx.compose.material.icons.outlined.Notifications
import androidx.compose.material.icons.outlined.Warning
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.BriefingRelationship
import com.mycorrhizal.crm.model.network.BriefingUpcomingDate
import com.mycorrhizal.crm.model.network.ContactBriefing
import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.util.DateFormat
import com.mycorrhizal.crm.model.util.DateFormat.display
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.theme.MycorrhizalFonts
import com.mycorrhizal.crm.ui.R

/**
 * M11 — the N2 prep-view briefing for Android (N2): "everything the user
 * wants to remember in the five
 * minutes before seeing someone, scannable in under a minute". A read-only
 * rendering of GET /contacts/:id/briefing; the backend assembles every block,
 * so each section here just reads the server-provided values (the cadence/
 * health card in particular must never recompute health locally — see M12).
 *
 * Every block degrades gracefully when its source is empty — a contact with
 * no activities, no agenda, no cadence policy, no relationships renders the
 * same layout with each card showing its empty state.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PrepViewScreen(
    onBack: () -> Unit,
    /** Opens the given contact's detail record (relationship rows). */
    onOpenContact: (Int) -> Unit,
    viewModel: PrepViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(stringResource(R.string.prep_title), style = MaterialTheme.typography.titleLarge)
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading -> LoadingSkeleton()
                state.error != null || state.briefing == null ->
                    PrepErrorState(
                        message = state.error ?: stringResource(R.string.prep_not_found),
                        onRetry = viewModel::load,
                    )
                else -> PrepViewContent(
                    briefing = state.briefing!!,
                    // The session's date_format, like ContactDetailScreen threads
                    // for its birthday rows; "eu" is the app-wide default.
                    dateFormat = state.dateFormat ?: DateFormat.EU,
                    onOpenContact = onOpenContact,
                )
            }
        }
    }
}

@Composable
private fun PrepErrorState(message: String, onRetry: () -> Unit) {
    Column(
        modifier = Modifier.fillMaxSize().padding(32.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(
            text = message,
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Button(onClick = onRetry, modifier = Modifier.padding(top = 16.dp)) {
            Text(stringResource(R.string.action_retry))
        }
    }
}

@Composable
internal fun PrepViewContent(
    briefing: ContactBriefing,
    dateFormat: String,
    onOpenContact: (Int) -> Unit,
) {
    LazyColumn(
        modifier = Modifier
            .fillMaxSize()
            .testTag("prep-view-list"),
    ) {
        item { PrepHeader(briefing) }

        briefing.cadence?.let { cadence ->
            item {
                PrepCard(stringResource(R.string.prep_cadence_title), icon = Icons.Outlined.Warning) {
                    CadenceBlock(health = cadence.health, dateFormat = dateFormat)
                }
            }
        }

        if (briefing.openAgendaItems.isNotEmpty()) {
            item {
                PrepCard(stringResource(R.string.prep_agenda_title), icon = Icons.Outlined.ChatBubbleOutline) {
                    briefing.openAgendaItems.forEach { item ->
                        Bullet(item.content)
                    }
                }
            }
        }

        item {
            PrepCard(stringResource(R.string.prep_last_interaction_title), icon = Icons.Outlined.Event) {
                LastInteractionBlock(briefing, dateFormat = dateFormat)
            }
        }

        if (briefing.relationships.isNotEmpty()) {
            item {
                PrepCard(stringResource(R.string.prep_relationships_title), icon = Icons.Outlined.Favorite) {
                    briefing.relationships.forEach { rel ->
                        RelationshipRow(rel, onOpenContact)
                    }
                }
            }
        }

        if (briefing.lifeEvents.isNotEmpty()) {
            item {
                PrepCard(stringResource(R.string.prep_life_events_title), icon = Icons.Outlined.DateRange) {
                    briefing.lifeEvents.forEach { event ->
                        val label = event.type?.replace('_', ' ')?.takeIf { it.isNotBlank() }
                        val description = event.description?.takeIf { it.isNotBlank() }
                        val line = when {
                            label != null && description != null -> "$label — $description"
                            label != null -> label
                            description != null -> description
                            else -> return@forEach
                        }
                        Bullet(line)
                    }
                }
            }
        }

        if (briefing.upcomingReminders.isNotEmpty()) {
            item {
                PrepCard(stringResource(R.string.prep_reminders_title), icon = Icons.Outlined.Notifications) {
                    briefing.upcomingReminders.forEach { reminder ->
                        val whenDue = formatTimestamp(reminder.remindAt, dateFormat)
                        val line = if (whenDue.isBlank()) {
                            reminder.message.orEmpty()
                        } else {
                            "${reminder.message.orEmpty()} ($whenDue)"
                        }
                        Bullet(line)
                    }
                }
            }
        }

        if (briefing.upcomingDates.isNotEmpty()) {
            item {
                PrepCard(stringResource(R.string.prep_upcoming_dates_title), icon = Icons.Outlined.Cake) {
                    briefing.upcomingDates.forEach { d ->
                        UpcomingDateRow(d, dateFormat)
                    }
                }
            }
        }

        item { Box(modifier = Modifier.size(32.dp)) }
    }
}

@Composable
private fun PrepHeader(briefing: ContactBriefing) {
    Card(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHighest),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(16.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            ContactAvatar(
                photoUri = briefing.photoThumbnail,
                contentDescription = null,
                size = 56.dp,
            )
            Column(modifier = Modifier.weight(1f)) {
                // #208: the contact name is the de facto page heading but
                // carried no heading semantics.
                Text(
                    text = briefing.name.ifBlank { stringResource(R.string.prep_unknown_contact) },
                    style = MaterialTheme.typography.titleLarge,
                    modifier = Modifier.semantics { heading() },
                )
                Text(
                    text = stringResource(R.string.prep_subtitle),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (briefing.kind == "animal") {
                AssistChip(onClick = {}, label = { Text(stringResource(R.string.contact_kind_animal)) })
            }
        }
    }
}

@Composable
private fun CadenceBlock(health: com.mycorrhizal.crm.model.network.BriefingCadenceHealth, dateFormat: String) {
    if (health.hasQualifyingInteraction) {
        if (health.overdueBy > 0) {
            Text(
                text = stringResource(R.string.prep_cadence_overdue, health.overdueBy),
                style = MaterialTheme.typography.bodyLarge,
                // #200: amber text on the cream page was 2.07:1 — illegible. The
                // warning semantics ride the Icons.Outlined.Warning icon on the
                // contact header; the words themselves use the page's own
                // onSurface (web parity kept the amber as the *accent* only).
                color = MaterialTheme.colorScheme.onSurface,
                modifier = Modifier.padding(vertical = 2.dp),
            )
        } else {
            Text(
                text = stringResource(R.string.prep_cadence_on_track),
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.tertiary,
                modifier = Modifier.padding(vertical = 2.dp),
            )
        }
        val nextDue = formatTimestamp(health.nextDue, dateFormat)
        if (nextDue.isNotBlank()) {
            Text(
                text = stringResource(R.string.prep_cadence_next_due, nextDue),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        val lastInteraction = formatTimestamp(health.lastInteraction, dateFormat)
        if (lastInteraction.isNotBlank()) {
            Text(
                text = stringResource(R.string.prep_cadence_last_interaction, lastInteraction),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    } else {
        Text(
            text = stringResource(R.string.prep_cadence_no_interactions_yet),
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun LastInteractionBlock(briefing: ContactBriefing, dateFormat: String) {
    val lastActivity = briefing.lastActivity
    if (lastActivity != null) {
        val title = lastActivity.title.ifBlank { lastActivity.type.orEmpty() }
        Text(
            text = buildString {
                append(title)
                if (lastActivity.type?.isNotBlank() == true) append(" (${lastActivity.type})")
            },
            style = MaterialTheme.typography.bodyLarge,
            fontWeight = FontWeight.Medium,
        )
        val whenItHappened = formatTimestamp(lastActivity.date, dateFormat)
        if (whenItHappened.isNotBlank()) {
            Text(
                text = whenItHappened,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        val description = lastActivity.description
        if (description?.isNotBlank() == true) {
            Text(
                text = description,
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.padding(top = 4.dp),
            )
        }
    } else {
        Text(
            text = stringResource(R.string.prep_last_interaction_none),
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }

    if (briefing.recentNotes.isNotEmpty()) {
        androidx.compose.material3.HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp))
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Icon(
                Icons.Outlined.Notes,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.size(18.dp),
            )
            Text(stringResource(R.string.prep_recent_notes_title), style = MaterialTheme.typography.labelMedium)
        }
        briefing.recentNotes.forEach { note ->
            Bullet(note.content.orEmpty())
        }
    }
}

@Composable
private fun RelationshipRow(rel: BriefingRelationship, onOpenContact: (Int) -> Unit) {
    val name = rel.otherPartyName?.takeIf { it.isNotBlank() }
        ?: stringResource(R.string.prep_unknown_contact)
    val label = rel.displayToken?.replace('_', ' ')?.takeIf { it.isNotBlank() }
    val text = if (label != null) "$name — $label" else name
    val targetId = rel.otherPartyContactId

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .then(if (targetId != null) Modifier.clickable { onOpenContact(targetId) } else Modifier)
            .padding(horizontal = 16.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(
            text = text,
            style = MaterialTheme.typography.bodyLarge,
            modifier = Modifier.weight(1f),
        )
        // Web renders a "View" chip here; on a phone a trailing chevron is the
        // conventional tap affordance, and it mirrors the contact-detail rows
        // that link onward ("Activities", "Reminders", …).
        if (targetId != null) {
            Icon(
                imageVector = Icons.AutoMirrored.Outlined.ArrowForward,
                contentDescription = stringResource(R.string.cd_open_contact),
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.size(18.dp),
            )
        }
    }
}

@Composable
private fun UpcomingDateRow(d: BriefingUpcomingDate, dateFormat: String) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Icon(
            imageVector = if (d.label == "birthday") Icons.Outlined.Cake else Icons.Outlined.Celebration,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(18.dp),
        )
        val labelRes = if (d.label == "birthday") {
            R.string.prep_upcoming_dates_birthday
        } else {
            R.string.prep_upcoming_dates_anniversary
        }
        Text(
            text = "${stringResource(labelRes)} ${formatUpcomingDate(d.date, dateFormat)}",
            style = MaterialTheme.typography.bodyLarge,
            modifier = Modifier.weight(1f),
        )
        AssistChip(
            onClick = {},
            label = { Text(stringResource(R.string.prep_upcoming_dates_in_days, d.daysUntil)) },
        )
    }
}

@Composable
private fun Bullet(text: String) {
    if (text.isBlank()) return
    Text(
        text = "•  $text",
        style = MaterialTheme.typography.bodyLarge,
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 3.dp),
    )
}

/** A prep-view section card: mono caption header + body content. */
@Composable
private fun PrepCard(
    title: String,
    icon: ImageVector,
    content: @Composable () -> Unit,
) {
    Card(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHighest),
    ) {
        Column(modifier = Modifier.padding(vertical = 8.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                Icon(
                    icon,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.size(18.dp),
                )
                Text(
                    text = title,
                    // T63 Android port: section captions get IBM Plex Mono,
                    // mirroring the web's caption treatment (SectionCard does
                    // the same on the contact detail screen).
                    style = MaterialTheme.typography.labelLarge.copy(fontFamily = MycorrhizalFonts.mono),
                    color = MaterialTheme.colorScheme.primary,
                )
            }
            content()
        }
    }
}

/**
 * Formats an ISO-8601 timestamp as a date in the user's date format, or ""
 * when unparseable/absent.
 *
 * **UTC, not the device zone** — see [DateFormat.formatTimestamp], the single
 * shared implementation (M11 + M12 must render the same value the same way;
 * this used to be a private copy that could drift).
 */
private fun formatTimestamp(iso: String?, format: String): String =
    DateFormat.formatTimestamp(iso, format)

/**
 * Formats an upcoming-date value (YYYY-MM-DD or --MM-DD) in the user's date
 * format, honoring the yearless form the way birthdays are shown elsewhere
 * ("25 December", not the raw "--12-25"). Falls back to the raw value when
 * the shape is unexpected.
 */
private fun formatUpcomingDate(value: String, format: String): String {
    return runCatching {
        when {
            value.startsWith("--") && value.length == 7 -> {
                PartialDate(
                    month = value.substring(2, 4).toInt(),
                    day = value.substring(5, 7).toInt(),
                ).display(format)
            }
            value.length == 10 && value[4] == '-' && value[7] == '-' -> {
                PartialDate(
                    year = value.substring(0, 4).toInt(),
                    month = value.substring(5, 7).toInt(),
                    day = value.substring(8, 10).toInt(),
                ).display(format)
            }
            else -> value
        }
    }.getOrDefault(value)
}
