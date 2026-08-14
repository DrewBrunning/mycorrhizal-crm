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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.BriefingRelationship
import com.mycorrhizal.crm.model.network.BriefingUpcomingDate
import com.mycorrhizal.crm.model.network.ContactBriefing
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.theme.MycorrhizalFonts
import com.mycorrhizal.crm.ui.R
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/**
 * M11 — the N2 prep-view briefing for Android (docs/fork-plan/tickets/
 * 22-N2-prep-view.md): "everything the user wants to remember in the five
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
private fun PrepViewContent(
    briefing: ContactBriefing,
    onOpenContact: (Int) -> Unit,
) {
    LazyColumn(modifier = Modifier.fillMaxSize()) {
        item { PrepHeader(briefing) }

        briefing.cadence?.let { cadence ->
            item {
                PrepCard(stringResource(R.string.prep_cadence_title), icon = Icons.Outlined.Warning) {
                    CadenceBlock(health = cadence.health)
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
                LastInteractionBlock(briefing)
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
                        val label = event.type?.replace('_', ' ')?.takeIf { it.isNotBlank() } ?: return@forEach
                        Bullet(event.description?.let { "$label — $it" } ?: label)
                    }
                }
            }
        }

        if (briefing.upcomingReminders.isNotEmpty()) {
            item {
                PrepCard(stringResource(R.string.prep_reminders_title), icon = Icons.Outlined.Notifications) {
                    briefing.upcomingReminders.forEach { reminder ->
                        val whenDue = formatDateOnly(reminder.remindAt)
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
                        UpcomingDateRow(d)
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
                Text(
                    text = briefing.name.ifBlank { stringResource(R.string.prep_unknown_contact) },
                    style = MaterialTheme.typography.titleLarge,
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
private fun CadenceBlock(health: com.mycorrhizal.crm.model.network.BriefingCadenceHealth) {
    if (health.hasQualifyingInteraction) {
        if (health.overdueBy > 0) {
            Text(
                text = stringResource(R.string.prep_cadence_overdue, health.overdueBy),
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.error,
                modifier = Modifier.padding(vertical = 2.dp),
            )
        } else {
            Text(
                text = stringResource(R.string.prep_cadence_on_track),
                style = MaterialTheme.typography.bodyLarge,
                modifier = Modifier.padding(vertical = 2.dp),
            )
        }
        val nextDue = formatDateOnly(health.nextDue)
        if (nextDue.isNotBlank()) {
            Text(
                text = stringResource(R.string.prep_cadence_next_due, nextDue),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        val lastInteraction = formatDateOnly(health.lastInteraction)
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
private fun LastInteractionBlock(briefing: ContactBriefing) {
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
        val whenItHappened = formatDateOnly(lastActivity.date)
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
    }
}

@Composable
private fun UpcomingDateRow(d: BriefingUpcomingDate) {
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
            text = "${stringResource(labelRes)} ${d.date}",
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

/** Formats an ISO-8601 timestamp as a compact date, or "" when unparseable/absent. */
private fun formatDateOnly(iso: String?): String {
    if (iso.isNullOrBlank()) return ""
    return runCatching {
        Instant.parse(iso).atZone(ZoneId.systemDefault()).format(DATE_ONLY)
    }.getOrDefault("")
}

private val DATE_ONLY: DateTimeFormatter = DateTimeFormatter.ofPattern("yyyy-MM-dd")
