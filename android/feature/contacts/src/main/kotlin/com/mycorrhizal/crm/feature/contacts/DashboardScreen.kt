package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Cake
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.Email
import androidx.compose.material.icons.outlined.Menu
import androidx.compose.material.icons.outlined.Notifications
import androidx.compose.material.icons.outlined.Repeat
import androidx.compose.material.icons.outlined.Shuffle
import androidx.compose.material.icons.outlined.SkipNext
import androidx.compose.material.icons.outlined.Star
import androidx.compose.material.icons.outlined.Warning
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.Birthday
import com.mycorrhizal.crm.model.network.DashboardRandomContact
import com.mycorrhizal.crm.model.network.DashboardReminder
import com.mycorrhizal.crm.model.network.OverdueCadence
import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.network.ReminderRecurrence
import com.mycorrhizal.crm.model.util.DateFormat
import com.mycorrhizal.crm.model.util.DateFormat.display
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.theme.MycorrhizalColors
import java.time.Instant
import java.time.LocalDate

/**
 * Dashboard (M10) — the M3 composite widgets rendered from one
 * `GET /dashboard` call: the favorites quick-access block (issue #212),
 * overdue cadences, upcoming birthdays, upcoming reminders (with
 * complete/skip actions), and random "stay in touch" contacts. Each widget
 * is a section that shows its empty text when its block is empty, mirroring
 * web's per-column empty cards; the overdue section is hidden entirely when
 * clear (web comment: "an all-clear dashboard stays clean").
 *
 * Every card is tappable through to the contact; reminder complete/skip is
 * optimistic (the row leaves the widget immediately and returns on failure).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DashboardScreen(
    onOpenContact: (Int) -> Unit,
    // Issue #150: null hides the hamburger — there is no drawer at Expanded.
    onMenuClick: (() -> Unit)? = {},
    viewModel: DashboardViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    // A failed complete/skip is a transient toast on top of the intact
    // dashboard — it must NOT blank the widgets the way a failed load does.
    val actionError = state.actionError
    if (actionError != null) {
        LaunchedEffect(actionError) {
            snackbarHostState.showSnackbar(actionError)
            viewModel.onActionErrorShown()
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    onMenuClick?.let { onMenu ->
                        // #214: Material3's default IconButton state-layer is
                        // 40dp — the semantics node it exposes to accessibility
                        // services is that 40dp box, below the 48dp minimum
                        // (WCAG 2.5.8), even though minimumInteractiveComponentSize()
                        // pads the pointer-input hit region. An explicit outer
                        // size constrains the inner default down to 48dp.
                        AccessibleIconButton(onClick = onMenu) {
                            Icon(Icons.Outlined.Menu, contentDescription = stringResource(R.string.cd_menu))
                        }
                    }
                },
                title = {
                    Text(stringResource(R.string.nav_dashboard), style = MaterialTheme.typography.titleLarge)
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
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading -> LoadingSkeleton()
                state.error != null -> DashboardErrorState(state.error!!, onRetry = viewModel::load)
                else -> DashboardContent(
                    state = state,
                    dateFormat = state.dateFormat ?: DateFormat.EU,
                    onOpenContact = onOpenContact,
                    onCompleteReminder = viewModel::completeReminder,
                )
            }
        }
    }
}

/**
 * The dashboard's widget sections, extracted from [DashboardScreen] so the
 * widget rendering (favorites block included — issue #212) can be tested
 * without Hilt. Pure render of a
 * [DashboardUiState] plus callbacks — the ViewModel's state machine and the
 * wire parse are pinned in DashboardViewModelTest / ApiClientTest.
 */
@Composable
internal fun DashboardContent(
    state: DashboardUiState,
    dateFormat: String,
    onOpenContact: (Int) -> Unit,
    onCompleteReminder: (id: Int, skip: Boolean) -> Unit,
) {
    var pendingSkip by remember { mutableStateOf<DashboardReminder?>(null) }

    LazyColumn(
        modifier = Modifier.fillMaxSize().testTag("dashboard-list"),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        // Issue #212: the favorites quick-access block, placed first like web's
        // Column 1. Empty state present, exactly like the other widgets.
        item { DashboardSectionHeader(stringResource(R.string.dashboard_favorites), Icons.Outlined.Star) }
        if (state.favorites.isEmpty()) {
            item { DashboardEmptyRow(stringResource(R.string.dashboard_no_favorites)) }
        } else {
            items(state.favorites, key = { it.id }) { contact ->
                FavoriteContactRow(contact, onClick = { onOpenContact(contact.id) })
            }
        }
        if (state.overdueCadences.isNotEmpty()) {
            item { DashboardSectionHeader(stringResource(R.string.dashboard_overdue_cadences), Icons.Outlined.Warning) }
            items(state.overdueCadences, key = { it.policy?.id ?: "contact-${it.contactId}" }) { cadence ->
                OverdueRow(cadence, dateFormat, onClick = {
                    if (cadence.contactId > 0) onOpenContact(cadence.contactId.toInt())
                })
            }
        }
        item { DashboardSectionHeader(stringResource(R.string.dashboard_birthdays), Icons.Outlined.Cake) }
        if (state.birthdays.isEmpty()) {
            item { DashboardEmptyRow(stringResource(R.string.dashboard_no_birthdays)) }
        } else {
            items(state.birthdays, key = { it.contactId }) { birthday ->
                BirthdayRow(birthday, dateFormat, onClick = {
                    if (birthday.contactId > 0) onOpenContact(birthday.contactId.toInt())
                })
            }
        }
        item { DashboardSectionHeader(stringResource(R.string.dashboard_reminders), Icons.Outlined.Notifications) }
        if (state.upcomingReminders.isEmpty()) {
            item { DashboardEmptyRow(stringResource(R.string.dashboard_no_reminders)) }
        } else {
            items(state.upcomingReminders, key = { it.id }) { reminder ->
                ReminderRow(
                    reminder = reminder,
                    dateFormat = dateFormat,
                    isCompleting = state.completingId == reminder.id,
                    onComplete = { onCompleteReminder(reminder.id, false) },
                    onSkip = { pendingSkip = reminder },
                    onClick = { reminder.contactId?.takeIf { it > 0 }?.let(onOpenContact) },
                )
            }
        }
        item { DashboardSectionHeader(stringResource(R.string.dashboard_random_contacts), Icons.Outlined.Shuffle) }
        if (state.randomContacts.isEmpty()) {
            item { DashboardEmptyRow(stringResource(R.string.dashboard_no_contacts)) }
        } else {
            items(state.randomContacts, key = { it.id }) { contact ->
                RandomContactRow(contact, onClick = { onOpenContact(contact.id) })
            }
        }
    }

    pendingSkip?.let { reminder ->
        AlertDialog(
            onDismissRequest = { pendingSkip = null },
            title = { Text(stringResource(R.string.cd_skip_reminder)) },
            text = { Text(stringResource(R.string.reminder_skip_confirm)) },
            confirmButton = {
                TextButton(
                    onClick = {
                        onCompleteReminder(reminder.id, true)
                        pendingSkip = null
                    },
                ) {
                    Text(stringResource(R.string.reminder_skip))
                }
            },
            dismissButton = {
                TextButton(onClick = { pendingSkip = null }) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }
}

/** Full-screen load failure with retry — replaces the widgets, never shows alongside them. */
@Composable
private fun DashboardErrorState(message: String, onRetry: () -> Unit) {
    Column(
        modifier = Modifier.fillMaxSize().padding(32.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(
            text = message,
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
        )
        Button(onClick = onRetry, modifier = Modifier.padding(top = 16.dp)) {
            Text(stringResource(R.string.action_retry))
        }
    }
}

@Composable
private fun DashboardSectionHeader(text: String, icon: ImageVector) {
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
        Icon(icon, contentDescription = null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(18.dp))
        // #208: section titles carried no heading semantics, so TalkBack's
        // heading navigation found nothing on the dashboard. One fix here
        // covers every section header (favorites — issue #212 — birthdays,
        // reminders, random contacts, and the conditional overdue-cadences
        // header).
        Text(
            text,
            style = MaterialTheme.typography.titleMedium,
            color = MaterialTheme.colorScheme.primary,
            modifier = Modifier.semantics { heading() },
        )
    }
}

@Composable
private fun DashboardEmptyRow(text: String) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Text(
            text = text,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(12.dp),
        )
    }
}

@Composable
private fun OverdueRow(cadence: OverdueCadence, dateFormat: String, onClick: () -> Unit) {
    val name = cadence.contactName.ifBlank { stringResource(R.string.dashboard_unknown_contact) }
    val nextDue = DateFormat.formatTimestamp(cadence.health?.nextDue, dateFormat)
    Card(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick),
        border = BorderStroke(1.dp, MycorrhizalColors.chanterelle),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(12.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            ContactAvatar(photoUri = cadence.photoThumbnail, contentDescription = null, size = 40.dp)
            Column(modifier = Modifier.weight(1f)) {
                Text(name, style = MaterialTheme.typography.bodyLarge)
                if (nextDue.isNotBlank()) {
                    Text(
                        text = stringResource(R.string.cadence_next_due, nextDue),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            DashboardChip(
                text = stringResource(R.string.cadence_overdue_by, cadence.health?.overdueBy ?: 0),
                leadingIcon = Icons.Outlined.Warning,
                containerColor = MycorrhizalColors.chanterelle,
                contentColor = Color.White,
            )
        }
    }
}

@Composable
private fun BirthdayRow(birthday: Birthday, dateFormat: String, onClick: () -> Unit) {
    val today = isBirthdayToday(birthday.birthday)
    Card(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick),
        border = if (today) BorderStroke(1.dp, MaterialTheme.colorScheme.tertiary) else null,
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(12.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            ContactAvatar(photoUri = birthday.photoThumbnail, contentDescription = null, size = 40.dp)
            Column {
                Text(birthday.name, style = MaterialTheme.typography.bodyLarge)
                Text(
                    text = birthdayLine(birthday, dateFormat),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun ReminderRow(
    reminder: DashboardReminder,
    dateFormat: String,
    isCompleting: Boolean,
    onComplete: () -> Unit,
    onSkip: () -> Unit,
    onClick: () -> Unit,
) {
    val overdue = isOverdue(reminder.remindAt)
    val dateText = DateFormat.formatTimestamp(reminder.remindAt, dateFormat)
    Card(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick),
        border = if (overdue) BorderStroke(1.dp, MycorrhizalColors.chanterelle) else null,
    ) {
        Row(modifier = Modifier.fillMaxWidth().padding(12.dp), verticalAlignment = Alignment.Top) {
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = reminder.message ?: stringResource(R.string.reminder_title_fallback),
                    style = MaterialTheme.typography.bodyLarge,
                )
                if (reminder.contactName.isNotBlank()) {
                    Text(
                        text = reminder.contactName,
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Row(
                    modifier = Modifier.padding(top = 4.dp),
                    horizontalArrangement = Arrangement.spacedBy(4.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    if (dateText.isNotBlank()) {
                        DashboardChip(
                            text = dateText,
                            leadingIcon = if (overdue) Icons.Outlined.Warning else null,
                            containerColor = if (overdue) MycorrhizalColors.chanterelle else MaterialTheme.colorScheme.surfaceVariant,
                            contentColor = if (overdue) Color.White else MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    val recurrence = reminder.recurrence
                    if (recurrence != null && recurrence != ReminderRecurrence.ONCE) {
                        recurrenceLabel(recurrence)?.let { label ->
                            DashboardChip(
                                text = label,
                                leadingIcon = Icons.Outlined.Repeat,
                                containerColor = MaterialTheme.colorScheme.surfaceVariant,
                                contentColor = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                    if (reminder.byMail == true) {
                        DashboardChip(
                            text = stringResource(R.string.reminder_email_chip),
                            leadingIcon = Icons.Outlined.Email,
                            containerColor = MaterialTheme.colorScheme.surfaceVariant,
                            contentColor = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
            Row {
                AccessibleIconButton(onClick = onSkip, enabled = !isCompleting) {
                    Icon(
                        Icons.Outlined.SkipNext,
                        contentDescription = stringResource(R.string.cd_skip_reminder),
                        tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                AccessibleIconButton(onClick = onComplete, enabled = !isCompleting) {
                    Icon(
                        Icons.Outlined.CheckCircle,
                        contentDescription = stringResource(R.string.cd_complete_reminder),
                        tint = MaterialTheme.colorScheme.tertiary,
                    )
                }
            }
        }
    }
}

@Composable
private fun RandomContactRow(contact: DashboardRandomContact, onClick: () -> Unit) {
    Card(modifier = Modifier.fillMaxWidth().clickable(onClick = onClick)) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(12.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            ContactAvatar(photoUri = contact.photoThumbnail, contentDescription = null, size = 40.dp)
            Text(randomContactName(contact), style = MaterialTheme.typography.bodyLarge)
        }
    }
}

// Issue #212: the favorites quick-access row. Same wire shape and tap-to-open
// behavior as RandomContactRow — web's dashboard favorites block is a plain
// contact list; the only difference is the section it lives in.
@Composable
private fun FavoriteContactRow(contact: DashboardRandomContact, onClick: () -> Unit) {
    Card(modifier = Modifier.fillMaxWidth().clickable(onClick = onClick)) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(12.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            ContactAvatar(photoUri = contact.photoThumbnail, contentDescription = null, size = 40.dp)
            Text(randomContactName(contact), style = MaterialTheme.typography.bodyLarge)
        }
    }
}

@Composable
private fun DashboardChip(
    text: String,
    leadingIcon: ImageVector? = null,
    containerColor: Color,
    contentColor: Color,
) {
    Surface(shape = RoundedCornerShape(8.dp), color = containerColor, contentColor = contentColor) {
        Row(
            modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(2.dp),
        ) {
            if (leadingIcon != null) {
                Icon(leadingIcon, contentDescription = null, modifier = Modifier.size(12.dp))
            }
            Text(text, style = MaterialTheme.typography.labelSmall)
        }
    }
}

/** The birthday line: formatted date, plus " (N years old)" when a full date is present. */
@Composable
private fun birthdayLine(birthday: Birthday, dateFormat: String): String {
    val raw = birthday.birthday ?: return ""
    val age = birthdayAge(raw)
    val monthDay = runCatching {
        when {
            raw.startsWith("--") && raw.length == 7 -> PartialDate(
                month = raw.substring(2, 4).toInt(),
                day = raw.substring(5, 7).toInt(),
            ).display(dateFormat)
            raw.length == 10 && raw[4] == '-' && raw[7] == '-' -> {
                val month = raw.substring(5, 7).toInt()
                val day = raw.substring(8, 10).toInt()
                PartialDate(month = month, day = day).display(dateFormat)
            }
            else -> null
        }
    }.getOrNull()
    return when {
        // A future birth year is nonsense data; show the date without an age rather
        // than a negative one (web computes the same raw subtraction, but renders it).
        age != null && age >= 0 && monthDay != null ->
            "$monthDay ${stringResource(R.string.dashboard_years_old, age)}"
        monthDay != null -> monthDay
        else -> raw
    }
}

/** Whole-years age from a `YYYY-MM-DD` birthday, null for yearless (`--MM-DD`) values. */
private fun birthdayAge(raw: String): Int? {
    if (raw.startsWith("--")) return null
    val parts = raw.split('-')
    if (parts.size != 3) return null
    val year = parts[0].toIntOrNull() ?: return null
    return LocalDate.now().year - year
}

/** Matches web's `isBirthdayToday`: month+day equality against today (yearless-safe). */
private fun isBirthdayToday(birthday: String?): Boolean {
    if (birthday.isNullOrBlank()) return false
    val parts = birthday.split('-').filter { it.isNotEmpty() }
    if (parts.size < 2) return false
    val month = parts[parts.size - 2].toIntOrNull() ?: return false
    val day = parts[parts.size - 1].toIntOrNull() ?: return false
    val today = LocalDate.now()
    return today.monthValue == month && today.dayOfMonth == day
}

/** Matches web's `isOverdue`: the reminder's `remind_at` is in the past. */
private fun isOverdue(remindAt: String?): Boolean {
    if (remindAt.isNullOrBlank()) return false
    return runCatching { Instant.parse(remindAt).isBefore(Instant.now()) }.getOrDefault(false)
}

/** Display name for a random contact, matching web's `getContactName` (nickname-preferred). */
private fun randomContactName(contact: DashboardRandomContact): String {
    val nickname = contact.nickname
    return when {
        !nickname.isNullOrBlank() ->
            listOf(nickname, contact.lastname.orEmpty()).filter { it.isNotBlank() }.joinToString(" ")
        else -> listOfNotNull(contact.firstname, contact.lastname).joinToString(" ").ifBlank { "#${contact.id}" }
    }
}

/** Localized recurrence label, mirroring the web `reminders.recurrence.*` keys; null for unknown. */
@Composable
private fun recurrenceLabel(token: String): String? = when (token) {
    ReminderRecurrence.ONCE -> stringResource(R.string.reminder_recurrence_once)
    ReminderRecurrence.WEEKLY -> stringResource(R.string.reminder_recurrence_weekly)
    ReminderRecurrence.MONTHLY -> stringResource(R.string.reminder_recurrence_monthly)
    ReminderRecurrence.QUARTERLY -> stringResource(R.string.reminder_recurrence_quarterly)
    ReminderRecurrence.SIX_MONTHS -> stringResource(R.string.reminder_recurrence_six_months)
    ReminderRecurrence.YEARLY -> stringResource(R.string.reminder_recurrence_yearly)
    else -> null
}
